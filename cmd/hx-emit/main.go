// hx-emit: low-latency event emitter for hx shell hooks.
// Reads pre/post events via args, appends JSONL to spool.
// No-op if .paused exists or spool unwritable.

package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func xdgDataHome() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share")
}

func spoolDir() string {
	if v := os.Getenv("HX_SPOOL_DIR"); v != "" {
		return v
	}
	return filepath.Join(xdgDataHome(), "hx", "spool")
}

func pausedFile() string {
	return filepath.Join(xdgDataHome(), "hx", ".paused")
}

func isPaused() bool {
	_, err := os.Stat(pausedFile())
	return err == nil
}

func ensureSpoolDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

type preEvent struct {
	T    string  `json:"t"`
	Ts   float64 `json:"ts"`
	Sid  string  `json:"sid"`
	Seq  int     `json:"seq"`
	Cmd  string  `json:"cmd"`
	Cwd  string  `json:"cwd"`
	Tty  string  `json:"tty"`
	Host string  `json:"host"`
}

type postEvent struct {
	T     string  `json:"t"`
	Ts    float64 `json:"ts"`
	Sid   string  `json:"sid"`
	Seq   int     `json:"seq"`
	Exit  int     `json:"exit"`
	DurMs int64   `json:"dur_ms"`
	Pipe  []int   `json:"pipe"`
}

func appendEvent(line string) error {
	dir := spoolDir()
	if err := ensureSpoolDir(dir); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + "\n")
	return err
}

func main() {
	if isPaused() {
		os.Exit(0)
	}
	if len(os.Args) < 2 {
		os.Exit(1)
	}
	mode := os.Args[1]
	ts := float64(time.Now().UnixNano()) / 1e9

	switch mode {
	case "pre":
		// pre SID SEQ CMD_B64 CWD TTY HOST
		if len(os.Args) < 8 {
			os.Exit(1)
		}
		cmdB64 := os.Args[4]
		cmd, err := base64.StdEncoding.DecodeString(cmdB64)
		if err != nil {
			cmd = []byte(cmdB64) // fallback: use as literal if not valid base64
		}
		seq, _ := strconv.Atoi(os.Args[3])
		ev := preEvent{
			T:    "pre",
			Ts:   ts,
			Sid:  os.Args[2],
			Seq:  seq,
			Cmd:  string(cmd),
			Cwd:  os.Args[5],
			Tty:  os.Args[6],
			Host: os.Args[7],
		}
		b, _ := json.Marshal(ev)
		if err := appendEvent(string(b)); err != nil {
			os.Exit(1)
		}
	case "post":
		// post SID SEQ EXIT DUR_MS
		if len(os.Args) < 6 {
			os.Exit(1)
		}
		seq, _ := strconv.Atoi(os.Args[3])
		exit, _ := strconv.Atoi(os.Args[4])
		dur, _ := strconv.ParseInt(os.Args[5], 10, 64)
		ev := postEvent{
			T:     "post",
			Ts:    ts,
			Sid:   os.Args[2],
			Seq:   seq,
			Exit:  exit,
			DurMs: dur,
			Pipe:  []int{},
		}
		b, _ := json.Marshal(ev)
		if err := appendEvent(string(b)); err != nil {
			os.Exit(1)
		}
	default:
		os.Exit(1)
	}
	os.Exit(0)
}
