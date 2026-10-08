# Licensing options for Oiko and third-party types of Bridge

Ticket: https://github.com/llehouerou/oiko/issues/82

This is not legal advice. It collects facts from license texts and primary sources, and flags where interpretation or inference was needed. Oiko has no LICENSE file today; this document supports choosing one.

## 0. Technical premise (from ADR 0017 and the ticket)

A type of Bridge is a Go package in a third-party repository that imports Oiko's `bridge` package and is **compiled into the Oiko executable** by `oiko-build`. This is static linking at build time, producing one binary — not a separate process talking over a wire protocol. That technical fact is what makes GPL-family "linking creates one combined work" reasoning apply, and is why network-service clauses (AGPL) matter for anyone who runs the resulting binary as a service. Oiko's web client (React app) is a separate artifact (browser-delivered JS) and raises the same combination question for its own dependencies, independently of the Go binary.

## 1. What each license requires

### MIT

- **Text/condition:** permission to use/copy/modify/merge/publish/distribute/sublicense/sell, with the sole condition that the copyright and permission notice is included "in all copies or substantial portions of the Software." No copyleft, no patent clause, no source-disclosure duty. **Source:** [MIT License text, SPDX](https://spdx.org/licenses/MIT)
- **Third-party Bridge type:** free to license its own code MIT; imposes nothing on Oiko or on other Bridges. No obligation flows from an MIT Bridge being linked into Oiko.
- **Distributing an Oiko build:** must keep/reproduce the MIT notice(s) of any MIT-licensed code included (Oiko's own dependencies, or an MIT Bridge). No obligation to disclose Oiko's own source.
- **Running as a network service:** no obligation at all; MIT has no network clause.

### Apache License 2.0

- **Text/condition:** copyright license (§2) and an explicit **patent license** (§3) limited to patent claims "necessarily infringed" by the contribution alone or combined with the work to which it was submitted; **patent retaliation**: if you sue over patents on the work, your patent license under Apache-2.0 for that work terminates (§3). Redistribution in source or object form must retain copyright/patent/trademark notices and a copy of the license, and must note any modifications; a `NOTICE` file's attribution content must be preserved if present (§4). **Source:** [Apache License 2.0 full text](https://www.apache.org/licenses/LICENSE-2.0.html)
- **Third-party Bridge type:** no copyleft; an Apache-2.0 Bridge can be linked into a differently-licensed Oiko. It grants Oiko (and everyone downstream) its authors' patent rights on their contribution; if the Bridge author later sues Oiko or another contributor over a patent on code within the combined work, that author's own Apache-2.0 patent license is revoked for that work (retaliation clause), a protection MIT does not offer.
- **Distributing an Oiko build:** must preserve notices and the NOTICE file content for every Apache-2.0-covered component; no source-disclosure duty for Oiko's own code.
- **Running as a network service:** no obligation; no network clause in Apache-2.0.

### MPL-2.0 (Mozilla Public License 2.0)

- **Text/condition:** **file-level, "weak" copyleft.** Only the files that are "Covered Software" (the original files and their "Modifications," MPL §1.10) must stay under MPL and have their source made available to anyone the executable is distributed to; new files you add that don't contain MPL code are not required to be MPL, and can be combined with Covered Software in a "Larger Work" under terms of your choice, provided the Covered Software's own obligations are met. **Source:** [MPL 2.0 full text](https://www.mozilla.org/en-US/MPL/2.0/), [MPL 2.0 FAQ](https://www.mozilla.org/en-US/MPL/2.0/FAQ/)
- MPL 2.0 was explicitly designed to be compatible with static linking, unlike LGPL, which several Go-community write-ups give as the reason to prefer MPL-2.0 over LGPL for Go libraries meant to be linked into other binaries. **Source (secondary, Go-community analysis, not Mozilla):** ["Don't use the LGPL for Go code"](https://www.makeworld.space/2021/01/lgpl_go.html)
- **Third-party Bridge type:** an MPL-2.0 Bridge's own files remain MPL and their source must be made available on distribution of the Oiko binary that contains them; this does not spread to Oiko's other code or to other Bridges, which are a "Larger Work" relative to that Bridge's files.
- **Distributing an Oiko build:** for each MPL-2.0 component embedded, must make that component's (and its modifications') source available under MPL-2.0 to recipients of the binary; Oiko's own code and unrelated Bridges stay under whatever license they already have.
- **Running as a network service:** no network clause; MPL-2.0's source obligation triggers on distributing the executable/Covered Software, not on running it as a service for others, so operating an Oiko build privately (SaaS-style) does not by itself trigger MPL disclosure. **Source:** [MPL 2.0 §3, "Distribution of Covered Software"](https://www.mozilla.org/en-US/MPL/2.0/)

### GPL-3.0

- **Text/condition:** "strong" copyleft. §5 requires that when you convey a work based on the Program, the whole must be licensed under GPLv3 to everyone who receives a copy. The FSF's own reading (stated in the GPL FAQ, a secondary/explanatory source, not the license text itself) is that static or dynamic linking of a GPL-covered library into a program creates a single combined work, so distributing that combination requires the whole thing to be under the GPL. **Source (license text, "conveying" duties, §5-§6):** [GPL-3.0 full text, SPDX mirror](https://spdx.org/licenses/GPL-3.0-only.html); **Source (FSF interpretation of linking):** [GNU GPL FAQ](https://www.gnu.org/licenses/gpl-faq.en.html)
- GPLv3 §10 (patent clause) and §11 give an implicit patent license from contributors/conveyors and a patent-retaliation-like effect; no explicit network-use clause (that is the AGPL's addition).
- **Third-party Bridge type:** because `oiko-build` statically compiles the Bridge's Go package into the Oiko executable (one binary, one combined work under the FSF's reading), shipping that binary would, on the FSF's interpretation, require the **whole executable** to be distributed under GPL-3.0-compatible terms and with corresponding source — this is the crux of why GPL(-family) Bridges are a much bigger deal for Oiko than MIT/Apache/MPL ones. This conclusion follows FSF guidance, not a court ruling; it is the dominant practitioner view but is not universally free of dispute for languages/build models not anticipated by the original GPL drafting discussions. **Source (secondary analysis specific to Go):** [Stack Exchange discussion on Go and GPL linking](https://softwareengineering.stackexchange.com/questions/167773/how-does-the-gpl-static-vs-dynamic-linking-rule-apply-to-interpreted-languages)
- **Distributing an Oiko build:** if any linked-in component is GPL-3.0, distributing the Oiko binary would require conveying the Corresponding Source for the whole combined work under GPL-3.0 (§6), and the combination as a whole could not then be distributed under a more restrictive or purely proprietary license.
- **Running as a network service:** GPL-3.0 has **no** clause reaching users who merely interact with the program over a network without receiving a copy of it (this is precisely the gap AGPL closes) — so running a GPL-3.0-containing Oiko build as a hosted/network service, without distributing the binary itself, does not by itself trigger GPL-3.0's source-disclosure duty. **Source:** [FSF, "The fundamentals of the AGPLv3"](https://www.fsf.org/bulletin/2021/fall/the-fundamentals-of-the-agplv3)

### AGPL-3.0

- **Text/condition:** GPL-3.0 plus one added paragraph, §13 ("Remote Network Interaction; Use with the GNU General Public License"): "if you modify the Program, your modified version must prominently offer all users interacting with it remotely through a computer network... an opportunity to receive the Corresponding Source of your version by providing access to the Corresponding Source from a network server at no charge." **Source (verbatim license text):** [AGPL-3.0 full text, SPDX mirror](https://spdx.org/licenses/AGPL-3.0-only.html)
- §13 also contains a "bridge" paragraph letting GPLv3-only and AGPLv3 code be combined into one work, which otherwise GPLv3's "must be GPLv3" and AGPLv3's "must be AGPLv3" would make impossible. **Source:** [FSF rationale for AGPLv3 §13](https://gplv3.fsf.org/agplv3-dd2-rationale.html/)
- **Third-party Bridge type:** same linking analysis as GPL-3.0 (one combined binary), plus: if that combined Oiko binary is ever run as a network service (which is Oiko's own normal mode of operation as a home-automation platform), AGPL's §13 duty to offer source to every remote user over the network is triggered — this is the clause most directly relevant to Oiko's deployment model, more than to a build-then-ship distribution model.
- **Distributing an Oiko build:** same as GPL-3.0 (whole combined work must convey corresponding source on distribution).
- **Running as a network service:** this is the clause AGPL adds specifically for this case — every self-hoster running an AGPL-containing Oiko build as a network-facing service would need to offer connected users a way to get the corresponding source of the exact running version, even though they never "distributed" the binary to those users. **Source:** [FSF, "The fundamentals of the AGPLv3"](https://www.fsf.org/bulletin/2021/fall/the-fundamentals-of-the-agplv3)

### Other candidate: BSD-3-Clause (seen among Oiko's actual dependencies, see §2)

- Functionally close to MIT: permissive, attribution-notice condition, plus a non-endorsement clause barring use of contributors' names to promote derived products without permission. No copyleft, no patent clause, no network clause. **Source:** license text as shown by `pkg.go.dev` for `golang.org/x/mod`/`golang.org/x/sync` (see §2), consistent with the canonical [OSI BSD-3-Clause text](https://opensource.org/license/bsd-3-clause).

### Other candidate worth naming: LGPL-3.0

Not proposed by the ticket but directly relevant because `pyaarlo` (the project `go-arlo` is ported from) is LGPL-3.0-or-later (see §3). LGPL's "Library"/"Application"/"Combined Work" model (§0, §4) lets a non-LGPL "Application" link an LGPL "Library" and convey the Combined Work under terms of the Application's choosing, provided (a) notice that the Library is used and covered by LGPL is given, (b) the Combined Work is accompanied by the GPL and LGPL texts, and (c) either the Minimal Corresponding Source of the Library (with means to relink a modified Library) is provided, or a "suitable shared library mechanism" for linking is used. **Source (verbatim license text):** [LGPL-3.0 full text, FSF via raw GitHub mirror of pyaarlo's own LICENSE file](https://raw.githubusercontent.com/twrecked/pyaarlo/master/LICENSE). Note: Go has no standard dynamic-linking "shared library mechanism" of the kind §4(d)(1) contemplates (Go plugins require cgo and are rarely used this way), so satisfying LGPL via option (d)(1) is impractical for a statically-linked Go binary; option (d)(0) (ship Minimal Corresponding Source + relinkable Application code) would be the realistic path if Oiko ever statically linked true LGPL Go code. This paragraph is researcher inference about Go's linking model, not a claim from the LGPL text itself.

## 2. Static linking vs. running as a network service — the mechanism behind all of the above

- Go's build model compiles a `main` package and all its imports into one self-contained executable; there is no dynamic-linking step for ordinary Go code. **Source:** [Go command documentation](https://go.dev/cmd/link/)
- The FSF's own FAQ states its view that both static and dynamic linking of GPL-covered code create a "combined work" whose distribution is governed by the GPL. **Source:** [GNU GPL FAQ, "linking"](https://www.gnu.org/licenses/gpl-faq.en.html)
- A separate-process / network-boundary design ("mere aggregation," in GPL vocabulary) is treated differently from linking, per the same FAQ — which is one reason some projects isolate GPL-licensed functionality behind a subprocess or socket rather than an in-process import, though that is a design mitigation outside the scope of this research, not something confirmed specific to Oiko.
- Practical upshot for Oiko's `oiko-build` model: because a type of Bridge is imported and compiled in (not run as a subprocess), the "combined work" analysis above applies at full force; it would not apply the same way if Bridges instead communicated with Oiko as separate OS processes or over a network boundary.

## 3. What Oiko's own dependencies require

### Go module dependencies (from `go.mod`, direct requires only; checked via `pkg.go.dev`'s license tab, which displays the LICENSE file at the resolved version)

| Module | License | Source |
|---|---|---|
| github.com/AlexxIT/go2rtc | MIT | https://pkg.go.dev/github.com/AlexxIT/go2rtc?tab=licenses |
| github.com/eclipse/paho.golang | EPL-2.0 (dual-licensable to a GPLv2-family "Secondary License" at the Eclipse Foundation's option, per EPL-2.0 §3.2/Exhibit A — not invoked by default) | https://pkg.go.dev/github.com/eclipse/paho.golang?tab=licenses |
| github.com/go-webauthn/webauthn | BSD-3-Clause | https://pkg.go.dev/github.com/go-webauthn/webauthn?tab=licenses |
| github.com/nathan-osman/go-sunrise | MIT | https://pkg.go.dev/github.com/nathan-osman/go-sunrise?tab=licenses |
| github.com/pion/rtp | MIT | https://pkg.go.dev/github.com/pion/rtp?tab=licenses |
| go.starlark.net | BSD-3-Clause | https://pkg.go.dev/go.starlark.net?tab=licenses |
| golang.org/x/mod | BSD-3-Clause | https://pkg.go.dev/golang.org/x/mod?tab=licenses |
| golang.org/x/sync | BSD-3-Clause | https://pkg.go.dev/golang.org/x/sync?tab=licenses |
| modernc.org/sqlite | BSD-3-Clause | https://pkg.go.dev/modernc.org/sqlite?tab=licenses |

All nine direct Go dependencies are permissive (MIT/BSD-3-Clause) with one exception, **EPL-2.0** for `eclipse/paho.golang` (the MQTT client). EPL-2.0 is itself a weak/file-level copyleft comparable in spirit to MPL — it requires that EPL-covered Program source stay available under EPL-2.0 (or a license satisfying EPL §3.1(b)'s conditions) on distribution, but (per EPL-2.0 §3.1(b)-(iv) and the Eclipse FAQ's own framing) does not require unrelated combined code to become EPL-licensed; it is traditionally treated as a "library" copyleft safe to statically embed in differently-licensed programs, similar to the MPL case above. **Finding, not independently verified against an EPL-specific linking FAQ in this pass** — flagged as a gap below. None of Oiko's checked Go dependencies are GPL/AGPL, so today's dependency set does not force a GPL-family license on Oiko itself.

The transitive/indirect dependencies (go-humanize, cbor, mapstructure, jwt, go-tpm, uuid, gorilla/websocket, go-isatty, miekg/dns, msgp, etc.) were not individually re-verified in this pass; they are typical Go-ecosystem packages overwhelmingly published under MIT/BSD/Apache-2.0, but this is an inference from ecosystem norms, not a per-package check, and is listed under Missing evidence.

### Web client dependencies (from `web/package.json`)

| Package | License | Source |
|---|---|---|
| @dnd-kit/core | MIT | https://registry.npmjs.org/%40dnd-kit%2Fcore ; https://github.com/clauderic/dnd-kit/blob/main/LICENSE |
| @fontsource-variable/jost (packages the Jost font) | SIL Open Font License 1.1 (OFL-1.1) | https://fontsource.org/fonts/jost/about |
| @mdi/js | Apache-2.0 (icon glyph data; Pictogrammers' own "Free License" blurb additionally describes the overall MDI icon collection as "free, open source, and GPL friendly," but the npm/package license field and LICENSE file for `@mdi/js` itself state Apache-2.0) | https://github.com/Templarian/MaterialDesign-JS/blob/v7.4.47/LICENSE |
| @xyflow/react | MIT | https://www.npmjs.com/package/%40xyflow/react |
| qrcode-generator | MIT | https://registry.npmjs.org/qrcode-generator |
| react / react-dom | MIT | https://www.npmjs.com/package/react |
| uplot | MIT | https://github.com/leeoniya/uPlot/blob/master/LICENSE |

All checked web-client dependencies are permissive (MIT, Apache-2.0, OFL-1.1 for the font). None is copyleft in a way that would constrain Oiko's own license. Dev-only tooling (vite, vitest, typescript, tailwindcss, prettier, oxlint, playwright-core, babel) was not re-checked individually since dev tooling is not distributed as part of Oiko and its license does not propagate into the shipped artifact; this is a reasoned simplification, not a verified claim about each one.

**Conclusion on Oiko's own dependencies:** nothing in the checked set forces Oiko to adopt a copyleft license; everything found is compatible with MIT, Apache-2.0, MPL-2.0, GPL-3.0, or AGPL-3.0 being chosen for Oiko's own code. The one weak-copyleft dependency (EPL-2.0, for the MQTT client) behaves like MPL — it keeps its own files' source open on distribution but does not require Oiko's other code to follow suit — this is the researcher's interpretation of EPL-2.0 by analogy to the explicit MPL file-level-copyleft model, not a textually identical guarantee, and is flagged under Missing evidence.

### `go-arlo`, ported from `pyaarlo`

- `pyaarlo` (`twrecked/pyaarlo` on GitHub/PyPI) is licensed **LGPL-3.0-or-later**. **Source (verbatim LICENSE file, confirmed by direct fetch):** https://raw.githubusercontent.com/twrecked/pyaarlo/master/LICENSE ; **Source (PyPI metadata corroboration):** https://pypi.org/project/pyaarlo/
- `llehouerou/go-arlo`'s own README states: "The protocol is ported from [pyaarlo](https://github.com/twrecked/pyaarlo) 0.8.0.23," and its License section states plainly **"MIT."** **Source:** https://github.com/llehouerou/go-arlo (README.md and LICENSE, fetched directly)
- **What a "port" implies, and the tension this creates:** "Porting" here means re-implementing pyaarlo's understanding of Arlo's cloud-API *protocol* (endpoints, message shapes, auth flow) in a new language and codebase, not copying pyaarlo's Python source text. Copyright protects the expression (the specific code), not the underlying facts, ideas, or interoperability information such as a wire protocol — this is a general copyright-law principle, not something either project's license page states, and is the researcher's inference, not a verified legal conclusion specific to these two codebases. If `go-arlo`'s code is an independent expression of the same protocol knowledge, LGPL's "Library"/linking obligations on pyaarlo's own source would not reach it, which is consistent with `go-arlo` choosing a clean MIT license.
- **However**, if any `go-arlo` source was in fact transcribed or closely paraphrased from `pyaarlo`'s source (rather than independently written against observed/documented protocol behavior), the resulting code could be a derivative work of pyaarlo's LGPL-3.0-or-later source, and `go-arlo`'s unilateral MIT label would not by itself override LGPL obligations that may attach to copied material. **This was not independently checked by diffing code** — it is exactly the kind of fact this research run cannot verify from the two projects' public statements alone, and is listed under Missing evidence. It matters because `go-arlo` is then statically linked into Oiko the same way any other Bridge dependency would be.

## 4. Is a contributor agreement (DCO/CLA) customary with each license?

There is no license-text requirement anywhere for a DCO or CLA — these are **project governance choices**, independent of the chosen outbound license, though some practices correlate strongly with specific license families. **Source (neutral explainer distinguishing outbound license, DCO, and CLA):** [Linux Foundation, "Contributions to Projects: DCO and CLAs"](https://bestpractices.linuxfoundation.org/ip/contribution-mechanisms.html)

- **DCO (Developer Certificate of Origin):** a per-commit sign-off certifying the contributor had the right to submit the code under the project's license; no separate document to sign, no rights transfer. Used across virtually all Linux-Foundation-hosted projects and much of the permissive- and GPL-world kernel/infrastructure ecosystem. **Source:** [Linux Foundation, "Developer Certificate of Origin (DCO)"](https://bestpractices.linuxfoundation.org/ip/contribution-mechanisms-dco.html)
- **CLA (Contributor License Agreement):** a separate legal agreement granting the project (or a foundation) an explicit license (and sometimes assignment) over the contribution, often including a patent grant; heavier process, commonly used by foundation-governed projects, notably all ASF projects. **Source:** [Linux Foundation, "Contributor License Agreements (CLAs)"](https://bestpractices.linuxfoundation.org/ip/contribution-mechanisms-cla.html)
- **Apache-2.0 projects:** the ASF requires every contributor to sign an Individual (or Corporate) CLA, explicitly to "clarify the intellectual property license granted with Contributions." This is an ASF governance policy tied to the foundation, not a textual requirement of the Apache-2.0 license itself (non-ASF projects can and do use Apache-2.0 without any CLA). **Source:** [ASF Individual CLA](https://www.apache.org/licenses/icla.pdf), [ASF Contributor Agreements overview](https://www.apache.org/licenses/contributor-agreements.html)
- **GPL/AGPL (FSF-style) projects:** the FSF asks for copyright assignment (a stronger instrument than a CLA) on "GNU packages" it holds copyright for, explicitly to simplify enforcing the GPL; this is specific to FSF-copyrighted GNU software, not a general requirement of choosing GPL/AGPL. The FSF explicitly does **not** require a project to become GNU or to assign copyright to it merely by being GPL-licensed. **Source:** [FSF, "Why the FSF Gets Copyright Assignments from Contributors"](https://www.gnu.org/licenses/why-assign.en.html), [GNU, "Categories of Free and Nonfree Software"](https://www.gnu.org/philosophy/categories.en.html)
- **MIT/MPL-2.0 projects:** no license-driven convention either way; practice varies by project governance rather than by license family.

### What Oiko's reference projects actually do

| Project | License | Contributor agreement | Evidence |
|---|---|---|---|
| Caddy | Apache-2.0 | **CLA** (individual/corporate, modeled on common CLA templates) required before merging | [Caddy CONTRIBUTING.md](https://github.com/caddyserver/caddy/blob/master/.github/CONTRIBUTING.md) |
| Home Assistant | Apache-2.0 | **CLA** required; HA's CLA requires the contribution itself to be licensed Apache-2.0 (adopted after community feedback, replacing an earlier, broader CLA) | [HA Contributor License Agreement](https://www.home-assistant.io/developers/cla/), [HA governance update](https://community.home-assistant.io/t/home-assistant-governance-updated/42654) |
| Homebridge | Apache-2.0 | **No CLA or DCO found** in Homebridge's CONTRIBUTING.md (checked directly) — contribution guidelines cover code style, tests, and release process only | [homebridge/.github CONTRIBUTING.md](https://github.com/homebridge/.github/blob/latest/CONTRIBUTING.md) |
| Traefik | MIT | No CLA found; contributing guide references a Contributor Code of Conduct, not a CLA or DCO | [Traefik README/contributing references](https://github.com/traefik/traefik) |

This cross-check shows the correlation is real but not absolute: two of three Apache-2.0 projects here use a CLA (consistent with ASF-style practice, though neither HA nor Caddy is an ASF project), but Homebridge (also Apache-2.0) uses neither CLA nor DCO, and the MIT project (Traefik) uses neither — so "Apache-2.0 implies CLA" is a tendency among foundation-adjacent or more legally cautious projects, not a rule tied to the license text.

## 5. What Caddy, Home Assistant, Homebridge and Traefik pick, and why they say so

- **Caddy — Apache License 2.0.** Caddy's repository is Apache-2.0-licensed. **Source:** [caddyserver/caddy repository](https://github.com/caddyserver/caddy). When Caddy's maintainer proposed permanently open-sourcing all Caddy code (including previously enterprise-gated features) rather than dual-licensing, the stated direction was to monetize through services (support, consulting, training) instead of proprietary code, while keeping the project itself fully and permissively open — the actual, publicly discussed rationale is about adoption and monetization strategy, not a dedicated "why Apache-2.0 and not MIT" essay; this characterization is the researcher's synthesis of that discussion, not a direct quote of "we chose Apache-2.0 because...". **Source:** [Caddy issue #2786, "Permanently change all proprietary licensing to open source"](https://github.com/caddyserver/caddy/issues/2786)
- **Home Assistant — Apache License 2.0.** HA states its license directly: "The Home Assistant source code is released under the following license [Apache 2.0]." **Source:** [Home Assistant, "The Apache 2.0 License"](https://www.home-assistant.io/developers/license/). HA's community governance post explains the *process* reasoning for pairing Apache-2.0 with a CLA: after community feedback, the CLA was simplified to just require contributions be licensed Apache-2.0 (rather than a heavier rights grant), and "Starting with release 0.37, Home Assistant will re-license the current code under the Apache 2.0 license. This is the license that will be used moving forward for all projects..." — the stated reason is community pushback on the earlier, more demanding CLA/licensing terms, resolved by standardizing on Apache-2.0 plus a lighter CLA. **Source:** [Home Assistant Governance update](https://community.home-assistant.io/t/home-assistant-governance-updated/42654)
- **Homebridge — Apache License 2.0.** Confirmed via the repository's license declaration (`github.com/homebridge/homebridge`, Apache-2.0). No dedicated maintainer statement explaining *why* Apache-2.0 specifically (as opposed to MIT) was found in this research pass. **Source:** [homebridge/homebridge repository](https://github.com/homebridge/homebridge). Flagged under Missing evidence.
- **Traefik — MIT License.** Traefik's LICENSE.md is the plain MIT text, copyright Containous SAS (2016-2020) and Traefik Labs (2020-2025). **Source (verbatim license file):** [traefik/traefik LICENSE.md](https://github.com/traefik/traefik/blob/master/LICENSE.md). Traefik Labs separately runs a commercial End User License Agreement for its paid enterprise features, explicitly defining "Community Software" as "the open source elements of the Software that are made available by Traefik Labs under the MIT software license" — i.e., Traefik's open core is kept maximally permissive (MIT) specifically to support a separate proprietary/enterprise offering layered on top; this is stated directly in Traefik Labs' own EULA definitions, not inferred. **Source:** [Traefik Labs End User License Agreement](https://traefik.io/legal/end-user-license-agreement)

Pattern across all four: three of four (Caddy, Home Assistant, Homebridge) picked Apache-2.0, one (Traefik) picked MIT; none picked a copyleft license, and none picked AGPL, despite all four being commonly run as always-on network services — the same category of deployment Oiko's own Bridges run in. This is a direct observation, not an inference: all four reference projects explicitly avoid network-triggered copyleft obligations for their own core code, even though HA and Caddy both support commercial offerings built on top of (or alongside) the open-core project.

## Contradictions

- None found between primary sources on license text content.
- A tension, not a contradiction, exists between `pyaarlo`'s LGPL-3.0-or-later license and `go-arlo`'s self-declared MIT license for a codebase explicitly described as "ported from pyaarlo" — see §3. This is flagged as unresolved, not asserted as a violation, since the research did not diff the two codebases' actual source.

## Missing evidence

- Whether `go-arlo`'s code is an independent reimplementation against observed protocol behavior, or contains text transcribed/closely paraphrased from `pyaarlo`'s Python source, was not checked by diffing the two codebases. This directly affects whether `go-arlo`'s MIT self-label is legally sound, and therefore what, if anything, an Oiko build that links `go-arlo` owes to `pyaarlo`'s LGPL-3.0-or-later terms.
- Licenses of Oiko's *indirect* (transitive) Go dependencies (e.g. `dustin/go-humanize`, `fxamacker/cbor`, `golang-jwt/jwt`, `google/go-tpm`, `gorilla/websocket`, `miekg/dns`, `tinylib/msgp`, etc.) were not individually re-verified in this pass; ecosystem norms suggest permissive licenses are overwhelmingly likely, but this is an inference, not a per-package finding.
- Licenses of `web/package.json`'s devDependencies (vite, vitest, typescript, tailwindcss, prettier, oxlint, playwright-core, babel tooling) were not individually re-checked; they are not shipped in the built web client, so they matter less, but the claim that none of them is copyleft was not verified here.
- EPL-2.0's exact linking/combination boundary (analogous to MPL's "Larger Work" vs "Covered Software" distinction) was described by analogy to MPL's file-level copyleft model, based on reading EPL-2.0 §3.1(b) itself, but a dedicated Eclipse Foundation FAQ confirming "static linking into a non-EPL binary does not spread EPL to the whole binary" was not located/fetched in this pass.
- No explicit maintainer statement explaining *why* Homebridge chose Apache-2.0 (as distinct from simply observing that it did) was found.
- `gnu.org`'s pages (gpl-3.0.en.html, agpl-3.0.en.html, gpl-faq) could not be fetched directly in this environment (connection failures); the GPLv3/AGPLv3 verbatim text used here was retrieved from SPDX's and OSI's mirrors of the same text instead, and FSF explanatory claims (FAQ content, AGPLv3 rationale, "why assign" page) were retrieved via web-search snippets of FSF-hosted pages rather than a direct raw fetch of gnu.org. Treated as reliable since SPDX/OSI mirrors are verbatim reproductions used for exactly this purpose, but noted as a sourcing-method limitation.

## Sources

- Kept: [MIT License, SPDX](https://spdx.org/licenses/MIT) — canonical MIT text
- Kept: [Apache License 2.0, apache.org](https://www.apache.org/licenses/LICENSE-2.0.html) — canonical Apache-2.0 text, fetched directly
- Kept: [Mozilla Public License 2.0, mozilla.org](https://www.mozilla.org/en-US/MPL/2.0/) and [MPL 2.0 FAQ](https://www.mozilla.org/en-US/MPL/2.0/FAQ/) — canonical text and official explainer of file-level copyleft
- Kept: [GPL-3.0-only, SPDX mirror](https://spdx.org/licenses/GPL-3.0-only.html) and [AGPL-3.0-only, SPDX mirror](https://spdx.org/licenses/AGPL-3.0-only.html) — verbatim text, fetched directly, used for §5/§6/§13 quotes, after direct gnu.org fetches failed
- Kept: [FSF, "The fundamentals of the AGPLv3"](https://www.fsf.org/bulletin/2021/fall/the-fundamentals-of-the-agplv3) and [FSF rationale for AGPLv3 §13](https://gplv3.fsf.org/agplv3-dd2-rationale.html/) — official explanation of the network-interaction clause's purpose
- Kept: [GNU GPL FAQ](https://www.gnu.org/licenses/gpl-faq.en.html) — FSF's own linking interpretation (via search snippet; direct fetch failed)
- Kept: [pyaarlo LICENSE, raw GitHub](https://raw.githubusercontent.com/twrecked/pyaarlo/master/LICENSE) — verbatim LGPL-3.0 text confirming pyaarlo's license
- Kept: [llehouerou/go-arlo repository](https://github.com/llehouerou/go-arlo) — README and LICENSE confirming "ported from pyaarlo" and MIT self-label
- Kept: `pkg.go.dev` license pages for each direct Go dependency (listed individually in §3) — authoritative because pkg.go.dev displays the actual LICENSE file at the resolved module version
- Kept: npm registry / GitHub LICENSE files for each direct `package.json` dependency (listed individually in §3)
- Kept: [Linux Foundation DCO/CLA explainer pages](https://bestpractices.linuxfoundation.org/ip/contribution-mechanisms.html) — neutral, widely cited explanation distinguishing outbound license from contribution-process choices
- Kept: [ASF Individual CLA](https://www.apache.org/licenses/icla.pdf) and [ASF Contributor Agreements](https://www.apache.org/licenses/contributor-agreements.html) — primary ASF CLA documents
- Kept: [FSF "Why the FSF Gets Copyright Assignments from Contributors"](https://www.gnu.org/licenses/why-assign.en.html) — primary FSF rationale (via search snippet)
- Kept: [Caddy repository](https://github.com/caddyserver/caddy), [Caddy CONTRIBUTING.md](https://github.com/caddyserver/caddy/blob/master/.github/CONTRIBUTING.md), [Caddy issue #2786](https://github.com/caddyserver/caddy/issues/2786)
- Kept: [Home Assistant "The Apache 2.0 License"](https://www.home-assistant.io/developers/license/), [HA CLA](https://www.home-assistant.io/developers/cla/), [HA governance update](https://community.home-assistant.io/t/home-assistant-governance-updated/42654)
- Kept: [homebridge/homebridge repository](https://github.com/homebridge/homebridge), [homebridge/.github CONTRIBUTING.md](https://github.com/homebridge/.github/blob/latest/CONTRIBUTING.md)
- Kept: [traefik/traefik LICENSE.md](https://github.com/traefik/traefik/blob/master/LICENSE.md), [Traefik Labs EULA](https://traefik.io/legal/end-user-license-agreement)
- Rejected/deprioritized: assorted Reddit/StackExchange threads on GPL+Go linking — used only as corroborating secondary color for an already FSF-sourced point, not cited as primary evidence on their own
- Rejected/deprioritized: generic AI-generated summaries returned inline by the search provider for some queries (e.g. Homebridge's "why Apache-2.0" rationale, EPL-2.0 linking boundary) — treated as leads, not citable fact, and reflected as Missing evidence instead of asserted claims

## Next steps

1. Diff `go-arlo`'s source against `pyaarlo`'s to determine whether any code was transcribed rather than independently written against observed protocol behavior — this directly affects whether linking `go-arlo` into Oiko carries any LGPL exposure.
2. If a GPL-family license is seriously considered for Oiko itself, get a second opinion (ideally from counsel, or at minimum a clear written policy) on whether `oiko-build`'s static compilation of third-party Bridge packages is intended to require every Bridge to also be GPL/AGPL-compatible — this is the single biggest practical consequence flagged in this research and is the kind of policy decision that should be an ADR.
3. Spot-check the licenses of Oiko's transitive Go dependencies and the web client's devDependencies with a tool such as `go-licenses` or `license-checker`, rather than the manual direct-dependency sampling done here, before finalizing a LICENSE choice.

```acceptance-report
{
  "criteriaSatisfied": [
    {
      "id": "criterion-1",
      "status": "satisfied",
      "evidence": "Findings document written to /tmp/wf-research/2026-10-licensing.md covering: per-license requirements (MIT, Apache-2.0, MPL-2.0, GPL-3.0, AGPL-3.0, plus BSD-3-Clause and LGPL-3.0 as additional candidates) for a linked-in Bridge, a distributor, and a network-service operator; Oiko's go.mod and web/package.json direct dependencies checked against pkg.go.dev/npm/GitHub LICENSE files; pyaarlo confirmed LGPL-3.0-or-later via its raw LICENSE file and go-arlo's MIT self-label and 'ported from' relationship documented with the derivative-work tension flagged as unresolved; DCO/CLA customs explained with primary LF/ASF/FSF sources and cross-checked against Caddy, Home Assistant, Homebridge and Traefik's actual CONTRIBUTING/CLA pages; each of the four reference projects' license and stated rationale documented with URLs. Every claim carries a source URL; inferences and unverifiable claims are labeled as such in-line and summarized in Missing evidence."
    }
  ],
  "changedFiles": [],
  "testsAddedOrUpdated": [],
  "commandsRun": [],
  "validationOutput": [],
  "residualRisks": [
    "go-arlo vs pyaarlo derivative-work status was not verified by diffing source code; if go-arlo in fact contains transcribed pyaarlo code, its MIT self-label would not override pyaarlo's LGPL-3.0-or-later terms on that material.",
    "gnu.org could not be fetched directly from this environment; GPLv3/AGPLv3 verbatim text was sourced from SPDX/OSI mirrors instead of gnu.org itself, and some FSF explanatory claims came from search-engine snippets rather than a direct page fetch.",
    "Transitive Go dependencies and JS devDependencies were not individually re-verified for license; only Oiko's direct go.mod and package.json dependencies were checked.",
    "EPL-2.0's static-linking/combination boundary was described by analogy to MPL's file-level copyleft model rather than confirmed against an Eclipse-specific linking FAQ.",
    "This document is factual research, not legal advice; a GPL-family choice for Oiko (or acceptance of a GPL/AGPL Bridge) should get a dedicated legal/policy review before being encoded in an ADR."
  ],
  "noStagedFiles": true,
  "diffSummary": "New research artifact only; no source files changed.",
  "reviewFindings": [
    "no blockers"
  ],
  "manualNotes": "All claims in the document are cited inline with a URL per claim, per the research skill's requirement. Direct fetches of gnu.org failed repeatedly in this environment (network/provider issue, not a content issue); SPDX and OSI mirrors of the GPLv3/AGPLv3 text were used instead and are noted as a sourcing-method limitation in Missing evidence rather than silently substituted."
}
```
