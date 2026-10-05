package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSaveThenLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")

	var missing []string
	if err := Load(path, Format{}, &missing); err != nil || missing != nil {
		t.Fatalf("missing file: got %v, %v; want nil, nil", missing, err)
	}

	for _, want := range [][]string{{"a", "b"}, {"c"}} {
		if err := Save(path, Format{}, want); err != nil {
			t.Fatal(err)
		}
		var got []string
		if err := Load(path, Format{}, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	}

	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v", entries)
	}
}

func TestDocumentWithoutFormatIsFormat1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	write(t, path, `{"refresh_token": "r1"}`)

	var got struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := Load(path, Format{}, &got); err != nil || got.RefreshToken != "r1" {
		t.Fatalf("got %+v, %v; want r1", got, err)
	}
	if read(t, path) != `{"refresh_token": "r1"}` {
		t.Fatal("rewritten although nothing migrated")
	}
	assertFiles(t, path, "token.json")
}

// renamed is a test-only format 2 of a list of names: each becomes an object.
var renamed = Format{Migrations: []func(json.RawMessage) (json.RawMessage, error){
	func(data json.RawMessage) (json.RawMessage, error) {
		var names []string
		if err := json.Unmarshal(data, &names); err != nil {
			return nil, err
		}
		objects := []map[string]string{}
		for _, n := range names {
			objects = append(objects, map[string]string{"name": n})
		}
		return json.Marshal(objects)
	},
}}

type named struct{ Name string }

func TestLoadMigratesKeepingACopy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")
	if err := Save(path, Format{}, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	format1 := read(t, path)

	var got []named
	if err := Load(path, renamed, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []named{{"a"}}) {
		t.Fatalf("got %+v, want [{a}]", got)
	}
	if read(t, path+".v1") != format1 {
		t.Fatalf("copy %q, want the format 1 document %q", read(t, path+".v1"), format1)
	}

	// Migrated on disk: loading again migrates nothing, so makes no copy.
	os.Remove(path + ".v1")
	got = nil
	if err := Load(path, renamed, &got); err != nil || !reflect.DeepEqual(got, []named{{"a"}}) {
		t.Fatalf("reloaded %+v, %v; want [{a}]", got, err)
	}
	assertFiles(t, path, "devices.json")
}

func TestLoadMigratesADocumentWithoutFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")
	write(t, path, `["a"]`)

	var got []named
	if err := Load(path, renamed, &got); err != nil || !reflect.DeepEqual(got, []named{{"a"}}) {
		t.Fatalf("got %+v, %v; want [{a}]", got, err)
	}
	if read(t, path+".v1") != `["a"]` {
		t.Fatalf("copy %q, want the bare document", read(t, path+".v1"))
	}
}

func TestLoadRefusesANewerFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")
	if err := Save(path, renamed, []named{{"a"}}); err != nil {
		t.Fatal(err)
	}
	before := read(t, path)

	var got []string
	err := Load(path, Format{}, &got)
	if err == nil || !strings.Contains(err.Error(), path+".v1") {
		t.Fatalf("got %v; want a refusal naming %s.v1", err, path)
	}
	if read(t, path) != before {
		t.Fatal("refused document rewritten")
	}
}

func TestLoadRefusesAFormatOlderThanItsMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")
	write(t, path, `["a"]`)
	dropped := Format{Oldest: 2, Through: "v1.9.0"}

	var got []named
	err := Load(path, dropped, &got)
	if err == nil || !strings.Contains(err.Error(), "v1.9.0") {
		t.Fatalf("got %v; want a refusal naming v1.9.0", err)
	}
	assertFiles(t, path, "devices.json")
}

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// assertFiles checks the directory of path holds exactly names.
func assertFiles(t *testing.T, path string, names ...string) {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Dir(path))
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if !reflect.DeepEqual(got, names) {
		t.Fatalf("files %v, want %v", got, names)
	}
}
