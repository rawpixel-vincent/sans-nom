package writer

import (
	"context"
	"testing"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

func TestRefreshAsyncReturnsPacket(t *testing.T) {
	packet := scene.StoryPacket{
		ID: "next",
		Nodes: []scene.SceneNode{{
			ID:    "start",
			Beats: []scene.Beat{{Text: "A beat."}},
		}},
	}
	svc := Service{Planner: StaticPlanner{Next: packet}}

	job, err := svc.RefreshAsync(context.Background(), Input{
		Current:        packet,
		LatestDecision: "wait",
		ContextText:    "writer context",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := job.Await(ctx)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if result.Packet.ID != "next" {
		t.Fatalf("packet id = %q", result.Packet.ID)
	}
}

func TestRefreshAsyncPassesWriterContext(t *testing.T) {
	packet := scene.StoryPacket{
		ID: "current",
		Nodes: []scene.SceneNode{{
			ID:    "start",
			Beats: []scene.Beat{{Text: "A beat."}},
		}},
	}
	planner := &recordingPlanner{next: packet}
	svc := Service{Planner: planner}

	job, err := svc.RefreshAsync(context.Background(), Input{
		Current:        packet,
		LatestDecision: "ask-tonto",
		ContextText:    "assembled writer context",
	})
	if err != nil {
		t.Fatal(err)
	}
	result := job.Await(context.Background())
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if planner.input.ContextText != "assembled writer context" || planner.input.LatestDecision != "ask-tonto" {
		t.Fatalf("planner missed input: %#v", planner.input)
	}
}

func TestRefreshAsyncRejectsInvalidReplacement(t *testing.T) {
	svc := Service{Planner: invalidPlanner{}}

	job, err := svc.RefreshAsync(context.Background(), Input{})
	if err != nil {
		t.Fatal(err)
	}
	result := job.Await(context.Background())
	if result.Err == nil {
		t.Fatal("expected invalid packet error")
	}
}

type recordingPlanner struct {
	input Input
	next  scene.StoryPacket
}

func (p *recordingPlanner) Plan(_ context.Context, input Input) (scene.StoryPacket, error) {
	p.input = input
	return p.next, p.next.Validate()
}

type invalidPlanner struct{}

func (invalidPlanner) Plan(context.Context, Input) (scene.StoryPacket, error) {
	return scene.StoryPacket{}, nil
}
