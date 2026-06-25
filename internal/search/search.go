package search

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/mrcawood/History_eXtended/internal/config"
	"github.com/mrcawood/History_eXtended/internal/ollama"
	"github.com/mrcawood/History_eXtended/internal/query"
)

const (
	defaultLimit    = 50
	candidateLimit  = 200
	ftsCandidateCap = 200
)

type scoredRow struct {
	row   Row
	score int
}

// Search returns command history rows for interactive search / hx search.
func Search(ctx context.Context, conn *sql.DB, cfg *config.Config, req Request) ([]Row, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = defaultLimit
	}

	rows, err := fetchCandidates(ctx, conn, cfg, req)
	if err != nil {
		return nil, err
	}
	rows = excludeSelf(rows)
	if req.Dedup {
		rows = dedupRows(rows)
	}
	if req.Mode != ModeSemantic {
		rows = sortByRecency(rows)
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

func sortByRecency(rows []Row) []Row {
	if len(rows) < 2 {
		return rows
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].StartedAt > rows[j].StartedAt
	})
	return rows
}

func fetchCandidates(ctx context.Context, conn *sql.DB, cfg *config.Config, req Request) ([]Row, error) {
	q := strings.TrimSpace(req.Query)
	switch req.Mode {
	case ModeSemantic:
		if cfg != nil && cfg.OllamaEnabled && ollama.Available(ctx, cfg.OllamaBaseURL) && q != "" {
			rows, err := semanticSearch(ctx, conn, cfg, req, q)
			if err == nil && len(rows) > 0 {
				return rows, nil
			}
		}
		// Fall through to fuzzy when Ollama unavailable or empty semantic result.
		fallthrough
	case ModeFuzzy:
		if q == "" {
			return queryRecent(conn, req, candidateLimit)
		}
		rows, err := queryPrefixFTS(conn, req, buildPrefixFTSQuery(tokenize(q)), ftsCandidateCap)
		if err != nil || len(rows) == 0 {
			rows, err = queryRecent(conn, req, candidateLimit)
			if err != nil {
				return nil, err
			}
		}
		return rankFuzzy(rows, q), nil
	case ModePrefix:
		if q == "" {
			return queryRecent(conn, req, candidateLimit)
		}
		pq := buildPrefixFTSQuery(tokenize(q))
		if pq == "" {
			return queryRecent(conn, req, candidateLimit)
		}
		return queryPrefixFTS(conn, req, pq, candidateLimit)
	case ModeFTS:
		if q == "" {
			return queryRecent(conn, req, candidateLimit)
		}
		escaped := strings.ReplaceAll(q, "\"", "\"\"")
		if strings.Contains(escaped, " ") {
			escaped = "\"" + escaped + "\""
		}
		return queryPrefixFTS(conn, req, escaped, candidateLimit)
	default:
		return queryRecent(conn, req, candidateLimit)
	}
}

func semanticSearch(ctx context.Context, conn *sql.DB, cfg *config.Config, req Request, q string) ([]Row, error) {
	base, err := queryPrefixFTS(conn, req, buildPrefixFTSQuery(tokenize(q)), ftsCandidateCap)
	if err != nil {
		return nil, err
	}
	if len(base) == 0 {
		base, err = queryRecent(conn, req, ftsCandidateCap)
		if err != nil {
			return nil, err
		}
	}
	cands := rowsToCandidates(base)
	embedFn := func(ctx context.Context, texts []string) ([][]float32, error) {
		return ollama.Embed(ctx, cfg.OllamaBaseURL, cfg.OllamaEmbedModel, texts)
	}
	ranked, err := query.RerankBySemantic(ctx, q, cands, embedFn)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]Row, len(base))
	for _, r := range base {
		byID[r.EventID] = r
	}
	out := make([]Row, 0, len(ranked))
	for _, c := range ranked {
		if r, ok := byID[c.EventID]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func rowsToCandidates(rows []Row) []query.Candidate {
	out := make([]query.Candidate, len(rows))
	for i, r := range rows {
		exit := 0
		if r.ExitCode != nil {
			exit = *r.ExitCode
		}
		out[i] = query.Candidate{
			EventID:   r.EventID,
			SessionID: r.SessionID,
			Cmd:       r.Cmd,
			Cwd:       r.Cwd,
			ExitCode:  exit,
			StartedAt: r.StartedAt,
		}
	}
	return out
}

func rankFuzzy(rows []Row, q string) []Row {
	scored := make([]scoredRow, 0, len(rows))
	for _, r := range rows {
		s := fuzzyScore(q, r.Cmd)
		if s < 0 {
			continue
		}
		scored = append(scored, scoredRow{row: r, score: s})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].row.StartedAt > scored[j].row.StartedAt
	})
	out := make([]Row, len(scored))
	for i, s := range scored {
		out[i] = s.row
	}
	return out
}

func tokenize(q string) []string {
	var tokens []string
	for _, t := range strings.Fields(strings.ToLower(q)) {
		t = strings.Trim(t, "\"'")
		if t != "" {
			tokens = append(tokens, t)
		}
	}
	return tokens
}

func buildPrefixFTSQuery(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, len(tokens))
	for i, t := range tokens {
		t = strings.ReplaceAll(t, "\"", "\"\"")
		parts[i] = t + "*"
	}
	return strings.Join(parts, " OR ")
}

func filterClause(req Request) (string, []interface{}) {
	var clauses []string
	var args []interface{}
	switch req.Filter {
	case FilterHost:
		if req.Host != "" {
			clauses = append(clauses, "s.host = ?")
			args = append(args, req.Host)
		}
	case FilterDir:
		if req.Cwd != "" {
			clauses = append(clauses, "e.cwd = ?")
			args = append(args, req.Cwd)
		}
	case FilterSession:
		if req.SessionID != "" {
			clauses = append(clauses, "e.session_id = ?")
			args = append(args, req.SessionID)
		}
	}
	if req.NoImport {
		clauses = append(clauses, "(e.origin IS NULL OR e.origin != 'import')")
		clauses = append(clauses, "e.session_id NOT LIKE 'import-%'")
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

const baseSelect = `
	SELECT e.event_id, e.session_id, e.seq, e.exit_code, e.duration_ms, e.cwd,
	       COALESCE(c.cmd_text, ''), e.started_at, COALESCE(e.git_branch, ''), COALESCE(e.git_commit, ''),
	       COALESCE(s.host, ''), COALESCE(e.origin, 'live')
	FROM events e
	LEFT JOIN command_dict c ON e.cmd_id = c.cmd_id
	LEFT JOIN sessions s ON s.session_id = e.session_id`

func queryRecent(conn *sql.DB, req Request, limit int) ([]Row, error) {
	where, args := filterClause(req)
	q := baseSelect + " WHERE 1=1" + where + " ORDER BY e.started_at DESC LIMIT ?"
	args = append(args, limit)
	return runQuery(conn, q, args...)
}

func queryPrefixFTS(conn *sql.DB, req Request, ftsQuery string, limit int) ([]Row, error) {
	if ftsQuery == "" {
		return queryRecent(conn, req, limit)
	}
	where, args := filterClause(req)
	q := `
		SELECT e.event_id, e.session_id, e.seq, e.exit_code, e.duration_ms, e.cwd,
		       COALESCE(c.cmd_text, ''), e.started_at, COALESCE(e.git_branch, ''), COALESCE(e.git_commit, ''),
		       COALESCE(s.host, ''), COALESCE(e.origin, 'live')
		FROM events_fts
		JOIN events e ON e.event_id = events_fts.rowid
		LEFT JOIN command_dict c ON e.cmd_id = c.cmd_id
		LEFT JOIN sessions s ON s.session_id = e.session_id
		WHERE events_fts MATCH ?` + where + `
		ORDER BY e.started_at DESC
		LIMIT ?`
	allArgs := append([]interface{}{ftsQuery}, args...)
	allArgs = append(allArgs, limit)
	return runQuery(conn, q, allArgs...)
}

func runQuery(conn *sql.DB, q string, args ...interface{}) ([]Row, error) {
	rows, err := conn.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Row
	for rows.Next() {
		var r Row
		var exit sql.NullInt64
		var dur sql.NullInt64
		var seq int
		if err := rows.Scan(&r.EventID, &r.SessionID, &seq, &exit, &dur, &r.Cwd, &r.Cmd,
			&r.StartedAt, &r.GitBranch, &r.GitCommit, &r.Host, &r.Origin); err != nil {
			continue
		}
		_ = seq
		if exit.Valid {
			v := int(exit.Int64)
			r.ExitCode = &v
		}
		if dur.Valid {
			v := dur.Int64
			r.DurationMs = &v
		}
		r.DupCount = 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func excludeSelf(rows []Row) []Row {
	out := rows[:0]
	for _, r := range rows {
		if !IsSelfCmd(r.Cmd) {
			out = append(out, r)
		}
	}
	return out
}

func dedupRows(rows []Row) []Row {
	type agg struct {
		row   Row
		count int
	}
	byCmd := make(map[string]*agg)
	for _, r := range rows {
		if a, ok := byCmd[r.Cmd]; ok {
			a.count++
			chosen := preferDedupRow(a.row, r)
			chosen.DupCount = a.count
			a.row = chosen
			continue
		}
		r.DupCount = 1
		byCmd[r.Cmd] = &agg{row: r, count: 1}
	}
	seen := make(map[string]bool)
	out := make([]Row, 0, len(byCmd))
	for _, r := range rows {
		if seen[r.Cmd] {
			continue
		}
		seen[r.Cmd] = true
		out = append(out, byCmd[r.Cmd].row)
	}
	return out
}

func preferDedupRow(current, candidate Row) Row {
	if candidate.StartedAt > current.StartedAt {
		return candidate
	}
	if candidate.StartedAt < current.StartedAt {
		return current
	}
	return preferSameTimeRow(current, candidate)
}

func preferSameTimeRow(a, b Row) Row {
	ra, rb := originRank(a.Origin), originRank(b.Origin)
	if ra != rb {
		if rb > ra {
			return b
		}
		return a
	}
	aHasExit := a.ExitCode != nil
	bHasExit := b.ExitCode != nil
	if aHasExit != bHasExit {
		if bHasExit {
			return b
		}
		return a
	}
	return a
}

func originRank(origin string) int {
	switch origin {
	case "live":
		return 3
	case "sync":
		return 2
	case "import":
		return 1
	default:
		return 0
	}
}

// ParseFilter parses a filter name from CLI/config.
func ParseFilter(s string) (Filter, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "global", "all":
		return FilterGlobal, nil
	case "host", "local":
		return FilterHost, nil
	case "dir", "directory", "cwd":
		return FilterDir, nil
	case "session":
		return FilterSession, nil
	default:
		return FilterGlobal, fmt.Errorf("unknown filter %q", s)
	}
}

// ParseMode parses a search mode name from CLI/config.
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "fuzzy":
		return ModeFuzzy, nil
	case "prefix":
		return ModePrefix, nil
	case "fts", "fulltext":
		return ModeFTS, nil
	case "semantic":
		return ModeSemantic, nil
	default:
		return ModeFuzzy, fmt.Errorf("unknown mode %q", s)
	}
}

// FilterName returns the CLI name for a filter.
func FilterName(f Filter) string {
	switch f {
	case FilterHost:
		return "host"
	case FilterDir:
		return "dir"
	case FilterSession:
		return "session"
	default:
		return "global"
	}
}

// NextFilter cycles filter scope (atuin Ctrl-R).
func NextFilter(f Filter) Filter {
	switch f {
	case FilterGlobal:
		return FilterHost
	case FilterHost:
		return FilterDir
	case FilterDir:
		return FilterSession
	default:
		return FilterGlobal
	}
}

// NextMode cycles search mode (atuin Ctrl-S).
func NextMode(m Mode) Mode {
	switch m {
	case ModeFuzzy:
		return ModePrefix
	case ModePrefix:
		return ModeFTS
	case ModeFTS:
		return ModeSemantic
	default:
		return ModeFuzzy
	}
}

// ModeName returns the CLI name for a mode.
func ModeName(m Mode) string {
	switch m {
	case ModePrefix:
		return "prefix"
	case ModeFTS:
		return "fts"
	case ModeSemantic:
		return "semantic"
	default:
		return "fuzzy"
	}
}
