// hxd: ingestion daemon for hx.
// Tails spool, pairs pre/post events, batch-inserts into SQLite.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/history-extended/hx/internal/config"
	"github.com/history-extended/hx/internal/db"
	"github.com/history-extended/hx/internal/ingest"
	"github.com/history-extended/hx/internal/spool"
	"github.com/history-extended/hx/internal/store"
)

func dbPath() string {
	c, err := config.Load()
	if err == nil {
		return c.DbPath
	}
	if v := os.Getenv("HX_DB_PATH"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "hx", "hx.db")
}

func spoolDir() string {
	c, err := config.Load()
	if err == nil {
		return c.SpoolDir
	}
	if v := os.Getenv("HX_SPOOL_DIR"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "hx", "spool")
}

func pidPath() string {
	c, err := config.Load()
	if err == nil {
		return filepath.Join(filepath.Dir(c.DbPath), "hxd.pid")
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "hx", "hxd.pid")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "hx", "hxd.pid")
}

func writePid(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
}

func main() {
	dbc, err := db.Open(dbPath())
	if err != nil {
		os.Stderr.WriteString("hxd: " + err.Error() + "\n")
		os.Exit(1)
	}
	defer dbc.Close()

	if err := writePid(pidPath()); err != nil {
		os.Stderr.WriteString("hxd: cannot write pid file: " + err.Error() + "\n")
		os.Exit(1)
	}
	defer os.Remove(pidPath())

	st := store.New(dbc)
	eventsPath := spool.EventsPath(spoolDir())

	// Poll loop: ingest, sleep
	tick := 3 * time.Second
	for {
		n, err := ingest.Run(st, eventsPath)
		if err != nil {
			os.Stderr.WriteString("hxd: ingest: " + err.Error() + "\n")
		}
		if n > 0 {
			// Could update last_ingest_at file for hx status
		}
		time.Sleep(tick)
	}
}
