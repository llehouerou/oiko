package oiko

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/llehouerou/oiko/internal/build"
	"github.com/llehouerou/oiko/internal/release"
)

const hue = "example.com/oiko-hue"

func TestPlanOf(t *testing.T) {
	b := build.Build{Version: "v0.2.0", Types: []build.Type{
		{Type: "zigbee2mqtt", BuiltIn: true, Package: build.Oiko + "/internal/zigbee2mqtt", Module: build.Oiko, Version: "v0.2.0"},
		{Type: "hue", Package: hue, Module: hue, Version: "v1.0.0"},
		{Type: "hue-bis", Package: hue, Module: hue, Version: "v1.0.0"}, // same package: once
		{Type: "sonos", Package: "example.com/sonos/oiko", Module: "example.com/sonos", Version: "v0.1.0"},
	}}
	oiko, with := planOf(b, []release.Status{
		{Module: build.Oiko, Current: "v0.2.0", Newest: "v0.3.0", Newer: true},
		{Module: hue, Current: "v1.0.0", Newest: "v1.1.0", Breaking: "v2.0.0", Newer: true},
		{Module: "example.com/sonos", Current: "v0.1.0", Newest: "v0.1.0"},
	})
	if oiko != "v0.3.0" || !slices.Equal(with, []string{"example.com/oiko-hue@v1.1.0", "example.com/sonos/oiko@v0.1.0"}) {
		t.Errorf("planOf: %s %v", oiko, with)
	}
	// Nothing the proxy knows of: everything as built.
	if oiko, with := planOf(b, nil); oiko != "v0.2.0" || !slices.Equal(with, []string{"example.com/oiko-hue@v1.0.0", "example.com/sonos/oiko@v0.1.0"}) {
		t.Errorf("planOf without statuses: %s %v", oiko, with)
	}
}

func TestReleased(t *testing.T) {
	if err := released(build.Build{Version: "v0.2.0", Types: []build.Type{{Type: "homekit", BuiltIn: true}, {Type: "hue", Version: "v1.0.0"}}}); err != nil {
		t.Error(err)
	}
	for name, b := range map[string]build.Build{
		"Oiko from a checkout":    {Types: []build.Type{}},
		"a type from a directory": {Version: "v0.2.0", Types: []build.Type{{Type: "hue", Package: hue, Module: hue}}},
	} {
		if err := released(b); err == nil {
			t.Errorf("%s: released", name)
		}
	}
}

func TestUpgradeRefuses(t *testing.T) {
	for inst, want := range map[string]string{
		"nixos":  "Nix manages",
		"docker": "Docker image",
		"binary": "built from a checkout", // this test's executable
	} {
		var out bytes.Buffer
		if err := upgrade(inst, []string{"-y"}, strings.NewReader(""), &out); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", inst, err)
		}
	}
	if err := upgrade("binary", []string{"hue"}, strings.NewReader(""), io.Discard); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Errorf("an argument: %v", err)
	}
}

// An Oiko adding hue v1.0.0 upgrades to the v1.1.0 a module proxy lists, and
// keeps the executable it replaces.
func TestUpgradeFromAFixtureProxy(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a whole Oiko twice")
	}
	env, err := exec.Command("go", "env", "GOPROXY", "GOMODCACHE").Output()
	if err != nil {
		t.Fatal(err)
	}
	goproxy, cache, _ := strings.Cut(strings.TrimSpace(string(env)), "\n")
	// The fixture serves hue; Oiko's dependencies come from the module cache,
	// else the usual proxy. Fetched modules go to a cache of the test's own.
	t.Setenv("GOPROXY", fixtureProxy(t).URL+",file://"+cache+"/cache/download,"+goproxy)
	t.Setenv("GOMODCACHE", t.TempDir())
	t.Setenv("GOFLAGS", "-modcacherw")
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOPRIVATE", "")
	t.Setenv("GONOPROXY", "")

	exe := filepath.Join(t.TempDir(), "oiko")
	if err := build.Make(".", []string{hue + "@v1.0.0"}, exe); err != nil {
		t.Fatal(err)
	}
	b := build.Build{Version: "v0.3.0", Types: []build.Type{{Type: "hue", Package: hue, Module: hue, Version: "v1.0.0"}}}
	c := release.New(b)
	if err := c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	statuses, _ := c.Statuses()
	oiko, with := planOf(b, statuses)
	if oiko != "v0.3.0" || !slices.Equal(with, []string{hue + "@v1.1.0"}) {
		t.Fatalf("planOf: %s %v, from %+v", oiko, with, statuses)
	}
	if err := replace(exe, ".", with); err != nil { // Oiko from this checkout
		t.Fatal(err)
	}
	for file, want := range map[string]string{exe: "v1.1.0", exe + ".old": "v1.0.0"} {
		out, err := exec.Command(file, "-version").CombinedOutput()
		if err != nil || !slices.ContainsFunc(strings.Split(string(out), "\n"), func(l string) bool {
			return slices.Equal(strings.Fields(l), []string{"hue", "added", hue, hue, want})
		}) {
			t.Errorf("%s -version: %v\n%s", file, err, out)
		}
	}
	if _, err := os.Stat(exe + ".new"); err == nil {
		t.Error("the new executable is left beside the replaced one")
	}
}

// fixtureProxy is a module proxy serving hue, the type of Bridge of
// internal/build/testdata, at v1.0.0 and v1.1.0.
func fixtureProxy(t *testing.T) *httptest.Server {
	dir := "internal/build/testdata/hue"
	files := map[string][]byte{hue + "/@v/list": []byte("v1.0.0\nv1.1.0\n")}
	for _, v := range []string{"v1.0.0", "v1.1.0"} {
		var z bytes.Buffer
		w := zip.NewWriter(&z)
		for _, name := range []string{"go.mod", "hue.go"} {
			content, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if name == "go.mod" {
				files[hue+"/@v/"+v+".mod"] = content
			}
			f, err := w.Create(hue + "@" + v + "/" + name)
			if err != nil {
				t.Fatal(err)
			}
			f.Write(content)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		files[hue+"/@v/"+v+".zip"] = z.Bytes()
		files[hue+"/@v/"+v+".info"] = fmt.Appendf(nil, `{"Version": %q, "Time": "2026-10-01T00:00:00Z"}`, v)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := files[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv
}
