# Releases carry the built web client

A release of Oiko is a semver tag, `vX.Y.Z`, on a commit of its own: a child of the released commit on `main` that adds the built web client, `web/dist`, and that no branch contains. `scripts/release` builds the client and makes that commit through a temporary index, leaving the working tree and `main` alone. The Go proxy serves a tag's tree, so the module of a release embeds its web client, and an Oiko with other types of Bridge (ADR 0017) builds anywhere Go is installed. `cmd/oiko-build` does it, like xcaddy for Caddy: it writes a main package importing the types asked for into a temporary module and builds it.

That commit also stamps the version in `internal/build/release.go`, empty on `main`. Go records a module's version only when it builds it from the proxy or a tagged checkout; Nix builds from a source tree, where Go records none, so without the stamp no Nix build of a release knows its version, and none could tell that a newer one exists (#79). Go's recorded version comes first; the stamp is read only when there is none.

## Considered Options

- **Committing `web/dist` on `main`**: no release step, but every change to the client leaves a built copy in the history and in every diff.
- **The client as a release asset `oiko-build` downloads**: keeps the module free of built files, but `go build` alone no longer gives a working Oiko, and the embed needs files on disk anyway.
- **Serving the client from a directory given at runtime**: the binary is no longer self-contained.

## Consequences

- A commit of `main` that is not released has no web client in the Go module cache: build it from a checkout instead (`oiko-build -oiko ../oiko`).
- Pseudo-versions of later commits on `main` do not count from the last release, whose tag is not one of their ancestors.
- Each release keeps one built client in the repository's objects, a few hundred KiB.
- The flake builds the client from source and ignores what a release carries, except its stamped version.
- A Nix build of a commit of `main` has no version: its Build says unknown.
