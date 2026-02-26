// Package config loads hx config from YAML. Env overrides take precedence.
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds resolved paths and settings. Paths use XDG defaults when not in file.
type Config struct {
	SpoolDir             string  `yaml:"spool_dir"`
	BlobDir              string  `yaml:"blob_dir"`
	DbPath               string  `yaml:"db_path"`
	RetentionEventsMonths int     `yaml:"retention_events_months"`
	RetentionBlobsDays   int     `yaml:"retention_blobs_days"`
	BlobDiskCapGB        float64 `yaml:"blob_disk_cap_gb"`
	AllowlistMode        bool    `yaml:"allowlist_mode"`
	AllowlistBins        []string `yaml:"allowlist_bins"`
	IgnorePatterns       []string `yaml:"ignore_patterns"`
}

type rawConfig struct {
	SpoolDir             string   `yaml:"spool_dir"`
	BlobDir              string   `yaml:"blob_dir"`
	DbPath               string   `yaml:"db_path"`
	RetentionEventsMonths int      `yaml:"retention_events_months"`
	RetentionBlobsDays   int      `yaml:"retention_blobs_days"`
	BlobDiskCapGB        float64  `yaml:"blob_disk_cap_gb"`
	AllowlistMode        bool     `yaml:"allowlist_mode"`
	AllowlistBins        []string `yaml:"allowlist_bins"`
	IgnorePatterns       []string `yaml:"ignore_patterns"`
}

// Load reads config from XDG_CONFIG_HOME/hx/config.yaml. Missing file uses defaults.
// Env overrides: HX_SPOOL_DIR, HX_BLOB_DIR, HX_DB_PATH.
func Load() (*Config, error) {
	dataHome := xdgDataHome()
	configHome := xdgConfigHome()
	path := filepath.Join(configHome, "hx", "config.yaml")

	c := &Config{
		SpoolDir:             filepath.Join(dataHome, "hx", "spool"),
		BlobDir:              filepath.Join(dataHome, "hx", "blobs"),
		DbPath:               filepath.Join(dataHome, "hx", "hx.db"),
		RetentionEventsMonths: 12,
		RetentionBlobsDays:   90,
		BlobDiskCapGB:        2.0,
	}

	b, err := os.ReadFile(path)
	if err == nil {
		var raw rawConfig
		if err := yaml.Unmarshal(b, &raw); err != nil {
			return nil, err
		}
		if raw.SpoolDir != "" {
			c.SpoolDir = resolvePath(raw.SpoolDir, dataHome)
		}
		if raw.BlobDir != "" {
			c.BlobDir = resolvePath(raw.BlobDir, dataHome)
		}
		if raw.DbPath != "" {
			c.DbPath = resolvePath(raw.DbPath, dataHome)
		}
		if raw.RetentionEventsMonths > 0 {
			c.RetentionEventsMonths = raw.RetentionEventsMonths
		}
		if raw.RetentionBlobsDays > 0 {
			c.RetentionBlobsDays = raw.RetentionBlobsDays
		}
		if raw.BlobDiskCapGB > 0 {
			c.BlobDiskCapGB = raw.BlobDiskCapGB
		}
		c.AllowlistMode = raw.AllowlistMode
		if len(raw.AllowlistBins) > 0 {
			c.AllowlistBins = raw.AllowlistBins
		}
		if len(raw.IgnorePatterns) > 0 {
			c.IgnorePatterns = raw.IgnorePatterns
		}
	}

	// Env overrides
	if v := os.Getenv("HX_SPOOL_DIR"); v != "" {
		c.SpoolDir = v
	}
	if v := os.Getenv("HX_BLOB_DIR"); v != "" {
		c.BlobDir = v
	}
	if v := os.Getenv("HX_DB_PATH"); v != "" {
		c.DbPath = v
	}

	return c, nil
}

func xdgDataHome() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share")
}

func xdgConfigHome() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

// resolvePath expands $XDG_DATA_HOME, $HOME in paths from config file.
func resolvePath(p, dataHome string) string {
	return filepath.Clean(os.Expand(p, func(key string) string {
		if key == "XDG_DATA_HOME" {
			return dataHome
		}
		if key == "XDG_CONFIG_HOME" {
			return xdgConfigHome()
		}
		if key == "HOME" {
			home, _ := os.UserHomeDir()
			return home
		}
		return ""
	}))
}
