package clitui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type fakePlayController struct {
	turns       []TurnView
	submissions []SubmittedDecision
	done        bool
	nextErrs    []error
	submitErrs  []error
}

func (f *fakePlayController) NextTurn(context.Context) (TurnView, error) {
	if len(f.nextErrs) > 0 {
		err := f.nextErrs[0]
		f.nextErrs = f.nextErrs[1:]
		return TurnView{}, err
	}
	if len(f.turns) == 0 {
		return TurnView{Turn: 99, Prose: "done"}, nil
	}
	turn := f.turns[0]
	f.turns = f.turns[1:]
	return turn, nil
}

func (f *fakePlayController) Submit(_ context.Context, decision SubmittedDecision) (bool, error) {
	f.submissions = append(f.submissions, decision)
	if len(f.submitErrs) > 0 {
		err := f.submitErrs[0]
		f.submitErrs = f.submitErrs[1:]
		return false, err
	}
	return f.done, nil
}

type recoverableFakePlayController struct {
	*fakePlayController
}

func (f recoverableFakePlayController) IsRecoverableError(error) bool {
	return true
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
	model = playPress(model, tea.KeyMsg{Type: tea.KeyRight})
	if model.scroll == 0 {
		t.Fatal("expected scroll to move down")
	}
	model = playPress(model, tea.KeyMsg{Type: tea.KeyLeft})
	if model.scroll != 0 {
		t.Fatalf("scroll = %d, want 0 after left page", model.scroll)
	}
	model = playPress(model, tea.KeyMsg{Type: tea.KeyRight})
	model = playPress(model, tea.KeyMsg{Type: tea.KeyHome})
	if model.scroll != 0 {
		t.Fatalf("scroll = %d, want 0", model.scroll)
	}
}

func TestPlayModelRendersNarrationBox(t *testing.T) {
	controller := &fakePlayController{turns: []TurnView{{Turn: 1, Prose: "A boxed story line.", Choices: []ChoiceView{{ID: "a", Label: "A"}}}}}
	model := loadedPlayModel(t, controller)
	next, _ := model.Update(tea.WindowSizeMsg{Width: 42, Height: 18})
	model = next.(PlayModel)
	view := model.View()
	for _, want := range []string{
		"  +",
		"  | A boxed story line.",
		"story",
		"left/right page story",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestPlayModelRecoverableErrorRetriesOnEnter(t *testing.T) {
	controller := recoverableFakePlayController{&fakePlayController{
		nextErrs: []error{errors.New("read tcp: can't assign requested address")},
		turns:    []TurnView{{Turn: 1, Prose: "Recovered."}},
	}}
	model := NewPlayModel(context.Background(), controller)
	next, _ := model.Update(model.Init()())
	model = next.(PlayModel)
	if !model.retryable || !strings.Contains(model.View(), "Disconnected. Press Enter to retry") {
		t.Fatalf("recoverable error not rendered:\n%s", model.View())
	}
	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(PlayModel)
	if cmd == nil || !model.loading {
		t.Fatal("expected retry command after enter")
	}
	next, _ = model.Update(cmd())
	model = next.(PlayModel)
	if model.err != nil || !model.hasTurn || !strings.Contains(model.View(), "Recovered.") {
		t.Fatalf("retry did not load turn: %#v\n%s", model.err, model.View())
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
