package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/juanpabloaj/workpulse/internal/collect"
	"github.com/juanpabloaj/workpulse/internal/model"
)

var (
	titleStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	headerStyle      = lipgloss.NewStyle().Bold(true)
	detailStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	errorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	blockedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	runningStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	idleStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	doneStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	tableCellStyle   = lipgloss.NewStyle()
	selectedRowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("57"))
	tableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("62"))
)

type refreshMsg struct {
	snapshot model.Snapshot
	err      error
}

type keyMap struct {
	Up         key.Binding
	Down       key.Binding
	Refresh    key.Binding
	ToggleLive key.Binding
	ToggleAll  key.Binding
	Quit       key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Refresh, k.ToggleLive, k.ToggleAll, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Up, k.Down, k.Refresh, k.ToggleLive, k.ToggleAll, k.Quit}}
}

type filterMode string

const (
	filterLiveRecent filterMode = "live+recent"
	filterAll        filterMode = "all"
)

type Model struct {
	ctx       context.Context
	collector *collect.Collector
	help      help.Model
	keys      keyMap
	snapshot  model.Snapshot
	filtered  []model.Session
	err       error
	width     int
	height    int
	mode      filterMode
	cursor    int
	tableRows int
}

func NewModel(ctx context.Context, collector *collect.Collector) Model {
	return Model{
		ctx:       ctx,
		collector: collector,
		help:      help.New(),
		keys: keyMap{
			Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
			Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
			Refresh:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
			ToggleLive: key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "live+recent")),
			ToggleAll:  key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "all")),
			Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		},
		mode: filterLiveRecent,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Refresh):
			return m, m.refreshCmd()
		case key.Matches(msg, m.keys.ToggleLive):
			m.mode = filterLiveRecent
			m.applyRows()
			m.applyLayout()
			return m, nil
		case key.Matches(msg, m.keys.ToggleAll):
			m.mode = filterAll
			m.applyRows()
			m.applyLayout()
			return m, nil
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.applyLayout()
	case refreshMsg:
		m.err = msg.err
		if msg.err == nil {
			m.snapshot = msg.snapshot
			m.applyRows()
		}
		m.applyLayout()
		return m, tickCmd()
	case tickMessage:
		return m, m.refreshCmd()
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
		}
	}

	return m, nil
}

func (m Model) View() string {
	header := m.headerView()
	body := m.tableView()
	detail := m.detailView()
	helpView := m.helpView()
	return strings.Join([]string{header, body, detail, helpView}, "\n\n")
}

func (m *Model) applyLayout() {
	headerHeight := lipgloss.Height(m.headerView())
	detailHeight := detailPanelOuterHeight()
	helpHeight := lipgloss.Height(m.helpView())
	blockSpacing := 3

	available := m.height - headerHeight - detailHeight - helpHeight - blockSpacing
	if available < 3 {
		available = 3
	}
	m.tableRows = available - 1
	if m.tableRows < 1 {
		m.tableRows = 1
	}
}

func (m *Model) applyRows() {
	m.filtered = filterSessions(m.snapshot.Sessions, m.mode)
	if len(m.filtered) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
}

func (m Model) headerView() string {
	header := titleStyle.Render("workpulse") + "  " +
		fmt.Sprintf("sessions %d/%d  mode %s  collected %s", len(m.filtered), len(m.snapshot.Sessions), m.mode, m.snapshot.CollectedAt.Format("15:04:05"))
	if len(m.filtered) > 0 {
		header += "  " + stateSummary(m.filtered)
	}

	if m.err != nil {
		header += "\n" + errorStyle.Render(m.err.Error())
	}
	return header
}

func (m Model) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		snapshot, err := m.collector.Collect(m.ctx)
		return refreshMsg{snapshot: snapshot, err: err}
	}
}

func (m Model) detailView() string {
	if len(m.filtered) == 0 || m.cursor >= len(m.filtered) {
		return detailStyle.Render("No sessions found.")
	}

	s := m.filtered[m.cursor]
	pid := "-"
	cpu := "-"
	ram := "-"
	if s.Process != nil {
		pid = fmt.Sprintf("%d", s.Process.PID)
		cpu = fmt.Sprintf("%.1f%%", s.Process.CPU)
		ram = fmt.Sprintf("%d MB", s.Process.RSSMB)
	}

	panelWidth := max(40, m.width-2)
	frameWidth := detailStyle.GetHorizontalFrameSize()
	contentWidth := max(20, panelWidth-frameWidth)

	lines := []string{
		fmt.Sprintf("%s  %s", headerStyle.Render("Selected"), sessionStateLabel(s.State)),
		fmt.Sprintf("Agent: %s   PID: %s   Project: %s", s.Agent, pid, firstNonEmpty(s.Project, "-")),
		fmt.Sprintf("Session: %s", firstNonEmpty(s.ID, "-")),
		fmt.Sprintf("Model: %s   Provider: %s", firstNonEmpty(s.Model, "-"), firstNonEmpty(s.Provider, "-")),
		fmt.Sprintf("Branch: %s   CPU: %s   RAM: %s", firstNonEmpty(s.Branch, "-"), cpu, ram),
		fmt.Sprintf("Origin: %s", s.Origin),
		fmt.Sprintf("Tool: %s   Calls: %d   Errors: %d", firstNonEmpty(s.Tools.LastTool, "-"), s.Tools.ToolCalls, s.Tools.ToolErrors),
		fmt.Sprintf("Tokens: in %d  out %d  cache %d", s.Usage.InputTokens, s.Usage.OutputTokens, s.Usage.CacheReadTokens),
		fmt.Sprintf("Subagents: %d   Updated: %s", s.Subagents, formatTimeAgo(s.LastEventAt)),
		fmt.Sprintf("CWD: %s", firstNonEmpty(s.CWD, "-")),
		fmt.Sprintf("Last user: %s", firstNonEmpty(s.LastUserText, "-")),
		fmt.Sprintf("Last event: %s", firstNonEmpty(s.LastEvent, "-")),
		fmt.Sprintf("Source: %s", firstNonEmpty(s.Source, "-")),
	}
	lines = truncateLines(lines, contentWidth)
	content := strings.Join(lines, "\n")
	panelHeight := detailPanelContentHeight()
	return detailStyle.Width(panelWidth).Height(panelHeight).Render(content)
}

func (m Model) helpView() string {
	helpView := m.help.View(m.keys)
	if m.width <= 0 {
		return helpView
	}
	return truncatePlainText(helpView, m.width)
}

type columnSpec struct {
	title string
	width int
}

func (m Model) tableView() string {
	columns := []columnSpec{
		{title: "St", width: 3},
		{title: "PID", width: 5},
		{title: "Agent", width: 6},
		{title: "State", width: 7},
		{title: "Project", width: 14},
		{title: "Branch", width: 8},
		{title: "Model", width: 13},
		{title: "Origin", width: 7},
		{title: "Tool", width: 10},
		{title: "CPU%", width: 4},
		{title: "RAM", width: 5},
		{title: "Err", width: 3},
		{title: "Last Event", width: 18},
	}

	headerCells := make([]string, 0, len(columns))
	for _, col := range columns {
		headerCells = append(headerCells, renderHeaderCell(col.title, col.width))
	}

	lines := []string{strings.Join(headerCells, "")}
	if len(m.filtered) == 0 {
		lines = append(lines, "No sessions found.")
		lines = padTableLines(lines, m.tableRows+1)
		return strings.Join(lines, "\n")
	}

	start, end := visibleRange(m.cursor, m.tableRows, len(m.filtered))
	for i := start; i < end; i++ {
		row := renderSessionRow(m.filtered[i], columns)
		if i == m.cursor {
			row = selectedRowStyle.Render(row)
		}
		lines = append(lines, row)
	}
	lines = padTableLines(lines, m.tableRows+1)
	return strings.Join(lines, "\n")
}

func renderSessionRow(s model.Session, columns []columnSpec) string {
	pid := "-"
	cpu := "0.0"
	ram := "0"
	if s.Process != nil {
		pid = fmt.Sprintf("%d", s.Process.PID)
		cpu = fmt.Sprintf("%.1f", s.Process.CPU)
		ram = fmt.Sprintf("%dM", s.Process.RSSMB)
	}

	values := []string{
		stateIcon(s.State),
		pid,
		string(s.Agent),
		string(s.State),
		firstNonEmpty(s.Project, "-"),
		firstNonEmpty(s.Branch, "-"),
		firstNonEmpty(shortenModel(s.Model), "-"),
		shortOrigin(s.Origin),
		firstNonEmpty(shortText(s.Tools.LastTool, 10), "-"),
		cpu,
		ram,
		fmt.Sprintf("%d", s.Tools.ToolErrors),
		firstNonEmpty(shortText(s.LastEvent, 18), "-"),
	}

	cells := make([]string, 0, len(values))
	for i, value := range values {
		cells = append(cells, renderCell(value, columns[i].width))
	}
	return strings.Join(cells, "")
}

func visibleRange(cursor, height, total int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	if height <= 0 || total <= height {
		return 0, total
	}
	start := cursor - height/2
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > total {
		end = total
		start = max(0, end-height)
	}
	return start, end
}

func fitText(value string, width int) string {
	return padRight(shortText(value, width), width)
}

func padTableLines(lines []string, target int) []string {
	if len(lines) >= target {
		return lines
	}
	width := 0
	for _, line := range lines {
		if w := lipgloss.Width(line); w > width {
			width = w
		}
	}
	if width == 0 {
		width = 1
	}
	blank := strings.Repeat(" ", width)
	for len(lines) < target {
		lines = append(lines, blank)
	}
	return lines
}

func renderHeaderCell(value string, width int) string {
	return tableHeaderStyle.Render(" " + fitText(value, width) + " ")
}

func renderCell(value string, width int) string {
	return tableCellStyle.Render(" " + fitText(value, width) + " ")
}

func padRight(value string, width int) string {
	current := lipgloss.Width(value)
	if current >= width {
		return value
	}
	return value + strings.Repeat(" ", width-current)
}

func shortenModel(modelName string) string {
	return shortText(modelName, 13)
}

func filterSessions(sessions []model.Session, mode filterMode) []model.Session {
	if mode == filterAll {
		return append([]model.Session(nil), sessions...)
	}

	filtered := make([]model.Session, 0, len(sessions))
	for _, session := range sessions {
		if session.Process != nil || isRecentForDisplay(session) {
			filtered = append(filtered, session)
		}
	}
	return filtered
}

func isRecentForDisplay(session model.Session) bool {
	if session.LastEventAt.IsZero() {
		return false
	}
	return time.Since(session.LastEventAt) <= 24*time.Hour
}

func shortOrigin(origin model.SessionOrigin) string {
	switch origin {
	case model.OriginLiveIndex:
		return "live"
	case model.OriginTranscript:
		return "recent"
	case model.OriginMerged:
		return "merged"
	default:
		return string(origin)
	}
}

func shortText(value string, width int) string {
	return truncatePlainText(value, width)
}

func sessionStateLabel(state model.SessionState) string {
	label := string(state)
	switch state {
	case model.StateRunning:
		return runningStyle.Render(label)
	case model.StateBlocked:
		return blockedStyle.Render(label)
	case model.StateError:
		return errorStyle.Render(label)
	case model.StateIdle:
		return idleStyle.Render(label)
	default:
		return label
	}
}

func stateSummary(sessions []model.Session) string {
	var blocked, errors, running, idle, done int
	for _, session := range sessions {
		switch session.State {
		case model.StateBlocked:
			blocked++
		case model.StateError:
			errors++
		case model.StateRunning:
			running++
		case model.StateIdle:
			idle++
		case model.StateDone:
			done++
		}
	}

	parts := []string{
		blockedStyle.Render(fmt.Sprintf("blocked %d", blocked)),
		errorStyle.Render(fmt.Sprintf("error %d", errors)),
		runningStyle.Render(fmt.Sprintf("running %d", running)),
		idleStyle.Render(fmt.Sprintf("idle %d", idle)),
		doneStyle.Render(fmt.Sprintf("done %d", done)),
	}
	return strings.Join(parts, "  ")
}

func stateIcon(state model.SessionState) string {
	switch state {
	case model.StateRunning:
		return "●"
	case model.StateBlocked:
		return "◆"
	case model.StateError:
		return "▲"
	case model.StateIdle:
		return "○"
	case model.StateDone:
		return "·"
	default:
		return "?"
	}
}

type tickMessage struct{}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return tickMessage{}
	})
}

func formatTimeAgo(ts time.Time) string {
	if ts.IsZero() {
		return "-"
	}
	d := time.Since(ts).Round(time.Second)
	return d.String() + " ago"
}

func detailPanelContentHeight() int {
	return 14
}

func detailPanelOuterHeight() int {
	return detailPanelContentHeight() + 2
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func truncateLines(lines []string, width int) []string {
	if width <= 0 {
		return lines
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, truncatePlainText(line, width))
	}
	return out
}

func truncatePlainText(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}

	runes := []rune(value)
	out := make([]rune, 0, len(runes))
	for _, r := range runes {
		next := string(append(out, r))
		if lipgloss.Width(next) >= width {
			break
		}
		out = append(out, r)
	}
	return string(out) + "…"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
