package build

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Make writes to output the executable of an Oiko of version oiko, or from
// the checkout in that directory, adding the types of Bridge with lists (ADR
// 0017): package@version, or package=directory for a module being developed.
// It builds from a module of its own in a temporary directory, with the go
// command, which must be installed.
func Make(oiko string, with []string, output string) error {
	output, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "oiko-build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	main := "package main\n\nimport (\n\t\"" + Oiko + "\"\n"
	if err := goCmd(dir, "mod", "init", "oiko-build"); err != nil {
		return err
	}
	if err := require(dir, Oiko, oiko); err != nil {
		return err
	}
	for _, w := range with {
		pkg, version, ok := strings.Cut(w, "@")
		if !ok {
			pkg, version, ok = strings.Cut(w, "=")
		}
		if !ok || pkg == "" || version == "" {
			return fmt.Errorf("%s: want package@version or package=directory", w)
		}
		if err := require(dir, pkg, version); err != nil {
			return err
		}
		main += "\t_ \"" + pkg + "\"\n"
	}
	main += ")\n\nfunc main() { oiko.Main() }\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0o644); err != nil {
		return err
	}
	if err := goCmd(dir, "mod", "tidy"); err != nil {
		return err
	}
	return goCmd(dir, "build", "-o", output, ".")
}

// require makes the module of package pkg a requirement at version, or
// replaced by the directory version names.
func require(dir, pkg, version string) error {
	if strings.HasPrefix(version, "v") {
		return goCmd(dir, "get", pkg+"@"+version)
	}
	local, err := filepath.Abs(version)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(local, "go.mod")); err != nil {
		return errors.New(version + " holds no Go module")
	}
	// The package is taken as its module's root package.
	return goCmd(dir, "mod", "edit", "-require="+pkg+"@v0.0.0", "-replace="+pkg+"="+local)
}

func goCmd(dir string, args ...string) error {
	cmd := exec.Command("go", args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go %s: %w", strings.Join(args, " "), err)
	}
	return nil
}
