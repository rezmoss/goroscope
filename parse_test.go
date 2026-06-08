package main

import (
	"os"
	"testing"
	"time"
)

const sampleBlock = `goroutine 18 [select, 5 minutes]:
main.worker(0xc000112000, 0x3)
	/app/main.go:42 +0x118
net/http.(*ServeMux).ServeHTTP(0x0?, {0x101429490, 0x157eb733a2d0}, 0x157eb727e500)
	/usr/local/go/src/net/http/server.go:2828 +0x190
created by main.run in goroutine 1
	/app/main.go:20 +0x58

goroutine 1 [IO wait]:
main.leak(...)
	/app/main.go:25`

func TestParseFields(t *testing.T) {
	gs := Parse(sampleBlock)
	if len(gs) != 2 {
		t.Fatalf("expected 2 goroutines, got %d", len(gs))
	}

	g := gs[0]
	if g.ID != 18 {
		t.Errorf("ID: got %d want 18", g.ID)
	}
	if g.State != "select" {
		t.Errorf("State: got %q want %q", g.State, "select")
	}
	if g.Wait != 5*time.Minute {
		t.Errorf("Wait: got %v want 5m", g.Wait)
	}
	if g.WaitText != "5 minutes" {
		t.Errorf("WaitText: got %q want %q", g.WaitText, "5 minutes")
	}
	if g.CreatedBy != "main.run" {
		t.Errorf("CreatedBy: got %q want %q", g.CreatedBy, "main.run")
	}
	if len(g.Frames) != 2 {
		t.Fatalf("Frames: got %d want 2", len(g.Frames))
	}
	if g.Frames[0].Func != "main.worker" {
		t.Errorf("Frames[0].Func: got %q want %q", g.Frames[0].Func, "main.worker")
	}
	if g.Frames[0].File != "/app/main.go" || g.Frames[0].Line != 42 {
		t.Errorf("Frames[0] file:line: got %s:%d want /app/main.go:42", g.Frames[0].File, g.Frames[0].Line)
	}
	// The method receiver must survive; only the arg list is stripped.
	if g.Frames[1].Func != "net/http.(*ServeMux).ServeHTTP" {
		t.Errorf("Frames[1].Func: got %q want %q", g.Frames[1].Func, "net/http.(*ServeMux).ServeHTTP")
	}

	// Elided args "(...)" must be stripped just like a normal arg list.
	if gs[1].Frames[0].Func != "main.leak" {
		t.Errorf("leak Func: got %q want %q", gs[1].Frames[0].Func, "main.leak")
	}
}

func TestParseRealDump(t *testing.T) {
	data, err := os.ReadFile("testdata/sample.txt")
	if err != nil {
		t.Skipf("no captured dump: %v", err)
	}
	gs := Parse(string(data))
	if len(gs) == 0 {
		t.Fatal("parsed zero goroutines from real dump")
	}

	var leaks int
	for _, g := range gs {
		if len(g.Frames) == 0 {
			t.Errorf("goroutine %d has no frames", g.ID)
		}
		if g.State == "" {
			t.Errorf("goroutine %d has empty state", g.ID)
		}
		for _, f := range g.Frames {
			if f.Func == "" {
				t.Errorf("goroutine %d has an empty frame func", g.ID)
			}
		}
		if g.Frames[0].Func == "main.leak" {
			leaks++
		}
	}
	if leaks == 0 {
		t.Error("expected to find leaked main.leak goroutines in the real dump")
	}
	t.Logf("parsed %d goroutines, %d leaking at main.leak", len(gs), leaks)
}
