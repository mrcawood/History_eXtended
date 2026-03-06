package cmdutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizePath_Home(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home dir:", err)
	}
	tests := []struct {
		path   string
		expect string
	}{
		{filepath.Join(home, "foo"), "$HOME/foo"},
		{filepath.Join(home, ".launchpad", "x"), "$HOME/.launchpad/x"},
		{home, "$HOME"},
		{"/other/path", "/other/path"},
		{"", ""},
	}
	// Unset HX_DATA_DIR so we test $HOME replacement
	old := os.Getenv("HX_DATA_DIR")
	os.Unsetenv("HX_DATA_DIR")
	defer func() {
		if old != "" {
			os.Setenv("HX_DATA_DIR", old)
		}
	}()

	for _, tt := range tests {
		got := NormalizePath(tt.path)
		if got != tt.expect {
			t.Errorf("NormalizePath(%q) = %q, want %q", tt.path, got, tt.expect)
		}
	}
}

func TestNormalizePath_HXDataDir(t *testing.T) {
	tmp := t.TempDir()
	os.Setenv("HX_DATA_DIR", tmp)
	defer os.Unsetenv("HX_DATA_DIR")

	tests := []struct {
		path   string
		expect string
	}{
		{tmp, "$HX_DATA_DIR"},
		{filepath.Join(tmp, "hx.db"), "$HX_DATA_DIR/hx.db"},
		{filepath.Join(tmp, "hx", "spool"), "$HX_DATA_DIR/hx/spool"},
		{"/other/path", "/other/path"},
	}
	for _, tt := range tests {
		got := NormalizePath(tt.path)
		if got != tt.expect {
			t.Errorf("NormalizePath(%q) with HX_DATA_DIR=%q = %q, want %q", tt.path, tmp, got, tt.expect)
		}
	}
}

func TestTruncateLeft(t *testing.T) {
	tests := []struct {
		s      string
		max    int
		expect string
	}{
		{"short", 10, "short"},
		{"/a/b/c/History_eXtended", 20, "…/History_eXtended"},
		{"abc", 2, "bc"},
		{"", 5, ""},
	}
	for _, tt := range tests {
		got := TruncateLeft(tt.s, tt.max)
		if got != tt.expect {
			t.Errorf("TruncateLeft(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.expect)
		}
	}
}

func TestFormatTimestamp(t *testing.T) {
	// Test raw mode
	got := FormatTimestamp(1700000000, true)
	if got != "1700000000" {
		t.Errorf("FormatTimestamp(..., true) = %q, want raw epoch", got)
	}
	// Test formatted mode (relative part varies with current time)
	got = FormatTimestamp(1700000000, false)
	if !strings.Contains(got, "2023-11-14") {
		t.Errorf("FormatTimestamp(..., false) = %q, want ISO date 2023-11-14", got)
	}
	if !strings.Contains(got, "ago") {
		t.Errorf("FormatTimestamp(..., false) = %q, want relative 'ago'", got)
	}
}

func TestTruncateRight(t *testing.T) {
	tests := []struct {
		s      string
		max    int
		expect string
	}{
		{"short", 10, "short"},
		{"very long command here", 15, "very long co…"},
		{"abc", 2, "ab"},
		{"", 5, ""},
	}
	for _, tt := range tests {
		got := TruncateRight(tt.s, tt.max)
		if got != tt.expect {
			t.Errorf("TruncateRight(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.expect)
		}
	}
}

func TestTruncateCwdTail_PathBoundary(t *testing.T) {
	tests := []struct {
		path   string
		max    int
		substr string // path boundary: should keep tail components, not mid-string cut
	}{
		{"/a/b/c/History_eXtended", 25, "History_eXtended"},
		{"/a/b/c/History_eXtended", 30, "c/History_eXtended"},
		{"/short", 20, "/short"},
	}
	for _, tt := range tests {
		got := TruncateCwdTail(tt.path, tt.max)
		if !strings.Contains(got, tt.substr) {
			t.Errorf("TruncateCwdTail(%q, %d) = %q, want to contain %q (path boundary)", tt.path, tt.max, got, tt.substr)
		}
		if tt.path != "" && tt.max > 0 && len(got) > tt.max {
			t.Errorf("TruncateCwdTail(%q, %d) = %q (len %d) exceeds max %d", tt.path, tt.max, got, len(got), tt.max)
		}
	}
}

func TestShortenPath_Powerlevel10kStyle(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home dir")
	}
	tests := []struct {
		path   string
		max    int
		expect string // substring to check, or exact match
		exact  bool
	}{
		// Force abbreviation: full path ~32 chars, max 25
		{filepath.Join(home, "projects", "History_eXtended"), 25, "$HOME/p/History_eXtended", true},
		// Middle dirs abbreviated: alpha->a, beta->b, gamma->g
		{filepath.Join(home, "alpha", "beta", "gamma", "longdir"), 28, "$HOME/a/b/g/longdir", true},
		// /usr/local/bin -> /usr/l/bin when max 12
		{"/usr/local/bin", 12, "/usr/l/bin", true},
		{"/single", 20, "/single", true},
	}
	for _, tt := range tests {
		got := ShortenPath(tt.path, tt.max)
		if tt.exact {
			if got != tt.expect {
				t.Errorf("ShortenPath(%q, %d) = %q, want %q", tt.path, tt.max, got, tt.expect)
			}
		} else {
			if !strings.Contains(got, tt.expect) {
				t.Errorf("ShortenPath(%q, %d) = %q, want to contain %q", tt.path, tt.max, got, tt.expect)
			}
		}
		if len(got) > tt.max && tt.max > 0 {
			t.Errorf("ShortenPath(%q, %d) = %q (len %d) exceeds max %d", tt.path, tt.max, got, len(got), tt.max)
		}
	}
}
