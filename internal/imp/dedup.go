// Package imp provides the history import pipeline.
package imp

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
)

// DedupHash returns sha256(source_file + line_num + cmd) for idempotency.
func DedupHash(sourceFile string, lineNum int, cmd string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", sourceFile, lineNum, cmd)))
	return hex.EncodeToString(h[:])
}

// RecordOrSkip inserts hash into import_dedup. Returns true if new (should insert event), false if duplicate.
func RecordOrSkip(db *sql.DB, hash string) (inserted bool, err error) {
	res, err := db.Exec(`INSERT OR IGNORE INTO import_dedup (dedup_hash) VALUES (?)`, hash)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
