// Command journaletto runs the Journaletto HTTP service.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"journaletto/internal/config"
	"journaletto/internal/provider"
	"journaletto/internal/server"
	"journaletto/internal/store"
	"journaletto/internal/worker"
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
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	prov, err := provider.New(cfg.Provider)
	if err != nil {
		log.Fatalf("provider: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	w := &worker.Worker{DB: db, Steps: worker.DefaultSteps(prov), Concurrency: cfg.WorkerConcurrency, MaxRetries: cfg.MaxRetries, Backoff: time.Second}
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		if err := w.Run(ctx); err != nil {
			log.Printf("worker: %v", err)
		}
	}()

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{Addr: addr, Handler: server.New(cfg, db)}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()
	log.Printf("journaletto %s (%s) listening on %s", cfg.Version, cfg.Commit, addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-workerDone
}
