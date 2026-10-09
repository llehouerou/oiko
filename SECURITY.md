# Security policy

This one policy covers every repository of Oiko's ecosystem:
[oiko](https://github.com/llehouerou/oiko),
[oiko-catalogue](https://github.com/llehouerou/oiko-catalogue),
[oiko-netatmo](https://github.com/llehouerou/oiko-netatmo),
[oiko-arlo](https://github.com/llehouerou/oiko-arlo) and
[go-arlo](https://github.com/llehouerou/go-arlo). Each of the others carries a short
`SECURITY.md` pointing here.

## Reporting a vulnerability

Report it privately through GitHub's private vulnerability reporting, on the repository that
holds the bug, so the advisory and its private fork sit next to the code: for Oiko itself,
[report a vulnerability](https://github.com/llehouerou/oiko/security/advisories/new); for the
others, the "Report a vulnerability" button of their Security tab. There is no email address, and
a vulnerability never goes in a public issue or discussion.

A report comes with a reproduction against a real release (`oiko -version`, the steps, what an
attacker gains), checked by a human. Using AI to find a bug is fine; an unverified AI-generated
report is closed without assessment.

## Supported versions

The latest release only: nothing is backported
([ADR 0019](docs/adr/0019-release-and-compatibility-policy.md)). A fix ships as a patch release.
A fix that can't land without breaking something ships as the next minor, with the break at the
top of its release notes; during v0 a minor may break, and no compatibility layer is added.

## What to expect

One maintainer handles reports in spare time, and promises:

- an acknowledgement within 7 days;
- an assessment within 14 days: accepted or declined, with a severity;
- then a status update at least every two weeks until the fix ships. The fix itself has no
  deadline.

Disclosure is coordinated: the report stays private until the advisory is published, at most
90 days after the report unless both sides agree otherwise.

## Disclosure

- A GitHub Security Advisory is published with the fix release, and the release notes link it.
- A CVE is requested through GitHub for every accepted vulnerability, so the Go vulnerability
  database imports it and `govulncheck` warns about it.
- The reporter is credited unless they decline.
- There is no bounty.

## Trust model

Trusted:

- The host: whoever reads Oiko's log or its data directory already controls it
  ([ADR 0026](docs/adr/0026-setup-link-in-the-log-recovery-through-the-host.md)).
- Admins doing what their Access level allows.
- Compiled-in types of Bridge: they run in Oiko's process with its full trust
  ([ADR 0017](docs/adr/0017-bridge-types-compiled-in-through-a-public-contract.md)).

Not trusted beyond their Access level: Members, Guests, Kiosks and Programs. Not trusted at all:
the network, the LAN included
([ADR 0027](docs/adr/0027-the-network-never-authenticates.md)).

## In scope

- Sign-in, Sessions, Tokens, Kiosks and Access levels.
- The HTTP/WebSocket API and the web client.
- A Code Step escaping Starlark.
- Memory beyond what
  [ADR 0034](docs/adr/0034-no-rate-limits-the-exposed-surface-bounds-memory-and-the-audit-log.md)
  bounds.
- The built-in types of Bridge (zigbee2mqtt, HomeKit) and the first-party ones (oiko-netatmo,
  oiko-arlo and its go-arlo client).
- Release artifacts.
- The catalogue's jobs and its `index.json`.

## Out of scope

- Anything that needs the host
  ([ADR 0026](docs/adr/0026-setup-link-in-the-log-recovery-through-the-host.md)).
- An Admin doing what their Access level allows.
- Third-party types of Bridge: report to their authors
  ([ADR 0017](docs/adr/0017-bridge-types-compiled-in-through-a-public-contract.md)).
- The devices and vendor clouds themselves.
- A CVE in a dependency with no reachable impact.
- Plain request flooding: Oiko has no rate limits
  ([ADR 0034](docs/adr/0034-no-rate-limits-the-exposed-surface-bounds-memory-and-the-audit-log.md)).

## The catalogue

The [catalogue](https://llehouerou.github.io/oiko-catalogue/) lists released types of Bridge
without reviewing them
([ADR 0020](docs/adr/0020-a-catalogue-of-types-of-bridge-indexed-from-a-manifest.md)): a listing
is not an endorsement. Report a malicious type privately on
[oiko-catalogue](https://github.com/llehouerou/oiko-catalogue/security/advisories/new); it is
delisted, with no advisory. An advisory is published only for a flaw in the catalogue's own jobs.
