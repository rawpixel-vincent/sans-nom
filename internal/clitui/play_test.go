package clitui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type fakePlayController struct {
	turns       []TurnView
	submissions []SubmittedDecision
	done        bool
}

func (f *fakePlayController) NextTurn(context.Context) (TurnView, error) {
	if len(f.turns) == 0 {
		return TurnView{Turn: 99, Prose: "done"}, nil
	}
	turn := f.turns[0]
	f.turns = f.turns[1:]
	return turn, nil
}

func (f *fakePlayController) Submit(_ context.Context, decision SubmittedDecision) (bool, error) {
	f.submissions = append(f.submissions, decision)
	return f.done, nil
}

func TestPlayModelLoadsTurn(t *testing.T) {
	controller := &fakePlayController{turns: []TurnView{{Turn: 1, Prose: "First light.", Choices: []ChoiceView{{ID: "a", Label: "Go"}}}}}
	model := NewPlayModel(context.Background(), controller)
	msg := model.Init()()
	next, _ := model.Update(msg)
	model = next.(PlayModel)

	if !model.hasTurn || model.turn.Turn != 1 || !strings.Contains(model.View(), "First light.") {
		t.Fatalf("turn not rendered: %#v\n%s", model.turn, model.View())
	}
}

func TestPlayModelChoiceNavigationAndSubmit(t *testing.T) {
	controller := &fakePlayController{turns: []TurnView{{Turn: 1, Prose: "Choose.", Choices: []ChoiceView{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}}}}
	model := loadedPlayModel(t, controller)
	model = playPress(model, tea.KeyMsg{Type: tea.KeyDown})
	if model.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", model.cursor)
	}
	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(PlayModel)
	if !model.loading || cmd == nil {
		t.Fatalf("expected submit command")
	}
	msg := cmd()
	next, _ = model.Update(msg)
	_ = next.(PlayModel)
	if got := controller.submissions[0].ChoiceID; got != "b" {
		t.Fatalf("submitted choice = %q, want b", got)
	}
}

func TestPlayModelTextSubmit(t *testing.T) {
	controller := &fakePlayController{turns: []TurnView{{Turn: 1, Prose: "Act.", Choices: []ChoiceView{{ID: "a", Label: "A"}}}}}
	model := loadedPlayModel(t, controller)
	model = playPress(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("open hatch")})
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected submit command")
	}
	_ = cmd()
	if got := controller.submissions[0].FreeText; got != "open hatch" {
		t.Fatalf("submitted text = %q, want open hatch", got)
	}
}

func TestPlayModelTerminalHandoffAllowsEmptySubmit(t *testing.T) {
	controller := &fakePlayController{turns: []TurnView{{Turn: 3, Prose: "Continue.", TerminalHandoff: true}}}
	model := loadedPlayModel(t, controller)
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected submit command")
	}
	_ = cmd()
	if len(controller.submissions) != 1 {
		t.Fatalf("submissions = %d, want 1", len(controller.submissions))
	}
	if got := controller.submissions[0]; got.ChoiceID != "" || got.FreeText != "" {
		t.Fatalf("submitted = %#v, want empty continuation", got)
	}
}

func TestPlayModelScrollsNarration(t *testing.T) {
	controller := &fakePlayController{turns: []TurnView{{Turn: 1, Prose: strings.Repeat("line words words words\n", 80)}}}
	model := loadedPlayModel(t, controller)
	next, _ := model.Update(tea.WindowSizeMsg{Width: 50, Height: 14})
	model = next.(PlayModel)
	model = playPress(model, tea.KeyMsg{Type: tea.KeyPgDown})
	if model.scroll == 0 {
		t.Fatal("expected scroll to move down")
	}
	model = playPress(model, tea.KeyMsg{Type: tea.KeyHome})
	if model.scroll != 0 {
		t.Fatalf("scroll = %d, want 0", model.scroll)
	}
}

func loadedPlayModel(t *testing.T, controller *fakePlayController) PlayModel {
	t.Helper()
	model := NewPlayModel(context.Background(), controller)
	next, _ := model.Update(model.Init()())
	return next.(PlayModel)
}

func playPress(model PlayModel, msg tea.KeyMsg) PlayModel {
	next, _ := model.Update(msg)
	return next.(PlayModel)
}
