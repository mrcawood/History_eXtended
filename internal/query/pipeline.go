package query

import (
	"context"
	"database/sql"
	"strings"

	"github.com/mrcawood/History_eXtended/internal/config"
	"github.com/mrcawood/History_eXtended/internal/ollama"
)

const candidateLimit = 50
const resultLimit = 20

// RetrieveOpts configures Retrieve behavior.
type RetrieveOpts struct {
	NoFallback bool
}

// RetrieveMeta holds explainability data for a retrieval (no sensitive content).
type RetrieveMeta struct {
	Keywords         []string
	FTSQuery         string
	FTSCount         int
	UsedFallback     bool
	SemanticReranked bool
}

// RetrieveResult is the result of Retrieve.
type RetrieveResult struct {
	Candidates []Candidate
	Meta       RetrieveMeta
}

// Retrieve finds evidence for a question: keyword-based FTS candidates, optionally semantic re-rank.
// If FTS returns 0 results and NoFallback is false, falls back to recent events and sets Meta.UsedFallback.
func Retrieve(ctx context.Context, conn *sql.DB, question string, cfg *config.Config, opts *RetrieveOpts) (*RetrieveResult, error) {
	if opts == nil {
		opts = &RetrieveOpts{}
	}
	res := &RetrieveResult{Meta: RetrieveMeta{}}
	if strings.TrimSpace(question) == "" {
		return res, nil
	}

	keywords := ExtractKeywords(question)
	res.Meta.Keywords = keywords
	ftsQuery := BuildFTSQuery(keywords)
	res.Meta.FTSQuery = ftsQuery

	var candidates []Candidate
	var err error
	if ftsQuery != "" {
		candidates, err = ftsCandidatesWithQuery(conn, ftsQuery, candidateLimit)
		if err != nil {
			return nil, err
		}
	}
	res.Meta.FTSCount = len(candidates)

	if len(candidates) == 0 {
		if opts.NoFallback {
			return res, nil
		}
		candidates, err = recentCandidates(conn, candidateLimit)
		if err != nil {
			return nil, err
		}
		res.Meta.UsedFallback = true
	}
	if len(candidates) == 0 {
		return res, nil
	}

	if cfg != nil && cfg.OllamaEnabled && ollama.Available(ctx, cfg.OllamaBaseURL) {
		embedFn := func(ctx context.Context, texts []string) ([][]float32, error) {
			return ollama.Embed(ctx, cfg.OllamaBaseURL, cfg.OllamaEmbedModel, texts)
		}
		ranked, err := RerankBySemantic(ctx, question, candidates, embedFn)
		if err != nil {
			res.Candidates = candidates[:min(resultLimit, len(candidates))]
			return res, nil
		}
		candidates = ranked
		res.Meta.SemanticReranked = true
	}

	limit := resultLimit
	if len(candidates) < limit {
		limit = len(candidates)
	}
	res.Candidates = candidates[:limit]
	return res, nil
}

func ftsCandidatesWithQuery(conn *sql.DB, ftsQuery string, limit int) ([]Candidate, error) {
	if ftsQuery == "" {
		return nil, nil
	}
	rows, err := conn.Query(`
		SELECT e.event_id, e.session_id, e.seq, e.exit_code, e.cwd, COALESCE(c.cmd_text, ''), e.started_at
		FROM events_fts
		JOIN events e ON e.event_id = events_fts.rowid
		LEFT JOIN command_dict c ON e.cmd_id = c.cmd_id
		WHERE events_fts MATCH ?
		ORDER BY e.started_at DESC
		LIMIT ?
	`, ftsQuery, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanCandidates(rows)
}

func recentCandidates(conn *sql.DB, limit int) ([]Candidate, error) {
	rows, err := conn.Query(`
		SELECT e.event_id, e.session_id, e.seq, e.exit_code, e.cwd, COALESCE(c.cmd_text, ''), e.started_at
		FROM events e
		LEFT JOIN command_dict c ON e.cmd_id = c.cmd_id
		ORDER BY e.started_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanCandidates(rows)
}

func scanCandidates(rows *sql.Rows) ([]Candidate, error) {
	var out []Candidate
	for rows.Next() {
		var c Candidate
		var exitCode *int
		if err := rows.Scan(&c.EventID, &c.SessionID, &c.Seq, &exitCode, &c.Cwd, &c.Cmd, &c.StartedAt); err != nil {
			continue
		}
		if exitCode != nil {
			c.ExitCode = *exitCode
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
