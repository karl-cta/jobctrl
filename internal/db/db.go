package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_journal=WAL&_foreign_keys=on&_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	if err := runMigrations(db, path); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrations: %w", err)
	}
	return db, nil
}

// backupBeforeMigration copies the database next to itself before a schema
// change touches it, so an upgrade can always be rolled back by hand.
// `VACUUM INTO` produces a consistent snapshot even with WAL pages pending,
// which a plain file copy would miss. Returns the backup path.
func backupBeforeMigration(db *sql.DB, path string) (string, error) {
	if path == "" || path == ":memory:" {
		return "", nil
	}
	if _, err := os.Stat(path); err != nil {
		return "", nil // brand-new database, nothing to protect
	}
	backup := fmt.Sprintf("%s.backup-%s", path, time.Now().UTC().Format("20060102-150405"))
	if _, err := db.Exec(`VACUUM INTO ?`, backup); err != nil {
		return "", fmt.Errorf("backup to %s: %w", backup, err)
	}
	return backup, nil
}

func runMigrations(db *sql.DB, path string) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return err
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	// Find what is pending first: the backup is only worth taking when a
	// migration is actually going to run against existing data.
	var pending []int
	for i := range files {
		version := i + 1
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			pending = append(pending, i)
		}
	}
	if len(pending) == 0 {
		return nil
	}
	var applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&applied); err != nil {
		return err
	}
	if applied > 0 {
		// Existing database about to change shape: snapshot it first.
		backup, err := backupBeforeMigration(db, path)
		if err != nil {
			return err
		}
		if backup != "" {
			log.Printf("Database backup written to %s before applying %d migration(s)", backup, len(pending))
		}
	}

	for _, i := range pending {
		name := files[i]
		version := i + 1

		content, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		if _, err := db.Exec(string(content)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}

		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			return fmt.Errorf("record migration %s: %w", name, err)
		}
	}
	return nil
}
