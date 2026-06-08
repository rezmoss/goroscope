package main

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Frame is one stack entry: func + file:line.
type Frame struct {
	Func string
	File string
	Line int
}

// Goroutine is one parsed goroutine from a pprof dump.
type Goroutine struct {
	ID        int
	State     string        // "running", "chan receive", "IO wait", ...
	Wait      time.Duration // blocked duration (0 if not reported)
	WaitText  string        // raw text e.g. "5 minutes"
	Frames    []Frame       // top of stack first
	CreatedBy string        // spawning func, if known
}

// matches "goroutine 18 [select, 5 minutes]:"
var header = regexp.MustCompile(`^goroutine (\d+) \[([^\]]+)\]:$`)

// matches duration hint e.g. "5 minutes"
var waitText = regexp.MustCompile(`(\d+)\s+(minutes?|hours?)`)

// Parse turns a pprof goroutine?debug=2 body into Goroutines. Unparsable frames
// skipped, never fatal.
func Parse(dump string) []Goroutine {
	var out []Goroutine

	// goroutines separated by blank lines
	for _, block := range strings.Split(strings.ReplaceAll(dump, "\r\n", "\n"), "\n\n") {
		block = strings.TrimRight(block, "\n")
		if block == "" {
			continue
		}
		if g, ok := parseBlock(block); ok {
			out = append(out, g)
		}
	}
	return out
}

func parseBlock(block string) (Goroutine, bool) {
	lines := strings.Split(block, "\n")

	m := header.FindStringSubmatch(lines[0])
	if m == nil {
		return Goroutine{}, false
	}

	id, _ := strconv.Atoi(m[1])
	g := Goroutine{ID: id}

	// bracket: "state" or "state, 5 minutes"; first field is state
	g.State, _, _ = strings.Cut(m[2], ", ")
	if _, rest, hasRest := strings.Cut(m[2], ", "); hasRest {
		if w := waitText.FindStringSubmatch(rest); w != nil {
			g.WaitText = w[0]
			g.Wait = parseWait(w[1], w[2])
		}
	}

	// remaining lines: (func, \tfile) pairs, maybe trailing "created by"
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "created by ") {
			g.CreatedBy = cleanFunc(strings.TrimPrefix(line, "created by "))
			i++ // skip its file line
			continue
		}
		if strings.HasPrefix(line, "\t") {
			continue // stray file line
		}
		fr := Frame{Func: cleanFunc(line)}
		if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "\t") {
			fr.File, fr.Line = parseFileLine(lines[i+1])
			i++
		}
		g.Frames = append(g.Frames, fr)
	}

	return g, true
}

func parseWait(n, unit string) time.Duration {
	v, _ := strconv.Atoi(n)
	switch {
	case strings.HasPrefix(unit, "minute"):
		return time.Duration(v) * time.Minute
	case strings.HasPrefix(unit, "hour"):
		return time.Duration(v) * time.Hour
	}
	return 0
}

// cleanFunc strips arg list + "in goroutine N" suffix so same call sites collapse.
// arg list is the last "(...)", so receiver "(*Server)" survives.
//
//	net/http.(*ServeMux).ServeHTTP(0x0, ...) -> net/http.(*ServeMux).ServeHTTP
//	created by main.main in goroutine 1      -> main.main
func cleanFunc(s string) string {
	s = strings.TrimSpace(s)
	if before, _, found := strings.Cut(s, " in goroutine "); found {
		s = before
	}
	if strings.HasSuffix(s, ")") {
		if open := strings.LastIndex(s, "("); open != -1 {
			s = s[:open]
		}
	}
	return strings.TrimSpace(s)
}

func parseFileLine(s string) (string, int) {
	s = strings.TrimSpace(s)
	if before, _, found := strings.Cut(s, " "); found {
		s = before
	}
	if colon := strings.LastIndex(s, ":"); colon != -1 {
		line, _ := strconv.Atoi(s[colon+1:])
		return s[:colon], line
	}
	return s, 0
}
