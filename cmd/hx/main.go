// hx: CLI for History eXtended.
// Commands: status, pause, resume (M1 slice A/B).

package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mrcawood/History_eXtended/internal/artifact"
	"github.com/mrcawood/History_eXtended/internal/blob"
	"github.com/mrcawood/History_eXtended/internal/cmdutil"
	"github.com/mrcawood/History_eXtended/internal/config"
	"github.com/mrcawood/History_eXtended/internal/db"
	"github.com/mrcawood/History_eXtended/internal/export"
	"github.com/mrcawood/History_eXtended/internal/imp"
	"github.com/mrcawood/History_eXtended/internal/ollama"
	"github.com/mrcawood/History_eXtended/internal/query"
	"github.com/mrcawood/History_eXtended/internal/retention"
	"github.com/mrcawood/History_eXtended/internal/store"
	"github.com/mrcawood/History_eXtended/internal/sync"
)

func getConfig() *config.Config {
	c, err := config.Load()
	if err != nil {
		return nil
	}
	return c
}

func pausedFile() string {
	if c := getConfig(); c != nil {
		return filepath.Join(filepath.Dir(c.SpoolDir), ".paused")
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "hx", ".paused")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "hx", ".paused")
}

func spoolDir() string {
	if c := getConfig(); c != nil {
		return c.SpoolDir
	}
	if v := os.Getenv("HX_SPOOL_DIR"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "hx", "spool")
}

func dbPath() string {
	if c := getConfig(); c != nil {
		return c.DbPath
	}
	if v := os.Getenv("HX_DB_PATH"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "hx", "hx.db")
}

func pidFile() string {
	if c := getConfig(); c != nil {
		return filepath.Join(filepath.Dir(c.DbPath), "hxd.pid")
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "hx", "hxd.pid")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "hx", "hxd.pid")
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
	fmt.Printf("  spool:   %s\n", cmdutil.NormalizePath(spool))
	fmt.Printf("  db:      %s\n", cmdutil.NormalizePath(dbPath()))
	if eventsExist {
		fmt.Printf("  events:  %s\n", cmdutil.NormalizePath(eventsPath))
	}
	cfg := getConfig()
	if cfg != nil {
		if cfg.AllowlistMode {
			fmt.Printf("  allowlist: on (%v)\n", cfg.AllowlistBins)
		} else if len(cfg.IgnorePatterns) > 0 {
			fmt.Printf("  ignore: %v\n", cfg.IgnorePatterns)
		}
	}
}

func cmdPause() {
	dir := filepath.Dir(pausedFile())
	if err := os.MkdirAll(dir, 0750); err != nil {
		fmt.Fprintf(os.Stderr, "hx: cannot create %s: %v\n", dir, err)
		os.Exit(1)
	}
	if err := os.WriteFile(pausedFile(), []byte{}, 0600); err != nil {
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

func cmdLast(args []string) {
	rawTime := false
	for _, a := range args {
		if a == "--raw-time" {
			rawTime = true
			break
		}
	}
	conn, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx last: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()
	sessionID, host, startedAt, events, err := fetchLastSession(conn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx last: %v\n", err)
		os.Exit(1)
	}
	if sessionID == "" {
		fmt.Fprintf(os.Stderr, "hx last: no events found\n")
		os.Exit(0)
	}
	fmt.Printf("Session: %s\n", sessionID)
	fmt.Printf("Host:    %s\n", host)
	fmt.Printf("Started: %s\n", cmdutil.FormatTimestamp(startedAt, rawTime))
	fmt.Printf("Events:  %d\n\n", len(events))
	showSeq := collectShowSeqs(events)
	printLastEvents(events, showSeq)
}

type lastEvent struct {
	seq  int
	exit *int
	cwd  string
	cmd  string
}

func fetchLastSession(conn *sql.DB) (string, string, float64, []lastEvent, error) {
	var sessionID string
	err := conn.QueryRow(`
		SELECT session_id FROM events ORDER BY COALESCE(ended_at, started_at) DESC LIMIT 1
	`).Scan(&sessionID)
	if err != nil {
		return "", "", 0, nil, nil
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
		return "", "", 0, nil, err
	}
	defer func() { _ = rows.Close() }()
	var events []lastEvent
	for rows.Next() {
		var e lastEvent
		if err := rows.Scan(&e.seq, &e.exit, &e.cwd, &e.cmd); err != nil {
			continue
		}
		events = append(events, e)
	}
	return sessionID, host, startedAt, events, nil
}

func collectShowSeqs(events []lastEvent) map[int]bool {
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
	if len(showSeq) == 0 {
		for _, e := range events {
			showSeq[e.seq] = true
		}
	}
	return showSeq
}

func printLastEvents(events []lastEvent, showSeq map[int]bool) {
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

type findOpts struct {
	wide      bool
	compact   bool
	noSelf    bool
	noImport  bool
}

func parseFindArgs(args []string) (string, findOpts) {
	var opts findOpts
	var queryParts []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--wide":
			opts.wide = true
		case "--compact":
			opts.compact = true
		case "--no-self":
			opts.noSelf = true
		case "--no-import":
			opts.noImport = true
		default:
			queryParts = append(queryParts, args[i])
		}
	}
	query := strings.TrimSpace(strings.Join(queryParts, " "))
	if !opts.wide && !opts.compact {
		if os.Getenv("HX_FIND_DEFAULT") == "wide" {
			opts.wide = true
		} else {
			opts.compact = true
		}
	}
	return query, opts
}

func isSelfCmd(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	return cmd == "hx" || strings.HasPrefix(cmd, "hx ") ||
		cmd == "./bin/hx" || strings.HasPrefix(cmd, "./bin/hx ")
}

func cmdFind(args []string) {
	query, opts := parseFindArgs(args)
	if query == "" || query == "--help" || query == "-h" {
		fmt.Fprintf(os.Stderr, "hx find: usage: hx find <text> [--compact|--wide] [--no-self] [--no-import]\n")
		fmt.Fprintf(os.Stderr, "  Set HX_FIND_DEFAULT=wide to keep legacy output.\n")
		if query == "" {
			os.Exit(1)
		}
		os.Exit(0)
	}
	conn, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx find: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()

	// FTS5: escape double-quotes; use as phrase for multi-word
	escaped := strings.ReplaceAll(query, "\"", "\"\"")
	if strings.Contains(escaped, " ") {
		escaped = "\"" + escaped + "\""
	}

	sqlQuery := `
		SELECT e.event_id, e.session_id, e.seq, e.exit_code, e.cwd, COALESCE(c.cmd_text, '')
		FROM events_fts
		JOIN events e ON e.event_id = events_fts.rowid
		LEFT JOIN command_dict c ON e.cmd_id = c.cmd_id
		WHERE events_fts MATCH ?`
	queryArgs := []interface{}{escaped}
	if opts.noImport {
		sqlQuery += ` AND e.session_id NOT LIKE 'import-%'`
	}
	sqlQuery += ` ORDER BY e.started_at DESC LIMIT 100`

	rows, err := conn.Query(sqlQuery, queryArgs...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx find: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = rows.Close() }()

	// Collect rows, apply --no-self filter, limit to 20
	var results []findRow
	for rows.Next() {
		var r findRow
		if err := rows.Scan(&r.eventID, &r.sessionID, &r.seq, &r.exitCode, &r.cwd, &r.cmd); err != nil {
			continue
		}
		if opts.noSelf && isSelfCmd(r.cmd) {
			continue
		}
		results = append(results, r)
		if len(results) >= 20 {
			break
		}
	}

	tw := cmdutil.TerminalWidth()
	w := os.Stdout
	if opts.wide {
		printFindWide(results, tw, w)
	} else {
		printFindCompact(results, tw, w)
	}
	if len(results) == 0 {
		fmt.Println("(no matches)")
	}
}

// findRow holds one hx find result row.
type findRow struct {
	eventID   int64
	sessionID string
	seq       int
	exitCode  *int
	cwd       string
	cmd       string
}

func printFindCompact(rows []findRow, termWidth int, w io.Writer) {
	idW, exitW := 8, 5
	cwdW := 18
	if termWidth < 80 {
		cwdW = 14
	}
	cmdW := termWidth - idW - exitW - cwdW - 6
	if cmdW < 10 {
		cmdW = 10
		cwdW = termWidth - idW - exitW - cmdW - 6
		if cwdW < 8 {
			cwdW = 8
		}
	}

	fmt.Fprintf(w, "%-*s %-*s %-*s %s\n", idW, "id", exitW, "exit", cwdW, "cwd", "cmd")
	fmt.Fprintln(w, strings.Repeat("-", termWidth))

	for _, r := range rows {
		exit := "-"
		if r.exitCode != nil {
			exit = fmt.Sprintf("%d", *r.exitCode)
		}
		cwdNorm := cmdutil.NormalizePath(r.cwd)
		cwdShow := cmdutil.TruncateLeft(cwdNorm, cwdW)
		cmdShow := cmdutil.TruncateRight(r.cmd, cmdW)
		fmt.Fprintf(w, "%-*d %-*s %-*s %s\n", idW, r.eventID, exitW, exit, cwdW, cwdShow, cmdShow)
	}
}

func printFindWide(rows []findRow, termWidth int, w io.Writer) {
	sessW, cwdW := 24, 40
	cmdW := 50
	if termWidth > 80 {
		cmdW = termWidth - 8 - sessW - 4 - 4 - cwdW - 6
		if cmdW < 20 {
			cmdW = 20
		}
	}
	sep := termWidth
	if sep < 100 {
		sep = 100
	}
	fmt.Fprintf(w, "%-8s %-*s %4s %4s %-*s %s\n", "event_id", sessW, "session_id", "seq", "exit", cwdW, "cwd", "cmd")
	fmt.Fprintln(w, strings.Repeat("-", sep))

	for _, r := range rows {
		exit := "-"
		if r.exitCode != nil {
			exit = fmt.Sprintf("%d", *r.exitCode)
		}
		cwdNorm := cmdutil.NormalizePath(r.cwd)
		cwdShow := cmdutil.TruncateRight(cwdNorm, cwdW)
		cmdShow := cmdutil.TruncateRight(r.cmd, cmdW)
		fmt.Fprintf(w, "%-8d %-*s %4d %4s %-*s %s\n", r.eventID, sessW, r.sessionID, r.seq, exit, cwdW, cwdShow, cmdShow)
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
	defer func() { _ = conn.Close() }()
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
	var noLLM bool
	var questionParts []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--file", "-f":
			if i+1 < len(args) {
				filePath = args[i+1]
				i++
			}
		case "--no-llm":
			noLLM = true
		default:
			questionParts = append(questionParts, args[i])
		}
	}
	question := strings.Join(questionParts, " ")

	conn, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx query: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()

	if filePath != "" {
		cmdQueryByFile(conn, filePath)
		return
	}
	if strings.TrimSpace(question) == "" {
		fmt.Fprintf(os.Stderr, "hx query: usage: hx query \"<question>\" [--no-llm]   OR   hx query --file <path>\n")
		os.Exit(1)
	}
	cmdQueryByQuestion(conn, question, noLLM)
}

func cmdQueryByFile(conn *sql.DB, filePath string) {
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

func cmdQueryByQuestion(conn *sql.DB, question string, noLLM bool) {
	cfg := getConfig()
	if cfg == nil {
		cfg = &config.Config{OllamaEnabled: true, OllamaBaseURL: "http://localhost:11434", OllamaEmbedModel: "nomic-embed-text", OllamaChatModel: "llama3.2"}
	}
	candidates, err := query.Retrieve(context.Background(), conn, question, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx query: %v\n", err)
		os.Exit(1)
	}
	if len(candidates) == 0 {
		fmt.Println("No matching events found.")
		return
	}
	fmt.Printf("Evidence (%d):\n\n", len(candidates))
	fmt.Printf("%-8s %-24s %4s %4s %-40s %s\n", "event_id", "session_id", "seq", "exit", "cwd", "cmd")
	fmt.Println(strings.Repeat("-", 100))
	for _, c := range candidates {
		exit := fmt.Sprintf("%d", c.ExitCode)
		cmdShort := c.Cmd
		if len(cmdShort) > 38 {
			cmdShort = cmdShort[:35] + "..."
		}
		cwdShort := c.Cwd
		if len(cwdShort) > 38 {
			cwdShort = cwdShort[:35] + "..."
		}
		fmt.Printf("%-8d %-24s %4d %4s %-40s %s\n", c.EventID, c.SessionID, c.Seq, exit, cwdShort, cmdShort)
	}

	if !noLLM && cfg.OllamaEnabled && ollama.Available(context.Background(), cfg.OllamaBaseURL) {
		topN := 5
		if len(candidates) < topN {
			topN = len(candidates)
		}
		var b strings.Builder
		b.WriteString("Question: ")
		b.WriteString(question)
		b.WriteString("\n\nEvidence snippets (session_id, event_id, cmd):\n")
		for i := 0; i < topN; i++ {
			c := candidates[i]
			b.WriteString("- ")
			b.WriteString(c.SessionID)
			b.WriteString(" event ")
			b.WriteString(fmt.Sprintf("%d", c.EventID))
			b.WriteString(": ")
			b.WriteString(c.Cmd)
			b.WriteString("\n")
		}
		b.WriteString("\nSummarize in 2-3 sentences what the user did, citing session/event IDs. Be concise.")

		summary, err := ollama.Generate(context.Background(), cfg.OllamaBaseURL, cfg.OllamaChatModel, b.String())
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nOllama unavailable, skipping summary: %v\n", err)
		} else if s := strings.TrimSpace(summary); s != "" {
			fmt.Println()
			fmt.Println("Summary:")
			fmt.Println(s)
		}
	}
}

func cmdImport(args []string) {
	var filePath, host, shell string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--file", "-f":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "hx import: --file requires path\n")
				os.Exit(1)
			}
			filePath = args[i+1]
			i++
		case "--host":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "hx import: --host requires value\n")
				os.Exit(1)
			}
			host = args[i+1]
			i++
		case "--shell":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "hx import: --shell requires zsh|bash|auto\n")
				os.Exit(1)
			}
			shell = args[i+1]
			i++
		}
	}
	if filePath == "" {
		fmt.Fprintf(os.Stderr, "hx import: usage: hx import --file <path> [--host label] [--shell zsh|bash|auto]\n")
		os.Exit(1)
	}
	if shell == "" {
		shell = "auto"
	}
	conn, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx import: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()
	inserted, skipped, truncated, err := imp.Run(conn, filePath, host, shell)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx import: %v\n", err)
		os.Exit(1)
	}
	if truncated {
		fmt.Fprintf(os.Stderr, "hx import: file truncated at %d lines (use smaller file or increase limit)\n", imp.MaxLines)
	}
	fmt.Printf("Imported %d events (skipped %d duplicates)\n", inserted, skipped)
}

func cmdPin(args []string) {
	var sessionID string
	useLast := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--session", "-s":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "hx pin: --session requires session ID\n")
				os.Exit(1)
			}
			sessionID = args[i+1]
			i++
		case "last", "--last":
			useLast = true
		}
	}
	conn, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx pin: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()
	st := store.New(conn)
	if useLast && sessionID == "" {
		sid, err := st.LastSessionID()
		if err != nil {
			fmt.Fprintf(os.Stderr, "hx pin: %v\n", err)
			os.Exit(1)
		}
		sessionID = sid
	}
	if sessionID == "" {
		fmt.Fprintf(os.Stderr, "hx pin: usage: hx pin --session <SID>   OR   hx pin --last\n")
		os.Exit(1)
	}
	if err := st.PinSession(sessionID); err != nil {
		if err == sql.ErrNoRows {
			fmt.Fprintf(os.Stderr, "hx pin: session %q not found\n", sessionID)
		} else {
			fmt.Fprintf(os.Stderr, "hx pin: %v\n", err)
		}
		os.Exit(1)
	}
	fmt.Printf("Pinned session %s\n", sessionID)
}

func cmdForget(args []string) {
	var since string
	for i := 0; i < len(args); i++ {
		if args[i] == "--since" && i+1 < len(args) {
			since = args[i+1]
			break
		}
	}
	if since == "" {
		fmt.Fprintf(os.Stderr, "hx forget: usage: hx forget --since 15m|1h|24h|7d\n")
		os.Exit(1)
	}
	d, err := parseSince(since)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx forget: %v\n", err)
		os.Exit(1)
	}
	conn, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx forget: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()
	n, err := retention.ForgetSince(conn, d)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx forget: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Forgot %d events\n", n)
}

func parseSince(s string) (time.Duration, error) {
	if s == "7d" {
		return 7 * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func cmdExport(args []string) {
	sessionID := ""
	useLast := false
	redact := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--session", "-s":
			if i+1 < len(args) {
				sessionID = args[i+1]
				i++
			}
		case "--last":
			useLast = true
		case "--redacted":
			redact = true
		}
	}
	conn, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx export: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()
	if useLast && sessionID == "" {
		st := store.New(conn)
		sid, err := st.LastSessionID()
		if err != nil {
			fmt.Fprintf(os.Stderr, "hx export: %v\n", err)
			os.Exit(1)
		}
		sessionID = sid
	}
	if sessionID == "" {
		fmt.Fprintf(os.Stderr, "hx export: usage: hx export [--session <SID>|--last] [--redacted]\n")
		os.Exit(1)
	}
	exp, err := export.ExportSession(conn, sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx export: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(export.Markdown(exp, redact))
}

func cmdDump() {
	conn, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx dump: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()

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
	defer func() { _ = rows.Close() }()

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

func cmdSync(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "hx sync: usage: hx sync <init|status|push|pull> [options]\n")
		os.Exit(1)
	}
	switch args[0] {
	case "init":
		cmdSyncInit(args[1:])
	case "status":
		cmdSyncStatus()
	case "push":
		cmdSyncPush()
	case "pull":
		cmdSyncPull()
	default:
		fmt.Fprintf(os.Stderr, "hx sync: unknown subcommand %q\n", args[0])
		os.Exit(1)
	}
}

func cmdSyncInit(args []string) {
	var storeArg string
	vaultName := "default"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--store":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "hx sync init: --store requires path\n")
				os.Exit(1)
			}
			storeArg = args[i+1]
			i++
		case "--vault-name":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "hx sync init: --vault-name requires value\n")
				os.Exit(1)
			}
			vaultName = args[i+1]
			i++
		case "--no-encrypt":
			// TODO: store encrypt=false
		}
	}
	if storeArg == "" || !strings.HasPrefix(storeArg, "folder:") {
		fmt.Fprintf(os.Stderr, "hx sync init: usage: hx sync init --store folder:/path/to/HXSync [--vault-name NAME]\n")
		os.Exit(1)
	}
	storePath := strings.TrimPrefix(storeArg, "folder:")
	storePath = strings.TrimSuffix(storePath, "/")

	conn, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx sync init: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()

	vaultID := "vault-" + vaultName
	nodeID := sync.NewNodeID()
	_, err = conn.Exec(`
		INSERT OR REPLACE INTO sync_vaults (vault_id, name, store_type, store_path, encrypt) VALUES (?, ?, 'folder', ?, 0)
	`, vaultID, vaultName, storePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx sync init: %v\n", err)
		os.Exit(1)
	}
	_, err = conn.Exec(`INSERT OR REPLACE INTO sync_nodes (node_id, vault_id, label) VALUES (?, ?, ?)`, nodeID, vaultID, "this-device")
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx sync init: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Sync initialized. Vault: %s, Node: %s, Store: %s\n", vaultID, nodeID, storePath)
}

func cmdSyncStatus() {
	conn, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx sync status: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()

	var vaultID, storePath, nodeID string
	err = conn.QueryRow(`SELECT v.vault_id, v.store_path, n.node_id FROM sync_vaults v JOIN sync_nodes n ON n.vault_id = v.vault_id LIMIT 1`).Scan(&vaultID, &storePath, &nodeID)
	if err != nil {
		fmt.Println("No sync vault configured. Run: hx sync init --store folder:/path/to/HXSync")
		return
	}
	fmt.Printf("Vault: %s\n", vaultID)
	fmt.Printf("Store: %s\n", storePath)
	fmt.Printf("Node:  %s\n", nodeID)

	var pending, imported int
	_ = conn.QueryRow(`SELECT COUNT(*) FROM events WHERE origin='live' AND event_id NOT IN (SELECT event_id FROM sync_published_events WHERE vault_id=?)`, vaultID).Scan(&pending)
	_ = conn.QueryRow(`SELECT COUNT(*) FROM imported_segments WHERE vault_id=?`, vaultID).Scan(&imported)
	fmt.Printf("Pending (unpublished): %d events\n", pending)
	fmt.Printf("Imported segments: %d\n", imported)
}

func cmdSyncPush() {
	conn, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx sync push: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()

	var vaultID, storePath, nodeID string
	err = conn.QueryRow(`SELECT v.vault_id, v.store_path, n.node_id FROM sync_vaults v JOIN sync_nodes n ON n.vault_id = v.vault_id LIMIT 1`).Scan(&vaultID, &storePath, &nodeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx sync push: no sync vault. Run hx sync init first.\n")
		os.Exit(1)
	}
	fs := sync.NewFolderStore(storePath)
	res, err := sync.Push(conn, fs, vaultID, nodeID, nil, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx sync push: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Pushed %d segments (%d events)\n", res.SegmentsPublished, res.EventsPublished)
}

func cmdSyncPull() {
	conn, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx sync pull: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()

	cfg := getConfig()
	blobDir := blob.BlobDir()
	if cfg != nil {
		blobDir = cfg.BlobDir
	}

	var vaultID, storePath string
	err = conn.QueryRow(`SELECT vault_id, store_path FROM sync_vaults LIMIT 1`).Scan(&vaultID, &storePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx sync pull: no sync vault. Run hx sync init first.\n")
		os.Exit(1)
	}
	fs := sync.NewFolderStore(storePath)
	res, err := sync.Import(conn, fs, blobDir, vaultID, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx sync pull: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Imported: %d segments, %d blobs. Skipped: %d segments. Tombstones: %d. Errors: %d\n",
		res.SegmentsImported, res.BlobsImported, res.SegmentsSkipped, res.TombstonesApplied, res.Errors)
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}
	runCommand(os.Args[1], os.Args[2:])
}

func printUsage() {
	fmt.Println("hx: History eXtended - terminal flight recorder")
	fmt.Println("Usage: hx <status|pause|resume|last|dump|find|attach|query|import|pin|forget|export|sync> [args...]")
}

func runCommand(cmd string, args []string) {
	switch cmd {
	case "status":
		cmdStatus()
	case "pause":
		cmdPause()
	case "resume":
		cmdResume()
	case "last":
		cmdLast(args)
	case "dump":
		cmdDump()
	case "find":
		cmdFind(args)
	case "attach":
		cmdAttach(args)
	case "query":
		cmdQuery(args)
	case "import":
		cmdImport(args)
	case "pin":
		cmdPin(args)
	case "forget":
		cmdForget(args)
	case "export":
		cmdExport(args)
	case "sync":
		cmdSync(args)
	default:
		fmt.Fprintf(os.Stderr, "hx: unknown command %q\n", cmd)
		os.Exit(1)
	}
}
