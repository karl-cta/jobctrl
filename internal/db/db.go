package db

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// dsnParams are applied by the modernc driver on every new connection.
//
// Foreign keys are ON, so deleting an application cascades to its children.
// Any future migration that rebuilds a parent table (DROP TABLE, or the
// create/copy/drop/rename table swap) must run with foreign keys OFF, or the
// implicit DELETE of the old table fires ON DELETE CASCADE and wipes every
// interview, contact and timeline event. `PRAGMA foreign_keys` is a no-op
// inside a transaction, so this needs a change in runMigrations, not just a
// line in the migration file.
const dsnParams = "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+dsnParams)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	// sql.Open is lazy: connect now so a bad path fails here with a clear
	// message instead of an opaque driver error from the first migration.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, openError(path, err)
	}

	if err := runMigrations(db, path, migrationsFS); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrations: %w", err)
	}
	return db, nil
}

// openError names the database path and, for the errors SQLite reports when
// the directory is missing or not writable (which modernc words as "out of
// memory (14)"), says what to check.
func openError(path string, err error) error {
	var se *sqlite.Error
	if errors.As(err, &se) {
		switch se.Code() & 0xff {
		case sqlite3.SQLITE_CANTOPEN, sqlite3.SQLITE_READONLY:
			return fmt.Errorf("cannot open database %s: check that the directory %s exists and is writable (%w)", path, filepath.Dir(path), err)
		}
	}
	return fmt.Errorf("cannot open database %s: %w", path, err)
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

func runMigrations(db *sql.DB, path string, migrations fs.FS) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrations, "migrations")
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

		content, err := fs.ReadFile(migrations, "migrations/"+name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if err := applyMigration(db, name, version, string(content)); err != nil {
			return err
		}
	}
	return nil
}

// applyMigration runs one migration script and records its version in a
// single transaction, so a crash or a failing statement leaves neither a
// half-applied schema nor a version marked as done. SQLite DDL is
// transactional; migration files must not contain their own BEGIN/COMMIT.
// With one open connection, only tx may be used until it ends.
func applyMigration(db *sql.DB, name string, version int, script string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", name, err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(script); err != nil {
		return fmt.Errorf("apply migration %s: %w", name, err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
		return fmt.Errorf("record migration %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", name, err)
	}
	return nil
}
