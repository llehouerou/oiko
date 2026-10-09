package plug

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestGuide keeps Oiko's guide, which quotes this package, true to it: each
// Go block of docs/write-a-bridge.md must be found verbatim in one of the
// package's files. Oiko's own check: an author's repository has no guide.
func TestGuide(t *testing.T) {
	guide, err := os.ReadFile("../../write-a-bridge.md")
	if err != nil {
		t.Fatal(err)
	}
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, string(data))
	}
	blocks := regexp.MustCompile("(?ms)^```go\n(.*?)^```$").FindAllStringSubmatch(string(guide), -1)
	if len(blocks) == 0 {
		t.Fatal("no Go block in the guide")
	}
	for _, b := range blocks {
		if !slices.ContainsFunc(files, func(f string) bool { return strings.Contains(f, b[1]) }) {
			t.Errorf("a Go block of the guide is in none of the package's files:\n%s", b[1])
		}
	}
}
