// prism/internal/events/events.go
// Package events models the JSONL event stream emitted by the prism dlt runner.
// See ADR-006 and the design spec section on IPC.
package events

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Event represents a single structured event emitted by the Python runner over stdout.
type Event struct {
	Event    string `json:"event"`
	Source   string `json:"source,omitempty"`
	Entity   string `json:"entity,omitempty"`
	Rows     int    `json:"rows,omitempty"`
	LoadID   string `json:"load_id,omitempty"`
	Files    int    `json:"files,omitempty"`
	Entities int    `json:"entities,omitempty"`
	Duration int    `json:"duration_ms,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Message  string `json:"message,omitempty"`
}

// Parse reads JSONL events from r, calling fn for each. Non-JSON lines are
// reported via fn as Event{Event:"runner.warn", Message: line}. Returns the
// first hard error (read failure or fn-returned error).
func Parse(r io.Reader, fn func(Event) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			if err := fn(Event{Event: "runner.warn", Message: line}); err != nil {
				return err
			}
			continue
		}
		if err := fn(e); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read events: %w", err)
	}
	return nil
}
