package main

import (
	"embed"
	"log"
	"net/http"
	"os"

	"job-ctrl/internal/db"
	"job-ctrl/internal/server"
)

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

	log.Printf("Starting JobCtrl on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
