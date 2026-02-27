package query

import (
	"context"
	"database/sql"
	"strings"

	"github.com/history-extended/hx/internal/config"
	"github.com/history-extended/hx/internal/ollama"
)

const candidateLimit = 50
const resultLimit = 20

// Retrieve finds evidence for a question: FTS candidates, optionally semantic re-rank.
func Retrieve(ctx context.Context, conn *sql.DB, question string, cfg *config.Config) ([]Candidate, error) {
	if strings.TrimSpace(question) == "" {
		return nil, nil
	}
	candidates, err := ftsCandidates(conn, question, candidateLimit)
	if err != nil {
		return nil, err
	}
	// Fallback: if FTS returns nothing, use recent events
	if len(candidates) == 0 {
		candidates, err = recentCandidates(conn, candidateLimit)
		if err != nil {
			return nil, err
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	if cfg != nil && cfg.OllamaEnabled && ollama.Available(ctx, cfg.OllamaBaseURL) {
		embedFn := func(ctx context.Context, texts []string) ([][]float32, error) {
			return ollama.Embed(ctx, cfg.OllamaBaseURL, cfg.OllamaEmbedModel, texts)
		}
		ranked, err := RerankBySemantic(ctx, question, candidates, embedFn)
		if err != nil {
			return candidates[:min(resultLimit, len(candidates))], nil
		}
		candidates = ranked
	}

	limit := resultLimit
	if len(candidates) < limit {
		limit = len(candidates)
	}
	return candidates[:limit], nil
}

func ftsCandidates(conn *sql.DB, query string, limit int) ([]Candidate, error) {
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
		LIMIT ?
	`, escaped, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCandidates(rows)
}

func recentCandidates(conn *sql.DB, limit int) ([]Candidate, error) {
	rows, err := conn.Query(`
		SELECT e.event_id, e.session_id, e.seq, e.exit_code, e.cwd, COALESCE(c.cmd_text, '')
		FROM events e
		LEFT JOIN command_dict c ON e.cmd_id = c.cmd_id
		ORDER BY e.started_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCandidates(rows)
}

func scanCandidates(rows *sql.Rows) ([]Candidate, error) {
	var out []Candidate
	for rows.Next() {
		var c Candidate
		var exitCode *int
		if err := rows.Scan(&c.EventID, &c.SessionID, &c.Seq, &exitCode, &c.Cwd, &c.Cmd); err != nil {
			continue
		}
		if exitCode != nil {
			c.ExitCode = *exitCode
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
