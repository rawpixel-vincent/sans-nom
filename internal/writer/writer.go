package writer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

type Planner interface {
	Plan(ctx context.Context, input Input) (scene.StoryPacket, error)
}

type Input struct {
	Current              scene.StoryPacket
	LatestDecision       string
	ContextText          string
	RunID                string
	ArtifactDir          string
	SpentMotifs          []string
	RequiredMovement     []string
	ContextBudget        any
	RegisteredActorIDs   []string
	ActorManifestSummary string
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
		artifactErr := writeInputFallback(input)
		packet, err := s.Planner.Plan(ctx, input)
		if err == nil {
			err = packet.Validate()
		}
		if writeErr := writeResultArtifacts(input, packet, err); artifactErr == nil {
			artifactErr = writeErr
		}
		if err == nil && artifactErr != nil {
			err = fmt.Errorf("write writer artifacts: %w", artifactErr)
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

func (p StaticPlanner) ModelMetadata() modelmeta.Metadata {
	return prompt.StaticRunner{}.ModelMetadata()
}

type ClaudePlanner struct {
	Runner prompt.Runner
}

func (p ClaudePlanner) ModelMetadata() modelmeta.Metadata {
	if p.Runner == nil {
		return modelmeta.Metadata{}
	}
	provider, ok := p.Runner.(interface {
		ModelMetadata() modelmeta.Metadata
	})
	if !ok {
		return modelmeta.Metadata{}
	}
	return provider.ModelMetadata()
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
	_ = writeTextArtifact(input.ArtifactDir, "input.txt", text)
	raw, err := p.Runner.Run(ctx, text)
	if err != nil {
		return scene.StoryPacket{}, err
	}
	_ = writeTextArtifact(input.ArtifactDir, "raw-output.json", raw)
	var packet scene.StoryPacket
	dec := json.NewDecoder(bytes.NewBufferString(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&packet); err != nil {
		return scene.StoryPacket{}, fmt.Errorf("parse writer packet: %w", err)
	}
	return packet, packet.Validate()
}

func DefaultAwaitTimeout() time.Duration {
	return 15 * time.Minute
}

func writeInputFallback(input Input) error {
	if input.ArtifactDir == "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(input.ArtifactDir, "input.txt")); err == nil {
		return nil
	}
	text := input.ContextText
	if text == "" {
		data, err := json.MarshalIndent(input.Current, "", "  ")
		if err != nil {
			return err
		}
		text = string(data)
	}
	return writeTextArtifact(input.ArtifactDir, "input.txt", text)
}

func writeResultArtifacts(input Input, packet scene.StoryPacket, runErr error) error {
	if input.ArtifactDir == "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(input.ArtifactDir, "raw-output.json")); errors.Is(err, os.ErrNotExist) && packet.ID != "" {
		if writeErr := writeJSONArtifact(input.ArtifactDir, "raw-output.json", packet); writeErr != nil {
			return writeErr
		}
	}
	metadata := map[string]any{
		"run_id":            input.RunID,
		"latest_decision":   input.LatestDecision,
		"spent_motifs":      input.SpentMotifs,
		"required_movement": input.RequiredMovement,
		"context_budget":    input.ContextBudget,
		"created_at":        time.Now().UTC().Format(time.RFC3339),
		"accepted":          runErr == nil,
	}
	if runErr != nil {
		metadata["error"] = runErr.Error()
		if err := writeTextArtifact(input.ArtifactDir, "validation-error.txt", runErr.Error()+"\n"); err != nil {
			return err
		}
	} else if err := writeJSONArtifact(input.ArtifactDir, "accepted-packet.json", packet); err != nil {
		return err
	}
	return writeJSONArtifact(input.ArtifactDir, "metadata.json", metadata)
}

func writeTextArtifact(dir string, name string, text string) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeJSONArtifact(dir string, name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeTextArtifact(dir, name, string(data))
}
