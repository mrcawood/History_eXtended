// hx: CLI for History eXtended.
// Commands: status, pause, resume (M1 slice A/B).

package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	"github.com/mrcawood/History_eXtended/internal/spool"
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

func cmdDebug() {
	fmt.Println("hx debug")
	fmt.Println("")

	// 1. Pidfile validity
	pidPath := pidFile()
	pidOk := false
	var pid int
	if b, err := os.ReadFile(pidPath); err == nil {
		if p, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && p > 0 {
			pid = p
			if err := syscall.Kill(pid, 0); err == nil {
				pidOk = true
			}
		}
	}
	if pidOk {
		comm := processComm(pid)
		isHxd := strings.Contains(comm, "hxd")
		if isHxd {
			fmt.Printf("  daemon:  running (pid %d) ✓\n", pid)
		} else {
			fmt.Printf("  daemon:  WARN pid %d is %q, expected hxd (stale pidfile?)\n", pid, comm)
		}
	} else {
		fmt.Printf("  daemon:  not running (no pidfile or process gone)\n")
	}

	// 2. Spool
	spoolDir := spoolDir()
	eventsPath := spool.EventsPath(spoolDir)
	spoolExists := false
	var spoolMtime time.Time
	var spoolLines int64
	if fi, err := os.Stat(eventsPath); err == nil {
		spoolExists = true
		spoolMtime = fi.ModTime()
		if f, err := os.Open(eventsPath); err == nil {
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				spoolLines++
			}
			_ = f.Close()
		}
	}
	if spoolExists {
		fmt.Printf("  spool:   %s\n", cmdutil.NormalizePath(eventsPath))
		fmt.Printf("           %d lines, mtime %s\n", spoolLines, spoolMtime.Format("2006-01-02 15:04:05"))
	} else {
		fmt.Printf("  spool:   %s - MISSING (no capture; hooks not loaded?)\n", cmdutil.NormalizePath(eventsPath))
	}

	// 3. DB
	dbPath := dbPath()
	conn, err := db.Open(dbPath)
	if err != nil {
		fmt.Printf("  db:      %s - ERROR %v\n", cmdutil.NormalizePath(dbPath), err)
		fmt.Println("")
		return
	}
	defer func() { _ = conn.Close() }()
	var eventCount int
	_ = conn.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&eventCount)
	var lastStarted float64
	_ = conn.QueryRow(`SELECT MAX(started_at) FROM events`).Scan(&lastStarted)
	fmt.Printf("  db:      %s\n", cmdutil.NormalizePath(dbPath))
	fmt.Printf("           %d events", eventCount)
	if eventCount > 0 && lastStarted > 0 {
		fmt.Printf(", newest %s", cmdutil.FormatTimestamp(lastStarted, false))
	}
	fmt.Println()
	fmt.Println("")
}

func processComm(pid int) string {
	// Try /proc on Linux first
	commPath := filepath.Join("/proc", strconv.Itoa(pid), "comm")
	if b, err := os.ReadFile(commPath); err == nil {
		return strings.TrimSpace(string(b))
	}
	// Fallback: ps (Unix); pid from daemon config, not user input
	// #nosec G204
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return "?"
	}
	return strings.TrimSpace(string(out))
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
	wide        bool
	compact     bool
	debug       bool
	forceWide   bool
	includeSelf bool // default false = exclude self; --include-self to show
	noSelf      bool // backwards compat: same as default
	noImport    bool
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
		case "--debug":
			opts.debug = true
		case "--force-wide":
			opts.forceWide = true
		case "--include-self":
			opts.includeSelf = true
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
	if cmd == "hx" || strings.HasPrefix(cmd, "hx ") {
		return true
	}
	if cmd == "./bin/hx" || strings.HasPrefix(cmd, "./bin/hx ") {
		return true
	}
	// Piped or env-prefixed: "COLUMNS=80 hx find make", "foo | hx query make"
	if strings.Contains(cmd, "| hx ") || strings.Contains(cmd, "| hx") {
		return true
	}
	if strings.Contains(cmd, " hx ") || strings.HasSuffix(cmd, " hx") {
		return true
	}
	return false
}

func cmdFind(args []string) {
	query, opts := parseFindArgs(args)
	if query == "" {
		fmt.Fprintf(os.Stderr, "hx find: usage: hx find <text> [--compact|--wide|--debug] [--include-self] [--no-import]\n")
		fmt.Fprintf(os.Stderr, "  Set HX_FIND_DEFAULT=wide to keep legacy output. Run 'hx find --help' for details.\n")
		os.Exit(1)
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
		SELECT e.event_id, e.session_id, e.seq, e.started_at, e.exit_code, e.cwd, COALESCE(c.cmd_text, '')
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

	// Collect rows; default exclude self (unless --include-self); --no-self for backwards compat
	var results []findRow
	for rows.Next() {
		var r findRow
		if err := rows.Scan(&r.eventID, &r.sessionID, &r.seq, &r.startedAt, &r.exitCode, &r.cwd, &r.cmd); err != nil {
			continue
		}
		excludeSelf := !opts.includeSelf || opts.noSelf
		if excludeSelf && isSelfCmd(r.cmd) {
			continue
		}
		results = append(results, r)
		if len(results) >= 20 {
			break
		}
	}

	tw := cmdutil.TerminalWidth()
	if cols := os.Getenv("COLUMNS"); cols != "" {
		if n, err := strconv.Atoi(cols); err == nil && n > 0 {
			tw = n
		}
	}
	w := os.Stdout
	mode := "compact"
	if opts.debug {
		mode = "debug"
	} else if opts.wide {
		mode = "wide"
	}
	std1Rows := make([]cmdutil.Std1Row, len(results))
	for i, r := range results {
		std1Rows[i] = cmdutil.Std1Row{EventID: r.eventID, SessionID: r.sessionID, Seq: r.seq, StartedAt: r.startedAt, ExitCode: r.exitCode, Cwd: r.cwd, Cmd: r.cmd}
	}
	cmdutil.RenderStandard1(std1Rows, mode, tw, opts.forceWide, w)
	if len(results) == 0 {
		fmt.Println("(no matches)")
	}
}

// findRow holds one hx find result row.
type findRow struct {
	eventID   int64
	sessionID string
	seq       int
	startedAt float64
	exitCode  *int
	cwd       string
	cmd       string
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

type queryOpts struct {
	filePath    string
	noLLM       bool
	verbose     bool // show Ollama unavailable message (otherwise suppressed)
	wide        bool
	compact     bool
	debug       bool
	forceWide   bool
	includeSelf bool
	noSelf      bool // backwards compat: same as default (exclude self)
	noImport    bool
	noFallback  bool
	explain     bool
}

func parseQueryArgs(args []string) (question string, opts queryOpts) {
	var questionParts []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--file", "-f":
			if i+1 < len(args) {
				opts.filePath = args[i+1]
				i++
			}
		case "--no-llm":
			opts.noLLM = true
		case "--verbose", "-v":
			opts.verbose = true
		case "--wide":
			opts.wide = true
		case "--compact":
			opts.compact = true
		case "--debug":
			opts.debug = true
		case "--force-wide":
			opts.forceWide = true
		case "--include-self":
			opts.includeSelf = true
		case "--no-self":
			opts.noSelf = true
		case "--no-import":
			opts.noImport = true
		case "--no-fallback":
			opts.noFallback = true
		case "--explain":
			opts.explain = true
		default:
			questionParts = append(questionParts, args[i])
		}
	}
	question = strings.Join(questionParts, " ")
	if !opts.wide && !opts.compact {
		opts.compact = true
	}
	return question, opts
}

func cmdQuery(args []string) {
	question, opts := parseQueryArgs(args)

	conn, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx query: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()

	if opts.filePath != "" {
		cmdQueryByFile(conn, opts.filePath)
		return
	}
	if strings.TrimSpace(question) == "" {
		fmt.Fprintf(os.Stderr, "hx query: usage: hx query \"<question>\" [--no-llm] [--no-fallback] [--explain] [--compact|--wide]   OR   hx query --file <path>\n")
		os.Exit(1)
	}
	cmdQueryByQuestion(conn, question, opts)
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

func filterQueryCandidates(candidates []query.Candidate, opts queryOpts) []query.Candidate {
	var filtered []query.Candidate
	for _, c := range candidates {
		if !opts.includeSelf && isSelfCmd(c.Cmd) {
			continue
		}
		if opts.noImport && strings.HasPrefix(c.SessionID, "import-") {
			continue
		}
		filtered = append(filtered, c)
	}
	return filtered
}

func printQueryExplain(meta query.RetrieveMeta) {
	fmt.Fprintf(os.Stderr, "keywords: %v\n", meta.Keywords)
	fmt.Fprintf(os.Stderr, "fts_query: %q\n", meta.FTSQuery)
	fmt.Fprintf(os.Stderr, "fts_candidates: %d\n", meta.FTSCount)
	fmt.Fprintf(os.Stderr, "used_fallback: %v\n", meta.UsedFallback)
	fmt.Fprintf(os.Stderr, "semantic_reranked: %v\n", meta.SemanticReranked)
}

func printQueryFallbackNotice(meta query.RetrieveMeta) {
	kws := strings.Join(meta.Keywords, " ")
	if kws == "" {
		fmt.Fprintf(os.Stderr, "No searchable keywords extracted. Showing recent events.\n")
		return
	}
	fmt.Fprintf(os.Stderr, "No matches for keywords: %s. Showing recent events.\n", kws)
	fmt.Fprintf(os.Stderr, "Try: hx find <keyword>\n")
}

func queryTermWidth() int {
	w := cmdutil.TerminalWidth()
	if cols := os.Getenv("COLUMNS"); cols != "" {
		if n, err := strconv.Atoi(cols); err == nil && n > 0 {
			return n
		}
	}
	if !cmdutil.IsTerminal(os.Stdout) {
		return 120
	}
	return w
}

func queryRenderMode(opts queryOpts) string {
	if opts.debug {
		return "debug"
	}
	if opts.wide {
		return "wide"
	}
	return "compact"
}

func printQueryLLMSummary(question string, candidates []query.Candidate, cfg *config.Config, opts queryOpts) {
	if opts.noLLM || cfg == nil || !cfg.OllamaEnabled || !ollama.Available(context.Background(), cfg.OllamaBaseURL) {
		return
	}
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
		if opts.verbose {
			fmt.Fprintf(os.Stderr, "\nOllama unavailable (model %s). Start with: ollama run %s\n", cfg.OllamaChatModel, cfg.OllamaChatModel)
		}
		return
	}
	if s := strings.TrimSpace(summary); s != "" {
		fmt.Println()
		fmt.Println("Summary:")
		fmt.Println(s)
	}
}

func cmdQueryByQuestion(conn *sql.DB, question string, opts queryOpts) {
	cfg := getConfig()
	if cfg == nil {
		cfg = &config.Config{OllamaEnabled: true, OllamaBaseURL: "http://localhost:11434", OllamaEmbedModel: "nomic-embed-text", OllamaChatModel: "llama3.2"}
	}
	retrieveOpts := &query.RetrieveOpts{NoFallback: opts.noFallback}
	result, err := query.Retrieve(context.Background(), conn, question, cfg, retrieveOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx query: %v\n", err)
		os.Exit(1)
	}
	candidates := filterQueryCandidates(result.Candidates, opts)

	if opts.explain {
		printQueryExplain(result.Meta)
	}

	if len(candidates) == 0 {
		if opts.noFallback && result.Meta.FTSCount == 0 && len(result.Meta.Keywords) > 0 {
			fmt.Fprintf(os.Stderr, "No matches for keywords: %s. Try: hx find <keyword>\n", strings.Join(result.Meta.Keywords, " "))
		}
		fmt.Println("No matching events found.")
		return
	}

	if result.Meta.UsedFallback {
		printQueryFallbackNotice(result.Meta)
	}

	termWidth := queryTermWidth()
	fmt.Printf("Results (%d):\n\n", len(candidates))
	mode := queryRenderMode(opts)
	std1Rows := make([]cmdutil.Std1Row, len(candidates))
	for i, c := range candidates {
		ec := c.ExitCode
		std1Rows[i] = cmdutil.Std1Row{
			EventID: c.EventID, SessionID: c.SessionID, Seq: c.Seq,
			StartedAt: c.StartedAt, ExitCode: &ec, Cwd: c.Cwd, Cmd: c.Cmd,
		}
	}
	cmdutil.RenderStandard1(std1Rows, mode, termWidth, opts.forceWide, os.Stdout)
	fmt.Println()

	printQueryLLMSummary(question, candidates, cfg, opts)
}

func isConfigFile(base string) bool {
	switch base {
	case ".zshrc", ".bashrc", ".profile", ".bash_profile", ".zprofile":
		return true
	}
	return false
}

func suggestHistoryFile(configBase string) string {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "~"
	}
	switch configBase {
	case ".zshrc", ".zprofile":
		return filepath.Join(home, ".zsh_history")
	case ".bashrc", ".bash_profile", ".profile":
		return filepath.Join(home, ".bash_history")
	}
	return filepath.Join(home, ".zsh_history")
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
		fmt.Fprintf(os.Stderr, "hx import: usage: hx import --file <path> [--host label] [--shell zsh|bash|auto] [--force]\n")
		os.Exit(1)
	}

	// Safeguard: warn if importing a config file instead of history
	base := filepath.Base(filepath.Clean(filePath))
	if isConfigFile(base) {
		suggest := suggestHistoryFile(base)
		fmt.Fprintf(os.Stderr, "hx import: %q looks like a shell config file, not command history.\n", base)
		fmt.Fprintf(os.Stderr, "  Did you mean: hx import --file %s\n", suggest)
		fmt.Fprintf(os.Stderr, "  Use --force to import anyway.\n")
		force := false
		for _, a := range args {
			if a == "--force" {
				force = true
				break
			}
		}
		if !force {
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "  Proceeding with --force.\n")
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
	if inserted > 0 {
		fmt.Printf("  Try: hx find <text>  or  hx dump  or  hx last\n")
	}
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

func cmdDump(args []string) {
	conn, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hx dump: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()

	wide := false
	for _, a := range args {
		if a == "--wide" {
			wide = true
			break
		}
	}

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

	var dumpRows []findRow
	for rows.Next() {
		var r findRow
		if err := rows.Scan(&r.eventID, &r.sessionID, &r.seq, &r.exitCode, &r.cwd, &r.cmd); err != nil {
			continue
		}
		dumpRows = append(dumpRows, r)
	}

	tw := cmdutil.TerminalWidth()
	if wide {
		printDumpWide(dumpRows, tw)
	} else {
		printDumpCompact(dumpRows, tw)
	}
}

func printDumpCompact(rows []findRow, termWidth int) {
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

	fmt.Printf("%-*s %-*s %-*s %s\n", idW, "id", exitW, "exit", cwdW, "cwd", "cmd")
	fmt.Println(strings.Repeat("-", termWidth))

	var prevSess string
	for _, r := range rows {
		if prevSess != "" && prevSess != r.sessionID {
			fmt.Println()
		}
		prevSess = r.sessionID

		exit := "-"
		if r.exitCode != nil {
			exit = fmt.Sprintf("%d", *r.exitCode)
		}
		cwdNorm := cmdutil.NormalizePath(r.cwd)
		cwdShow := cmdutil.TruncateLeft(cwdNorm, cwdW)
		cmdShow := cmdutil.TruncateRight(r.cmd, cmdW)
		fmt.Printf("%-*d %-*s %-*s %s\n", idW, r.eventID, exitW, exit, cwdW, cwdShow, cmdShow)
	}
}

func printDumpWide(rows []findRow, termWidth int) {
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

	fmt.Printf("%-8s %-*s %4s %4s %-*s %s\n", "event_id", sessW, "session_id", "seq", "exit", cwdW, "cwd", "cmd")
	fmt.Println(strings.Repeat("-", sep))

	var prevSess string
	for _, r := range rows {
		if prevSess != "" && prevSess != r.sessionID {
			fmt.Println()
		}
		prevSess = r.sessionID

		exit := "-"
		if r.exitCode != nil {
			exit = fmt.Sprintf("%d", *r.exitCode)
		}
		cwdNorm := cmdutil.NormalizePath(r.cwd)
		cwdShow := cmdutil.TruncateRight(cwdNorm, cwdW)
		cmdShow := cmdutil.TruncateRight(r.cmd, cmdW)
		fmt.Printf("%-8d %-*s %4d %4s %-*s %s\n", r.eventID, sessW, r.sessionID, r.seq, exit, cwdW, cwdShow, cmdShow)
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
	os.Exit(runCLI(os.Args))
}

// runCLI is the CLI entrypoint. Returns exit code (0 = success).
func runCLI(args []string) int {
	if len(args) < 2 {
		printRootHelp(os.Stdout)
		return 0
	}
	cmd := args[1]
	cmdArgs := args[2:]

	// Root-level help: hx --help, hx -h, hx help [<cmd>]
	if cmd == "--help" || cmd == "-h" {
		printRootHelp(os.Stdout)
		return 0
	}
	if cmd == "help" {
		if len(cmdArgs) > 0 {
			printSubcommandHelp(os.Stdout, cmdArgs[0])
		} else {
			printRootHelp(os.Stdout)
		}
		return 0
	}

	// Subcommand help: hx <cmd> --help or hx <cmd> -h
	if argsHasHelp(cmdArgs) && isKnownCommand(cmd) {
		printSubcommandHelp(os.Stdout, cmd)
		return 0
	}

	runCommand(cmd, cmdArgs)
	return 0 // runCommand exits on error
}

func argsHasHelp(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}

func isKnownCommand(cmd string) bool {
	known := map[string]bool{
		"status": true, "pause": true, "resume": true, "last": true, "dump": true,
		"debug": true, "find": true, "attach": true, "query": true, "import": true,
		"pin": true, "forget": true, "export": true, "sync": true,
	}
	return known[cmd]
}

func printRootHelp(w io.Writer) {
	_, _ = fmt.Fprintln(w, "hx: History eXtended - terminal flight recorder")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Usage: hx <command> [args...]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  status    capture state, daemon health, paths")
	_, _ = fmt.Fprintln(w, "  pause     stop capturing")
	_, _ = fmt.Fprintln(w, "  resume    resume capturing")
	_, _ = fmt.Fprintln(w, "  last      last session summary, failure context")
	_, _ = fmt.Fprintln(w, "  find      full-text search over commands")
	_, _ = fmt.Fprintln(w, "  dump      last 20 events (debug)")
	_, _ = fmt.Fprintln(w, "  debug     diagnostics: daemon PID, spool, DB event count")
	_, _ = fmt.Fprintln(w, "  attach    link artifact to session")
	_, _ = fmt.Fprintln(w, "  query     evidence-backed search (optional Ollama)")
	_, _ = fmt.Fprintln(w, "  import    import shell history file")
	_, _ = fmt.Fprintln(w, "  pin       pin session (exempt from retention)")
	_, _ = fmt.Fprintln(w, "  forget    delete events in time window")
	_, _ = fmt.Fprintln(w, "  export    export session as markdown")
	_, _ = fmt.Fprintln(w, "  sync      multi-device sync (init, status, push, pull)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Getting started:")
	_, _ = fmt.Fprintln(w, "  Import-only:  hx import --file ~/.zsh_history  # then hx find <text>")
	_, _ = fmt.Fprintln(w, "  Live capture: source src/hooks/hx.zsh (zsh) or src/hooks/bash/hx.bash (bash)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Note: Set HX_FIND_DEFAULT=wide for legacy find output.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Run 'hx help <command>' or 'hx <command> --help' for subcommand help.")
}

var helpRegistry = map[string]func(io.Writer){
	"status": func(w io.Writer) {
		_, _ = fmt.Fprintln(w, "hx status")
		_, _ = fmt.Fprintln(w, "")
		_, _ = fmt.Fprintln(w, "Show capture state, daemon health, and configured paths (spool, db, events).")
	},
	"find": func(w io.Writer) {
		_, _ = fmt.Fprintln(w, "hx find: usage: hx find <text> [--compact|--wide|--debug] [--include-self] [--no-import]")
		_, _ = fmt.Fprintln(w, "  Full-text search over commands.")
		_, _ = fmt.Fprintln(w, "  --compact       compact (default): id, when, exit, cwd, cmd")
		_, _ = fmt.Fprintln(w, "  --wide          more fidelity: absolute time, wider cwd/cmd (no session_id)")
		_, _ = fmt.Fprintln(w, "  --debug         adds session_id, seq")
		_, _ = fmt.Fprintln(w, "  --include-self  show hx / ./bin/hx commands (default: excluded)")
		_, _ = fmt.Fprintln(w, "  --no-self       deprecated alias for default (exclude self)")
		_, _ = fmt.Fprintln(w, "  --no-import     exclude import-* sessions")
		_, _ = fmt.Fprintln(w, "  --force-wide    keep wide at COLUMNS<120 (else auto-fallback to compact)")
	},
	"last": func(w io.Writer) {
		_, _ = fmt.Fprintln(w, "hx last [--raw-time]")
		_, _ = fmt.Fprintln(w, "")
		_, _ = fmt.Fprintln(w, "Show last session summary with failure context. --raw-time shows epoch seconds.")
	},
	"import": func(w io.Writer) {
		_, _ = fmt.Fprintln(w, "hx import: usage: hx import --file <path> [--host label] [--shell zsh|bash|auto] [--force]")
		_, _ = fmt.Fprintln(w, "  Import shell history. Idempotent; duplicates skipped.")
		_, _ = fmt.Fprintln(w, "  Safeguard: blocks .zshrc/.bashrc/.profile (use ~/.zsh_history or ~/.bash_history). --force to override.")
	},
	"query": func(w io.Writer) {
		_, _ = fmt.Fprintln(w, "hx query: usage: hx query \"<question>\" [options]   OR   hx query --file <path>")
		_, _ = fmt.Fprintln(w, "  Natural-language search. Extracts keywords (strips stopwords), searches FTS by OR across tokens.")
		_, _ = fmt.Fprintln(w, "  --no-llm        skip Ollama summary")
		_, _ = fmt.Fprintln(w, "  --no-fallback   when no FTS match, return empty (default: show recent events with notice)")
		_, _ = fmt.Fprintln(w, "  --explain       print keywords, fts_query, fts_candidates, used_fallback, semantic_reranked")
		_, _ = fmt.Fprintln(w, "  --verbose       show Ollama unavailable hint (otherwise suppressed)")
		_, _ = fmt.Fprintln(w, "  --compact       compact (default): id, when, exit, cwd, cmd")
		_, _ = fmt.Fprintln(w, "  --wide          more fidelity: absolute time, wider cwd/cmd (no session_id)")
		_, _ = fmt.Fprintln(w, "  --debug         adds session_id, seq")
		_, _ = fmt.Fprintln(w, "  --include-self  show hx / ./bin/hx commands (default: excluded)")
		_, _ = fmt.Fprintln(w, "  --no-self       deprecated alias for default (exclude self)")
		_, _ = fmt.Fprintln(w, "  --no-import     exclude import-* sessions")
	},
	"sync": func(w io.Writer) {
		_, _ = fmt.Fprintln(w, "hx sync: usage: hx sync <init|status|push|pull> [options]")
		_, _ = fmt.Fprintln(w, "")
		_, _ = fmt.Fprintln(w, "  init --store folder:/path/to/HXSync [--vault-name NAME]")
		_, _ = fmt.Fprintln(w, "  status   show vault, pending events, imported segments")
		_, _ = fmt.Fprintln(w, "  push     publish local events to store")
		_, _ = fmt.Fprintln(w, "  pull     import from store into local DB")
	},
	"attach": func(w io.Writer) {
		_, _ = fmt.Fprintln(w, "hx attach: usage: hx attach --file <path> [--to last|session_id]")
		_, _ = fmt.Fprintln(w, "  Link artifact to session.")
	},
	"export": func(w io.Writer) {
		_, _ = fmt.Fprintln(w, "hx export: usage: hx export [--session <SID>|--last] [--redacted]")
		_, _ = fmt.Fprintln(w, "  Export session as markdown.")
	},
	"forget": func(w io.Writer) {
		_, _ = fmt.Fprintln(w, "hx forget: usage: hx forget --since 15m|1h|24h|7d")
		_, _ = fmt.Fprintln(w, "  Delete events in time window.")
	},
	"pin": func(w io.Writer) {
		_, _ = fmt.Fprintln(w, "hx pin: usage: hx pin --session <SID>   OR   hx pin --last")
		_, _ = fmt.Fprintln(w, "  Pin session (exempt from retention).")
	},
	"pause": func(w io.Writer) {
		_, _ = fmt.Fprintln(w, "hx pause")
		_, _ = fmt.Fprintln(w, "  Stop capturing.")
	},
	"resume": func(w io.Writer) {
		_, _ = fmt.Fprintln(w, "hx resume")
		_, _ = fmt.Fprintln(w, "  Resume capturing.")
	},
	"dump": func(w io.Writer) {
		_, _ = fmt.Fprintln(w, "hx dump [--wide]")
		_, _ = fmt.Fprintln(w, "  Print last 20 events. Compact by default (id, exit, cwd, cmd); --wide for full columns.")
		_, _ = fmt.Fprintln(w, "  Sessions are separated by a blank line.")
	},
	"debug": func(w io.Writer) {
		_, _ = fmt.Fprintln(w, "hx debug")
		_, _ = fmt.Fprintln(w, "")
		_, _ = fmt.Fprintln(w, "  Run diagnostics: daemon PID validity, spool file (line count, mtime),")
		_, _ = fmt.Fprintln(w, "  DB event count and most recent timestamp.")
	},
}

func printSubcommandHelp(w io.Writer, cmd string) {
	if fn, ok := helpRegistry[cmd]; ok {
		fn(w)
		return
	}
	_, _ = fmt.Fprintf(w, "No help for %q. Run 'hx --help' for commands.\n", cmd)
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
		cmdDump(args)
	case "debug":
		cmdDebug()
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
		fmt.Fprintf(os.Stderr, "Run 'hx --help' for usage.\n")
		os.Exit(1)
	}
}
