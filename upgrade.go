package oiko

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/llehouerou/oiko/internal/build"
	"github.com/llehouerou/oiko/internal/release"
)

// upgrade rebuilds this Oiko, a plain binary, with Oiko and each added type
// of Bridge at their newest Release that cannot break them, once the human
// agrees (ADR 0019), and replaces its executable, keeping the previous one
// beside it: oiko upgrade [-y].
func upgrade(inst string, args []string, in io.Reader, out io.Writer) error {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	yes := fs.Bool("y", false, "upgrade without asking")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("usage: oiko upgrade [-y]")
	}
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		return err
	}
	if inst == "nixos" || strings.HasPrefix(exe, "/nix/store/") {
		return errors.New("upgrade: Nix manages this Oiko: change the oiko flake input and the versions of its bridges, then rebuild, as the dashboard's banner tells")
	}
	if inst == "docker" {
		return errors.New("upgrade: this Oiko runs from a Docker image: replace the oiko-build command of its Dockerfile, then rebuild the image, as the dashboard's banner tells")
	}
	b := build.Current()
	if err := released(b); err != nil {
		return err
	}
	c := release.New(b)
	if err := c.Check(context.Background()); err != nil {
		return fmt.Errorf("upgrade: %w", err)
	}
	statuses, _ := c.Statuses()
	for _, s := range statuses {
		if s.Newer {
			fmt.Fprintf(out, "%s %s → %s\n", s.Module, s.Current, s.Newest)
		}
	}
	for _, s := range statuses {
		if s.Breaking != "" {
			fmt.Fprintf(out, "%s: %s may break this build, not applied: upgrade by hand, following its release notes\n", s.Module, s.Breaking)
		}
	}
	if !slices.ContainsFunc(statuses, func(s release.Status) bool { return s.Newer }) {
		fmt.Fprintln(out, "Nothing to upgrade.")
		return nil
	}
	if _, err := exec.LookPath("go"); err != nil {
		return errors.New("upgrade: Go is not installed, and Oiko is rebuilt with it: see https://go.dev/doc/install")
	}
	if !*yes {
		fmt.Fprint(out, "Upgrade? [y/N] ")
		answer, _ := bufio.NewReader(in).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			fmt.Fprintln(out, "Nothing changed.")
			return nil
		}
	}
	oiko, with := planOf(b, statuses)
	if err := replace(exe, oiko, with); err != nil {
		return fmt.Errorf("upgrade: %w", err)
	}
	fmt.Fprintf(out, "Upgraded %s. Restart Oiko to run it; to roll back: mv %s.old %s\n", exe, exe, exe)
	return nil
}

// released fails unless Oiko and every added type of Bridge in b have a
// version, which a development build lacks: one from a checkout, or adding a
// type from a directory.
func released(b build.Build) error {
	if b.Version == "" {
		return errors.New("upgrade: Oiko's version is unknown, it was built from a checkout: rebuild it with oiko-build")
	}
	for _, t := range b.Types {
		if !t.BuiltIn && t.Version == "" {
			return fmt.Errorf("upgrade: the type %s (%s) has no version, it was built from a directory: rebuild it with oiko-build", t.Type, t.Package)
		}
	}
	return nil
}

// planOf is how to rebuild b, as build.Make takes it: Oiko's version and each
// added package@version, once and sorted. Oiko and each added type go to their
// newest Release when the statuses say it is newer, which never may break
// them, and stay as built otherwise.
func planOf(b build.Build, statuses []release.Status) (oiko string, with []string) {
	version := func(module, current string) string {
		if i := slices.IndexFunc(statuses, func(s release.Status) bool { return s.Module == module }); i >= 0 && statuses[i].Newer {
			return statuses[i].Newest
		}
		return current
	}
	for _, t := range b.Types {
		if w := t.Package + "@" + version(t.Module, t.Version); !t.BuiltIn && !slices.Contains(with, w) {
			with = append(with, w)
		}
	}
	slices.Sort(with)
	return version(build.Oiko, b.Version), with
}

// replace builds the Oiko of version oiko with the types in with beside the
// executable exe, links exe to exe.old, replacing an older one, then renames
// the new one over exe: there is an executable at exe at every moment.
func replace(exe, oiko string, with []string) error {
	next, old := exe+".new", exe+".old"
	// Fails early, before a whole build, when the directory is not writable.
	if err := os.WriteFile(next, nil, 0o755); err != nil {
		return err
	}
	if err := build.Make(oiko, with, next); err != nil {
		os.Remove(next)
		return err
	}
	if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Link(exe, old); err != nil {
		return err
	}
	return os.Rename(next, exe)
}
