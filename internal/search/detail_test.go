package search

import "testing"

func TestFormatDetailOneFieldPerLine(t *testing.T) {
	d := &EventDetail{
		Row: Row{
			EventID:   42,
			SessionID: "d8c53dee-dd0e-4f0f-875f-e8c1ee165abc",
			Cmd:       "git status",
			Host:      "FoX",
			Origin:    "sync",
			Cwd:       "/home/user/proj",
		},
		Seq: 3,
	}
	out := FormatDetail(d)
	if stringsContainsLine(out, "event_id: 42  session:") {
		t.Fatalf("event_id and session should be on separate lines: %q", out)
	}
	if !stringsContainsLine(out, "session:  d8c53dee-dd0e-4f0f-875f-e8c1ee165abc") {
		t.Fatalf("missing session line: %q", out)
	}
	if !stringsContainsLine(out, "cwd:      /home/user/proj") {
		t.Fatalf("missing cwd line: %q", out)
	}
}

func stringsContainsLine(s, line string) bool {
	for _, ln := range stringsSplitLines(s) {
		if ln == line {
			return true
		}
	}
	return false
}

func stringsSplitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
