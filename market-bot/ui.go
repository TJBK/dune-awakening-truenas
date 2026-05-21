package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type uiSnapshot struct {
	mode      string
	db        string
	catalogN  int
	dryRun    bool
	paused    bool
	buyEvery  time.Duration
	listEvery time.Duration
	started   time.Time
	nextBuy   time.Time
	nextList  time.Time

	bought  int64
	spent   int64
	created int64
	topped  int64
	pruned  int64
	errors  int64
	lastBuy time.Time
	lastLst time.Time
}

type uiLogMsg string
type uiSnapshotMsg uiSnapshot

type marketUI struct {
	program *tea.Program
	cmds    chan string
	base    uiSnapshot
}

func newMarketUI(mode, db string, catalogN int, dryRun bool, buyEvery, listEvery time.Duration) *marketUI {
	ui := &marketUI{
		cmds: make(chan string, 8),
		base: uiSnapshot{mode: mode, db: db, catalogN: catalogN, dryRun: dryRun, buyEvery: buyEvery, listEvery: listEvery, started: time.Now()},
	}
	m := marketUIModel{cmds: ui.cmds, snap: ui.base, tab: 0, maxLogs: 14}
	ui.program = tea.NewProgram(m, tea.WithAltScreen())
	go func() {
		_, _ = ui.program.Run()
	}()
	return ui
}

func (ui *marketUI) Commands() <-chan string { return ui.cmds }

func (ui *marketUI) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && ui.program != nil {
			ui.program.Send(uiLogMsg(line))
		}
	}
	return len(p), nil
}

func (ui *marketUI) Render(ex *Exchange, nextBuy, nextList time.Time) {
	if ui.program == nil {
		return
	}
	s := ui.base
	s.nextBuy = nextBuy
	s.nextList = nextList
	s.paused = ex.paused
	s.bought = ex.totalBought
	s.spent = ex.totalSpent
	s.created = ex.totalCreated
	s.topped = ex.totalTopped
	s.pruned = ex.totalPruned
	s.errors = ex.totalErrors
	s.lastBuy = ex.lastBuy
	s.lastLst = ex.lastList
	ui.program.Send(uiSnapshotMsg(s))
}

func (ui *marketUI) Close() {
	if ui.program != nil {
		ui.program.Quit()
	}
}

type marketUIModel struct {
	cmds    chan<- string
	snap    uiSnapshot
	logs    []string
	maxLogs int
	tab     int
	w       int
	h       int
}

var (
	uiOrange = lipgloss.Color("#FFB000")
	uiGreen  = lipgloss.Color("#44FF88")
	uiRed    = lipgloss.Color("#FF5555")
	uiDim    = lipgloss.Color("#777777")
	uiPanel  = lipgloss.Color("#242424")

	uiTitle = lipgloss.NewStyle().Bold(true).Foreground(uiOrange)
	uiOK    = lipgloss.NewStyle().Bold(true).Foreground(uiGreen)
	uiBad   = lipgloss.NewStyle().Bold(true).Foreground(uiRed)
	uiHelp  = lipgloss.NewStyle().Foreground(uiDim)
	uiBox   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(uiDim).Padding(0, 1)
)

func (m marketUIModel) Init() tea.Cmd { return nil }

func (m marketUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case uiLogMsg:
		m.logs = append(m.logs, string(msg))
		if len(m.logs) > m.maxLogs {
			m.logs = m.logs[len(m.logs)-m.maxLogs:]
		}
	case uiSnapshotMsg:
		m.snap = uiSnapshot(msg)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.send("q")
			return m, tea.Quit
		case "tab", "right", "l":
			m.tab = (m.tab + 1) % 3
		case "shift+tab", "left", "h":
			m.tab = (m.tab + 2) % 3
		case "p":
			m.send("p")
		case "b":
			m.send("b")
		case "n":
			m.send("l")
		case "r":
			m.send("r")
		case "?":
			m.send("h")
		}
	}
	return m, nil
}

func (m marketUIModel) send(cmd string) {
	select {
	case m.cmds <- cmd:
	default:
	}
}

func (m marketUIModel) View() string {
	if m.w == 0 {
		return "Loading market bot…"
	}
	bodyW := m.w - 4
	if bodyW < 60 {
		bodyW = 60
	}
	var body string
	switch m.tab {
	case 1:
		body = m.viewActivity(bodyW)
	case 2:
		body = m.viewConfig(bodyW)
	default:
		body = m.viewOverview(bodyW)
	}
	header := uiTitle.Render("Dune Awakening Market Bot") + "  " + m.tabs()
	footer := uiHelp.Render("p pause  b buy now  n list now  r full tick  tab switch  q quit")
	return header + "\n" + body + "\n" + footer
}

func (m marketUIModel) tabs() string {
	labels := []string{"Overview", "Activity", "Config"}
	out := make([]string, len(labels))
	for i, l := range labels {
		if i == m.tab {
			out[i] = uiOK.Render("[" + l + "]")
		} else {
			out[i] = uiHelp.Render(" " + l + " ")
		}
	}
	return strings.Join(out, " ")
}

func (m marketUIModel) viewOverview(w int) string {
	state := uiOK.Render("RUNNING")
	if m.snap.paused {
		state = uiBad.Render("PAUSED")
	}
	dry := "false"
	if m.snap.dryRun {
		dry = uiTitle.Render("true")
	}
	left := fmt.Sprintf("Mode: %-8s State: %s\nDry-run: %s\nDB: %s\nCatalog: %d items\nUptime: %s",
		m.snap.mode, state, dry, m.snap.db, m.snap.catalogN, shortDuration(time.Since(m.snap.started)))
	right := fmt.Sprintf("Next buy:  %s\nNext list: %s\nBuy every:  %s\nList every: %s",
		formatCountdown(m.snap.nextBuy), formatCountdown(m.snap.nextList), m.snap.buyEvery, m.snap.listEvery)
	stats := fmt.Sprintf("Bought: %-8d Spent: %-12d Created: %-8d Topped: %-8d Pruned: %-8d Errors: %-8d\nLast buy:  %s\nLast list: %s",
		m.snap.bought, m.snap.spent, m.snap.created, m.snap.topped, m.snap.pruned, m.snap.errors, formatStatusTime(m.snap.lastBuy), formatStatusTime(m.snap.lastLst))
	cols := lipgloss.JoinHorizontal(lipgloss.Top,
		uiBox.Width(w/2-3).Render(left),
		uiBox.Width(w/2-3).Render(right),
	)
	return cols + "\n" + uiBox.Width(w-2).Render(uiTitle.Render("Stats")+"\n"+stats) + "\n" + m.recent(w)
}

func (m marketUIModel) viewActivity(w int) string {
	return uiBox.Width(w - 2).Render(uiTitle.Render("Activity Log") + "\n" + strings.Join(m.logLines(w-8), "\n"))
}

func (m marketUIModel) viewConfig(w int) string {
	cfg := fmt.Sprintf("Mode: %s\nDatabase: %s\nDry-run: %t\nCatalog items: %d\nBuy interval: %s\nList interval: %s\n\nKeys:\n  p pause/resume\n  b buy tick now\n  n list tick now\n  r full tick now\n  q quit",
		m.snap.mode, m.snap.db, m.snap.dryRun, m.snap.catalogN, m.snap.buyEvery, m.snap.listEvery)
	return uiBox.Width(w - 2).Render(uiTitle.Render("Config / Controls") + "\n" + cfg)
}

func (m marketUIModel) recent(w int) string {
	return uiBox.Width(w - 2).Render(uiTitle.Render("Recent") + "\n" + strings.Join(m.logLines(w-8), "\n"))
}

func (m marketUIModel) logLines(max int) []string {
	if len(m.logs) == 0 {
		return []string{uiHelp.Render("no log lines yet")}
	}
	lines := make([]string, 0, len(m.logs))
	for _, l := range m.logs {
		lines = append(lines, trimRunes(l, max))
	}
	return lines
}

func formatCountdown(t time.Time) string {
	if t.IsZero() {
		return "now"
	}
	d := time.Until(t)
	if d <= 0 {
		return "now"
	}
	return shortDuration(d)
}

func shortDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	mi := d / time.Minute
	d -= mi * time.Minute
	s := d / time.Second
	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", h, mi, s)
	}
	if mi > 0 {
		return fmt.Sprintf("%dm%02ds", mi, s)
	}
	return fmt.Sprintf("%ds", s)
}

func trimRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
