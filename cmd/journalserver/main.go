// Command journalserver runs the Journal Recall HTTP API.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/basium/journaletto/internal/config"
	"github.com/basium/journaletto/internal/httpapi"
)

func main() {
	configPath := flag.String("config", defaultConfigPath(), "path to the YAML config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	deps := httpapi.EntryDeps{
		JournalRoot:   cfg.JournalRoot,
		DevUserID:     cfg.DevUserID,
		MaxTags:       cfg.MaxTags,
		MaxEntryBytes: cfg.MaxEntryBytes,
	}
	mux := httpapi.NewRouter(deps)

	log.Printf("journalserver listening on %s (journal root: %s)", cfg.ListenAddr, cfg.JournalRoot)
	if err := http.ListenAndServe(cfg.ListenAddr, mux); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}

func defaultConfigPath() string {
	if p := os.Getenv("JOURNALETTO_CONFIG"); p != "" {
		return p
	}
	return "./config.yaml"
}
