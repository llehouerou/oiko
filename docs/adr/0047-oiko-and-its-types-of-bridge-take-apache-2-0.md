# Oiko and its types of Bridge take Apache-2.0, contributions under the license alone

A type of Bridge is compiled into the same binary as Oiko, so Oiko's license decides what a build may contain. oiko, oiko-catalogue, oiko-netatmo and oiko-arlo take Apache-2.0: any type of Bridge, open or not, can be compiled in, and a build only keeps the notices. The license also brings an explicit patent grant, and its §5 brings contributions in under the same terms. Contributions need no DCO sign-off and no CLA: contributors keep their copyright and license it under the repository's license. Relicensing would then need every contributor's consent, which this decision accepts. The catalogue lists only types under an open-source license: oiko-build compiles a type from its source, and a listing promises users they may build and patch what they run.

## Considered Options

- **MIT**: the same freedom for Bridges, but no patent grant and nothing said about contributions. Traefik's choice.
- **MPL-2.0**: would keep forks of Oiko's own files open while leaving Bridges free, at the cost of a source obligation on every distributed build that changes a file. None of the comparable projects (Caddy, Home Assistant, Homebridge, Traefik) takes copyleft.
- **GPL-3.0 or AGPL-3.0**: every distributed build, Bridges included, would have to be compatible and ship its source, and AGPL adds the source to network users, which is Oiko's normal mode. This rules out proprietary types of Bridge.
- **DCO sign-off**: certifies the right to submit, at the cost of a CI check and friction for first-timers. Relicensing would still need consent.
- **CLA**: would allow relicensing or dual licensing later, at the cost of a signing service and lost contributors.
- **A catalogue listing any license, or only Apache-2.0**: the first lets a listed type forbid building or patching it; the second turns away authors who prefer MIT, MPL or GPL for no gain.
- **A license field in the Manifest**: nothing would prove it matches the repository's LICENSE file.

## Consequences

- Each repository holds `LICENSE` and a root `NOTICE` naming "The <repository> Authors" (for example "The Oiko Authors") as copyright holder, never a person (AGENTS.md). Source files carry no license header.
- The catalogue reads the SPDX id GitHub detects on a type's repository (types are found through the `oiko-bridge` topic), lists the type only if that id is OSI-approved, and shows the license on its site. A type without a recognisable LICENSE file is not listed. The Manifest is unchanged (ADR 0020).
- The guide for authors of a type of Bridge recommends Apache-2.0 and requires nothing beyond the catalogue's rule. Unlisted types may take any license.
- Each release carries the notices of the third-party code in it (Go modules and the web client's bundle, including the EPL-2.0 MQTT client and the OFL-1.1 font), generated at release. CI fails when a new dependency's license is outside an allowlist of permissive licenses plus MPL-2.0, EPL-2.0 and, for fonts, OFL-1.1.
- go-arlo, a standalone Arlo client library, stays MIT, which is compatible with Apache-2.0. It rewrites in its own words the few items taken from pyaarlo's text and credits pyaarlo as a source it cross-checked the protocol against, so no Oiko build carries an LGPL obligation.
