package export

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/history-extended/hx/internal/artifact"
)

// SessionExport holds session + events + artifacts for export.
type SessionExport struct {
	SessionID string
	Host      string
	StartedAt float64
	Events    []EventExport
	Artifacts []ArtifactRef
}

// EventExport is an event for export.
type EventExport struct {
	Seq      int
	ExitCode int
	Cwd      string
	Cmd      string
}

// ArtifactRef is an attached artifact reference.
type ArtifactRef struct {
	ArtifactID int64
	Kind       string
	BlobPath   string
}

// Redact applies Skeletonize to text when redact is true.
func Redact(text string, redact bool) string {
	if !redact {
		return text
	}
	return artifact.Skeletonize(text)
}

// ExportSession loads session export data.
func ExportSession(conn *sql.DB, sessionID string) (*SessionExport, error) {
	var host string
	var startedAt float64
	err := conn.QueryRow(`SELECT host, started_at FROM sessions WHERE session_id = ?`, sessionID).Scan(&host, &startedAt)
	if err != nil {
		return nil, err
	}

	st := artifact.New(conn)
	events, err := st.GetSessionEvents(sessionID, 10000)
	if err != nil {
		return nil, err
	}
	// Reverse to chronological (GetSessionEvents returns DESC)
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	evExports := make([]EventExport, len(events))
	for i, e := range events {
		evExports[i] = EventExport{Seq: e.Seq, ExitCode: e.ExitCode, Cwd: e.Cwd, Cmd: e.Cmd}
	}

	rows, err := conn.Query(`SELECT artifact_id, kind, blob_path FROM artifacts WHERE linked_session_id = ?`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var artifacts []ArtifactRef
	for rows.Next() {
		var a ArtifactRef
		if err := rows.Scan(&a.ArtifactID, &a.Kind, &a.BlobPath); err != nil {
			continue
		}
		artifacts = append(artifacts, a)
	}

	return &SessionExport{
		SessionID: sessionID,
		Host:      host,
		StartedAt: startedAt,
		Events:    evExports,
		Artifacts: artifacts,
	}, nil
}

// Markdown formats the export as markdown.
func Markdown(exp *SessionExport, redact bool) string {
	var b strings.Builder
	b.WriteString("# Session Export\n\n")
	b.WriteString(fmt.Sprintf("- **Session:** %s\n", exp.SessionID))
	b.WriteString(fmt.Sprintf("- **Host:** %s\n", exp.Host))
	b.WriteString(fmt.Sprintf("- **Started:** %.0f\n\n", exp.StartedAt))
	b.WriteString("## Events\n\n")
	for _, e := range exp.Events {
		cmd := Redact(e.Cmd, redact)
		cwd := Redact(e.Cwd, redact)
		b.WriteString(fmt.Sprintf("- [%d] exit=%d  %s\n", e.Seq, e.ExitCode, cmd))
		if cwd != "" {
			b.WriteString(fmt.Sprintf("  cwd: %s\n", cwd))
		}
	}
	if len(exp.Artifacts) > 0 {
		b.WriteString("\n## Attached Artifacts\n\n")
		for _, a := range exp.Artifacts {
			b.WriteString(fmt.Sprintf("- %s (artifact %d): %s\n", a.Kind, a.ArtifactID, a.BlobPath))
		}
	}
	return b.String()
}
