package imp

import (
	"bufio"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"

	"github.com/history-extended/hx/internal/history"
	"github.com/history-extended/hx/internal/store"
)

const maxLines = 100_000

// Run imports a history file. Returns number of events inserted.
func Run(db *sql.DB, sourceFile, sourceHost, sourceShell string) (int, int, error) {
	path, err := filepath.Abs(sourceFile)
	if err != nil {
		path = sourceFile
	}
	lines, err := readLines(path)
	if err != nil {
		return 0, 0, err
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		// TODO: warn truncated
	}

	shell := sourceShell
	if shell == "" || shell == "auto" {
		shell = string(history.DetectFormat(lines))
	}

	batchID, err := newBatchID()
	if err != nil {
		return 0, 0, err
	}
	sessionID := "import-" + batchID
	st := store.New(db)

	importedAt := float64(time.Now().Unix())
	if err := st.EnsureImportSession(sessionID, batchID, path, sourceHost, importedAt); err != nil {
		return 0, 0, err
	}

	var inserted, skipped int
	seq := 0

	switch history.Format(shell) {
	case history.FormatZsh:
		for i, line := range lines {
			cmd, startedAt, durSec, ok := history.ParseZshExtended(line)
			if !ok {
				skipped++
				continue
			}
			seq++
			hash := DedupHash(path, i+1, cmd)
			if ins, _ := RecordOrSkip(db, hash); !ins {
				seq--
				continue
			}
			cmdID, err := st.CmdID(cmd, startedAt)
			if err != nil {
				skipped++
				continue
			}
			durMs := int64(durSec) * 1000
			ok2, _ := st.InsertImportEvent(cmd, startedAt, durMs, seq, sessionID, cmdID, "high", path, sourceHost, batchID)
			if ok2 {
				inserted++
			}
		}
	case history.FormatBash:
		for i := 0; i < len(lines); i++ {
			cmd, startedAt, ok := history.ParseBashTimestamped(lines, i)
			if !ok {
				// May be plain line (mixed format)
				cmd, plainOk := history.ParsePlain(lines[i])
				if plainOk {
					seq++
					hash := DedupHash(path, i+1, cmd)
					if ins, _ := RecordOrSkip(db, hash); ins {
						cmdID, err := st.CmdID(cmd, importedAt)
						if err == nil {
							if ok2, _ := st.InsertImportEvent(cmd, importedAt, 0, seq, sessionID, cmdID, "low", path, sourceHost, batchID); ok2 {
								inserted++
							}
						}
					} else {
						seq--
					}
				} else {
					skipped++
				}
				continue
			}
			seq++
			hash := DedupHash(path, i+2, cmd) // line_num of command line (i+2 = 1-based for second line)
			if ins, _ := RecordOrSkip(db, hash); !ins {
				seq--
				i++ // skip cmd line
				continue
			}
			cmdID, err := st.CmdID(cmd, startedAt)
			if err != nil {
				skipped++
				i++ // skip cmd line
				continue
			}
			ok2, _ := st.InsertImportEvent(cmd, startedAt, 0, seq, sessionID, cmdID, "medium", path, sourceHost, batchID)
			if ok2 {
				inserted++
			}
			i++ // consumed #ts + cmd
		}
	default: // FormatPlain
		for i, line := range lines {
			cmd, ok := history.ParsePlain(line)
			if !ok {
				skipped++
				continue
			}
			seq++
			hash := DedupHash(path, i+1, cmd)
			if ins, _ := RecordOrSkip(db, hash); !ins {
				seq--
				continue
			}
			cmdID, err := st.CmdID(cmd, importedAt)
			if err != nil {
				skipped++
				continue
			}
			ok2, _ := st.InsertImportEvent(cmd, importedAt, 0, seq, sessionID, cmdID, "low", path, sourceHost, batchID)
			if ok2 {
				inserted++
			}
		}
	}

	// Update session ended_at
	lastStarted := importedAt
	if inserted > 0 {
		var last float64
		_ = db.QueryRow(`SELECT MAX(started_at) FROM events WHERE session_id = ?`, sessionID).Scan(&last)
		if last > 0 {
			lastStarted = last
		}
	}
	_, _ = db.Exec(`UPDATE sessions SET ended_at = ? WHERE session_id = ?`, lastStarted, sessionID)

	// Record batch
	_, _ = db.Exec(
		`INSERT INTO import_batches (batch_id, source_file, source_shell, source_host, imported_at, event_count) VALUES (?, ?, ?, ?, ?, ?)`,
		batchID, path, shell, sourceHost, importedAt, inserted,
	)

	return inserted, skipped, nil
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}

func newBatchID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
