package filter

import (
	"path/filepath"
	"strings"

	"github.com/history-extended/hx/internal/config"
)

// ShouldCapture returns false if the command should be skipped (ignored or not allowlisted).
func ShouldCapture(cmd string, cfg *config.Config) bool {
	if cfg == nil {
		return true
	}
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false
	}
	for _, pattern := range cfg.IgnorePatterns {
		matched, err := filepath.Match(pattern, cmd)
		if err == nil && matched {
			return false
		}
	}
	if cfg.AllowlistMode {
		if len(cfg.AllowlistBins) == 0 {
			return false
		}
		first := strings.Fields(cmd)
		if len(first) == 0 {
			return false
		}
		bin := filepath.Base(first[0])
		for _, allowed := range cfg.AllowlistBins {
			if bin == allowed {
				return true
			}
		}
		return false
	}
	return true
}
