package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
)

// Skeletonize normalizes text for recurrence detection: timestamps, hex addrs, PIDs → placeholders.
// Returns skeleton_hash = sha256(skeleton_text).
func Skeletonize(text string) string {
	s := text
	// Unix timestamps: 1707734400.123, 1707734400
	s = regexp.MustCompile(`\b\d{10}(\.\d+)?\b`).ReplaceAllString(s, "<TS>")
	// ISO-like: 2024-02-12T10:30:00, 2024-02-12 10:30:00
	s = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T\s]\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:?\d{2})?`).ReplaceAllString(s, "<TS>")
	// Hex addresses: 0x7f8b2c3d4e5f
	s = regexp.MustCompile(`0x[0-9a-fA-F]+`).ReplaceAllString(s, "<ADDR>")
	// PIDs/TIDs in common patterns: "pid 12345", "TID 678", "process 999"
	s = regexp.MustCompile(`(?i)(pid|tid|process)\s+\d+`).ReplaceAllString(s, "$1 <ID>")
	s = regexp.MustCompile(`#\d+\s`).ReplaceAllString(s, "#<ID> ")
	return s
}

// SkeletonHash returns sha256(skeleton_text) as hex.
func SkeletonHash(text string) string {
	skel := Skeletonize(text)
	h := sha256.Sum256([]byte(skel))
	return hex.EncodeToString(h[:])
}
