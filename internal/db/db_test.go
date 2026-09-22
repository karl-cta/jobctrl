package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func backups(t *testing.T, path string) []string {
	t.Helper()
	m, err := filepath.Glob(path + ".backup-*")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestOpen_NoBackupForNewOrCurrentDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("open new db: %v", err)
	}
	db.Close()
	if got := backups(t, path); len(got) != 0 {
		t.Fatalf("a brand-new database must not be backed up, got %v", got)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	db.Close()
	if got := backups(t, path); len(got) != 0 {
		t.Fatalf("an up-to-date database must not be backed up, got %v", got)
	}
}

func TestOpen_BacksUpBeforePendingMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO applications (id, company_name, job_title, status) VALUES ('a1', 'Acme', 'Dev', 'Withdrawn')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Pretend the last migration has not run yet, like an upgraded binary
	// opening an older database.
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version = (SELECT MAX(version) FROM schema_migrations)`); err != nil {
		t.Fatalf("unrecord migration: %v", err)
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatalf("reopen with pending migration: %v", err)
	}
	defer db.Close()

	got := backups(t, path)
	if len(got) != 1 {
		t.Fatalf("expected exactly one backup file, got %v", got)
	}
	info, err := os.Stat(got[0])
	if err != nil || info.Size() == 0 {
		t.Fatalf("backup file unusable: %v", err)
	}

	// The backup is a real database holding the pre-migration state. Open it
	// raw: going through Open would migrate it too.
	old, err := sql.Open("sqlite", got[0])
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer old.Close()
	var status string
	if err := old.QueryRow(`SELECT status FROM applications WHERE id = 'a1'`).Scan(&status); err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if status != "Withdrawn" {
		t.Errorf("backup status = %q, want the pre-migration value Withdrawn", status)
	}

	// And the live database was migrated, with a visible trace.
	if err := db.QueryRow(`SELECT status FROM applications WHERE id = 'a1'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "Applied" {
		t.Errorf("live status = %q, want Applied after migration 006", status)
	}
	var events int
	if err := db.QueryRow(`SELECT COUNT(*) FROM timeline_events WHERE application_id = 'a1' AND description = 'Status changed from Withdrawn to Applied'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Errorf("migration trace events = %d, want 1", events)
	}
}
