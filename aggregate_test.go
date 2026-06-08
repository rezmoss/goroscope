package main

import (
	"os"
	"testing"
)

func TestAggregateGroupsRealDump(t *testing.T) {
	data, err := os.ReadFile("testdata/sample.txt")
	if err != nil {
		t.Skipf("no captured dump: %v", err)
	}
	snap := Aggregate(Parse(string(data)))

	if snap.Total != 10 {
		t.Errorf("Total: got %d want 10", snap.Total)
	}

	// The six leaked goroutines share one stack, so they must collapse into a
	// single group with Count 6 — that collapsing is the whole point.
	var leak *Group
	for i := range snap.Groups {
		if snap.Groups[i].Where == "main.leak" {
			leak = &snap.Groups[i]
		}
	}
	if leak == nil {
		t.Fatal("no group parked at main.leak")
	}
	if leak.Count != 6 {
		t.Errorf("leak group Count: got %d want 6", leak.Count)
	}
	if leak.State != "chan receive" {
		t.Errorf("leak group State: got %q want %q", leak.State, "chan receive")
	}

	// Groups must be sorted with the biggest first.
	for i := 1; i < len(snap.Groups); i++ {
		if snap.Groups[i-1].Count < snap.Groups[i].Count {
			t.Errorf("groups not sorted by count at %d", i)
		}
	}
}

func TestTrackerDetectsGrowth(t *testing.T) {
	tr := NewTracker(8)

	// First observation establishes the baseline.
	first := Aggregate(Parse(twoLeaks))
	first = tr.Observe(first)
	if first.Groups[0].Delta != 0 {
		t.Errorf("first Delta: got %d want 0", first.Groups[0].Delta)
	}
	if first.Groups[0].IsLeakSuspect() {
		t.Error("should not flag a leak on the very first snapshot")
	}

	// Second observation: the same stack now has more goroutines -> growth.
	second := Aggregate(Parse(fourLeaks))
	second = tr.Observe(second)
	g := second.Groups[0]
	if g.Delta != 2 {
		t.Errorf("growth Delta: got %d want 2", g.Delta)
	}
	if !g.IsLeakSuspect() {
		t.Error("a growing group above the threshold should be a leak suspect")
	}

	if got := tr.History(); len(got) != 2 || got[0] != 2 || got[1] != 4 {
		t.Errorf("history: got %v want [2 4]", got)
	}
}

const twoLeaks = `goroutine 1 [chan receive]:
main.leak(...)
	/app/main.go:25

goroutine 2 [chan receive]:
main.leak(...)
	/app/main.go:25`

const fourLeaks = `goroutine 1 [chan receive]:
main.leak(...)
	/app/main.go:25

goroutine 2 [chan receive]:
main.leak(...)
	/app/main.go:25

goroutine 3 [chan receive]:
main.leak(...)
	/app/main.go:25

goroutine 4 [chan receive]:
main.leak(...)
	/app/main.go:25`
