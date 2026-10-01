package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
	_ "time/tzdata" // time zones sent by the browser load even without system tzdata

	"job-ctrl/internal/db"
	"job-ctrl/internal/handlers"
	"job-ctrl/internal/server"
)

// defaultNoReplyDays is how long an application stays "Applied" before it is
// considered unanswered. JOB_CTRL_NO_REPLY_DAYS=0 disables the transition.
const defaultNoReplyDays = 30

// noReplyInterval is how often the background job re-checks.
const noReplyInterval = time.Hour

// shutdownTimeout lets in-flight requests finish on SIGTERM. It stays under
// Docker's default 10 s stop grace period so the database still gets closed
// (and its WAL checkpointed) before a SIGKILL.
const shutdownTimeout = 8 * time.Second

// noReplyDays reads JOB_CTRL_NO_REPLY_DAYS. Unset, invalid or negative values
// fall back to the default; 0 disables the feature.
func noReplyDays() int {
	raw := os.Getenv("JOB_CTRL_NO_REPLY_DAYS")
	if raw == "" {
		return defaultNoReplyDays
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		log.Printf("invalid JOB_CTRL_NO_REPLY_DAYS=%q, using %d", raw, defaultNoReplyDays)
		return defaultNoReplyDays
	}
	return n
}

var Version = "dev"

//go:embed frontend/dist
var frontendFS embed.FS

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run returns instead of exiting so its deferred cleanup always runs.
func run() error {
	dbPath := os.Getenv("JOB_CTRL_DB_PATH")
	if dbPath == "" {
		dbPath = "job-ctrl.db"
	}
	shownPath := dbPath
	if abs, err := filepath.Abs(dbPath); err == nil {
		shownPath = abs
	}
	log.Printf("Starting JobCtrl %s (database: %s)", Version, shownPath)

	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("close database: %v", err)
		}
	}()

	addr := os.Getenv("JOB_CTRL_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           server.New(database, frontendFS, Version),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// Generous on purpose: a large import upload or export must not be
		// cut off halfway.
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 5 * time.Minute,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Background job: move long-unanswered applications to "NoReply".
	jobCtx, cancelJob := context.WithCancel(context.Background())
	jobDone := make(chan struct{})
	if days := noReplyDays(); days > 0 {
		log.Printf("No-reply transition after %d days", days)
		go func() {
			defer close(jobDone)
			handlers.New(database).RunNoReplyJob(jobCtx, days, noReplyInterval)
		}()
	} else {
		close(jobDone)
	}
	// Runs before the database is closed (defers run last in, first out).
	defer func() {
		cancelJob()
		<-jobDone
	}()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("server error: %w", err)
	}
	log.Printf("Listening on %s", addr)
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
	}
	stop()

	log.Printf("Shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("shutdown: %v", err)
	}
	return nil
}
