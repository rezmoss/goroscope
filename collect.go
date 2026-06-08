package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Collector fetches goroutine dumps from a pprof endpoint.
type Collector struct {
	URL    string
	client *http.Client
}

// NewCollector for a base URL e.g. "http://localhost:6060"; appends pprof path.
func NewCollector(base string, timeout time.Duration) *Collector {
	base = strings.TrimRight(base, "/")
	if !strings.HasPrefix(base, "http") {
		base = "http://" + base
	}
	return &Collector{
		URL:    base + "/debug/pprof/goroutine?debug=2",
		client: &http.Client{Timeout: timeout},
	}
}

// Fetch gets + parses current goroutines.
func (c *Collector) Fetch() ([]Goroutine, error) {
	resp, err := c.client.Get(c.URL)
	if err != nil {
		return nil, fmt.Errorf("reaching %s: %w", c.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", c.URL, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	return Parse(string(body)), nil
}
