# Contributing

Outside contributions are welcome. Oiko has one maintainer working in spare time, so this page
says what helps most, where each thing goes and what to expect in return.

## Ways to help

- Report a bug you can reproduce.
- Answer a question, or tell how you use Oiko, in
  [Discussions](https://github.com/llehouerou/oiko/discussions).
- Fix a typo, or a page of the docs that is wrong or unclear.
- Write a type of Bridge for a system Oiko doesn't reach yet. It lives in your own repository
  and is listed in the [catalogue](https://llehouerou.github.io/oiko-catalogue/) (ADR 0020):
  see [Write a type of Bridge](docs/write-a-bridge.md). The core builds in only the local,
  standard protocols most homes meet (ADR 0021); a new built-in type starts with an issue.
- Take an agreed issue, such as one labelled `good first issue` or `help wanted`.

## Asking and proposing

Questions and ideas go to [Discussions](https://github.com/llehouerou/oiko/discussions), not to
Issues, which are kept for bugs and agreed work. Its categories:

- **Q&A**: how do I…, why does Oiko…
- **Ideas**: a feature you would like. Once the maintainer agrees, they open an issue for it (or
  convert the thread), and that issue is the one a pull request needs.
- **Types of Bridge**: asking for a type, or announcing one you wrote.
- **Announcements**: releases and breaking changes (ADR 0019), posted by the maintainer.

Questions about oiko-catalogue, oiko-netatmo or oiko-arlo come here too.

## Reporting a bug

Open an issue on the repository that holds the bug: a core feature on
[oiko](https://github.com/llehouerou/oiko/issues), a bug in an external type of Bridge on that
type's repository. Give `oiko -version`, what happened, what you expected and the steps to get
there. Redact your logs: names, addresses and tokens of your home have no place in a public issue.

A vulnerability is never an issue: report it privately through
[GitHub's private reporting](https://github.com/llehouerou/oiko/security/advisories/new) instead.

## Before a pull request: an issue first

Typos, doc fixes and obvious bug fixes can go straight to a pull request. Anything else needs an
issue the maintainer has agreed to first, from a bug report or an idea in Discussions: it saves you
writing a change that won't be taken.

## Develop

`nix develop` (or `direnv allow`) is the supported setup: it pins Go, Node, Mosquitto and the
Chromium the browser test drives, and turns on the hooks that run `scripts/check-public`.

```sh
direnv allow                       # or `nix develop`
make dev                           # API on :8080 + UI with hot reload on http://localhost:5180
make test                          # gofmt, go vet, Go tests, web lint, tests and browser test
make build                         # single ./oiko binary with the UI embedded
./oiko
go test -run=^$ -bench=. ./internal/zigbee2mqtt   # hot-path latency
make screenshots                   # docs/images/dashboard.png, from a made-up home
make icons                         # the PNG icons and the social preview, from their SVGs
```

Whoever visibly changes the built-in Dashboard reruns `make screenshots` in the same pull request.

Without Nix, install Go 1.27, Node 24, Mosquitto, make, Chromium (its path in `CHROMIUM`),
ImageMagick and the Jost font (for `make icons`), then run `git config core.hooksPath .githooks`.
Nothing promises this path keeps working.

CI runs an offline link check of every Markdown file (lychee), then `make test` in the dev shell.

After changing `web/package-lock.json` or `go.sum`, update `npmDepsHash` or `vendorHash` in
`nix/package.nix` (`nix build` prints the new one).

## Pull request terms

- CI is green (`make test`).
- Everything is in English, and nothing carries personal data: the rules are in
  [AGENTS.md](AGENTS.md), and the hooks run `scripts/check-public`.
- A hard-to-reverse decision comes with an ADR in [docs/adr](docs/adr); a new term goes into
  [GLOSSARY.md](GLOSSARY.md), and the code uses the glossary's words.
- The change follows the [compatibility rules](docs/upgrade.md#releases) (ADR 0019): the
  configuration, the API, what Code Steps see and the Bridge contract break only in a breaking
  release, and Oiko's own data is migrated, never broken.
- Commit messages say in plain words what changed and why. No Conventional Commits, no changelog
  line.

## AI-assisted contributions

They are accepted. You must have read every line, understand it and be able to defend it, and
your pull request description and replies are your own words. Unreviewed bulk output is closed.
[AGENTS.md](AGENTS.md) is the instruction file your agents follow in this repository.

## Response times

One maintainer, spare time, best effort: a reply usually comes within a couple of weeks. A
security report is acknowledged within 7 days.

## Conduct

Everyone taking part follows the [code of conduct](CODE_OF_CONDUCT.md). Report a breach to
conduct@oikohome.org.

## License

Oiko is under Apache-2.0 ([LICENSE](LICENSE), [NOTICE](NOTICE)). A contribution comes in under the
same license, as its §5 says: no DCO sign-off and no CLA, and you keep your copyright (ADR 0047).

## Releasing (maintainer)

```sh
scripts/release v0.3.1             # tags a release carrying the built web client (ADR 0018)
```

Releases follow [the compatibility rules](docs/upgrade.md#releases) (ADR 0019).
