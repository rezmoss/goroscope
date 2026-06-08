package main

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// renderUI draws the dashboard onto an in-memory simulation screen and returns
// what a terminal would show, line by line. This lets us assert on the live
// view without a real TTY.
func renderUI(t *testing.T, u *ui, w, h int) string {
	t.Helper()
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(w, h)

	u.screen = screen
	u.draw()

	cells, cw, ch := screen.GetContents()
	var b strings.Builder
	for y := range ch {
		for x := range cw {
			runes := cells[y*cw+x].Runes
			if len(runes) == 0 || runes[0] == 0 {
				b.WriteByte(' ')
			} else {
				b.WriteRune(runes[0])
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func TestDashboardShowsGrowingLeak(t *testing.T) {
	u := &ui{collector: NewCollector("http://localhost:6060", 0), tracker: NewTracker(120), top: 20}

	// First poll: 6 leaks. Baseline, so no growth yet.
	u.snap = u.tracker.Observe(Aggregate(Parse(sixLeaks)))
	// Second poll: 14 leaks at the same stack -> Delta +8, leak suspect.
	u.snap = u.tracker.Observe(Aggregate(Parse(fourteenLeaks)))

	out := renderUI(t, u, 90, 16)
	t.Logf("\n%s", out)

	for _, want := range []string{
		"goroscope",
		"14 goroutines",
		"main.leak",
		"+8",           // the growth delta is rendered
		"chan receive", // the leak's state
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}

	// The top group must be the leak (highest count, sorted first).
	if u.snap.Groups[0].Where != "main.leak" || !u.snap.Groups[0].IsLeakSuspect() {
		t.Errorf("top group should be the flagged leak, got %+v", u.snap.Groups[0])
	}
}

func TestSparkline(t *testing.T) {
	got := sparkline([]int{1, 2, 4, 8})
	if r := []rune(got); len(r) != 4 || r[0] != '▁' || r[3] != '█' {
		t.Errorf("sparkline: got %q", got)
	}
	if sparkline(nil) != "" {
		t.Error("empty history should give empty sparkline")
	}
}

func makeLeaks(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString("goroutine ")
		b.WriteString(itoa(i))
		b.WriteString(" [chan receive]:\nmain.leak(...)\n\t/app/examples/leaky-server/main.go:25\n\n")
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

var sixLeaks = makeLeaks(6)
var fourteenLeaks = makeLeaks(14)
