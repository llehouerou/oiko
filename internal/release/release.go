// Package release tells whether newer Releases (see CONTEXT.md) exist of Oiko
// and of the module of each type of Bridge a build adds, as the Go module
// proxy lists them. It never applies anything (ADR 0019).
//
// It follows Go's environment: the proxy is GOPROXY's first, none with "off"
// or only "direct"; a module matching GONOPROXY, by default GOPRIVATE, is
// never asked about.
package release

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"

	"github.com/llehouerou/oiko/internal/build"
)

// When Oiko asks the proxy: a minute after starting, the network up by then,
// every six hours after, and half an hour after a failed check.
const (
	first = time.Minute
	every = 6 * time.Hour
	retry = 30 * time.Minute
)

// Status is what the proxy lists of a module built into Oiko.
type Status struct {
	Module   string `json:"module"`
	Current  string `json:"current,omitempty"`  // the version built in; "" when unknown
	Newest   string `json:"newest,omitempty"`   // its newest Release that cannot break Current; "" when unknown
	Breaking string `json:"breaking,omitempty"` // its newest Release that may break Current, if any
	Newer    bool   `json:"newer"`              // Newest is newer than Current
}

// Checker checks the Releases of the modules of a Build.
type Checker struct {
	proxy   string // "" when nothing is checked
	private string // patterns of the modules never asked about
	client  *http.Client

	mu       sync.Mutex
	statuses []Status // replaced, never changed in place
	changed  chan struct{}
}

// New is the Checker of b, following Go's environment.
func New(b build.Build) *Checker {
	return newChecker(b, proxyOf(os.Getenv("GOPROXY")), cmp.Or(os.Getenv("GONOPROXY"), os.Getenv("GOPRIVATE")))
}

func newChecker(b build.Build, proxy, private string) *Checker {
	statuses := []Status{{Module: build.Oiko, Current: b.Version}}
	for _, t := range b.Types {
		if !t.BuiltIn && t.Module != "" && !slices.ContainsFunc(statuses, func(s Status) bool { return s.Module == t.Module }) {
			statuses = append(statuses, Status{Module: t.Module, Current: t.Version})
		}
	}
	return &Checker{proxy: proxy, private: private, client: &http.Client{Timeout: 30 * time.Second},
		statuses: statuses, changed: make(chan struct{})}
}

// proxyOf is the proxy GOPROXY's value names first, "" when none comes
// before "off".
func proxyOf(goproxy string) string {
	for p := range strings.FieldsFuncSeq(cmp.Or(goproxy, "https://proxy.golang.org,direct"), func(r rune) bool { return r == ',' || r == '|' }) {
		switch p = strings.TrimSpace(p); p {
		case "off":
			return ""
		case "direct":
		default:
			return strings.TrimSuffix(p, "/")
		}
	}
	return ""
}

// Statuses is each module's Status, Oiko's first, and a channel closed once
// they change.
func (c *Checker) Statuses() ([]Status, <-chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.statuses, c.changed
}

// Run checks until ctx ends.
func (c *Checker) Run(ctx context.Context) {
	if c.proxy == "" {
		log.Print("release: GOPROXY names no proxy, newer releases are not checked")
		return
	}
	wait := first
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = every
		if err := c.Check(ctx); err != nil && ctx.Err() == nil {
			log.Printf("release: %v; checking again in %v", err, retry)
			wait = retry
		}
	}
}

// Check asks the proxy about every module once; a failure changes nothing.
func (c *Checker) Check(ctx context.Context) error {
	if c.proxy == "" {
		return errors.New("GOPROXY names no proxy to check releases with")
	}
	statuses, _ := c.Statuses()
	statuses = slices.Clone(statuses)
	for i := range statuses {
		s := &statuses[i]
		if module.MatchPrefixPatterns(c.private, s.Module) {
			continue
		}
		versions, err := c.releases(ctx, s.Module)
		if err != nil {
			return err
		}
		s.Newest = newest(versions, func(v string) bool { return compatible(s.Current, v) })
		if semver.Major(s.Current) == "v0" {
			s.Breaking = newest(versions, func(v string) bool { return semver.Compare(v, s.Current) > 0 && !compatible(s.Current, v) })
		} else {
			next, err := c.releases(ctx, nextMajor(s.Module))
			if err != nil {
				return err
			}
			s.Breaking = newest(next, func(string) bool { return true })
		}
		s.Newer = s.Current != "" && s.Newest != "" && semver.Compare(s.Newest, s.Current) > 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !slices.Equal(statuses, c.statuses) {
		c.statuses = statuses
		close(c.changed)
		c.changed = make(chan struct{})
	}
	return nil
}

// releases are the Releases the proxy lists of the module at path: neither
// pre-releases, nor pseudo-versions, nor +incompatible ones. There are none
// when the proxy knows no such module.
func (c *Checker) releases(ctx context.Context, path string) ([]string, error) {
	escaped, err := module.EscapePath(path)
	if path == "" || err != nil {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.proxy+"/"+escaped+"/@v/list", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusGone:
		return nil, nil
	default:
		return nil, fmt.Errorf("%s: %s", req.URL, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var versions []string
	for _, v := range strings.Fields(string(body)) {
		if semver.IsValid(v) && semver.Prerelease(v) == "" && semver.Build(v) == "" {
			versions = append(versions, v)
		}
	}
	return versions, nil
}

// newest is the newest of versions that keep holds for, "" if none.
func newest(versions []string, keep func(string) bool) string {
	n := ""
	for _, v := range versions {
		if keep(v) && semver.Compare(v, n) > 0 {
			n = v
		}
	}
	return n
}

// compatible tells whether Release v cannot break what version current
// built (ADR 0019): during v0 a minor may break, so only a version of the
// same minor cannot; after, another major is another module path, so any
// version of this one cannot. An unknown current is compatible with all.
func compatible(current, v string) bool {
	return semver.Major(current) != "v0" || semver.MajorMinor(v) == semver.MajorMinor(current)
}

// nextMajor is the path of the major after the one of the module at path:
// example.com/m/v2 after example.com/m, gopkg.in/m.v3 after gopkg.in/m.v2.
func nextMajor(path string) string {
	prefix, major, ok := module.SplitPathVersion(path)
	if !ok {
		return ""
	}
	if major == "" {
		return prefix + "/v2"
	}
	n, err := strconv.Atoi(strings.TrimSuffix(major[2:], "-unstable"))
	if err != nil {
		return ""
	}
	return prefix + major[:2] + strconv.Itoa(n+1)
}
