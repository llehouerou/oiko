# A catalogue of types of Bridge, indexed from a manifest

Types of Bridge others write (ADR 0017) are found through a public catalogue, kept in a repository of its own, `llehouerou/oiko-catalogue`, apart from Oiko. A type opts in by giving its repository the GitHub topic `oiko-bridge` and its module, at the repository's root, a manifest, `oiko-bridge.json`: each type its root package registers, by name, with a description and an example of its section of the configuration (the section's body; the catalogue names it after the type). The catalogue's README specifies the format.

The manifest holds only what nothing else says. The minimum Oiko a version of a type needs is its `go.mod`'s requirement on `github.com/llehouerou/oiko`: Go's minimal version selection builds at least that Oiko anyway, so a declared minimum could only disagree with it. Versions are what the Go module proxy lists, whatever hosts the module, pre-releases left out. The manifest read is the latest release's, the one the catalogue pins.

A scheduled job builds the latest version of each type against the latest Oiko release with `oiko-build`, and runs the result's `-version`: the types the module registers must be the manifest's. It publishes `index.json` in the catalogue's repository: each module with its types, versions and their minimum Oiko, and whether its latest version builds with the latest Oiko, with the build's error when it does not. A manifest that is missing, invalid, or names other types than those registered is listed with the reason, not indexed. A pair already built, the same version of the type against the same Oiko, is not built again.

Building and running a type executes code nobody reviewed. The job doing it may only read; another job, which runs nothing of the types, checks the index and commits it.

## Considered Options

- **The minimum Oiko in the manifest**: what ADR 0019 first wrote, but redundant with `go.mod` and free to contradict it.
- **The manifest of the default branch**: fetched without the proxy, but it describes code no release carries, and can disagree with the version the catalogue pins.
- **Types not checked against the build**: no third-party code runs, but a misspelt type reaches users as a `bridges` section Oiko refuses.
- **One job building and publishing**: simpler, but a type's `init` could rewrite the index the catalogue turns into build commands.
- **A catalogue in Oiko's repository**: one repository less, but its schedule, history and Pages would sit in Oiko's, and it would follow Oiko's releases for no reason.

## Consequences

- While Oiko is private the job reads it with a read-only token, `GOPRIVATE` set; going public (#85) removes both, nothing else.
- A module registering types from several packages is not supported: `oiko-build` imports its root package.
- A type is listed only once released; one whose latest release fails to build stays listed, marked incompatible.
