package history

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withFormat makes history.db's format, for t, the one migrated from o with
// m, going through r first when older.
func withFormat(t *testing.T, o int, r string, m ...func(*sql.Tx) error) {
	t.Helper()
	savedOldest, savedThrough, savedMigrations := oldest, through, migrations
	oldest, through, migrations = o, r, m
	t.Cleanup(func() { oldest, through, migrations = savedOldest, savedThrough, savedMigrations })
}

// extra is a test-only migration to format 2: it adds a table.
func extra(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE extra (x INTEGER)`)
	return err
}

func userVersion(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func hasExtra(t *testing.T, path string) bool {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'extra'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func create(t *testing.T, path string) {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
}

func TestAFreshDatabaseIsCreatedInTheCurrentFormat(t *testing.T) {
	withFormat(t, 1, "", func(*sql.Tx) error {
		t.Fatal("migrated a fresh database")
		return nil
	})
	path := filepath.Join(t.TempDir(), "history.db")
	create(t, path)
	if v := userVersion(t, path); v != 2 {
		t.Fatalf("user_version %d, want 2", v)
	}
	if _, err := os.Stat(path + ".v1"); err == nil {
		t.Fatal("copied although nothing migrated")
	}
}

func TestOpenMigratesKeepingACopy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	create(t, path)
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 0`); err != nil { // as written before formats
		t.Fatal(err)
	}
	db.Close()

	withFormat(t, 1, "", extra)
	create(t, path)
	if v := userVersion(t, path); v != 2 || !hasExtra(t, path) {
		t.Fatalf("user_version %d, extra %v; want migrated to 2", v, hasExtra(t, path))
	}
	if _, err := os.Stat(path + ".v1"); err != nil {
		t.Fatalf("no copy: %v", err)
	}
	if hasExtra(t, path+".v1") || userVersion(t, path+".v1") != 0 {
		t.Fatal("the copy is not the database as it was")
	}

	// Migrated: opening again migrates nothing, so makes no copy.
	os.Remove(path + ".v1")
	create(t, path)
	if _, err := os.Stat(path + ".v1"); err == nil {
		t.Fatal("copied although nothing migrated")
	}
}

func TestOpenRefusesANewerFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	withFormat(t, 1, "", extra)
	create(t, path)
	withFormat(t, 1, "")

	_, err := Open(path)
	if err == nil || !strings.Contains(err.Error(), path+".v1") {
		t.Fatalf("got %v; want a refusal naming %s.v1", err, path)
	}
	if v := userVersion(t, path); v != 2 {
		t.Fatalf("user_version %d, want left at 2", v)
	}
}

func TestOpenRefusesAFormatOlderThanItsMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	create(t, path)
	withFormat(t, 2, "v1.9.0")

	_, err := Open(path)
	if err == nil || !strings.Contains(err.Error(), "v1.9.0") {
		t.Fatalf("got %v; want a refusal naming v1.9.0", err)
	}
}
