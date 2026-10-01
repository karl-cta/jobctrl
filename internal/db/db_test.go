package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
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
	// Pretend migrations 006 and later have not run yet, like an upgraded
	// binary opening an older database.
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version >= 6`); err != nil {
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

func count(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func TestOpen_Pragmas(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	var fk, busy int
	var journal string
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`PRAGMA busy_timeout`).Scan(&busy); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if fk != 1 || busy != 5000 || journal != "wal" {
		t.Errorf("foreign_keys=%d busy_timeout=%d journal_mode=%s, want 1, 5000, wal", fk, busy, journal)
	}
}

func TestOpen_DeletingApplicationCascades(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	for _, q := range []string{
		`INSERT INTO applications (id, company_name, job_title) VALUES ('a1', 'Acme', 'Dev')`,
		`INSERT INTO interviews (id, application_id) VALUES ('i1', 'a1')`,
		`INSERT INTO contacts (id, application_id, name) VALUES ('c1', 'a1', 'Ada')`,
		`INSERT INTO timeline_events (id, application_id, event_type, description) VALUES ('e1', 'a1', 'created', 'Created')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO interviews (id, application_id) VALUES ('i2', 'missing')`); err == nil {
		t.Error("inserting an interview for a missing application must fail")
	}

	if _, err := db.Exec(`DELETE FROM applications WHERE id = 'a1'`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"interviews", "contacts", "timeline_events"} {
		if n := count(t, db, `SELECT COUNT(*) FROM `+table); n != 0 {
			t.Errorf("%s left after deleting the application: %d", table, n)
		}
	}
}

// A database at version 6 still holds the children of applications deleted
// while foreign keys were off. Migration 007 must remove exactly those rows.
func TestOpen_Migration007RemovesOnlyOrphans(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO applications (id, company_name, job_title) VALUES ('a1', 'Acme', 'Dev'), ('a2', 'Globex', 'Ops')`,
		`INSERT INTO interviews (id, application_id) VALUES ('i1', 'a1')`,
		`INSERT INTO contacts (id, application_id, name) VALUES ('c1', 'a1', 'Ada')`,
		`INSERT INTO timeline_events (id, application_id, event_type, description) VALUES ('e1', 'a1', 'created', 'Created'), ('e2', 'a2', 'created', 'Created')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	db.Close()

	// Plain connection: foreign keys off, like every version before 007.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO interviews (id, application_id) VALUES ('i-orphan', 'deleted')`,
		`INSERT INTO contacts (id, application_id, name) VALUES ('c-orphan', 'deleted', 'Bob')`,
		`INSERT INTO timeline_events (id, application_id, event_type, description) VALUES ('e-orphan', 'deleted', 'created', 'Created')`,
		`DELETE FROM schema_migrations WHERE version >= 7`,
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	raw.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatalf("reopen at version 6: %v", err)
	}
	defer db.Close()

	if n := count(t, db, `SELECT COUNT(*) FROM schema_migrations WHERE version = 7`); n != 1 {
		t.Fatalf("migration 007 not recorded")
	}
	for _, id := range []string{"i-orphan", "c-orphan", "e-orphan"} {
		n := count(t, db, `SELECT (SELECT COUNT(*) FROM interviews WHERE id = ?) + (SELECT COUNT(*) FROM contacts WHERE id = ?) + (SELECT COUNT(*) FROM timeline_events WHERE id = ?)`, id, id, id)
		if n != 0 {
			t.Errorf("orphan %s survived migration 007", id)
		}
	}
	for table, want := range map[string]int{"applications": 2, "interviews": 1, "contacts": 1, "timeline_events": 2} {
		if n := count(t, db, `SELECT COUNT(*) FROM `+table); n != want {
			t.Errorf("%s: %d rows after migration 007, want %d", table, n, want)
		}
	}
	if n := count(t, db, `SELECT COUNT(*) FROM pragma_foreign_key_check`); n != 0 {
		t.Errorf("foreign_key_check reports %d violations after migration 007", n)
	}
	// The orphans are still in the pre-migration backup.
	if got := backups(t, path); len(got) != 1 {
		t.Errorf("expected one backup before migration 007, got %v", got)
	}
}

// A migration and its version row are one transaction: when any statement of
// the script fails, the statements before it are rolled back too and the
// version is not recorded, so the next start retries it cleanly.
func TestRunMigrations_FailingMigrationRecordsNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	migrations := fstest.MapFS{
		"migrations/001_ok.sql":     {Data: []byte("CREATE TABLE t (a INTEGER);\nCREATE TABLE u (x INTEGER);\n")},
		"migrations/002_broken.sql": {Data: []byte("ALTER TABLE t ADD COLUMN b INTEGER;\nINSERT INTO t (a) VALUES (1);\nINSERT INTO missing VALUES (1);\n")},
	}
	db, err := sql.Open("sqlite", path+dsnParams)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	for attempt := 1; attempt <= 2; attempt++ {
		err := runMigrations(db, path, migrations)
		if err == nil || !strings.Contains(err.Error(), "no such table: missing") {
			t.Fatalf("attempt %d: err = %v, want the broken statement's error", attempt, err)
		}
		if n := count(t, db, `SELECT COUNT(*) FROM schema_migrations WHERE version = 1`); n != 1 {
			t.Errorf("attempt %d: multi-statement migration 001 not recorded", attempt)
		}
		if n := count(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE name = 'u'`); n != 1 {
			t.Errorf("attempt %d: second statement of migration 001 not applied", attempt)
		}
		if n := count(t, db, `SELECT COUNT(*) FROM schema_migrations WHERE version = 2`); n != 0 {
			t.Errorf("attempt %d: failed migration 002 was recorded", attempt)
		}
		if n := count(t, db, `SELECT COUNT(*) FROM pragma_table_info('t') WHERE name = 'b'`); n != 0 {
			t.Errorf("attempt %d: column added by the failed migration was kept", attempt)
		}
		if n := count(t, db, `SELECT COUNT(*) FROM t`); n != 0 {
			t.Errorf("attempt %d: row inserted by the failed migration was kept", attempt)
		}
	}
}

func TestOpen_MissingDirectoryIsExplained(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "app.db")
	_, err := Open(path)
	if err == nil {
		t.Fatal("open in a missing directory must fail")
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "exists and is writable") {
		t.Errorf("error does not name the path and the likely cause: %v", err)
	}
}
