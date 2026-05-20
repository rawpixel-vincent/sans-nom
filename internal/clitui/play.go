package clitui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type ChoiceView struct {
	ID    string
	Label string
}

type PlayStatus struct {
	Message string
	Level   string
}

type TurnView struct {
	Turn             int
	PacketID         string
	NodeID           string
	Prose            string
	Choices          []ChoiceView
	TerminalHandoff  bool
	CommitLabel      string
	AudioPath        string
	Status           []PlayStatus
	ArtifactHint     string
	CompletedMessage string
}

type SubmittedDecision struct {
	ChoiceID string
	FreeText string
	RawInput string
	Exit     bool
}

type PlayController interface {
	NextTurn(context.Context) (TurnView, error)
	Submit(context.Context, SubmittedDecision) (bool, error)
}

type PlayStatusSink interface {
	SetStatusSink(func(PlayStatus))
}

type RecoverablePlayError interface {
	IsRecoverableError(error) bool
}

type playTurnMsg struct {
	turn TurnView
}

type playSubmitMsg struct {
	done bool
}

type playErrorMsg struct {
	err       error
	op        string
	decision  SubmittedDecision
	retryable bool
}

type playStatusMsg struct {
	status PlayStatus
}

type PlayModel struct {
	ctx        context.Context
	controller PlayController
	width      int
	height     int
	loading    bool
	done       bool
	err        error
	turn       TurnView
	hasTurn    bool
	cursor     int
	scroll     int
	input      string
	status     []PlayStatus
	errorOp    string
	errorInput SubmittedDecision
	retryable  bool
}

func NewPlayModel(ctx context.Context, controller PlayController) PlayModel {
	if ctx == nil {
		ctx = context.Background()
	}
	return PlayModel{ctx: ctx, controller: controller, loading: true}
}

func Play(ctx context.Context, controller PlayController) error {
	if controller == nil {
		return fmt.Errorf("play controller is required")
	}
	model := NewPlayModel(ctx, controller)
	program := tea.NewProgram(model, tea.WithAltScreen())
	if sink, ok := controller.(PlayStatusSink); ok {
		sink.SetStatusSink(func(status PlayStatus) {
			program.Send(playStatusMsg{status: status})
		})
	}
	if ctx != nil && ctx.Done() != nil {
		go func() {
			<-ctx.Done()
			program.Kill()
		}()
	}
	final, err := program.Run()
	if err != nil {
		return err
	}
	playModel, ok := final.(PlayModel)
	if !ok {
		return fmt.Errorf("unexpected play TUI model %T", final)
	}
	if playModel.err != nil {
		return playModel.err
	}
	return nil
}

func (m PlayModel) Init() tea.Cmd {
	return m.nextTurnCmd()
}

func (m PlayModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampScroll()
		return m, nil
	case playStatusMsg:
		if strings.TrimSpace(msg.status.Message) != "" {
			m.status = append(m.status, msg.status)
		}
		return m, nil
	case playTurnMsg:
		m.loading = false
		m.err = nil
		m.turn = msg.turn
		m.hasTurn = true
		m.cursor = 0
		m.scroll = 0
		m.input = ""
		m.status = append([]PlayStatus{}, msg.turn.Status...)
		m.clampScroll()
		return m, nil
	case playSubmitMsg:
		m.loading = false
		if msg.done {
			m.done = true
			return m, tea.Quit
		}
		m.hasTurn = false
		m.loading = true
		m.input = ""
		m.cursor = 0
		m.scroll = 0
		m.status = []PlayStatus{{Message: "Loading next turn..."}}
		return m, m.nextTurnCmd()
	case playErrorMsg:
		m.loading = false
		m.err = msg.err
		m.errorOp = msg.op
		m.errorInput = msg.decision
		m.retryable = msg.retryable
		m.status = append(m.status, PlayStatus{Message: msg.err.Error(), Level: "error"})
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m PlayModel) View() string {
	width := m.contentWidth()
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  SANS NOM")
	if m.hasTurn && m.turn.Turn > 0 {
		b.WriteString(fmt.Sprintf("  -  Turn %d", m.turn.Turn))
	}
	b.WriteString("\n")
	b.WriteString(strings.Repeat("-", minInt(width+4, maxInt(20, m.width))))
	b.WriteString("\n")
	if m.err != nil {
		b.WriteString("\n  Error\n\n")
		b.WriteString(indent(wrapText(m.err.Error(), width), "  "))
		if m.retryable {
			b.WriteString("\n\n  Disconnected. Press Enter to retry, or q to exit.\n")
		} else {
			b.WriteString("\n\n  q exit\n")
		}
		return b.String()
	}
	if m.done {
		b.WriteString("\n  Play stopped.\n\n")
		return b.String()
	}
	b.WriteString(m.viewStatus(width))
	if m.loading || !m.hasTurn {
		b.WriteString("\n  Working...\n\n")
		b.WriteString("  q exit\n")
		return b.String()
	}
	b.WriteString(m.viewProse(width))
	b.WriteString(m.viewChoices(width))
	b.WriteString(m.viewInput(width))
	b.WriteString("\n  up/down or j/k choice  left/right page story  enter submit  q exit\n")
	return b.String()
}

func (m PlayModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit
	}
	if m.err != nil {
		if key == "enter" && m.retryable {
			m.loading = true
			m.err = nil
			m.retryable = false
			m.status = append(m.status, PlayStatus{Message: "Retrying after reconnect..."})
			switch m.errorOp {
			case "submit":
				return m, m.submitCmd(m.errorInput)
			default:
				return m, m.nextTurnCmd()
			}
		}
		return m, nil
	}
	if m.loading {
		return m, nil
	}
	switch key {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down", "j":
		if m.cursor < len(m.turn.Choices)-1 {
			m.cursor++
		}
		return m, nil
	case "left":
		m.scroll -= maxInt(1, m.proseHeight()-1)
		m.clampScroll()
		return m, nil
	case "right":
		m.scroll += maxInt(1, m.proseHeight()-1)
		m.clampScroll()
		return m, nil
	case "home":
		m.scroll = 0
		return m, nil
	case "end":
		m.scroll = maxInt(0, len(m.proseLines())-m.proseHeight())
		return m, nil
	case "backspace", "ctrl+h":
		m.input = trimLastRune(m.input)
		return m, nil
	case "ctrl+w":
		m.input = trimLastWord(m.input)
		return m, nil
	case "esc":
		m.input = ""
		return m, nil
	case "enter":
		m.loading = true
		return m, m.submitCmd(m.selectedDecision())
	}
	if len(msg.Runes) > 0 {
		m.input += string(msg.Runes)
		return m, nil
	}
	return m, nil
}

func (m PlayModel) nextTurnCmd() tea.Cmd {
	return func() tea.Msg {
		turn, err := m.controller.NextTurn(m.ctx)
		if err != nil {
			return playErrorMsg{err: err, op: "next", retryable: m.isRecoverable(err)}
		}
		return playTurnMsg{turn: turn}
	}
}

func (m PlayModel) submitCmd(decision SubmittedDecision) tea.Cmd {
	return func() tea.Msg {
		done, err := m.controller.Submit(m.ctx, decision)
		if err != nil {
			return playErrorMsg{err: err, op: "submit", decision: decision, retryable: m.isRecoverable(err)}
		}
		return playSubmitMsg{done: done}
	}
}

func (m PlayModel) isRecoverable(err error) bool {
	recoverable, ok := m.controller.(RecoverablePlayError)
	return ok && recoverable.IsRecoverableError(err)
}

func (m PlayModel) selectedDecision() SubmittedDecision {
	input := strings.TrimSpace(m.input)
	if input != "" {
		return SubmittedDecision{FreeText: input, RawInput: input}
	}
	if len(m.turn.Choices) > 0 {
		choice := m.turn.Choices[m.cursor]
		return SubmittedDecision{ChoiceID: choice.ID, RawInput: choice.ID}
	}
	return SubmittedDecision{}
}

func (m PlayModel) viewStatus(width int) string {
	var b strings.Builder
	status := m.status
	if len(status) > 4 {
		status = status[len(status)-4:]
	}
	if len(status) == 0 && m.turn.CompletedMessage == "" && m.turn.AudioPath == "" && m.turn.CommitLabel == "" {
		return ""
	}
	b.WriteString("\n")
	for _, item := range status {
		message := strings.TrimSpace(item.Message)
		if message == "" {
			continue
		}
		prefix := "- "
		if item.Level == "error" {
			prefix = "! "
		}
		b.WriteString(indent(wrapText(prefix+message, width), "  "))
		b.WriteString("\n")
	}
	if m.turn.AudioPath != "" {
		b.WriteString(indent(wrapText("- Audio saved: "+m.turn.AudioPath, width), "  "))
		b.WriteString("\n")
	}
	if m.turn.CommitLabel != "" {
		b.WriteString(indent(wrapText("- "+m.turn.CommitLabel, width), "  "))
		b.WriteString("\n")
	}
	return b.String()
}

func (m PlayModel) viewProse(width int) string {
	lines := m.proseLines()
	height := m.proseHeight()
	m.clampScroll()
	start := minInt(m.scroll, len(lines))
	end := minInt(len(lines), start+height)
	boxWidth := maxInt(24, width)
	innerWidth := maxInt(12, boxWidth-4)
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  +")
	b.WriteString(strings.Repeat("-", boxWidth-2))
	b.WriteString("+\n")
	for _, line := range lines[start:end] {
		b.WriteString("  | ")
		b.WriteString(padRight(line, innerWidth))
		b.WriteString(" |\n")
	}
	for filler := end - start; filler < height; filler++ {
		b.WriteString("  | ")
		b.WriteString(strings.Repeat(" ", innerWidth))
		b.WriteString(" |\n")
	}
	footer := "story"
	if len(lines) > height {
		footer = fmt.Sprintf("story %d-%d/%d  left/right", start+1, end, len(lines))
	}
	b.WriteString("  | ")
	b.WriteString(padRight(footer, innerWidth))
	b.WriteString(" |\n")
	b.WriteString("  +")
	b.WriteString(strings.Repeat("-", boxWidth-2))
	b.WriteString("+\n")
	return b.String()
}

func (m PlayModel) viewChoices(width int) string {
	var b strings.Builder
	if len(m.turn.Choices) > 0 {
		b.WriteString("\n  Choices\n")
		for index, choice := range m.turn.Choices {
			cursor := "  "
			if index == m.cursor {
				cursor = "> "
			}
			text := strings.TrimSpace(choice.Label)
			if text == "" {
				text = choice.ID
			}
			line := fmt.Sprintf("%s%s", cursor, text)
			wrapped := wrapText(line, width)
			b.WriteString(indent(wrapped, "  "))
			b.WriteString("\n")
		}
	} else if m.turn.TerminalHandoff {
		b.WriteString("\n  Press Enter to continue, or type a direction.\n")
	} else {
		b.WriteString("\n  No choices available.\n")
	}
	return b.String()
}

func (m PlayModel) viewInput(width int) string {
	label := "> "
	if len(m.turn.Choices) == 0 {
		label = "continuation> "
	}
	line := label + m.input
	if m.loading {
		line += " ..."
	} else {
		line += "_"
	}
	return "\n" + indent(wrapText(line, width), "  ") + "\n"
}

func (m PlayModel) proseLines() []string {
	return wrapText(strings.TrimSpace(m.turn.Prose), m.proseTextWidth())
}

func (m PlayModel) proseHeight() int {
	if m.height <= 0 {
		return 16
	}
	reserved := 15 + len(m.turn.Choices)
	if len(m.status) > 0 {
		reserved += minInt(len(m.status), 4)
	}
	return maxInt(6, m.height-reserved)
}

func (m *PlayModel) clampScroll() {
	lines := m.proseLines()
	maxScroll := maxInt(0, len(lines)-m.proseHeight())
	if m.scroll < 0 {
		m.scroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.turn.Choices) && len(m.turn.Choices) > 0 {
		m.cursor = len(m.turn.Choices) - 1
	}
}

func (m PlayModel) contentWidth() int {
	if m.width <= 0 {
		return 88
	}
	return maxInt(32, m.width-4)
}

func (m PlayModel) proseTextWidth() int {
	return maxInt(12, m.contentWidth()-4)
}

func wrapText(text string, width int) []string {
	width = maxInt(12, width)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		if strings.TrimSpace(paragraph) == "" {
			lines = append(lines, "")
			continue
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := ""
		for _, word := range words {
			if line == "" {
				line = word
				continue
			}
			if runeLen(line)+1+runeLen(word) <= width {
				line += " " + word
				continue
			}
			lines = append(lines, line)
			line = word
			for runeLen(line) > width {
				head, tail := splitRunes(line, width)
				lines = append(lines, head)
				line = tail
			}
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func padRight(text string, width int) string {
	if runeLen(text) >= width {
		return text
	}
	return text + strings.Repeat(" ", width-runeLen(text))
}

func indent(lines []string, prefix string) string {
	if len(lines) == 0 {
		return prefix
	}
	var b strings.Builder
	for index, line := range lines {
		if index > 0 {
			b.WriteString("\n")
		}
		b.WriteString(prefix)
		b.WriteString(line)
	}
	return b.String()
}

func runeLen(text string) int {
	return len([]rune(text))
}

func splitRunes(text string, n int) (string, string) {
	runes := []rune(text)
	if n >= len(runes) {
		return text, ""
	}
	return string(runes[:n]), string(runes[n:])
}

func trimLastRune(text string) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[:len(runes)-1])
}

func trimLastWord(text string) string {
	text = strings.TrimRight(text, " \t")
	index := strings.LastIndexAny(text, " \t")
	if index < 0 {
		return ""
	}
	return strings.TrimRight(text[:index], " \t")
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
