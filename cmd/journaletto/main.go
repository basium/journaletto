// Command journaletto runs the Journaletto HTTP service.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // CGO-free SQLite driver

	"journaletto/internal/config"
	"journaletto/internal/server"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe /healthz on the configured port and exit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if *healthcheck {
		res, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", cfg.Port))
		if err != nil || res.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		return
	}

	if err := os.MkdirAll(cfg.DataRoot, 0o750); err != nil {
		log.Fatalf("data root: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o750); err != nil {
		log.Fatalf("db dir: %v", err)
	}
	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("journaletto %s (%s) listening on %s", cfg.Version, cfg.Commit, addr)
	log.Fatal(http.ListenAndServe(addr, server.New(cfg)))
}
