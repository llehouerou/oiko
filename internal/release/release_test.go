package release

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/llehouerou/oiko/internal/build"
)

// proxy is a fake module proxy listing the versions of each module, by path;
// it fails the test when asked about a private one, and answers 500 for a
// module named "broken".
func proxy(t *testing.T, lists map[string]string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), "/@v/list")
		switch {
		case strings.HasPrefix(path, "example.com/private"):
			t.Errorf("asked about %s", path)
		case strings.Contains(path, "broken"):
			http.Error(w, "boom", http.StatusInternalServerError)
		case lists[path] == "":
			http.NotFound(w, r)
		default:
			w.Write([]byte(lists[path]))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func added(module, version string) build.Type {
	return build.Type{Type: module, Module: module, Version: version}
}

func TestCheck(t *testing.T) {
	srv := proxy(t, map[string]string{
		build.Oiko:                    "v0.1.0\nv0.2.0\nv0.2.1\nv0.3.0\nv0.4.0-rc.1\n",
		build.Oiko + "/v2":            "v2.0.0\n", // not asked about during v0: v0.3.0 already breaks
		"example.com/hue":             "v1.2.0\nv2.0.0+incompatible\n",
		"example.com/lamp":            "v1.0.0\nv1.1.0\n",
		"example.com/lamp/v2":         "v2.0.0\nv2.1.0\n",
		"example.com/!sonos":          "v0.1.0\nv0.2.0\n", // escaped: example.com/Sonos
		"example.com/ahead":           "v0.2.0\n",
		"example.com/unknown-locally": "v1.0.0\n",
	})
	c := newChecker(build.Build{Version: "v0.2.0", Types: []build.Type{
		{Type: "zigbee2mqtt", BuiltIn: true, Module: build.Oiko, Version: "v0.2.0"},
		added("example.com/hue", "v1.2.0"),
		{Type: "hue-bis", Module: "example.com/hue", Version: "v1.2.0"}, // same module: checked once
		added("example.com/lamp", "v1.0.0"),
		added("example.com/Sonos", "v0.1.0"),
		added("example.com/ahead", "v0.2.1-0.20261004163119-85cb02bc0dfc"),
		added("example.com/unknown-locally", ""),
		added("example.com/private/x", "v1.0.0"),
		added("example.com/gone", "v1.0.0"),
		{Type: "nomodule", Package: "example.com/nomodule"},
	}}, srv.URL, "example.com/private")

	statuses, changed := c.Statuses()
	if err := c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	default:
		t.Error("a check finding newer releases does not tell")
	}
	want := []Status{
		{Module: build.Oiko, Current: "v0.2.0", Newest: "v0.2.1", Breaking: "v0.3.0", Newer: true},
		{Module: "example.com/hue", Current: "v1.2.0", Newest: "v1.2.0"},
		{Module: "example.com/lamp", Current: "v1.0.0", Newest: "v1.1.0", Breaking: "v2.1.0", Newer: true},
		{Module: "example.com/Sonos", Current: "v0.1.0", Newest: "v0.1.0", Breaking: "v0.2.0"},
		{Module: "example.com/ahead", Current: "v0.2.1-0.20261004163119-85cb02bc0dfc", Newest: "v0.2.0"},
		{Module: "example.com/unknown-locally", Newest: "v1.0.0"},
		{Module: "example.com/private/x", Current: "v1.0.0"},
		{Module: "example.com/gone", Current: "v1.0.0"},
	}
	if got, _ := c.Statuses(); !reflect.DeepEqual(got, want) {
		t.Errorf("after a check:\n got %+v\nwant %+v", got, want)
	}
	if statuses[0].Newest != "" {
		t.Error("a check changed the statuses handed out before it")
	}

	_, changed = c.Statuses()
	if err := c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
		t.Error("a check finding nothing new tells")
	default:
	}
}

func TestCheckFailing(t *testing.T) {
	srv := proxy(t, map[string]string{build.Oiko: "v0.3.0\n"})
	c := newChecker(build.Build{Version: "v0.2.0", Types: []build.Type{added("example.com/broken", "v1.0.0")}}, srv.URL, "")
	before, changed := c.Statuses()
	if err := c.Check(context.Background()); err == nil {
		t.Error("a proxy failing is not an error")
	}
	if after, _ := c.Statuses(); !reflect.DeepEqual(after, before) {
		t.Errorf("a failed check changed %+v into %+v", before, after)
	}
	select {
	case <-changed:
		t.Error("a failed check tells")
	default:
	}
}

func TestProxyOf(t *testing.T) {
	for goproxy, want := range map[string]string{
		"":                                   "https://proxy.golang.org",
		"https://goproxy.io/,direct":         "https://goproxy.io",
		"direct|https://a.example,https://b": "https://a.example",
		"off":                                "",
		"direct":                             "",
		"direct,off,https://a.example":       "",
	} {
		if got := proxyOf(goproxy); got != want {
			t.Errorf("proxyOf(%q) = %q, want %q", goproxy, got, want)
		}
	}
}

func TestNextMajor(t *testing.T) {
	for path, want := range map[string]string{
		"example.com/m":     "example.com/m/v2",
		"example.com/m/v2":  "example.com/m/v3",
		"gopkg.in/yaml.v3":  "gopkg.in/yaml.v4",
		"example.com/m/v1x": "example.com/m/v1x/v2",
	} {
		if got := nextMajor(path); got != want {
			t.Errorf("nextMajor(%q) = %q, want %q", path, got, want)
		}
	}
}
