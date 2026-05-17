package clitui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rawpixel-vincent/sans-nom/internal/storyapp"
)

func TestMainMenuEnterNewStoryReturnsCreateAction(t *testing.T) {
	model := press(NewModel("", nil), tea.KeyMsg{Type: tea.KeyEnter})

	if model.action.Kind != ActionNewStory {
		t.Fatalf("action = %#v, want new story", model.action)
	}
}

func TestMainMenuEnterExitReturnsExitAction(t *testing.T) {
	model := NewModel("", nil)
	model = press(model, tea.KeyMsg{Type: tea.KeyDown})
	model = press(model, tea.KeyMsg{Type: tea.KeyDown})
	model = press(model, tea.KeyMsg{Type: tea.KeyEnter})

	if model.action.Kind != ActionExit {
		t.Fatalf("action = %#v, want exit", model.action)
	}
}

func TestResumeSelectionReturnsStoryID(t *testing.T) {
	model := NewModel("root", func(root string) ([]storyapp.StorySummary, error) {
		if root != "root" {
			t.Fatalf("root = %q", root)
		}
		return []storyapp.StorySummary{{
			StoryID:    "story-one",
			Label:      "Story One",
			LatestTurn: 3,
			UpdatedAt:  "2026-01-01T00:00:00Z",
		}}, nil
	})

	model = press(model, tea.KeyMsg{Type: tea.KeyDown})
	model = press(model, tea.KeyMsg{Type: tea.KeyEnter})
	if model.screen != screenResume {
		t.Fatalf("screen = %v, want resume", model.screen)
	}
	model = press(model, tea.KeyMsg{Type: tea.KeyEnter})

	if model.action.Kind != ActionResumeStory || model.action.StoryID != "story-one" {
		t.Fatalf("action = %#v, want resume story-one", model.action)
	}
}

func TestEmptyResumeListSupportsBackAndExit(t *testing.T) {
	model := NewModel("root", func(string) ([]storyapp.StorySummary, error) {
		return nil, nil
	})

	model = press(model, tea.KeyMsg{Type: tea.KeyDown})
	model = press(model, tea.KeyMsg{Type: tea.KeyEnter})
	if model.screen != screenResume || model.itemCount() != 2 {
		t.Fatalf("unexpected empty resume screen: screen=%v count=%d", model.screen, model.itemCount())
	}
	model = press(model, tea.KeyMsg{Type: tea.KeyEnter})
	if model.screen != screenMain {
		t.Fatalf("enter on Back screen = %v, want main", model.screen)
	}

	model = press(model, tea.KeyMsg{Type: tea.KeyDown})
	model = press(model, tea.KeyMsg{Type: tea.KeyEnter})
	model = press(model, tea.KeyMsg{Type: tea.KeyDown})
	model = press(model, tea.KeyMsg{Type: tea.KeyEnter})
	if model.action.Kind != ActionExit {
		t.Fatalf("action = %#v, want exit", model.action)
	}
}

func press(model Model, msg tea.KeyMsg) Model {
	next, _ := model.Update(msg)
	return next.(Model)
}
