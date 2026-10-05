// Package build reads what an Oiko executable is made of, its Build (see
// CONTEXT.md): Oiko's version and each type of Bridge compiled in, with the
// package, module and version it comes from, as Go records them in every
// binary.
package build

import (
	"cmp"
	"runtime/debug"
	"strings"

	"github.com/llehouerou/oiko/bridge"
)

// Oiko is the module of Oiko itself, whose packages hold the built-in types.
const Oiko = "github.com/llehouerou/oiko"

// Build is what an Oiko executable is made of.
type Build struct {
	Version string `json:"version,omitempty"` // Oiko's; "" when unknown
	Types   []Type `json:"types"`
}

// Type is a type of Bridge compiled in.
type Type struct {
	Type    string `json:"type"`
	BuiltIn bool   `json:"builtIn"`
	Package string `json:"package"`
	Module  string `json:"module,omitempty"`  // "" when Go recorded none
	Version string `json:"version,omitempty"` // "" when unknown
}

// Current is the Build of the running executable.
func Current() Build {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		bi = &debug.BuildInfo{}
	}
	return Read(bi, release, bridge.Types())
}

// Read is the Build of the executable bi describes, with the types
// registered. stamped is Oiko's version as its release commit stamps it, used
// when Go recorded none for Oiko.
func Read(bi *debug.BuildInfo, stamped string, types []bridge.Registered) Build {
	modules := append([]*debug.Module{&bi.Main}, bi.Deps...)
	versionOf := func(m *debug.Module) string {
		if m.Path == Oiko {
			return cmp.Or(version(m), stamped)
		}
		return version(m)
	}
	b := Build{Types: []Type{}}
	if m := moduleOf(modules, Oiko); m != nil && m.Path == Oiko {
		b.Version = versionOf(m)
	}
	for _, r := range types {
		t := Type{Type: r.Type, Package: r.Package}
		if m := moduleOf(modules, r.Package); m != nil {
			t.Module, t.Version, t.BuiltIn = m.Path, versionOf(m), m.Path == Oiko
		}
		b.Types = append(b.Types, t)
	}
	return b
}

// moduleOf is the module among modules holding package pkg: the one with
// the longest path pkg is in.
func moduleOf(modules []*debug.Module, pkg string) *debug.Module {
	var found *debug.Module
	for _, m := range modules {
		if m.Path != "" && (pkg == m.Path || strings.HasPrefix(pkg, m.Path+"/")) && (found == nil || len(m.Path) > len(found.Path)) {
			found = m
		}
	}
	return found
}

// version is m's version as Go recorded it, a release or a pseudo-version,
// or "" for a development build or a module replaced by a directory.
func version(m *debug.Module) string {
	v := m.Version
	if m.Replace != nil {
		v = m.Replace.Version // "" for a directory
	}
	if v == "(devel)" {
		return ""
	}
	return v
}
