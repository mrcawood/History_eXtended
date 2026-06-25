package search

import (
	"database/sql"
	"fmt"
	"strings"
)

// EventDetail is full metadata for one event (hx show).
type EventDetail struct {
	Row
	Seq       int
	Tty       string
	Shell     string
	Artifacts []ArtifactLine
}

// ArtifactLine is a linked artifact reference.
type ArtifactLine struct {
	ArtifactID int64
	Kind       string
	BlobPath   string
}

// GetEvent loads one event by id.
func GetEvent(conn *sql.DB, eventID int64) (*EventDetail, error) {
	var d EventDetail
	var exit, dur sql.NullInt64
	err := conn.QueryRow(`
		SELECT e.event_id, e.session_id, e.seq, e.exit_code, e.duration_ms,
		       COALESCE(NULLIF(TRIM(e.cwd), ''), NULLIF(TRIM(s.initial_cwd), ''), ''),
		       COALESCE(c.cmd_text, ''), e.started_at, COALESCE(e.git_branch, ''), COALESCE(e.git_commit, ''),
		       COALESCE(s.host, ''), COALESCE(e.origin, 'live'),
		       COALESCE(s.tty, ''), COALESCE(s.shell, 'zsh')
		FROM events e
		LEFT JOIN command_dict c ON e.cmd_id = c.cmd_id
		LEFT JOIN sessions s ON s.session_id = e.session_id
		WHERE e.event_id = ?
	`, eventID).Scan(
		&d.EventID, &d.SessionID, &d.Seq, &exit, &dur, &d.Cwd, &d.Cmd,
		&d.StartedAt, &d.GitBranch, &d.GitCommit, &d.Host, &d.Origin, &d.Tty, &d.Shell,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("event %d not found", eventID)
	}
	if err != nil {
		return nil, err
	}
	if exit.Valid {
		v := int(exit.Int64)
		d.ExitCode = &v
	}
	if dur.Valid {
		v := dur.Int64
		d.DurationMs = &v
	}
	rows, err := conn.Query(`
		SELECT artifact_id, kind, blob_path FROM artifacts
		WHERE linked_event_id = ? OR (linked_event_id IS NULL AND linked_session_id = ?)
		ORDER BY artifact_id DESC LIMIT 10
	`, eventID, d.SessionID)
	if err != nil {
		return &d, nil
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var a ArtifactLine
		if err := rows.Scan(&a.ArtifactID, &a.Kind, &a.BlobPath); err != nil {
			continue
		}
		d.Artifacts = append(d.Artifacts, a)
	}
	return &d, nil
}

// FormatDetail renders human-readable metadata for preview/inspector.
func FormatDetail(d *EventDetail) string {
	var b strings.Builder
	fmt.Fprintf(&b, "command:  %s\n", d.Cmd)
	fmt.Fprintf(&b, "event_id: %d\n", d.EventID)
	fmt.Fprintf(&b, "session:  %s\n", d.SessionID)
	fmt.Fprintf(&b, "seq:      %d\n", d.Seq)
	fmt.Fprintf(&b, "when:     %s\n", RelTime(d.StartedAt))
	fmt.Fprintf(&b, "host:     %s\n", d.Host)
	fmt.Fprintf(&b, "origin:   %s\n", d.Origin)
	if d.Cwd != "" {
		fmt.Fprintf(&b, "cwd:      %s\n", d.Cwd)
	} else {
		b.WriteString("cwd:      -\n")
	}
	if d.ExitCode != nil {
		fmt.Fprintf(&b, "exit:     %d\n", *d.ExitCode)
	} else {
		b.WriteString("exit:     -\n")
	}
	if d.DurationMs != nil {
		fmt.Fprintf(&b, "duration: %dms\n", *d.DurationMs)
	}
	if d.GitBranch != "" || d.GitCommit != "" {
		fmt.Fprintf(&b, "git:      %s @ %s\n", d.GitBranch, d.GitCommit)
	}
	if d.Tty != "" {
		fmt.Fprintf(&b, "tty:      %s\n", d.Tty)
	}
	if d.Shell != "" {
		fmt.Fprintf(&b, "shell:    %s\n", d.Shell)
	}
	for _, a := range d.Artifacts {
		fmt.Fprintf(&b, "artifact: [%s] %s (id %d)\n", a.Kind, a.BlobPath, a.ArtifactID)
	}
	return strings.TrimRight(b.String(), "\n")
}
