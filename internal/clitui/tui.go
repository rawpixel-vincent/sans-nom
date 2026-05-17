package clitui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
	"github.com/rawpixel-vincent/sans-nom/internal/storyapp"
)

type ActionKind int

const (
	ActionNone ActionKind = iota
	ActionNewStory
	ActionResumeStory
	ActionExit
)

type Action struct {
	Kind    ActionKind
	StoryID string
}

type ListStoriesFunc func(root string) ([]storyapp.StorySummary, error)

type screen int

const (
	screenMain screen = iota
	screenResume
)

type Model struct {
	root        string
	listStories ListStoriesFunc
	screen      screen
	cursor      int
	stories     []storyapp.StorySummary
	err         error
	action      Action
	width       int
	height      int
}

func NewModel(root string, listStories ListStoriesFunc) Model {
	if strings.TrimSpace(root) == "" {
		root = story.DefaultRoot
	}
	if listStories == nil {
		listStories = storyapp.ListStories
	}
	return Model{
		root:        root,
		listStories: listStories,
	}
}

func Run(ctx context.Context, root string) (Action, error) {
	program := tea.NewProgram(NewModel(root, nil), tea.WithAltScreen())
	if ctx != nil && ctx.Done() != nil {
		go func() {
			<-ctx.Done()
			program.Kill()
		}()
	}
	final, err := program.Run()
	if err != nil {
		return Action{}, err
	}
	model, ok := final.(Model)
	if !ok {
		return Action{}, fmt.Errorf("unexpected TUI model %T", final)
	}
	if model.action.Kind == ActionNone {
		return Action{Kind: ActionExit}, nil
	}
	return model.action, nil
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.action = Action{Kind: ActionExit}
			return m, tea.Quit
		case "esc":
			if m.screen == screenResume {
				m.screen = screenMain
				m.cursor = 0
				m.err = nil
				return m, nil
			}
			m.action = Action{Kind: ActionExit}
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "j":
			if m.cursor < m.itemCount()-1 {
				m.cursor++
			}
			return m, nil
		case "enter":
			return m.handleEnter()
		}
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  SANS NOM\n")
	b.WriteString("  Runtime story launcher\n\n")
	if m.screen == screenMain {
		b.WriteString(m.viewMain())
	} else {
		b.WriteString(m.viewResume())
	}
	b.WriteString("\n  arrows/jk move  enter select  esc back  q exit\n")
	return b.String()
}

func (m Model) Action() Action {
	return m.action
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenMain:
		switch m.cursor {
		case 0:
			m.action = Action{Kind: ActionNewStory}
			return m, tea.Quit
		case 1:
			stories, err := m.listStories(m.root)
			m.stories = stories
			m.err = err
			m.screen = screenResume
			m.cursor = 0
			return m, nil
		default:
			m.action = Action{Kind: ActionExit}
			return m, tea.Quit
		}
	case screenResume:
		if len(m.stories) == 0 {
			if m.cursor == 0 {
				m.screen = screenMain
				m.cursor = 0
				m.err = nil
				return m, nil
			}
			m.action = Action{Kind: ActionExit}
			return m, tea.Quit
		}
		if m.cursor < len(m.stories) {
			m.action = Action{Kind: ActionResumeStory, StoryID: m.stories[m.cursor].StoryID}
			return m, tea.Quit
		}
		if m.cursor == len(m.stories) {
			m.screen = screenMain
			m.cursor = 0
			m.err = nil
			return m, nil
		}
		m.action = Action{Kind: ActionExit}
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) itemCount() int {
	if m.screen == screenMain {
		return 3
	}
	if len(m.stories) == 0 {
		return 2
	}
	return len(m.stories) + 2
}

func (m Model) viewMain() string {
	items := []struct {
		label string
		help  string
	}{
		{"New Story", "Create a durable story workspace, then start play."},
		{"Resume Story", "Pick an existing story workspace."},
		{"Exit", "Close the launcher."},
	}
	var b strings.Builder
	for i, item := range items {
		b.WriteString(menuLine(m.cursor == i, item.label, item.help))
	}
	return b.String()
}

func (m Model) viewResume() string {
	var b strings.Builder
	b.WriteString("  Resume Story\n")
	b.WriteString("  " + m.root + "\n\n")
	if m.err != nil {
		b.WriteString("  Could not list stories: " + m.err.Error() + "\n\n")
	}
	if len(m.stories) == 0 {
		b.WriteString(menuLine(m.cursor == 0, "Back", "Return to the main menu."))
		b.WriteString(menuLine(m.cursor == 1, "Exit", "Close the launcher."))
		return b.String()
	}
	for i, summary := range m.stories {
		b.WriteString(menuLine(m.cursor == i, summary.Label, summary.SecondaryText()))
	}
	b.WriteString(menuLine(m.cursor == len(m.stories), "Back", "Return to the main menu."))
	b.WriteString(menuLine(m.cursor == len(m.stories)+1, "Exit", "Close the launcher."))
	return b.String()
}

func menuLine(selected bool, label string, secondary string) string {
	prefix := "   "
	if selected {
		prefix = " > "
	}
	if strings.TrimSpace(secondary) == "" {
		return fmt.Sprintf("%s%s\n", prefix, label)
	}
	return fmt.Sprintf("%s%s\n     %s\n", prefix, label, secondary)
}
