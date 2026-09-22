package main

import (
	"context"
	"embed"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"job-ctrl/internal/db"
	"job-ctrl/internal/handlers"
	"job-ctrl/internal/server"
)

// defaultNoReplyDays is how long an application stays "Applied" before it is
// considered unanswered. JOB_CTRL_NO_REPLY_DAYS=0 disables the transition.
const defaultNoReplyDays = 30

// noReplyInterval is how often the background job re-checks.
const noReplyInterval = time.Hour

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
	dbPath := os.Getenv("JOB_CTRL_DB_PATH")
	if dbPath == "" {
		dbPath = "job-ctrl.db"
	}

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	addr := os.Getenv("JOB_CTRL_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	handler := server.New(database, frontendFS, Version)

	// Background job: move long-unanswered applications to "NoReply".
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if days := noReplyDays(); days > 0 {
		log.Printf("No-reply transition after %d days", days)
		go handlers.New(database).RunNoReplyJob(ctx, days, noReplyInterval)
	}

	log.Printf("Starting JobCtrl on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
