package oiko

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/llehouerou/oiko/internal/access"
)

// The host command and its socket are the last way back in (ADR 0026, 0035),
// and a contract (ADR 0019): oiko sign-in-link writes one hostRequest, as a
// line of JSON, to <data>/sign-in-link.sock, and Oiko answers one
// hostReply. The socket does nothing else.
const hostSocket = "sign-in-link.sock"

// hostRequest asks for the Persons, or, with Person, a Sign-in link for them.
type hostRequest struct {
	Person string `json:"person,omitempty"`
}

// hostReply is the Persons, or a Sign-in link and when it expires, or why
// the request was refused.
type hostReply struct {
	Persons []hostPerson `json:"persons,omitempty"`
	Link    string       `json:"link,omitempty"`
	Expires time.Time    `json:"expires,omitzero"`
	Error   string       `json:"error,omitempty"`
}

type hostPerson struct {
	ID    string       `json:"id"`
	Name  string       `json:"name"`
	Level access.Level `json:"level"`
}

// socketPath is the host socket in dataDir, refused if a socket address
// cannot hold it.
func socketPath(dataDir string) (string, error) {
	path := filepath.Join(dataDir, hostSocket)
	if max := len(syscall.RawSockaddrUnix{}.Path) - 1; len(path) > max {
		return "", fmt.Errorf("%s: the data directory's path is too long for a socket address, which holds %d bytes at most", path, max)
	}
	return path, nil
}

// listenHost listens on the host socket in dataDir, mode 0600, replacing one
// a crash left behind; one that an Oiko still answers is refused.
func listenHost(dataDir string) (net.Listener, error) {
	path, err := socketPath(dataDir)
	if err != nil {
		return nil, err
	}
	if c, err := net.Dial("unix", path); err == nil {
		c.Close()
		return nil, fmt.Errorf("%s: another Oiko runs on this data directory", path)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	// Created 0600 from the start, never open to others a moment: it hands out
	// Sign-in links, an Admin's too. The umask is the process's: files made
	// meanwhile are at most stricter.
	old := syscall.Umask(0o177)
	defer syscall.Umask(old)
	return net.Listen("unix", path)
}

// serveHost answers each connection to l, until ctx is done, with the
// Persons of acc or a Sign-in link at origin, the Public URL or else
// http://localhost.
func serveHost(ctx context.Context, l net.Listener, acc *access.Store, origin string) {
	go func() {
		<-ctx.Done()
		l.Close()
	}()
	for {
		c, err := l.Accept()
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("oiko: %s: %v", hostSocket, err)
			}
			return
		}
		go func() {
			defer c.Close()
			c.SetDeadline(time.Now().Add(10 * time.Second))
			var req hostRequest
			var reply hostReply
			if line, err := bufio.NewReader(io.LimitReader(c, 4<<10)).ReadBytes('\n'); err != nil {
				reply.Error = "a request is one line of JSON"
			} else if err := json.Unmarshal(line, &req); err != nil {
				reply.Error = err.Error()
			} else if req.Person == "" {
				reply.Persons = []hostPerson{}
				for _, p := range acc.HostPersons() {
					reply.Persons = append(reply.Persons, hostPerson{p.ID, p.Name, p.Level})
				}
			} else if secret, expires, err := acc.HostLink(req.Person); err != nil {
				reply.Error = err.Error()
			} else {
				reply.Link, reply.Expires = origin+"/sign-in#"+secret, expires
			}
			json.NewEncoder(c).Encode(reply)
		}()
	}
}

// signInLink runs oiko sign-in-link [id] on the Oiko of dataDir: without an
// id it lists the Persons, with one it prints a Sign-in link for them.
func signInLink(dataDir string, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("sign-in-link", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return errors.New("usage: oiko [-data <dir>] sign-in-link [<id>]")
	}
	path, err := socketPath(dataDir)
	if err != nil {
		return err
	}
	c, err := net.Dial("unix", path)
	if err != nil {
		return fmt.Errorf("sign-in-link: is Oiko running on %s, and is this its user? %w", dataDir, err)
	}
	defer c.Close()
	b, _ := json.Marshal(hostRequest{Person: fs.Arg(0)})
	if _, err := c.Write(append(b, '\n')); err != nil {
		return err
	}
	var reply hostReply
	if err := json.NewDecoder(c).Decode(&reply); err != nil {
		return fmt.Errorf("sign-in-link: %w", err)
	}
	switch {
	case reply.Error != "":
		return fmt.Errorf("sign-in-link: %s", reply.Error)
	case reply.Link != "":
		fmt.Fprintf(out, "Open this Sign-in link before %s; it signs in once:\n%s\n", reply.Expires.Local().Format("15:04"), reply.Link)
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tACCESS LEVEL\tID")
	for _, p := range reply.Persons {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", p.Name, p.Level, p.ID)
	}
	return tw.Flush()
}
