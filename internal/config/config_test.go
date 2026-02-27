package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// No config file - use defaults
	dir := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", dir)
	defer os.Unsetenv("XDG_CONFIG_HOME")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.SpoolDir == "" {
		t.Error("SpoolDir should not be empty")
	}
	if c.RetentionEventsMonths != 12 {
		t.Errorf("RetentionEventsMonths = %d, want 12", c.RetentionEventsMonths)
	}
	if !c.OllamaEnabled {
		t.Error("OllamaEnabled should be true by default")
	}
	if c.OllamaBaseURL != "http://localhost:11434" {
		t.Errorf("OllamaBaseURL = %q, want http://localhost:11434", c.OllamaBaseURL)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "hx")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.yaml")
	content := `spool_dir: /custom/spool
retention_events_months: 6
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	os.Setenv("XDG_CONFIG_HOME", dir)
	defer os.Unsetenv("XDG_CONFIG_HOME")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.SpoolDir != "/custom/spool" {
		t.Errorf("SpoolDir = %q, want /custom/spool", c.SpoolDir)
	}
	if c.RetentionEventsMonths != 6 {
		t.Errorf("RetentionEventsMonths = %d, want 6", c.RetentionEventsMonths)
	}
}

func TestLoadPathExpansion(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "hx")
	os.MkdirAll(configDir, 0755)
	configPath := filepath.Join(configDir, "config.yaml")
	content := `spool_dir: $XDG_DATA_HOME/hx/spool
`
	os.WriteFile(configPath, []byte(content), 0644)
	os.Setenv("XDG_CONFIG_HOME", dir)
	dataHome := filepath.Join(dir, "data")
	os.Setenv("XDG_DATA_HOME", dataHome)
	defer func() {
		os.Unsetenv("XDG_CONFIG_HOME")
		os.Unsetenv("XDG_DATA_HOME")
	}()

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := filepath.Join(dataHome, "hx", "spool")
	if c.SpoolDir != want {
		t.Errorf("SpoolDir = %q, want %q", c.SpoolDir, want)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "hx")
	os.MkdirAll(configDir, 0755)
	configPath := filepath.Join(configDir, "config.yaml")
	os.WriteFile(configPath, []byte("spool_dir: /from/file\n"), 0644)
	os.Setenv("XDG_CONFIG_HOME", dir)
	os.Setenv("HX_SPOOL_DIR", "/env/override")
	defer func() {
		os.Unsetenv("XDG_CONFIG_HOME")
		os.Unsetenv("HX_SPOOL_DIR")
	}()

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.SpoolDir != "/env/override" {
		t.Errorf("SpoolDir = %q, want /env/override (env takes precedence)", c.SpoolDir)
	}
}
