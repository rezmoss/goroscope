package main

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
)

// tick: tcell event posted each refresh so loop treats refetch like a keypress.
type tick struct{ at time.Time }

func (t tick) When() time.Time { return t.at }

// ui holds dashboard state between frames.
type ui struct {
	screen    tcell.Screen
	collector *Collector
	tracker   *Tracker
	top       int

	snap     Snapshot // last good aggregate
	err      error    // last fetch err
	selected int      // highlighted row
	showHelp bool
}

// runTUI drives the live dashboard until quit.
func runTUI(c *Collector, interval time.Duration, top int) error {
	screen, err := tcell.NewScreen()
	if err != nil {
		return fmt.Errorf("new screen: %w", err)
	}
	if err := screen.Init(); err != nil {
		return fmt.Errorf("init screen: %w", err)
	}
	defer screen.Fini()
	screen.SetStyle(tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite))

	u := &ui{screen: screen, collector: c, tracker: NewTracker(120), top: top}
	u.refresh()
	u.draw()

	// ticker -> event on interval
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case now := <-t.C:
				screen.PostEvent(tick{now})
			}
		}
	}()

	for {
		switch ev := screen.PollEvent().(type) {
		case *tcell.EventResize:
			screen.Sync()
			u.draw()
		case tick:
			u.refresh()
			u.draw()
		case *tcell.EventKey:
			if u.handleKey(ev) {
				return nil
			}
			u.draw()
		}
	}
}

// refresh pulls a fresh dump into the tracker.
func (u *ui) refresh() {
	gs, err := u.collector.Fetch()
	if err != nil {
		u.err = err
		return
	}
	u.err = nil
	u.snap = u.tracker.Observe(Aggregate(gs))
	if u.selected >= len(u.snap.Groups) {
		u.selected = max(0, len(u.snap.Groups)-1)
	}
}

// handleKey returns true to quit.
func (u *ui) handleKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyEscape, tcell.KeyCtrlC:
		return true
	case tcell.KeyUp:
		u.selected = max(0, u.selected-1)
	case tcell.KeyDown:
		u.selected = min(len(u.snap.Groups)-1, u.selected+1)
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'q':
			return true
		case 'h', '?':
			u.showHelp = !u.showHelp
		}
	}
	return false
}

func (u *ui) draw() {
	u.screen.Clear()
	width, height := u.screen.Size()

	yellow := tcell.StyleDefault.Foreground(tcell.ColorYellow)
	green := tcell.StyleDefault.Foreground(tcell.ColorGreen)
	red := tcell.StyleDefault.Foreground(tcell.ColorRed)
	dim := tcell.StyleDefault.Foreground(tcell.ColorGray)

	// header: target, total, sparkline
	header := fmt.Sprintf("goroscope  ▸ %s", u.collector.URL)
	drawText(u.screen, 1, 0, header, yellow.Bold(true))
	if u.err != nil {
		drawText(u.screen, 1, 1, "fetch error: "+u.err.Error(), red)
	} else {
		total := fmt.Sprintf("%s in %s", plural(u.snap.Total, "goroutine"), plural(len(u.snap.Groups), "stack"))
		drawText(u.screen, 1, 1, total, green)
		spark := sparkline(u.tracker.History())
		drawText(u.screen, len(total)+3, 1, spark, tcell.StyleDefault.Foreground(tcell.ColorAqua))
	}
	drawText(u.screen, 1, 2, "STATES  "+stateSummary(u.snap.ByState), dim)

	const top = 4
	drawText(u.screen, 1, top, fmt.Sprintf("%-7s %-6s %-14s %s", "COUNT", "Δ", "STATE", "WHERE"), green.Bold(true))

	// rows: leak suspects red, still-growing yellow
	rows := height - top - 3
	for i, g := range u.snap.Groups {
		if i >= rows || i >= u.top {
			break
		}
		y := top + 1 + i
		style := tcell.StyleDefault
		switch {
		case g.IsLeakSuspect():
			style = red.Bold(true)
		case g.Delta > 0:
			style = yellow
		}
		if i == u.selected {
			style = style.Reverse(true)
		}
		delta := ""
		if g.Delta != 0 {
			delta = fmt.Sprintf("%+d", g.Delta)
		}
		where := truncate(fmt.Sprintf("%s  (%s:%d)", g.Where, shortFile(g.WhereFile), g.WhereLine), width-32)
		drawText(u.screen, 1, y, fmt.Sprintf("%-7d %-6s %-14s %s", g.Count, delta, g.State, where), style)
	}

	// detail pane or help, then footer
	if u.showHelp {
		drawText(u.screen, 1, height-2, "↑/↓ select   h/? help   q/Esc quit", dim)
	} else if len(u.snap.Groups) > 0 {
		g := u.snap.Groups[min(u.selected, len(u.snap.Groups)-1)]
		created := g.CreatedBy
		if created == "" {
			created = "(top-level)"
		}
		detail := fmt.Sprintf("selected: %d goroutines • state %s • created by %s", g.Count, g.State, created)
		drawText(u.screen, 1, height-2, truncate(detail, width-2), tcell.StyleDefault.Foreground(tcell.ColorAqua))
	}
	drawText(u.screen, 1, height-1, "h/? help   q quit", dim)

	u.screen.Show()
}

// drawText writes text at (x,y), one cell at a time.
func drawText(s tcell.Screen, x, y int, text string, style tcell.Style) {
	for _, r := range text {
		s.SetContent(x, y, r, nil, style)
		x++
	}
}

func truncate(s string, maxLen int) string {
	if maxLen < 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen-1]) + "…"
}

// sparkline renders counts as block chars, scaled lo..hi.
func sparkline(vals []int) string {
	if len(vals) == 0 {
		return ""
	}
	blocks := []rune("▁▂▃▄▅▆▇█")
	lo, hi := vals[0], vals[0]
	for _, v := range vals {
		lo = min(lo, v)
		hi = max(hi, v)
	}
	out := make([]rune, len(vals))
	for i, v := range vals {
		idx := 0
		if hi > lo {
			idx = (v - lo) * (len(blocks) - 1) / (hi - lo)
		}
		out[i] = blocks[idx]
	}
	return string(out)
}
