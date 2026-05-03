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

	job, err := svc.RefreshAsync(context.Background(), packet, "wait")
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
