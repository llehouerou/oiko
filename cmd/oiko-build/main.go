// Command oiko-build builds an Oiko with types of Bridge added to the
// built-in ones (ADR 0017):
//
//	go run github.com/llehouerou/oiko/cmd/oiko-build@v0.1.0 \
//		-with example.com/oiko-hue@v1.2.0 -o oiko
//
// It builds the Oiko it comes from, unless -oiko names another version, or a
// checkout. A -with names the package registering a type, with the version of
// its module; a module being developed is given by its directory instead:
// -with example.com/oiko-hue=../oiko-hue. Go must be installed.
package main

import (
	"flag"
	"log"
	"os"
	"runtime/debug"
	"strings"

	"github.com/llehouerou/oiko/internal/build"
)

func main() {
	var with []string
	oiko := flag.String("oiko", "", "Oiko's version, or the directory of a checkout (default: this command's version)")
	flag.Func("with", "a type of Bridge to add: `package@version`, or package=directory (repeatable)", func(s string) error {
		with = append(with, s)
		return nil
	})
	out := flag.String("o", "oiko", "the executable to write")
	flag.Parse()
	if flag.NArg() > 0 {
		flag.Usage()
		os.Exit(2)
	}
	if *oiko == "" {
		if bi, ok := debug.ReadBuildInfo(); ok && strings.HasPrefix(bi.Main.Version, "v") {
			*oiko = bi.Main.Version
		} else {
			log.Fatal("oiko-build: not run at a released version: give -oiko")
		}
	}
	if err := build.Make(*oiko, with, *out); err != nil {
		log.Fatalf("oiko-build: %v", err)
	}
}
