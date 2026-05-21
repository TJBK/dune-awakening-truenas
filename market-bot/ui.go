package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

type marketUI struct {
	mu        sync.Mutex
	out       io.Writer
	mode      string
	db        string
	catalogN  int
	dryRun    bool
	buyEvery  time.Duration
	listEvery time.Duration
	started   time.Time
	logs      []string
	maxLogs   int
}

func newMarketUI(mode, db string, catalogN int, dryRun bool, buyEvery, listEvery time.Duration) *marketUI {
	return &marketUI{
		out:       os.Stdout,
		mode:      mode,
		db:        db,
		catalogN:  catalogN,
		dryRun:    dryRun,
		buyEvery:  buyEvery,
		listEvery: listEvery,
		started:   time.Now(),
		maxLogs:   12,
	}
}

func (ui *marketUI) Write(p []byte) (int, error) {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		ui.logs = append(ui.logs, line)
		if len(ui.logs) > ui.maxLogs {
			ui.logs = ui.logs[len(ui.logs)-ui.maxLogs:]
		}
	}
	return len(p), nil
}

func (ui *marketUI) Render(ex *Exchange, nextBuy, nextList time.Time) {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	fmt.Fprint(ui.out, "\033[2J\033[H")
	fmt.Fprintln(ui.out, "╭──────────────────────────────────────────────╮")
	fmt.Fprintln(ui.out, "│ Dune Awakening Market Bot                    │")
	fmt.Fprintln(ui.out, "╰──────────────────────────────────────────────╯")
	fmt.Fprintf(ui.out, " Mode: %-8s  Dry-run: %-5t  Paused: %-5t  Uptime: %s\n", ui.mode, ui.dryRun, ex.paused, shortDuration(time.Since(ui.started)))
	fmt.Fprintf(ui.out, " DB: %s\n", ui.db)
	fmt.Fprintf(ui.out, " Catalog: %d items\n", ui.catalogN)
	fmt.Fprintf(ui.out, " Buy every: %-8s  Next buy:  %s\n", ui.buyEvery, formatCountdown(nextBuy))
	fmt.Fprintf(ui.out, " List every: %-7s  Next list: %s\n", ui.listEvery, formatCountdown(nextList))
	fmt.Fprintln(ui.out)
	fmt.Fprintln(ui.out, " Stats")
	fmt.Fprintln(ui.out, " ─────")
	fmt.Fprintf(ui.out, " Bought: %-8d Spent: %-12d Created: %-8d Topped: %-8d Pruned: %-8d Errors: %-8d\n",
		ex.totalBought, ex.totalSpent, ex.totalCreated, ex.totalTopped, ex.totalPruned, ex.totalErrors)
	fmt.Fprintf(ui.out, " Last buy:  %s\n", formatStatusTime(ex.lastBuy))
	fmt.Fprintf(ui.out, " Last list: %s\n", formatStatusTime(ex.lastList))
	fmt.Fprintln(ui.out)
	fmt.Fprintln(ui.out, " Recent log")
	fmt.Fprintln(ui.out, " ──────────")
	if len(ui.logs) == 0 {
		fmt.Fprintln(ui.out, " (no log lines yet)")
	} else {
		for _, line := range ui.logs {
			fmt.Fprintf(ui.out, " %s\n", trimRunes(line, 120))
		}
	}
	fmt.Fprintln(ui.out)
	fmt.Fprintln(ui.out, " Commands then Enter: p pause  b buy  l list  r run  q quit  h help")
	fmt.Fprintln(ui.out, " Ctrl+C quit   -dryrun previews without DB writes   -ui=false disables this screen")
}

func (ui *marketUI) Close() {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	fmt.Fprint(ui.out, "\033[0m\n")
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
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
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
