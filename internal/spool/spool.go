package spool

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Event is a pre or post event (discriminated by T field).
type Event struct {
	T      string  `json:"t"`
	Ts     float64 `json:"ts"`
	Sid    string  `json:"sid"`
	Seq    int     `json:"seq"`
	Cmd    string  `json:"cmd"`
	Cwd    string  `json:"cwd"`
	Tty    string  `json:"tty"`
	Host   string  `json:"host"`
	Exit   int     `json:"exit"`
	DurMs  int64   `json:"dur_ms"`
	Pipe   []int   `json:"pipe"`
}

// Read opens events.jsonl and yields parsed events. Skips invalid lines.
func Read(eventsPath string) ([]Event, error) {
	f, err := os.Open(eventsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Event
	sc := bufio.NewScanner(f)
	lineNum := 0
	for sc.Scan() {
		lineNum++
		line := sc.Text()
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue // skip invalid lines
		}
		if e.T != "pre" && e.T != "post" {
			continue
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", eventsPath, err)
	}
	return out, nil
}

// EventsPath returns path to events.jsonl in spool dir.
func EventsPath(spoolDir string) string {
	return filepath.Join(spoolDir, "events.jsonl")
}
