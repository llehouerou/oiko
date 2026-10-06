package history

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/llehouerou/oiko/internal/home"
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
	withFormat(t, 1, "")
	create(t, path)
	withFormat(t, 2, "v1.9.0")

	_, err := Open(path)
	if err == nil || !strings.Contains(err.Error(), "v1.9.0") {
		t.Fatalf("got %v; want a refusal naming v1.9.0", err)
	}
}

func TestFormat2ReadsTheAPIOriginAsUnknownAndAddsTheAuditLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	create(t, path)
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	// Back to format 1: no audit table, and Commands from the API.
	if _, err := db.Exec(`DROP TABLE audit; DROP TABLE live_views; PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	target := home.TargetFlag("away")
	run := home.Origin{Automation: "auto", Step: "step", Run: uuid.NewV7()}
	for i, origin := range []any{"api", run} {
		data, _ := json.Marshal(map[string]any{"id": fmt.Sprint(i), "target": target, "status": "confirmed", "origin": origin, "time": time.Unix(int64(i), 0)})
		if _, err := db.Exec(`INSERT INTO commands (id, target, time, status, command) VALUES (?, ?, ?, ?, ?)`,
			fmt.Sprint(i), target.Key(), time.Unix(int64(i), 0).UnixNano(), "confirmed", string(data)); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cs, err := s.Commands(target)
	if err != nil || len(cs) != 2 || cs[0].Origin != run || cs[1].Origin != (home.Origin{}) {
		t.Fatalf("commands = %+v, %v; want the Run's, then of unknown origin", cs, err)
	}
	var entries int
	if err := s.db.QueryRow(`SELECT count(*) FROM audit`).Scan(&entries); err != nil || entries != 0 {
		t.Fatalf("audit: %d entries, %v; want an empty table", entries, err)
	}
	if v := userVersion(t, path); v != 3 {
		t.Fatalf("user_version %d, want 3", v)
	}
	copied, err := sql.Open("sqlite", "file:"+path+".v1")
	if err != nil {
		t.Fatal(err)
	}
	defer copied.Close()
	var api int
	if err := copied.QueryRow(`SELECT count(*) FROM commands WHERE command LIKE '%"origin":"api"%'`).Scan(&api); err != nil || api != 1 {
		t.Fatalf("history.db.v1 holds %d Commands from the API, %v; want the database as it was", api, err)
	}
}

func TestTheAuditLogIsNeverEdited(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.Exec(`INSERT INTO audit (time, event, actor_kind, subject_kind) VALUES (1, 'test', 'unknown', 'unknown')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE audit SET event = 'edited'`); err == nil {
		t.Fatal("an audit entry was edited")
	}
}
