package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fugue-labs/gollem/core"
)

// Option configures the TUI.
type Option func(*config)

type config struct {
	theme Theme
}

// Theme defines styles for different message types in the TUI.
type Theme struct {
	System lipgloss.Style
	User   lipgloss.Style
	Model  lipgloss.Style
	Tool   lipgloss.Style
	Result lipgloss.Style
	Status lipgloss.Style
}

// DefaultTheme returns the default color theme for the TUI.
func DefaultTheme() Theme {
	return Theme{
		System: lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true),
		User:   lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true),
		Model:  lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
		Tool:   lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		Result: lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
		Status: lipgloss.NewStyle().Background(lipgloss.Color("8")).Foreground(lipgloss.Color("15")).Padding(0, 1),
	}
}

// WithTheme sets a custom theme for the TUI.
func WithTheme(theme Theme) Option {
	return func(c *config) {
		c.theme = theme
	}
}

// step represents a single displayable event in the agent run.
type step struct {
	kind      string // "system", "user", "model", "tool-call", "tool-result", "error", "done"
	eventKind string
	content   string
	detail    string
}

// stepMsg is a bubbletea message carrying a new step.
type stepMsg struct {
	step step
}

// doneMsg signals the agent run has completed.
type doneMsg struct {
	err error
}

// model is the bubbletea model for the TUI.
type model struct {
	theme    Theme
	steps    []step
	usage    core.RunUsage
	status   string
	cost     string
	elapsed  string
	stepMode bool
	scroll   int // scroll offset from bottom
	width    int
	height   int
	done     bool
	err      error
	quitting bool
}

// newModel creates a new TUI model.
func newModel(theme Theme) model {
	return model{
		theme:  theme,
		width:  80,
		height: 24,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "s":
			m.stepMode = true
		case "a":
			m.stepMode = false
		case "up":
			if m.scroll < len(m.steps)-1 {
				m.scroll++
			}
		case "down":
			if m.scroll > 0 {
				m.scroll--
			}
		case "p", "left":
			if m.scroll < len(m.steps)-1 {
				m.scroll++
			}
		case "n", "right":
			if m.scroll > 0 {
				m.scroll--
			}
		case "g", "home":
			if len(m.steps) > 0 {
				m.scroll = len(m.steps) - 1
			}
		case "G", "end":
			m.scroll = 0
		case "e":
			m.scroll = scrollToFirstKind(m.steps, m.height, "error")
		case "c":
			m.scroll = scrollToFirstKind(m.steps, m.height, "checkpoint")
		case "d":
			m.scroll = scrollToFirstContent(m.steps, m.height, "first divergence")
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case stepMsg:
		m.steps = append(m.steps, msg.step)
		// Auto-scroll to bottom on new step.
		m.scroll = 0
	case doneMsg:
		m.done = true
		m.err = msg.err
		return m, tea.Quit
	}
	return m, nil
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	var sb strings.Builder

	// Calculate available lines for messages (reserve 2 for status bar).
	availableLines := m.height - 2
	if availableLines < 1 {
		availableLines = 1
	}

	// Render all steps.
	if m.width >= 100 && len(m.steps) > 0 {
		return m.renderSplitView(availableLines)
	}

	// Render all steps.
	var renderedLines []string
	for _, s := range m.steps {
		rendered := renderStep(s, m.theme, m.width)
		lines := strings.Split(rendered, "\n")
		renderedLines = append(renderedLines, lines...)
	}

	// Apply scroll offset.
	endIdx := len(renderedLines) - m.scroll
	if endIdx < 0 {
		endIdx = 0
	}
	startIdx := endIdx - availableLines
	if startIdx < 0 {
		startIdx = 0
	}

	// Display visible lines.
	visible := renderedLines[startIdx:endIdx]
	for _, line := range visible {
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Pad remaining space.
	for i := len(visible); i < availableLines; i++ {
		sb.WriteString("\n")
	}

	// Status bar.
	sb.WriteString(m.renderStatusBar())

	return sb.String()
}

func (m model) renderSplitView(availableLines int) string {
	timelineWidth := m.width / 2
	if timelineWidth < 44 {
		timelineWidth = 44
	}
	detailWidth := m.width - timelineWidth - 3
	if detailWidth < 32 {
		detailWidth = 32
	}

	selected := m.selectedStepIndex()
	end := len(m.steps) - m.scroll
	if end < 0 {
		end = 0
	}
	if end > len(m.steps) {
		end = len(m.steps)
	}
	start := end - availableLines
	if start < 0 {
		start = 0
	}
	visible := m.steps[start:end]

	leftLines := make([]string, 0, availableLines)
	for idx, s := range visible {
		prefix := "  "
		if start+idx == selected {
			prefix = "> "
		}
		line := prefix + singleLine(renderStep(s, m.theme, timelineWidth-2))
		leftLines = append(leftLines, truncateDisplay(line, timelineWidth))
	}
	for len(leftLines) < availableLines {
		leftLines = append(leftLines, "")
	}

	rightLines := splitDetailLines(m.steps[selected], detailWidth, availableLines)
	var sb strings.Builder
	for i := range availableLines {
		left := ""
		if i < len(leftLines) {
			left = leftLines[i]
		}
		right := ""
		if i < len(rightLines) {
			right = rightLines[i]
		}
		fmt.Fprintf(&sb, "%-*s │ %s\n", timelineWidth, left, right)
	}
	sb.WriteString(m.renderStatusBar())
	return sb.String()
}

func (m model) selectedStepIndex() int {
	if len(m.steps) == 0 {
		return 0
	}
	idx := len(m.steps) - 1 - m.scroll
	if idx < 0 {
		return 0
	}
	if idx >= len(m.steps) {
		return len(m.steps) - 1
	}
	return idx
}

// renderStep formats a single step with the appropriate style.
func renderStep(s step, theme Theme, width int) string {
	maxWidth := width - 4
	if maxWidth < 20 {
		maxWidth = 20
	}

	switch s.kind {
	case "system":
		return theme.System.Width(maxWidth).Render("[system] " + s.content)
	case "user":
		return theme.User.Width(maxWidth).Render("[user] " + s.content)
	case "model":
		return theme.Model.Width(maxWidth).Render("[model] " + s.content)
	case "tool-call":
		return theme.Tool.Width(maxWidth).Render("[tool] " + s.content)
	case "tool-result":
		return theme.Result.Width(maxWidth).Render("[result] " + s.content)
	case "error":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true).Width(maxWidth).Render("[error] " + s.content)
	case "done":
		return theme.Result.Width(maxWidth).Bold(true).Render("[done] " + s.content)
	case "checkpoint":
		return theme.System.Width(maxWidth).Render("[checkpoint] " + s.content)
	default:
		return s.content
	}
}

// renderStatusBar renders the bottom status bar with usage statistics.
func (m model) renderStatusBar() string {
	modeStr := "auto"
	if m.stepMode {
		modeStr = "step"
	}
	status := FormatTraceStatus(m.usage, m.status, m.cost, m.elapsed, modeStr)
	return m.theme.Status.Width(m.width).Render(status)
}

// FormatUsage formats usage statistics for display.
func FormatUsage(usage core.RunUsage, mode string) string {
	return FormatTraceStatus(usage, "", "", "", mode)
}

// FormatTraceStatus formats trace status plus usage statistics for display.
func FormatTraceStatus(usage core.RunUsage, status, cost, elapsed, mode string) string {
	var prefix []string
	if status != "" {
		prefix = append(prefix, "status: "+status)
	}
	if elapsed != "" {
		prefix = append(prefix, "elapsed: "+elapsed)
	}
	if cost != "" {
		prefix = append(prefix, "cost: "+cost)
	}
	lead := ""
	if len(prefix) > 0 {
		lead = strings.Join(prefix, " | ") + " | "
	}
	return fmt.Sprintf(
		"%stokens: %d in / %d out | requests: %d | tools: %d | mode: %s | n/p:step g/G:jump e:error c:checkpoint d:diverge q:quit",
		lead,
		usage.InputTokens, usage.OutputTokens,
		usage.Requests, usage.ToolCalls,
		mode,
	)
}

func scrollToFirstKind(steps []step, height int, kind string) int {
	for idx, step := range steps {
		if step.kind != kind {
			continue
		}
		visibleLines := height - 2
		if visibleLines < 1 {
			visibleLines = 1
		}
		scroll := len(steps) - idx - visibleLines
		if scroll < 0 {
			return 0
		}
		return scroll
	}
	return 0
}

func scrollToFirstContent(steps []step, height int, needle string) int {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return 0
	}
	for idx, step := range steps {
		if !strings.Contains(strings.ToLower(step.content), needle) && !strings.Contains(strings.ToLower(step.detail), needle) {
			continue
		}
		visibleLines := height - 2
		if visibleLines < 1 {
			visibleLines = 1
		}
		scroll := len(steps) - idx - visibleLines
		if scroll < 0 {
			return 0
		}
		return scroll
	}
	return 0
}

func splitDetailLines(s step, width, limit int) []string {
	title := "Detail"
	if s.eventKind != "" {
		title += ": " + s.eventKind
	} else if s.kind != "" {
		title += ": " + s.kind
	}
	content := strings.TrimSpace(s.detail)
	if content == "" {
		content = s.content
	}
	lines := []string{truncateDisplay(title, width)}
	for _, line := range strings.Split(content, "\n") {
		for _, wrapped := range wrapDisplayLine(line, width) {
			lines = append(lines, wrapped)
			if len(lines) >= limit {
				return lines
			}
		}
	}
	for len(lines) < limit {
		lines = append(lines, "")
	}
	return lines
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func truncateDisplay(value string, width int) string {
	if width <= 0 || len(value) <= width {
		return value
	}
	if width <= 3 {
		return value[:width]
	}
	return value[:width-3] + "..."
}

func wrapDisplayLine(line string, width int) []string {
	if width <= 0 || len(line) <= width {
		return []string{line}
	}
	var lines []string
	for len(line) > width {
		lines = append(lines, line[:width])
		line = line[width:]
	}
	lines = append(lines, line)
	return lines
}

func traceEventDetail(eventKind string, seq int, replayPolicy string, stepNum int, requestID string, payload map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "event: %s\nseq: %03d\n", eventKind, seq)
	if replayPolicy != "" {
		fmt.Fprintf(&b, "replay: %s\n", replayPolicy)
	}
	if stepNum > 0 {
		fmt.Fprintf(&b, "step: %d\n", stepNum)
	}
	if requestID != "" {
		fmt.Fprintf(&b, "request: %s\n", requestID)
	}
	if len(payload) > 0 {
		data, err := json.MarshalIndent(payload, "", "  ")
		if err == nil {
			fmt.Fprintf(&b, "payload:\n%s", data)
		}
	}
	return b.String()
}

// FormatToolCall formats a tool call for display.
func FormatToolCall(name string, argsJSON string) string {
	// Truncate very long args for display.
	args := argsJSON
	if len(args) > 200 {
		args = args[:200] + "..."
	}
	return fmt.Sprintf("%s(%s)", name, args)
}

// ExtractSteps extracts displayable steps from a list of model messages.
func ExtractSteps(messages []core.ModelMessage) []step {
	var steps []step
	for _, msg := range messages {
		switch m := msg.(type) {
		case core.ModelRequest:
			for _, part := range m.Parts {
				switch p := part.(type) {
				case core.SystemPromptPart:
					steps = append(steps, step{kind: "system", content: p.Content})
				case core.UserPromptPart:
					steps = append(steps, step{kind: "user", content: p.Content})
				case core.ToolReturnPart:
					content := fmt.Sprintf("%v", p.Content)
					steps = append(steps, step{kind: "tool-result", content: fmt.Sprintf("%s: %s", p.ToolName, content)})
				case core.RetryPromptPart:
					steps = append(steps, step{kind: "error", content: p.Content})
				}
			}
		case core.ModelResponse:
			for _, part := range m.Parts {
				switch p := part.(type) {
				case core.TextPart:
					steps = append(steps, step{kind: "model", content: p.Content})
				case core.ToolCallPart:
					steps = append(steps, step{kind: "tool-call", content: FormatToolCall(p.ToolName, p.ArgsJSON)})
				}
			}
		}
	}
	return steps
}

// DebugUI creates and runs a TUI that wraps an agent run, displaying messages
// as they happen with color-coded formatting and a status bar.
func DebugUI[T any](agent *core.Agent[T], prompt string, opts ...Option) (*core.RunResult[T], error) {
	cfg := &config{
		theme: DefaultTheme(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create the bubbletea model.
	m := newModel(cfg.theme)

	// Create and run the program.
	p := tea.NewProgram(m, tea.WithAltScreen())

	// Run the agent in a goroutine.
	var agentResult *core.RunResult[T]
	var agentErr error

	go func() {
		iter := agent.Iter(ctx, prompt)

		// Send initial steps.
		for _, s := range ExtractSteps(iter.Messages()) {
			p.Send(stepMsg{step: s})
		}

		for !iter.Done() {
			resp, err := iter.Next()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				p.Send(doneMsg{err: err})
				return
			}

			if resp != nil {
				for _, part := range resp.Parts {
					switch rp := part.(type) {
					case core.TextPart:
						p.Send(stepMsg{step: step{kind: "model", content: rp.Content}})
					case core.ToolCallPart:
						p.Send(stepMsg{step: step{kind: "tool-call", content: FormatToolCall(rp.ToolName, rp.ArgsJSON)}})
					}
				}

				// Update usage.
				p.Send(stepMsg{step: step{kind: "model", content: ""}})
			}
		}

		result, err := iter.Result()
		if err != nil {
			agentErr = err
			p.Send(doneMsg{err: err})
			return
		}

		agentResult = result
		p.Send(stepMsg{step: step{kind: "done", content: fmt.Sprintf("completed (requests: %d, tokens: %d)", result.Usage.Requests, result.Usage.TotalTokens())}})
		p.Send(doneMsg{err: nil})
	}()

	_, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("TUI error: %w", err)
	}

	if agentErr != nil {
		return nil, agentErr
	}

	return agentResult, nil
}
