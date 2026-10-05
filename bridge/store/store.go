// Package store keeps JSON documents in a data directory (ADR 0003), each in
// a format migrated forward on loading (ADR 0019). Oiko keeps its own state
// with it, and a type of Bridge its own, in its Env.DataDir:
//
//	var tokenFormat store.Format // format 1; add a migration to change it
//
//	store.Load(filepath.Join(env.DataDir, "token.json"), tokenFormat, &tok)
//	store.Save(filepath.Join(env.DataDir, "token.json"), tokenFormat, tok)
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Format is the format of a store: the oldest one it still migrates from, and
// the migrations from there to the current one.
type Format struct {
	Oldest  int    // oldest format migrated from; 0 is 1
	Through string // the release to go through first with an older store, e.g. "v1.9.0"
	// Migrations[i] rewrites the data of format Oldest+i into Oldest+i+1.
	Migrations []func(json.RawMessage) (json.RawMessage, error)
}

func (f Format) oldest() int { return max(f.Oldest, 1) }

// Current is the format Save writes.
func (f Format) Current() int { return f.oldest() + len(f.Migrations) }

// Check refuses a store at path in format version, when this Oiko reads its
// formats from oldest to current: a newer one names the copy to restore, an
// older one the release to go through first.
func Check(path string, version, oldest, current int, through string) error {
	if version > current {
		restore := fmt.Sprintf("%s.v%d", path, current)
		if _, err := os.Stat(restore); errors.Is(err, fs.ErrNotExist) {
			restore += " (there is none: the store did not exist before)"
		}
		return fmt.Errorf("%s is format %d, newer than this Oiko's %d: restore %s, or run a newer Oiko", path, version, current, restore)
	}
	if version < oldest {
		return fmt.Errorf("%s is format %d, older than this Oiko migrates from (%d): run %s first", path, version, oldest, through)
	}
	return nil
}

// envelope is a document as written: its data and their format. A document
// without one is format 1.
type envelope struct {
	Format int             `json:"format"`
	Data   json.RawMessage `json:"data"`
}

// Load decodes the document at path into v, migrating it to f's current
// format first, after keeping a copy of it named after the format it leaves.
// A missing file leaves v untouched: it is a first start, not an error.
func Load(path string, f Format, v any) error {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var e envelope
	if err := json.Unmarshal(raw, &e); err != nil || e.Format < 1 || e.Data == nil {
		e = envelope{Format: 1, Data: raw}
	}
	if err := Check(path, e.Format, f.oldest(), f.Current(), f.Through); err != nil {
		return err
	}
	if e.Format < f.Current() {
		if err := replace(fmt.Sprintf("%s.v%d", path, e.Format), raw); err != nil {
			return err
		}
		for _, m := range f.Migrations[e.Format-f.oldest():] {
			if e.Data, err = m(e.Data); err != nil {
				return fmt.Errorf("migrating %s: %w", path, err)
			}
		}
		if err := Save(path, f, e.Data); err != nil {
			return err
		}
	}
	return json.Unmarshal(e.Data, v)
}

// Save replaces the document at path with v in f's current format.
func Save(path string, f Format, v any) error {
	data, err := json.MarshalIndent(struct {
		Format int `json:"format"`
		Data   any `json:"data"`
	}{f.Current(), v}, "", "  ")
	if err != nil {
		return err
	}
	return replace(path, data)
}

// replace writes data to path atomically: a crash leaves either the old file
// or the new one, never a torn file.
func replace(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(dir) // make the rename itself durable
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
