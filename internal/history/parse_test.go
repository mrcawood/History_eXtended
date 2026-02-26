package history

import (
	"testing"
)

func TestParseZshExtended(t *testing.T) {
	tests := []struct {
		line        string
		wantCmd     string
		wantTs      float64
		wantDurSec  int
		wantOk      bool
	}{
		{`: 1458291931:15;make test`, "make test", 1458291931, 15, true},
		{`: 1625963751:0;ls`, "ls", 1625963751, 0, true},
		{`: 1700000000:120;pwd`, "pwd", 1700000000, 120, true},
		{`: 1458291931:15;`, "", 1458291931, 15, true}, // empty cmd allowed
		{`make test`, "", 0, 0, false},
	}
	for _, tt := range tests {
		cmd, ts, dur, ok := ParseZshExtended(tt.line)
		if ok != tt.wantOk || cmd != tt.wantCmd || ts != tt.wantTs || dur != tt.wantDurSec {
			t.Errorf("ParseZshExtended(%q) = (%q, %v, %d, %v), want (%q, %v, %d, %v)",
				tt.line, cmd, ts, dur, ok, tt.wantCmd, tt.wantTs, tt.wantDurSec, tt.wantOk)
		}
	}
}

func TestParseBashTimestamped(t *testing.T) {
	tests := []struct {
		lines   []string
		i       int
		wantCmd string
		wantTs  float64
		wantOk  bool
	}{
		{
			lines:   []string{"#1625963751", "make test"},
			i:       0,
			wantCmd: "make test",
			wantTs:  1625963751,
			wantOk:  true,
		},
		{
			lines:   []string{"#1700000000", "pwd"},
			i:       0,
			wantCmd: "pwd",
			wantTs:  1700000000,
			wantOk:  true,
		},
		{
			lines:   []string{"#1625963751"},
			i:       0,
			wantCmd: "",
			wantTs:  0,
			wantOk:  false,
		},
		{
			lines:   []string{"make test"},
			i:       0,
			wantCmd: "",
			wantTs:  0,
			wantOk:  false,
		},
	}
	for _, tt := range tests {
		cmd, ts, ok := ParseBashTimestamped(tt.lines, tt.i)
		if ok != tt.wantOk || cmd != tt.wantCmd || ts != tt.wantTs {
			t.Errorf("ParseBashTimestamped(%v, %d) = (%q, %v, %v), want (%q, %v, %v)",
				tt.lines, tt.i, cmd, ts, ok, tt.wantCmd, tt.wantTs, tt.wantOk)
		}
	}
}

func TestParsePlain(t *testing.T) {
	tests := []struct {
		line    string
		wantCmd string
		wantOk  bool
	}{
		{"make test", "make test", true},
		{"ls", "ls", true},
		{"  pwd  ", "pwd", true},
		{"", "", false},
		{"   ", "", false},
	}
	for _, tt := range tests {
		cmd, ok := ParsePlain(tt.line)
		if ok != tt.wantOk || cmd != tt.wantCmd {
			t.Errorf("ParsePlain(%q) = (%q, %v), want (%q, %v)", tt.line, cmd, ok, tt.wantCmd, tt.wantOk)
		}
	}
}

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		lines []string
		want  Format
	}{
		{[]string{`: 1458291931:15;make test`}, FormatZsh},
		{[]string{`#1625963751`, `make test`}, FormatBash},
		{[]string{`make test`, `ls`}, FormatPlain},
		{[]string{`#1625963751`, `make test`, `: 1458291931:15;cmd`}, FormatBash}, // first match wins
		{[]string{`: 1458291931:15;cmd`, `#1625963751`}, FormatZsh},
		{[]string{}, FormatPlain},
		{[]string{`  `}, FormatPlain},
	}
	for _, tt := range tests {
		got := DetectFormat(tt.lines)
		if got != tt.want {
			t.Errorf("DetectFormat(%v) = %v, want %v", tt.lines, got, tt.want)
		}
	}
}
