// Command goroscope is a live goroutine visualizer + leak detector. Points at a
// Go process exposing net/http/pprof, polls its goroutine dump, groups by stack;
// a leak shows as one group whose count keeps climbing.
//
//	goroscope                       # watch http://localhost:6060 live
//	goroscope -url host:6060        # remote target
//	goroscope -snapshot             # print one report and exit
package main

import (
	"cmp"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"
)

func main() {
	url := flag.String("url", "http://localhost:6060", "base URL of the target's pprof endpoint")
	interval := flag.Duration("interval", time.Second, "refresh interval for live mode")
	snapshot := flag.Bool("snapshot", false, "print one aggregated report to stdout and exit")
	top := flag.Int("top", 20, "maximum number of stack groups to show")
	flag.Parse()

	collector := NewCollector(*url, 5*time.Second)

	if *snapshot {
		if err := runSnapshot(os.Stdout, collector, *top); err != nil {
			fmt.Fprintln(os.Stderr, "goroscope:", err)
			os.Exit(1)
		}
		return
	}

	if err := runTUI(collector, *interval, *top); err != nil {
		fmt.Fprintln(os.Stderr, "goroscope:", err)
		os.Exit(1)
	}
}

// runSnapshot: fetch once, aggregate, print text. scriptable counterpart to TUI.
func runSnapshot(w io.Writer, c *Collector, top int) error {
	gs, err := c.Fetch()
	if err != nil {
		return fmt.Errorf("fetch goroutines: %w", err)
	}
	snap := Aggregate(gs)

	fmt.Fprintf(w, "goroscope  ▸ %s\n", c.URL)
	fmt.Fprintf(w, "%s in %s\n\n", plural(snap.Total, "goroutine"), plural(len(snap.Groups), "distinct stack"))
	fmt.Fprintf(w, "STATES  %s\n\n", stateSummary(snap.ByState))

	fmt.Fprintf(w, "  %-6s %-14s %s\n", "COUNT", "STATE", "WHERE")
	for i, g := range snap.Groups {
		if i >= top {
			fmt.Fprintf(w, "  ... %d more stacks\n", len(snap.Groups)-top)
			break
		}
		where := g.Where
		if g.WhereFile != "" {
			where = fmt.Sprintf("%s  (%s:%d)", g.Where, shortFile(g.WhereFile), g.WhereLine)
		}
		fmt.Fprintf(w, "  %-6d %-14s %s\n", g.Count, g.State, where)
	}
	return nil
}

// stateSummary -> "chan receive:6  IO wait:2  running:1", stable order.
func stateSummary(byState map[string]int) string {
	type kv struct {
		state string
		n     int
	}
	pairs := make([]kv, 0, len(byState))
	for s, n := range byState {
		pairs = append(pairs, kv{s, n})
	}
	slices.SortFunc(pairs, func(a, b kv) int {
		if c := cmp.Compare(b.n, a.n); c != 0 {
			return c
		}
		return cmp.Compare(a.state, b.state)
	})
	var parts []string
	for _, p := range pairs {
		parts = append(parts, fmt.Sprintf("%s:%d", p.state, p.n))
	}
	return strings.Join(parts, "  ")
}

// plural -> "1 goroutine" / "2 goroutines".
func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// shortFile keeps last 2 path elems: ".../leaky-server/main.go" -> "leaky-server/main.go".
func shortFile(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) <= 2 {
		return path
	}
	return strings.Join(parts[len(parts)-2:], "/")
}
