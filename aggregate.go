package main

import (
	"cmp"
	"slices"
	"strings"
)

// Group: goroutines w/ same stack. Few groups + climbing Count = leak.
type Group struct {
	Signature string // frame func names joined
	Count     int    // goroutines w/ this stack
	Delta     int    // change since first seen
	State     string
	Where     string // first non-runtime func
	WhereFile string
	WhereLine int
	CreatedBy string // where spawned
	Stack     []Frame
}

// aggregate view of one dump
type Snapshot struct {
	Total   int
	ByState map[string]int
	Groups  []Group // sorted by Count then Delta desc
}

// Aggregate collapses goroutines into groups keyed by stack.
func Aggregate(gs []Goroutine) Snapshot {
	snap := Snapshot{ByState: map[string]int{}}
	bySig := map[string]*Group{}

	for _, g := range gs {
		snap.Total++
		snap.ByState[g.State]++

		sig := signature(g)
		grp := bySig[sig]
		if grp == nil {
			where, file, line := whereParked(g)
			grp = &Group{
				Signature: sig,
				State:     g.State,
				Where:     where,
				WhereFile: file,
				WhereLine: line,
				CreatedBy: g.CreatedBy,
				Stack:     g.Frames,
			}
			bySig[sig] = grp
		}
		grp.Count++
	}

	for _, grp := range bySig {
		snap.Groups = append(snap.Groups, *grp)
	}
	sortGroups(snap.Groups)
	return snap
}

func sortGroups(gs []Group) {
	slices.SortFunc(gs, func(a, b Group) int {
		if c := cmp.Compare(b.Count, a.Count); c != 0 {
			return c
		}
		return cmp.Compare(b.Delta, a.Delta)
	})
}

// signature: ordered frame func names. same work -> same sig, ignores IDs/ptrs.
func signature(g Goroutine) string {
	var b strings.Builder
	for _, f := range g.Frames {
		b.WriteString(f.Func)
		b.WriteByte('\n')
	}
	return b.String()
}

// whereParked: topmost non-plumbing frame, or frame 0 if all runtime.
func whereParked(g Goroutine) (string, string, int) {
	for _, f := range g.Frames {
		if isPlumbing(f.Func) {
			continue
		}
		return f.Func, f.File, f.Line
	}
	if len(g.Frames) > 0 {
		f := g.Frames[0]
		return f.Func, f.File, f.Line
	}
	return "?", "", 0
}

func isPlumbing(fn string) bool {
	for _, p := range []string{"runtime.", "runtime/", "internal/poll.", "internal/race.", "sync.runtime_", "syscall."} {
		if strings.HasPrefix(fn, p) {
			return true
		}
	}
	return false
}

// Tracker: per-sig first count (for growth) + short total history (sparkline).
type Tracker struct {
	baseline   map[string]int
	history    []int
	maxHistory int
}

func NewTracker(maxHistory int) *Tracker {
	return &Tracker{baseline: map[string]int{}, maxHistory: maxHistory}
}

// Observe records a snap + sets each group's Delta (growth since first seen).
func (t *Tracker) Observe(snap Snapshot) Snapshot {
	for i := range snap.Groups {
		g := &snap.Groups[i]
		base, seen := t.baseline[g.Signature]
		if !seen {
			t.baseline[g.Signature] = g.Count
			base = g.Count
		}
		g.Delta = g.Count - base
	}
	sortGroups(snap.Groups)

	t.history = append(t.history, snap.Total)
	if len(t.history) > t.maxHistory {
		t.history = t.history[len(t.history)-t.maxHistory:]
	}
	return snap
}

// recorded totals, oldest first
func (t *Tracker) History() []int { return t.history }

// grown since start + enough to matter
func (g Group) IsLeakSuspect() bool {
	return g.Delta > 0 && g.Count >= leakMinCount
}

const leakMinCount = 3
