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
