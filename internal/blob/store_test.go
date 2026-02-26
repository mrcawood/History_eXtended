package blob

import (
	"strings"
	"testing"
)

func TestStore(t *testing.T) {
	dir := t.TempDir()
	content := []byte("hello world")

	sha, path, n, err := Store(dir, content)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if sha == "" || len(sha) != 64 {
		t.Errorf("sha = %q, want 64-char hex", sha)
	}
	if !strings.HasSuffix(path, ".zst") {
		t.Errorf("path = %q, want .zst suffix", path)
	}
	if n != len(content) {
		t.Errorf("byteLen = %d, want %d", n, len(content))
	}
	if !strings.HasPrefix(path, dir) {
		t.Errorf("path %q not under %q", path, dir)
	}

	// Dedupe: same content should return same path, no new file
	sha2, path2, n2, err := Store(dir, content)
	if err != nil {
		t.Fatalf("Store 2: %v", err)
	}
	if sha != sha2 || path != path2 || n2 != n {
		t.Errorf("dedupe: got %q %q %d, want same", sha2, path2, n2)
	}
}

func TestEventsPath(t *testing.T) {
	// EventsPath is in spool package, but we test blob.EventsPath equivalent
	// Actually EventsPath is in spool - no need. Just blob tests.
}
