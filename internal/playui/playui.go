package playui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/clitui"
	"github.com/rawpixel-vincent/sans-nom/internal/playrun"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

func Run(ctx context.Context, cfg playrun.Config) error {
	var err error
	cfg, err = playrun.RetryBlockedWriterRefreshIfNeeded(ctx, cfg)
	if err != nil {
		return err
	}
	if cfg.UseTUI {
		return RunTUI(ctx, cfg)
	}
	return RunRaw(ctx, cfg)
}

func RunTUI(ctx context.Context, cfg playrun.Config) error {
	cfg = withTraceSink(cfg)
	controller, err := playrun.NewLoopController(cfg)
	if err != nil {
		return err
	}
	return clitui.Play(ctx, tuiController{inner: controller})
}

func RunRaw(ctx context.Context, cfg playrun.Config) error {
	cfg = withTraceSink(cfg)
	cfg.LocalTurnNumbers = true
	controller, err := playrun.NewLoopController(cfg)
	if err != nil {
		return err
	}
	reader := bufio.NewReader(os.Stdin)
	totalTurns := cfg.Turns
	for turn := 1; totalTurns <= 0 || turn <= totalTurns; turn++ {
		view, err := controller.NextTurn(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("\n--- Turn %d ---\n\n%s\n", view.Turn, view.Prose)
		if view.TerminalHandoff {
			if bridge := playrun.TerminalHandoffBridge(view.Node); bridge != "" {
				fmt.Printf("\n%s\n", bridge)
			}
		}
		if len(view.Choices) > 0 {
			fmt.Println("\nChoices:")
			for _, choice := range view.Choices {
				fmt.Printf("- %s: %s\n", choice.ID, choice.Label)
			}
		}
		if view.AudioPath != "" {
			fmt.Printf("\nAudio saved: %s\n", filepath.ToSlash(view.AudioPath))
		}
		fmt.Printf("\n%s\n", view.CommitLabel)
		if totalTurns > 0 && turn == totalTurns {
			_, err := controller.Submit(ctx, playrun.SubmittedDecision{})
			return err
		}
		if len(view.Node.Choices) == 0 && !cfg.RefreshWrite {
			_, err := controller.Submit(ctx, playrun.SubmittedDecision{})
			if err != nil {
				return err
			}
			fmt.Println("\nNo choices available; play loop stopped.")
			return nil
		}
		terminalHandoff := len(view.Node.Choices) == 0 && cfg.RefreshWrite
		if terminalHandoff && cfg.Turns <= 0 {
			fmt.Println("\nNo choices in this handoff. Press Enter to continue, or type a direction for Writer.")
		}
		nextDecision := playrun.AutomatedDecision(view.Node)
		rawInput := ""
		if cfg.Turns <= 0 {
			if terminalHandoff {
				fmt.Print("\ncontinuation> ")
			} else {
				fmt.Print("\nchoice id or free text> ")
			}
			line, readErr := reader.ReadString('\n')
			if readErr != nil && strings.TrimSpace(line) == "" {
				fmt.Println("\nInput closed; play loop stopped.")
				return nil
			}
			fmt.Println()
			rawInput = strings.TrimSpace(line)
			if terminalHandoff && rawInput == "" {
				nextDecision = scene.PlayerDecision{}
			} else {
				nextDecision = playrun.ParseDecision(rawInput, view.Node)
			}
		}
		done, err := controller.Submit(ctx, playrun.SubmittedDecision{
			ChoiceID: nextDecision.ChoiceID,
			FreeText: nextDecision.FreeText,
			RawInput: rawInput,
		})
		if err != nil {
			return err
		}
		if done {
			if nextDecision.FreeText != "" && !cfg.RefreshWrite {
				fmt.Println("\nFree text needs Writer refresh enabled to create the next packet; play loop stopped.")
			}
			return nil
		}
	}
	return nil
}

type tuiController struct {
	inner *playrun.LoopController
}

func (c tuiController) SetStatusSink(sink func(clitui.PlayStatus)) {
	c.inner.SetStatusSink(func(status playrun.Status) {
		sink(clitui.PlayStatus{Message: status.Message, Level: status.Level})
	})
}

func (c tuiController) IsRecoverableError(err error) bool {
	return c.inner.IsRecoverableError(err)
}

func (c tuiController) NextTurn(ctx context.Context) (clitui.TurnView, error) {
	view, err := c.inner.NextTurn(ctx)
	if err != nil {
		return clitui.TurnView{}, err
	}
	return clitui.TurnView{
		Turn:            view.Turn,
		PacketID:        view.PacketID,
		NodeID:          view.NodeID,
		Prose:           view.Prose,
		Choices:         toTUIChoices(view.Choices),
		TerminalHandoff: view.TerminalHandoff,
		CommitLabel:     view.CommitLabel,
		AudioPath:       view.AudioPath,
		Status:          toTUIStatus(view.Status),
	}, nil
}

func (c tuiController) Submit(ctx context.Context, submitted clitui.SubmittedDecision) (bool, error) {
	return c.inner.Submit(ctx, playrun.SubmittedDecision{
		ChoiceID: submitted.ChoiceID,
		FreeText: submitted.FreeText,
		RawInput: submitted.RawInput,
		Exit:     submitted.Exit,
	})
}

func toTUIChoices(choices []playrun.ChoiceView) []clitui.ChoiceView {
	out := make([]clitui.ChoiceView, 0, len(choices))
	for _, choice := range choices {
		out = append(out, clitui.ChoiceView{ID: choice.ID, Label: choice.Label})
	}
	return out
}

func toTUIStatus(status []playrun.Status) []clitui.PlayStatus {
	out := make([]clitui.PlayStatus, 0, len(status))
	for _, item := range status {
		out = append(out, clitui.PlayStatus{Message: item.Message, Level: item.Level})
	}
	return out
}

func withTraceSink(cfg playrun.Config) playrun.Config {
	if !cfg.TraceJSONL || cfg.Trace != nil {
		return cfg
	}
	cfg.Trace = func(event any) error {
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	return cfg
}
