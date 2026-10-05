package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// An Oiko built from this checkout with a type of Bridge being developed runs
// that type's commands, and reports it as added.
func TestMakeAddsABridgeType(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a whole Oiko")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "oiko")
	if err := Make("../..", []string{"example.com/oiko-hue=testdata/hue"}, exe); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"bridges": {"hue": {}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(exe, "-data", dir, "hue", "ping").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "pong" {
		t.Fatalf("hue ping: %v: %s", err, out)
	}

	// Both given by directory, Oiko and the type have no version.
	out, err = exec.Command(exe, "-version").CombinedOutput()
	if err != nil {
		t.Fatalf("-version: %v: %s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if lines[0] != "oiko unknown" || !slices.ContainsFunc(lines, func(l string) bool {
		return slices.Equal(strings.Fields(l), []string{"hue", "added", "example.com/oiko-hue", "example.com/oiko-hue", "unknown"})
	}) || !slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, "zigbee2mqtt ") && strings.Contains(l, " built-in ") }) {
		t.Errorf("-version:\n%s", out)
	}
}
