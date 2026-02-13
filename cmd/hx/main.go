// hx: CLI for History eXtended.
// Commands: status, pause, resume (M1 slice A/B).

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/history-extended/hx/internal/artifact"
	"github.com/history-extended/hx/internal/db"
	_ "github.com/mattn/go-sqlite3"
)

func xdgDataHome() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share")
}

func pausedFile() string {
	return filepath.Join(xdgDataHome(), "hx", ".paused")
}

func spoolDir() string {
	if v := os.Getenv("HX_SPOOL_DIR"); v != "" {
		return v
	}
	return filepath.Join(xdgDataHome(), "hx", "spool")
}

func dbPath() string {
	if v := os.Getenv("HX_DB_PATH"); v != "" {
		return v
	}
	return filepath.Join(xdgDataHome(), "hx", "hx.db")
}

func pidFile() string {
	return filepath.Join(xdgDataHome(), "hx", "hxd.pid")
}

func daemonRunning() bool {
	b, err := os.ReadFile(pidFile())
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return false
	}
	// Signal 0 checks if process exists (Unix)
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	return true
}

func cmdStatus() {
	paused := false
	if _, err := os.Stat(pausedFile()); err == nil {
		paused = true
	}
	spool := spoolDir()
	eventsPath := filepath.Join(spool, "events.jsonl")
	eventsExist := false
	if _, err := os.Stat(eventsPath); err == nil {
		eventsExist = true
	}
	capture := "on"
	if paused {
		capture = "paused"
	}
	daemon := "not running"
	if daemonRunning() {
		daemon = "running"
	}
	fmt.Printf("hx status\n")
	fmt.Printf("  capture: %s\n", capture)
	fmt.Printf("  daemon:  %s\n", daemon)
	fmt.Printf("  spool:   %s\n", spool)
	fmt.Printf("  db:      %s\n", dbPath())
	if eventsExist {
		fmt.Printf("  events:  %s\n", eventsPath)
	}
}

func cmdPause() {
	dir := filepath.Dir(pausedFile())
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "hx: cannot create %s: %v\n", dir, err)
		os.Exit(1)
	}
	if err := os.WriteFile(pausedFile(), []byte{}, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "hx: cannot create pause flag: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Capture paused.")
}

func cmdResume() {
	if err := os.Remove(pausedFile()); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "hx: cannot remove pause flag: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Capture resumed.")
}

func cmdLast() {
	conn, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx last: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Last session = session of most recent event
	var sessionID string
	err = conn.QueryRow(`
		SELECT session_id FROM events ORDER BY COALESCE(ended_at, started_at) DESC LIMIT 1
	`).Scan(&sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx last: no events found\n")
		os.Exit(0)
	}
	var host string
	var startedAt float64
	_ = conn.QueryRow(`SELECT host, started_at FROM sessions WHERE session_id = ?`, sessionID).Scan(&host, &startedAt)

	rows, err := conn.Query(`
		SELECT e.seq, e.exit_code, e.cwd, COALESCE(c.cmd_text, '')
		FROM events e
		LEFT JOIN command_dict c ON e.cmd_id = c.cmd_id
		WHERE e.session_id = ?
		ORDER BY e.seq
	`, sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx last: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	type evt struct {
		seq  int
		exit *int
		cwd  string
		cmd  string
	}
	var events []evt
	for rows.Next() {
		var e evt
		if err := rows.Scan(&e.seq, &e.exit, &e.cwd, &e.cmd); err != nil {
			continue
		}
		events = append(events, e)
	}

	fmt.Printf("Session: %s\n", sessionID)
	fmt.Printf("Host:    %s\n", host)
	fmt.Printf("Started: %.0f\n", startedAt)
	fmt.Printf("Events:  %d\n\n", len(events))

	// Context window for failures: show 1 before, 1 after
	showSeq := make(map[int]bool)
	for i, e := range events {
		if e.exit != nil && *e.exit != 0 {
			if i > 0 {
				showSeq[events[i-1].seq] = true
			}
			showSeq[e.seq] = true
			if i < len(events)-1 {
				showSeq[events[i+1].seq] = true
			}
		}
	}
	// If no failures, show all
	if len(showSeq) == 0 {
		for _, e := range events {
			showSeq[e.seq] = true
		}
	}

	for _, e := range events {
		if !showSeq[e.seq] {
			continue
		}
		exit := 0
		if e.exit != nil {
			exit = *e.exit
		}
		mark := "  "
		if exit != 0 {
			mark = "**"
		}
		cmdShort := e.cmd
		if len(cmdShort) > 60 {
			cmdShort = cmdShort[:57] + "..."
		}
		fmt.Printf("%s [%d] exit=%d  %s\n", mark, e.seq, exit, cmdShort)
	}
}

func cmdFind(query string) {
	if query == "" {
		fmt.Fprintf(os.Stderr, "hx find: usage: hx find <text>\n")
		os.Exit(1)
	}
	conn, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx find: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	// FTS5: escape double-quotes; use as phrase for multi-word
	escaped := strings.ReplaceAll(query, "\"", "\"\"")
	if strings.Contains(escaped, " ") {
		escaped = "\"" + escaped + "\""
	}
	rows, err := conn.Query(`
		SELECT e.event_id, e.session_id, e.seq, e.exit_code, e.cwd, COALESCE(c.cmd_text, '')
		FROM events_fts
		JOIN events e ON e.event_id = events_fts.rowid
		LEFT JOIN command_dict c ON e.cmd_id = c.cmd_id
		WHERE events_fts MATCH ?
		ORDER BY e.started_at DESC
		LIMIT 20
	`, escaped)
	if err != nil {
		// Fallback: events_fts may not exist yet
		fmt.Fprintf(os.Stderr, "hx find: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	fmt.Printf("%-8s %-24s %4s %4s %-40s %s\n", "event_id", "session_id", "seq", "exit", "cwd", "cmd")
	fmt.Println(strings.Repeat("-", 100))

	n := 0
	for rows.Next() {
		n++
		var eventID int64
		var sessionID string
		var seq int
		var exitCode *int
		var cwd, cmd string
		if err := rows.Scan(&eventID, &sessionID, &seq, &exitCode, &cwd, &cmd); err != nil {
			continue
		}
		exit := "-"
		if exitCode != nil {
			exit = fmt.Sprintf("%d", *exitCode)
		}
		if len(cmd) > 38 {
			cmd = cmd[:35] + "..."
		}
		if len(cwd) > 38 {
			cwd = cwd[:35] + "..."
		}
		fmt.Printf("%-8d %-24s %4d %4s %-40s %s\n", eventID, sessionID, seq, exit, cwd, cmd)
	}
	if n == 0 {
		fmt.Println("(no matches)")
	}
}

func cmdAttach(args []string) {
	var filePath string
	var linkSessionID string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--file", "-f":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "hx attach: --file requires path\n")
				os.Exit(1)
			}
			filePath = args[i+1]
			i++
		case "--to":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "hx attach: --to requires last or session ID\n")
				os.Exit(1)
			}
			linkSessionID = args[i+1]
			i++
		}
	}
	if filePath == "" {
		fmt.Fprintf(os.Stderr, "hx attach: usage: hx attach --file <path> [--to last|session_id]\n")
		os.Exit(1)
	}
	conn, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx attach: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	st := artifact.New(conn)
	if linkSessionID == "" || linkSessionID == "last" {
		sid, err := st.LastSessionID()
		if err != nil {
			fmt.Fprintf(os.Stderr, "hx attach: %v\n", err)
			os.Exit(1)
		}
		linkSessionID = sid
	}
	aid, err := st.Attach(filePath, linkSessionID, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx attach: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Attached artifact %d to session %s\n", aid, linkSessionID)
}

func cmdQuery(args []string) {
	var filePath string
	for i := 0; i < len(args); i++ {
		if (args[i] == "--file" || args[i] == "-f") && i+1 < len(args) {
			filePath = args[i+1]
			break
		}
	}
	if filePath == "" {
		fmt.Fprintf(os.Stderr, "hx query: usage: hx query --file <path>\n")
		os.Exit(1)
	}
	conn, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx query: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	st := artifact.New(conn)
	sessions, err := st.QueryByFile(filePath, 10)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx query: %v\n", err)
		os.Exit(1)
	}
	if len(sessions) == 0 {
		fmt.Println("No similar artifacts found. Try 'hx attach --file <path> --to last' to link a log first.")
		return
	}
	fmt.Printf("Related sessions (%d):\n\n", len(sessions))
	for _, ls := range sessions {
		events, _ := st.GetSessionEvents(ls.SessionID, 5)
		fmt.Printf("  session: %s (artifact %d)\n", ls.SessionID, ls.ArtifactID)
		for _, e := range events {
			cmdShort := e.Cmd
			if len(cmdShort) > 50 {
				cmdShort = cmdShort[:47] + "..."
			}
			fmt.Printf("    [%d] exit=%d  %s\n", e.Seq, e.ExitCode, cmdShort)
		}
		fmt.Println()
	}
}

func cmdDump() {
	conn, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx dump: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	rows, err := conn.Query(`
		SELECT e.event_id, e.session_id, e.seq, e.exit_code, e.cwd, COALESCE(c.cmd_text, '')
		FROM events e
		LEFT JOIN command_dict c ON e.cmd_id = c.cmd_id
		ORDER BY e.event_id DESC
		LIMIT 20
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx dump: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	fmt.Printf("%-8s %-24s %4s %4s %-40s %s\n", "event_id", "session_id", "seq", "exit", "cwd", "cmd")
	fmt.Println(strings.Repeat("-", 100))

	for rows.Next() {
		var eventID int64
		var sessionID string
		var seq int
		var exitCode *int
		var cwd, cmd string
		if err := rows.Scan(&eventID, &sessionID, &seq, &exitCode, &cwd, &cmd); err != nil {
			continue
		}
		exit := "-"
		if exitCode != nil {
			exit = fmt.Sprintf("%d", *exitCode)
		}
		if len(cmd) > 38 {
			cmd = cmd[:35] + "..."
		}
		if len(cwd) > 38 {
			cwd = cwd[:35] + "..."
		}
		fmt.Printf("%-8d %-24s %4d %4s %-40s %s\n", eventID, sessionID, seq, exit, cwd, cmd)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("hx: History eXtended - terminal flight recorder")
		fmt.Println("Usage: hx <status|pause|resume|last|dump|find|attach|query>")
		os.Exit(0)
	}
	switch os.Args[1] {
	case "status":
		cmdStatus()
	case "pause":
		cmdPause()
	case "resume":
		cmdResume()
	case "last":
		cmdLast()
	case "dump":
		cmdDump()
	case "find":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "hx find: usage: hx find <text>\n")
			os.Exit(1)
		}
		cmdFind(strings.Join(os.Args[2:], " "))
	case "attach":
		cmdAttach(os.Args[2:])
	case "query":
		cmdQuery(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "hx: unknown command %q\n", os.Args[1])
		os.Exit(1)
	}
}
