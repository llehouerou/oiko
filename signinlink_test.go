package oiko

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/llehouerou/oiko/internal/access"
)

// hosted is an Oiko claimed by Alice serving the host socket in a temporary
// data directory: the directory, its access store, and Alice.
func hosted(t *testing.T) (string, *access.Store, access.Identity) {
	t.Helper()
	dir := t.TempDir()
	acc, err := access.Open(dir, time.Now, func(access.Entry) {})
	if err != nil {
		t.Fatal(err)
	}
	setup, _ := acc.Setup()
	session, err := acc.Claim(setup, "Alice", "")
	if err != nil {
		t.Fatal(err)
	}
	alice, _ := acc.Resolve(session)
	l, err := listenHost(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go serveHost(ctx, l, acc, "https://oiko.example")
	return dir, acc, alice
}

func TestTheHostListsThePersonsAndPrintsALinkForOne(t *testing.T) {
	dir, acc, alice := hosted(t)
	var out bytes.Buffer
	if err := signInLink(dir, nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Alice") || !strings.Contains(out.String(), "admin") || !strings.Contains(out.String(), alice.ID) {
		t.Errorf("listed:\n%s", out.String())
	}

	out.Reset()
	if err := signInLink(dir, []string{alice.ID}, &out); err != nil {
		t.Fatal(err)
	}
	_, secret, ok := strings.Cut(strings.TrimSpace(out.String()), "https://oiko.example/sign-in#")
	if !ok {
		t.Fatalf("printed:\n%s", out.String())
	}
	if _, err := acc.SignInWithLink(secret, "Firefox on Linux"); err != nil {
		t.Fatalf("the printed link: %v", err)
	}
	alice.Fresh = true
	if list, _ := acc.Sessions(alice, alice.ID); list[len(list)-1].Method != "host" {
		t.Errorf("the Session it opened: %+v; want from the host", list[len(list)-1])
	}

	if err := signInLink(dir, []string{"unknown"}, &out); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("a link for no one: %v", err)
	}
}

func TestTheHostSocketIsPrivateAndReplacedAfterACrash(t *testing.T) {
	dir, _, _ := hosted(t)
	info, err := os.Stat(filepath.Join(dir, hostSocket))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 || info.Mode().Type() != os.ModeSocket {
		t.Errorf("socket mode %v, want a socket, 0600", info.Mode())
	}
	if _, err := listenHost(dir); err == nil || !strings.Contains(err.Error(), "another Oiko") {
		t.Errorf("a second Oiko on the data directory: %v", err)
	}

	// A crash leaves the socket file, which nothing answers.
	stale := t.TempDir()
	l, err := net.Listen("unix", filepath.Join(stale, hostSocket))
	if err != nil {
		t.Fatal(err)
	}
	l.(*net.UnixListener).SetUnlinkOnClose(false)
	l.Close()
	l, err = listenHost(stale)
	if err != nil {
		t.Fatalf("over a stale socket: %v", err)
	}
	l.Close()
}

func TestADataPathTooLongForASocketFailsSayingSo(t *testing.T) {
	dir := filepath.Join(t.TempDir(), strings.Repeat("d", 120))
	for _, err := range []error{
		func() error { _, err := listenHost(dir); return err }(),
		signInLink(dir, nil, &bytes.Buffer{}),
	} {
		if err == nil || !strings.Contains(err.Error(), "too long for a socket address") {
			t.Errorf("%v; want the reason", err)
		}
	}
}
