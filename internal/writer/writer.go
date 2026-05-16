package writer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

type Planner interface {
	Plan(ctx context.Context, input Input) (scene.StoryPacket, error)
}

type Input struct {
	Current        scene.StoryPacket
	LatestDecision string
	ContextText    string
}

type Service struct {
	Planner Planner
}

type Job struct {
	done chan Result
	once sync.Once
}

type Result struct {
	Packet scene.StoryPacket
	Err    error
}

func (s Service) RefreshAsync(ctx context.Context, input Input) (*Job, error) {
	if s.Planner == nil {
		return nil, errors.New("writer planner is required")
	}
	job := &Job{done: make(chan Result, 1)}
	go func() {
		packet, err := s.Planner.Plan(ctx, input)
		if err == nil {
			err = packet.Validate()
		}
		job.done <- Result{Packet: packet, Err: err}
		close(job.done)
	}()
	return job, nil
}

func (j *Job) Await(ctx context.Context) Result {
	select {
	case result := <-j.done:
		return result
	case <-ctx.Done():
		return Result{Err: ctx.Err()}
	}
}

type StaticPlanner struct {
	Next scene.StoryPacket
	Err  error
}

func (p StaticPlanner) Plan(_ context.Context, input Input) (scene.StoryPacket, error) {
	if p.Err != nil {
		return scene.StoryPacket{}, p.Err
	}
	if p.Next.ID == "" {
		return input.Current, input.Current.Validate()
	}
	return p.Next, p.Next.Validate()
}

type ClaudePlanner struct {
	Runner prompt.Runner
}

func (p ClaudePlanner) Plan(ctx context.Context, input Input) (scene.StoryPacket, error) {
	if p.Runner == nil {
		return scene.StoryPacket{}, errors.New("claude planner needs runner")
	}
	contextText := input.ContextText
	if contextText == "" {
		data, err := json.MarshalIndent(input.Current, "", "  ")
		if err != nil {
			return scene.StoryPacket{}, err
		}
		contextText = string(data)
	}
	text, err := prompt.WriterPrompt(contextText, input.LatestDecision)
	if err != nil {
		return scene.StoryPacket{}, err
	}
	raw, err := p.Runner.Run(ctx, text)
	if err != nil {
		return scene.StoryPacket{}, err
	}
	var packet scene.StoryPacket
	dec := json.NewDecoder(bytes.NewBufferString(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&packet); err != nil {
		return scene.StoryPacket{}, fmt.Errorf("parse writer packet: %w", err)
	}
	return packet, packet.Validate()
}

func DefaultAwaitTimeout() time.Duration {
	return 45 * time.Second
}
