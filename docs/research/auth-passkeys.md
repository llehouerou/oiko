# Research: passkeys (WebAuthn) for a self-hosted Oiko behind a reverse proxy

Scope: what WebAuthn and passkeys require and make possible for Oiko (a Go stdlib `net/http` server serving a React SPA, a JSON API and SSE) when it runs behind a user-run reverse proxy that terminates TLS. This is research only. It lays out options and trade-offs and makes no decisions.

Evidence labels used below:
- **[direct]**: the cited source states it.
- **[interp]**: my reading of what a cited source says.
- **[inference]**: my own conclusion; no source states it.

Freshness: sources were read at the time of writing. The newest dated item is the go-webauthn CHANGELOG entry v0.18.2 (2026-09-19). Browser support moves quickly, so recheck version numbers before relying on them.

---

## Key findings

1. **One RP ID per credential, and it must be a domain.** A passkey only works for the RP ID it was created under. The RP ID must be the page's effective domain or a registrable-domain suffix of it. Hosts that are IP addresses, opaque or empty are rejected, so an Oiko reached by LAN IP (for example `https://192.0.2.10`) cannot use WebAuthn at all. `http://localhost` is the only non-HTTPS origin the spec allows. [direct, W3C WebAuthn L3]
2. **Several hostnames mean several RP IDs unless they share a parent domain.** `oiko.example.org` and `home.example.org` can both use RP ID `example.org`. `oiko.local` and `oiko.example.org` cannot share passkeys unless Related Origin Requests (ROR) is used. [direct/interp, spec + passkeys.dev]
3. **ROR now works in all three engines, but it does not solve the LAN case.** Support: Chrome/Edge 128+, Safari 18 / macOS 15+, and Firefox 152+ (landed in 2026; Mozilla's position is "positive"). Requirements: the RP ID must serve `https://<RP ID>/.well-known/webauthn`, clients cap the list at about 5 distinct registrable labels, and every listed origin must still be a secure-context domain origin. IP origins can never use it. [direct, passkeys.dev, Bugzilla, spec]
4. **Browsers require a valid certificate.** Since Chrome 110, Chrome refuses WebAuthn on pages with TLS certificate errors. Self-signed certificates on a `.local` name therefore break passkeys unless each device trusts the CA. Public CAs have not been allowed to issue certificates for internal names or reserved IPs since 2015. [direct, W3C list post by Chrome team, CA/B Forum]
5. **Username-less login is standard and well supported.** Passkeys are discoverable credentials by definition. The autofill UI (conditional mediation) works on Android, ChromeOS, iOS 16.1+, macOS and Windows; on Android, Firefox does not support it. Hybrid / cross-device sign-in (scan a QR code with a phone, plus Bluetooth proximity and internet on both devices) works with Android 9+ and iOS 16+ phones as the authenticator, and with clients on all major desktops. On Linux it works in Chrome and Edge only. [direct, passkeys.dev matrix, Microsoft Learn]
6. **Synced passkeys are the consumer default and give no useful attestation.** Apple, Google Password Manager (including Chrome on Windows, macOS and Linux since September 2024) and third-party managers sync passkeys. Out of the box, Windows Hello is the only first-party platform authenticator that creates device-bound passkeys and offers attestation for them. passkeys.dev recommends that most RPs leave `attestation` at `none`. [direct, passkeys.dev, Google blog]
7. **Requiring attestation would lock out most household devices.** Apple's synced passkeys return `none` attestation and an all-zero AAGUID. Several browser-extension password managers set the UV flag without actually verifying the user. [direct/interp, passkeys.dev known issues, Apple forum snippet]
8. **go-webauthn is the only Go library passkeys.dev lists. It is active but pre-1.0.** It is BSD-3-Clause and conformance-tested against FIDO tools. The 0.18.0 release (2026-08-27) brought large breaking changes; v0.18.2 followed on 2026-09-19. It requires Go ≥ 1.26 (toolchain 1.27.1). It already provides discoverable login, conditional mediation, ROR document generation, MDS-based attestation policy and backup-eligibility filtering. [direct, repository source]
9. **The ecosystem expects a fallback.** The canonical bootstrapping pattern is: offer passkey autofill on the username field, fall back to another login challenge if no passkey is used, then offer to create a passkey after sign-in. Account recovery must not depend on the lost passkey. [direct, passkeys.dev bootstrapping]

---

## 1. Relying Party ID rules

### 1.1 What the spec says
- The RP ID is "a valid domain string". A credential "can only be used for authentication with the same entity (as identified by RP ID) it was registered with". [direct] [W3C WebAuthn L3](https://www.w3.org/TR/webauthn-3/)
- In `create()` and `get()` the caller origin must not be opaque. The effective domain must be a valid domain: "Only the domain format of host is allowed here… in recognition of various issues with using direct IP address identification in concert with PKI-based security." IPv4, IPv6, opaque and empty hosts are rejected. [direct] [W3C WebAuthn L3, §5.1.3 / §5.1.4](https://www.w3.org/TR/webauthn-3/)
- `rp.id` / `rpId` must be equal to the effective domain or a registrable domain suffix of it. Example from the spec: for origin `https://login.example.com:1337`, `login.example.com` and `example.com` are valid, but `m.login.example.com` and `com` are not. The port is unrestricted. [direct] [W3C WebAuthn L3](https://www.w3.org/TR/webauthn-3/)
- Allowed schemes: the origin's scheme must be `https`, or the host must be `localhost` with scheme `http` (the spec's example is `http://localhost:8000`). [direct] [W3C WebAuthn L3](https://www.w3.org/TR/webauthn-3/)
- The server must check the origin in `clientDataJSON` against its own list of expected origins. go-webauthn requires `RPOrigins` to be configured. [direct] [go-webauthn `webauthn/types.go`](https://github.com/go-webauthn/webauthn/blob/master/webauthn/types.go)

### 1.2 How each access path behaves

| Access path | WebAuthn possible? | Notes |
|---|---|---|
| `https://oiko.example.org` (public name, valid cert via proxy) | Yes | RP ID `oiko.example.org` or `example.org`. [direct, spec] |
| `https://192.0.2.10` (LAN IP) | **No** | The spec rejects IP hosts before any RP ID or ROR processing. [direct, spec] |
| `http://192.0.2.10` | **No** | Not HTTPS, not localhost. [direct, spec] |
| `https://oiko.local` | Only with a cert the device trusts | `.local` is a valid domain string, so the spec allows it as RP ID. **[inference]** Without a public suffix list entry, the registrable domain is `oiko.local` itself. Public CAs may not issue for internal names (prohibited since 2015-11-01) ([CA/B Forum internal names](https://cabforum.org/working-groups/server/internal-names/)), and Chrome blocks WebAuthn on pages with TLS errors ([Chrome announcement, 2022-11-24](https://lists.w3.org/Archives/Public/public-webauthn/2022Nov/0135.html)). Each phone or tablet would have to trust a private CA. |
| `http://oiko.local` | **No** | Not a secure origin for WebAuthn. [direct, spec] |
| `http://localhost:PORT` | Yes | Allowed by the spec. Useful for development or on-host use only. Passkeys made here have RP ID `localhost` and do not work on the public name. [direct/inference] |
| A second public name, e.g. `oiko.example.net` while the RP ID is `example.org` | Only via ROR | Different registrable domains. Without ROR the user needs a second passkey per RP ID, and Oiko must store which RP ID each credential belongs to. [direct, passkeys.dev ROR "Existing Deployments"] |
| A split-horizon name: the same `oiko.example.org` resolved to the LAN IP indoors, with a cert valid for that name | Yes | **[inference]** To WebAuthn this is the same origin. This is the usual way to get passkeys on the LAN without a second RP ID. |

**[inference]** Oiko should not derive its RP ID or expected origin from the `Host` or `X-Forwarded-*` headers of each request. A proxy that adds "nothing but TLS" may pass client-supplied headers through, and the origin check exists precisely to resist spoofing. The RP ID and allowed origins are configuration. go-webauthn models them as static `Config.RPID` / `Config.RPOrigins`; it also supports per-ceremony origin binding (`WithLoginOrigin` / `WithRegistrationOrigin`), but only within the configured origins. [direct for the library fields; inference for the recommendation] ([types.go](https://github.com/go-webauthn/webauthn/blob/master/webauthn/types.go))

### 1.3 Related Origin Requests (ROR)
- **How it works:** if the requested RP ID is not a suffix of the caller's domain and the client supports ROR, the client fetches `https://<rpId>/.well-known/webauthn` "without credentials, without a referrer and using the https: scheme"; redirects must stay on `https:`. The client then checks the caller origin against the JSON `origins` list. [direct] [W3C WebAuthn L3 §5.11](https://www.w3.org/TR/webauthn-3/)
- **Limits:** labels are counted by registrable-domain label (`example.com` and `example.co.uk` both count as `example`). The spec requires clients to support at least 5; "there are no known clients which support more than 5". The response must be `application/json`. Origins that match the RP ID should not be listed. [direct] [passkeys.dev ROR](https://passkeys.dev/docs/advanced/related-origins/), [web.dev ROR](https://web.dev/articles/webauthn-related-origin-requests)
- **Support:** Chrome/Edge 128+ on all platforms, Safari on iOS 18+ / macOS 15+, Firefox 152+ (desktop and Android). [direct] [passkeys.dev device support, repository source](https://github.com/passkeydeveloper/passkeys.dev/blob/main/content/en/device-support/index.md). Firefox's implementation landed in Bug 2010193 (milestone 152, commits dated 2026-05-08) ([Bugzilla](https://bugzilla.mozilla.org/show_bug.cgi?id=2010193)). Mozilla's standards position was marked positive on 2026-03-11 ([mozilla/standards-positions#1052](https://github.com/mozilla/standards-positions/issues/1052)). web.dev (last updated 2024-08-22, with a January 2026 note) still says Firefox is "considering"; that is stale (see Contradictions).
- **Detection:** `PublicKeyCredential.getClientCapabilities()` reports `relatedOrigins`. [direct] [spec](https://www.w3.org/TR/webauthn-3/), [passkeys.dev ROR](https://passkeys.dev/docs/advanced/related-origins/)
- **Guidance:** passkeys.dev says "ROR is designed to be used when federation is *not* possible" and recommends OIDC-style federation first. [direct] [passkeys.dev ROR](https://passkeys.dev/docs/advanced/related-origins/)
- **Relevance to Oiko [inference]:** ROR could join two *public* hostnames under different registrable domains. It cannot help with LAN IP access, because IP origins fail before ROR runs. It also cannot help a `.local` name with a self-signed certificate, because the TLS-error block still applies on the calling page. go-webauthn can generate the document (`(*WebAuthn).RelatedOrigins()` returns an `http.Handler` for `/.well-known/webauthn`). [direct for the library] ([related_origins.go](https://github.com/go-webauthn/webauthn/blob/master/webauthn/related_origins.go))

### 1.4 Changing hostname later
**[inference]** A passkey is bound to its RP ID for good; nothing transfers it to another domain. If an install changes domain (for example from a dynamic-DNS name to an owned domain), every user must enrol new passkeys, unless the old domain stays reachable and ROR lists the new origin under the old RP ID. Choosing the broadest stable RP ID the user controls (for example `example.org` rather than `oiko.example.org`) keeps sibling subdomains open. The cost is that any other app on that domain could also request assertions for that RP ID.

---

## 2. Discoverable credentials and username-less login
- A passkey *is* a discoverable credential: the private key, credential ID and user handle are stored on the authenticator, so login needs no username first. [direct] [passkeys.dev terms](https://passkeys.dev/docs/reference/terms/)
- There are two username-less UIs:
  - a "Sign in with a passkey" button that calls `get()` with an empty `allowCredentials`;
  - the autofill UI: `mediation: "conditional"` with `autocomplete="username webauthn"` on the username field. The promise resolves only if the user picks a passkey, so the page keeps working as a normal login form. [direct] [passkeys.dev bootstrapping](https://passkeys.dev/docs/use-cases/bootstrapping/)
- Autofill (conditional get) support: Android with Chrome 108+ / Edge 122+ (**Firefox for Android: no**); ChromeOS 129+; iOS 16.1+ in all browsers; macOS Safari 16.1+, Chrome 108+, Firefox 122+, Edge 122+; Windows 11 22H2+ in Chrome 108+, Firefox 122+, Edge 122+; Ubuntu only through browser extensions. [direct] [passkeys.dev device support](https://passkeys.dev/device-support/)
- "Conditional create" (automatic passkey upgrade after a password login): Chrome 136+ on most platforms, Chrome 142+ on Android, Safari 18+ on iOS and macOS; **not** Edge or Firefox. It also needs OS and credential-manager support. [direct] [passkeys.dev device support](https://passkeys.dev/device-support/)
- User verification: `userVerification: "preferred"` is what passkeys.dev recommends for consumer flows. The UV flag must be checked on the server. [direct] [passkeys.dev bootstrapping](https://passkeys.dev/docs/use-cases/bootstrapping/). On iOS, Android and Windows the platform always performs UV with the device unlock method, so UV comes back `true` ([iOS](https://passkeys.dev/docs/reference/ios/), [Android](https://passkeys.dev/docs/reference/android/), [Windows](https://passkeys.dev/docs/reference/windows/)). Some browser-extension providers (1Password, Bitwarden and KeePassXC extensions, Proton Pass) set UV true without verifying. [direct] [passkeys.dev known issues](https://github.com/passkeydeveloper/passkeys.dev/blob/main/content/en/docs/reference/known-issues.md)
- **[inference]** Username-less login with a randomly generated user handle fits Oiko's "non-technical household members" goal. Account selection happens in the OS sheet. The server maps the returned `userHandle` to a user, which is what go-webauthn's `DiscoverableUserHandler` callback does.

## 3. Cross-device (hybrid) login and the shared wall tablet

### 3.1 Hybrid / Cross-Device Authentication (CDA)
- CDA lets a phone's passkey sign in on another device. It uses CTAP's "hybrid" transport, which authenticators and client platforms implement, not RPs. [direct] [passkeys.dev terms](https://passkeys.dev/docs/reference/terms/)
- Phones that can act as the authenticator: Android 9+ and iOS/iPadOS 16+. Devices that can act as the client (the device showing the QR code): Android 9+, ChromeOS 108+, iOS 16+, macOS 13+, Ubuntu in Chrome/Edge only, Windows 11 23H2+ at OS level. On Windows 10 and older Windows 11 builds, only Chrome/Edge 108+ can be the client. [direct] [passkeys.dev device support](https://passkeys.dev/device-support/), [Windows reference](https://passkeys.dev/docs/reference/windows/)
- Requirements: "both the Windows device and the mobile device must have Bluetooth enabled and connected to the Internet". Bluetooth is the proximity check. [direct] [Microsoft Learn, Support for passkeys in Windows](https://learn.microsoft.com/en-us/windows/security/identity-protection/passkeys/). Microsoft's Entra docs also say "users can't use cross-device registration or authentication if you enable attestation" (in their product) [direct] [Microsoft Learn FAQ](https://learn.microsoft.com/en-us/entra/identity/authentication/passkey-authenticator-faq).
- Persistent linking (no QR code after the first time) is supported only between Android phones and Windows 11 23H2+ or Chrome/Edge. iOS/iPadOS phones do not support it, so a QR code must be scanned every time. macOS does not support linking at OS level. [direct] [passkeys.dev Android](https://passkeys.dev/docs/reference/android/), [iOS](https://passkeys.dev/docs/reference/ios/), [macOS](https://passkeys.dev/docs/reference/macos/)
- RP-side signals: after a hybrid login `authenticatorAttachment` is `cross-platform`, and passkeys.dev suggests offering to create a local passkey then. [direct] [passkeys.dev bootstrapping](https://passkeys.dev/docs/use-cases/bootstrapping/). Client Hints (`hints: ["hybrid" | "security-key" | "client-device"]`) can steer the UI in Chrome/Edge 128+; Safari and Firefox do not support them, and they are hints only, never policy. [direct] [passkeys.dev client hints](https://passkeys.dev/docs/advanced/client-hints/)
- **[inference] Network dependency:** hybrid relies on the platforms' cloud tunnel service, so both devices need internet access. It does not depend on Oiko being reachable from the internet. It does depend on the RP origin being a valid HTTPS domain on the tablet (see §1).

### 3.2 Shared wall tablet: options and trade-offs (all [inference] unless cited)
- **Platform passkey stored on the tablet.** passkeys.dev warns that "all users that are able to unlock the current device will be able to access the account" ([bootstrapping](https://passkeys.dev/docs/use-cases/bootstrapping/)) [direct]. On a tablet signed in to one person's Apple or Google account, that passkey also syncs to that person's other devices. This suits a dedicated "tablet" or "household" account, not personal accounts.
- **Each person signs in with a phone over hybrid.** Works with no passkey stored on the tablet. The cost is a QR scan for iPhone users every time, Bluetooth on both devices, and a full sign-in each session. This is heavy for frequent wall-panel use, but good for occasional elevated actions by one person.
- **A security key (device-bound) kept by the tablet.** Possible on all platforms ("on security keys" in the matrix); iOS requires the key's PIN to be set up beforehand ([iOS reference](https://passkeys.dev/docs/reference/ios/)). It costs hardware and is a weaker fit for non-technical users.
- **Hybrid model:** a passkey enrols a long-lived device session for the tablet (a coarse "kiosk" role), and personal passkeys are used, from the user's phone via hybrid or elsewhere, only for privileged changes. WebAuthn does not cover device sessions; that belongs to the session-design research.

### 3.3 Guests
**[inference]** Asking temporary guests to create passkeys pollutes their credential managers with entries that outlive the visit. The ecosystem has no "temporary passkey" concept. Signal APIs can ask providers to hide stale credentials (`signalUnknownCredential`, `signalAllAcceptedCredentials` are in the spec's capability list [direct, [spec](https://www.w3.org/TR/webauthn-3/)]), but browser support was not checked. Guest access is probably better served by a non-passkey mechanism (invite link or one-time code) with a short-lived session.

## 4. Synced vs device-bound passkeys
- **Definitions:** a synced passkey (multi-device credential) is backup-eligible. A device-bound passkey (single-device credential) never leaves its authenticator. The BE (backup eligible) and BS (backup state) bits in authenticator data signal which kind a credential is. BE is fixed at creation; BS can change. [direct] [W3C WebAuthn L3 §6.1.3](https://www.w3.org/TR/webauthn-3/)
- **Out-of-the-box defaults:**
  - Synced passkeys: Android 9+, ChromeOS 129+, iOS 16+, macOS 13+. Ubuntu only via browser extensions. Windows Hello sync is "Planned", and Windows Hello creates device-bound passkeys. [direct] [passkeys.dev device support](https://passkeys.dev/device-support/), [Windows reference](https://passkeys.dev/docs/reference/windows/)
  - Chrome on Windows, macOS and Linux can save passkeys to Google Password Manager and sync them, which may require a GPM PIN. Announced in September 2024. [direct] [Google blog, Sept 2024](https://blog.google/innovation-and-ai/technology/safety-security/google-password-manager-passkeys-update-september-2024/)
  - Third-party providers (1Password, Bitwarden, etc.) can be used on Android 14+, iOS 17+, macOS 14+, Windows 11 25H2+, and via extensions elsewhere. [direct] [passkeys.dev device support](https://passkeys.dev/device-support/)
- **Trade-off [interp/inference]:**
  - Synced passkeys survive phone loss and replacement, which suits households and cuts recovery load. Their security then rests on the sync account (Apple Account / Google Account / password-manager vault).
  - Device-bound passkeys (security keys, Windows Hello) resist account takeover through the sync provider. They need a second credential per user for recovery.
  - go-webauthn can refuse backup-eligible credentials (`FilteringConfig.ProhibitBackupEligibility`) if a policy ever wants that, for example for an admin role. [direct] [types.go](https://github.com/go-webauthn/webauthn/blob/master/webauthn/types.go)

## 5. Platform support, including installed PWAs
- **Basic matrix (as of the passkeys.dev repository, read at time of writing):** see §2 for autofill and §3 for hybrid. All major OSes support passkeys in their default browser. [direct] [passkeys.dev device support](https://passkeys.dev/device-support/)
- **Browsers and webviews:**
  - System webviews (Android Custom Tabs/AuthTab, iOS `ASWebAuthenticationSession`) have full WebAuthn.
  - Embedded webviews are limited. Android `WebView` does not support WebAuthn directly (it needs Credential Manager glue). iOS `WKWebView` works only for the app's associated domain.
  - Edge WebView2 is listed as a system webview on Windows; embedded webviews are not supported there.
  [direct] [passkeys.dev Android](https://passkeys.dev/docs/reference/android/), [iOS](https://passkeys.dev/docs/reference/ios/)
- **Installed PWAs [interp/inference, limited evidence]:**
  - None of the primary sources I found has a dedicated "PWA" row.
  - Android installed PWAs (WebAPK) run in the browser engine. **[inference]** WebAuthn should behave as it does in Chrome; not verified in a primary source.
  - iOS Home Screen web apps run on WebKit. I found WebKit bug reports of WebAuthn misbehaving in standalone mode: [WebKit 241126](https://bugs.webkit.org/show_bug.cgi?id=241126), in which a second `get()` fails until reload, and [WebKit 273712](https://bugs.webkit.org/show_bug.cgi?id=273712), in which calls hang. I saw them through search summaries only and did not open them, so their current status is **unverified**.
  - **[inference]** Expect to test passkeys in an iOS Home Screen app explicitly. Always trigger `get()`/`create()` from a fresh user tap and give the user a visible retry.
  - Passkeys created in Safari and in a Home Screen app share the RP ID and the iCloud Keychain, so the same passkey should work in both. **[inference]**, not verified.
- **Linux desktops:** no OS-level platform authenticator. Use Chrome's GPM, browser-extension password managers or security keys; hybrid works only in Chrome/Edge. [direct] [passkeys.dev device support](https://passkeys.dev/device-support/)

## 6. Is requiring attestation worth it?
- **Spec default:** `attestation` defaults to `"none"`. Other values are `indirect`, `direct` and `enterprise`. [direct] [W3C WebAuthn L3 §5.4.7](https://www.w3.org/TR/webauthn-3/)
- **passkeys.dev guidance:** "We recommend that most relying parties not specify the attestation conveyance parameter… (thus defaulting to none), or instead explicitly use the value `indirect`… platforms are likely to obtain consent from the user for other types of attestation conveyances, which likely results in a larger fraction of unsuccessful credential creations." [direct] [passkeys.dev bootstrapping](https://passkeys.dev/docs/use-cases/bootstrapping/)
- **Availability:** in the device-support matrix, "Device-bound passkey attestation" is n/a for Android, ChromeOS, iOS, macOS and Ubuntu and supported only on Windows. [direct] [passkeys.dev device support](https://passkeys.dev/device-support/)
- **Apple:** Safari passkeys return attestation format `none` with an all-zero AAGUID even when `direct` is requested. I only have this from the Apple Developer Forums search result ([thread 713195](https://developer.apple.com/forums/thread/713195), [thread 726208](https://developer.apple.com/forums/thread/726208)); the page would not render for direct reading. **Confidence: medium.** Apple offers passkey attestation only for managed (MDM) devices ([Apple deployment guide](https://support.apple.com/guide/deployment/passkey-attestation-declarative-configuration-depd218e61b5/web)). [direct, from search snippet]
- **What attestation adds for Oiko [inference]:** proof of authenticator make and model, which matters for regulated workforce deployments. Requiring it would block iCloud Keychain, most Google Password Manager passkeys and third-party managers, which is most household devices. The AAGUID is still useful *without* attestation for display names ("iCloud Keychain", "Google Password Manager") in a passkey list. FIDO's synced-passkey paper describes RPs showing provider names this way ([FIDO Alliance, Synced Passkey Deployment, 2024](https://fidoalliance.org/wp-content/uploads/2024/05/Synced-Passkey-Deployment_-Emerging-Practices-for-Consumer-Use-Cases_2024-Final.pdf)) [interp]. Unattested AAGUIDs can be spoofed, so they are cosmetic only.
- **Library support if wanted later:** go-webauthn has `Config.MDS` (FIDO Metadata Service providers in `metadata/providers/memory` and `cached`), `Config.Attestation` policy, and AAGUID allow/deny lists. [direct] [types.go](https://github.com/go-webauthn/webauthn/blob/master/webauthn/types.go)

## 7. Go library: github.com/go-webauthn/webauthn
- **Identity and status:**
  - Fork of the Duo Labs library. It is the one Go library passkeys.dev lists, under "Other FIDO2/WebAuthn libraries" rather than "Updated for passkeys"; I don't know what that placement implies. ([passkeys.dev libraries](https://github.com/passkeydeveloper/passkeys.dev/blob/main/content/en/docs/tools-libraries/libraries.md)) [direct]
  - BSD-3-Clause. Owned by the `@go-webauthn/maintainers` team (CODEOWNERS). About 1.3k stars and 12 open issues per a search-result snapshot (metadata only, not verified live).
  - The README says it "is conformance tested against the conformance tools" and "is still version 0… there may be breaking changes without warning". [direct] [README](https://github.com/go-webauthn/webauthn)
- **Release cadence (CHANGELOG / releases):** v0.16.1 (2026-03-12) through v0.17.4 (2026-05-22), then **v0.18.0 (2026-08-27)**: "a fairly major milestone… quite a few breaking changes", with a MIGRATION.md, typed extensions, and ML-DSA when built with Go 1.27. Then v0.18.1 (2026-09-10, dependency updates only) and **v0.18.2 (2026-09-19)**: verification hardening (session challenge validation, TPM/SafetyNet/U2F checks). [direct] [CHANGELOG.md](https://github.com/go-webauthn/webauthn/blob/master/CHANGELOG.md), [releases](https://github.com/go-webauthn/webauthn/releases)
- **Go version policy:** `go 1.26.0` with `toolchain go1.27.1` in go.mod. Officially supports the latest Go minor, best effort for supported ones (currently 1.27 and 1.26). Its stated philosophy is "we aim to avoid backwards compatibility at the cost of security". [direct] [README](https://github.com/go-webauthn/webauthn), [go.mod](https://github.com/go-webauthn/webauthn/blob/master/go.mod). This is compatible with Oiko on Go 1.27.
- **Dependencies:** `fxamacker/cbor`, `golang-jwt/jwt/v5` (MDS blobs), `google/go-tpm`, `google/uuid`, `tinylib/msgp`, `go-viper/mapstructure`, `go-webauthn/x`. [direct] [go.mod](https://github.com/go-webauthn/webauthn/blob/master/go.mod)
- **API shape** (all [direct] from [`webauthn/login.go`, `registration.go`, `types.go`, `related_origins.go`](https://github.com/go-webauthn/webauthn/tree/master/webauthn)):
  - `webauthn.New(&Config{RPID, RPDisplayName, RPOrigins, RPTopOrigins, AttestationPreference, AuthenticatorSelection, Timeouts, MDS, Attestation, Filtering, …})`
  - The app implements the `User` interface: `WebAuthnID() []byte` (recommended random, up to 64 bytes), `WebAuthnName()`, `WebAuthnDisplayName()`, `WebAuthnCredentials() []Credential`.
  - Registration: `BeginRegistration(user, opts...)` / `BeginMediatedRegistration` (conditional create), returning `(*protocol.CredentialCreation, *SessionData)`, then `FinishRegistration(user, session, *http.Request)` or `CreateCredential(user, session, parsed)`.
  - Login with a known user: `BeginLogin` / `BeginMediatedLogin`, then `FinishLogin` / `ValidateLogin`.
  - Username-less: `BeginDiscoverableLogin` / `BeginDiscoverableMediatedLogin` (conditional UI), then `FinishPasskeyLogin(handler, session, req)`, which returns `(User, *Credential)`. The `DiscoverableUserHandler(rawID, userHandle)` callback resolves the user.
  - **Session state is the app's job:** `SessionData` "must be stored by the RP in a secure manner" between Begin and Finish. Since v0.18.2 the session challenge is validated on finish. **[inference]** This means a server-side store or an encrypted, bound cookie.
  - **Credential storage is the app's job:** the README documents a mapping table from the spec's Credential Record to `webauthn.Credential` (JSON or MessagePack encodings) and says the sign count must be written back after every login. The RP must also store the `rpId` scoping itself.
  - ROR: `(*WebAuthn).RelatedOrigins()` returns an `http.Handler` for `protocol.WellKnownPathWebAuthn`.
  - Client-capability enumeration and Signal API helpers were added in 0.18.0 (`client_capabilities.go`, `signals.go`). [direct, file list + CHANGELOG]
- **Security track record:** I could not load the repository's GitHub security-advisories page (fetch failed). A search for advisories turned up only advisories in *other* WebAuthn libraries (webauthn-rs GHSA-22w3-693w-x895, PHP webauthn-framework CVE-2026-30964), both origin/RP-ID matching bugs. Whether go-webauthn has published advisories is **unverified**. **[inference]** Those bugs are worth knowing as a class: origin matching is where libraries get things wrong.
- **Alternatives:** no other Go server library is listed on passkeys.dev. Writing WebAuthn verification by hand (CBOR/COSE parsing, attestation formats) is possible, but the spec's verification steps are long. **[inference]** The library is the pragmatic choice; I evaluated no Go alternative in depth.

## 8. Fallbacks the ecosystem expects
- **Canonical bootstrap:** a username field with `autocomplete="username webauthn"`, plus conditional `get()` on page load. If no passkey is chosen, "perform a 'legacy' user authentication… serve appropriate further login challenges (such as passwords, responding to SMS challenges, etc.)… These may include 'account recovery' steps". After any non-passkey sign-in, or after a cross-device sign-in, offer to create a passkey on the current device, gated by `isUserVerifyingPlatformAuthenticatorAvailable()`. [direct] [passkeys.dev bootstrapping](https://passkeys.dev/docs/use-cases/bootstrapping/)
- Google's RP guide: "It is recommended to keep existing authentication mechanisms like passwords and two-factor authentication while transitioning to passkeys due to compatibility and user readiness." [direct, from search snippet] [Google passkeys developer guide](https://developers.google.com/identity/passkeys/developer-guides)
- **Phishing caveat:** FIDO's 2025 paper notes that weaker fallback or external providers (federation, email) can bypass passkey phishing resistance: the account is only as strong as its weakest login path. [direct, from search snippet] [FIDO Alliance, Passkeys – The Journey to Prevent Phishing Pt3 (2025-03)](https://fidoalliance.org/wp-content/uploads/2025/03/Passkeys-The-Journey-to-Prevent-Phishing-Pt3.pdf)
- **Fallback candidates for Oiko** (**[inference]**; trade-offs only, no recommendation):
  - **Several passkeys per user**, for example phone plus laptop, or a second synced provider.
  - **Hybrid from a phone** when the current device has no passkey; this needs Bluetooth and internet on both devices.
  - **A hardware security key** as a device-bound backup.
  - **A password plus TOTP.** Universally available and phishable, and it adds password-storage surface.
  - **Admin-issued one-time enrolment/recovery codes or invite links.** These suit a self-hosted household with no email or SMS infrastructure: the household admin re-enrols a member who lost every device.
  - **Email magic links.** Need outbound mail, which a self-hosted hub may not have.
  - **External OIDC** (already in scope as optional). It moves recovery to the IdP.
  - **A local-console or CLI recovery path** for the sole admin, for example a command run on the host. This matters because a self-hosted install has no vendor support desk.

---

## Contradictions
- **Firefox ROR status.** web.dev ("As of January 2026, Firefox is still considering the feature", page last updated 2024-08-22) ([web.dev](https://web.dev/articles/webauthn-related-origin-requests)) disagrees with passkeys.dev (Firefox 152+) and Bugzilla 2010193 (RESOLVED FIXED, milestone 152) ([Bugzilla](https://bugzilla.mozilla.org/show_bug.cgi?id=2010193)). The newer primary sources (Bugzilla, Mozilla commits dated 2026-05-08) win; web.dev is stale.
- **go-webauthn latest version.** One search snapshot showed "v0.16.4 Latest Apr 9, 2026" and the releases page showed v0.17.4 (2026-05-22). The repository CHANGELOG at master shows v0.18.2 (2026-09-19). These are snapshots taken at different times, not a real conflict; the CHANGELOG is authoritative.
- Nothing else found.

## Missing evidence / unverified
- Installed-PWA behaviour (iOS Home Screen apps, Android WebAPK) has no primary-source support statement. The WebKit bugs cited were seen only as search summaries and their status is unknown.
- I could not read Apple's attestation behaviour (`none` with zero AAGUID) directly because the forum pages did not render; the evidence is search snippets plus the passkeys.dev matrix.
- go-webauthn's published security advisories (GitHub advisories page did not load).
- The exact release date of Firefox 152 to stable. I did not compute or verify it.
- Whether Chrome's TLS-error block also applies to certificates chained to a user-installed private CA. **[inference]** It should not, since the criterion is the same as for the "Not secure" or interstitial state. The source only says "the same used for showing danger interstitials or a 'Not secure' pill". Mobile trust stores (Android user CAs, iOS profile trust) were not researched.
- Browser support for the Signal API (`signalUnknownCredential` etc.), relevant for cleaning up guests' or revoked passkeys.
- Hybrid behaviour when the tablet itself is the *client* on iPadOS or Android tablets in kiosk or guided-access modes: not researched.

## Open points for the decision
1. **Canonical hostname and RP ID:** one public FQDN (with split-horizon DNS on the LAN), or the broader parent domain as RP ID? Remember that changing the RP ID later means every user re-enrols.
2. **LAN-only, IP, or `.local` access:** accept that passkeys are unavailable there (and offer another login path), require a trusted certificate on a domain name, or support ROR between two *public* names? ROR cannot rescue IP or untrusted-certificate access.
3. **Where RP ID and origins come from:** static configuration (with startup validation that they match the proxy's public URL) versus anything derived from request headers. The research points to static configuration [inference].
4. **Attestation policy:** `none` (passkeys.dev default) versus optional AAGUID-based display names versus any enforcement (for example security keys only for an admin role, via `Filtering`/`MDS`).
5. **Synced vs device-bound policy per role:** allow backup-eligible credentials everywhere, or forbid them for admins (`ProhibitBackupEligibility`)?
6. **Shared tablet model:** a household or kiosk account with a passkey on the tablet, per-person hybrid sign-in, a security key, or a device-session approach outside WebAuthn.
7. **Guests:** use passkeys at all, or a non-passkey, time-boxed mechanism?
8. **Fallback and recovery set:** which of password+TOTP / admin-issued codes / OIDC / host-CLI recovery to offer, given that the weakest path sets the real security level.
9. **Library adoption:** go-webauthn v0.18.x, pre-1.0 with breaking changes as recently as 2026-08-27. Accept a pinned version plus upgrade work, or wrap it behind an internal interface so the HTTP/API contract (ADR 0019 rules) does not follow library churn.
10. **WebAuthn session state storage** (`SessionData` between Begin and Finish): server-side store vs signed and encrypted cookie. This ties into the wider session design.

## Sources
- Kept:
  - W3C, Web Authentication Level 3 (https://www.w3.org/TR/webauthn-3/): normative RP ID, IP and localhost rules, ROR algorithm, attestation default, BE/BS flags.
  - passkeys.dev Device Support, rendered page and repository source (https://passkeys.dev/device-support/, https://github.com/passkeydeveloper/passkeys.dev/blob/main/content/en/device-support/index.md): the cross-vendor (W3C/FIDO community) support matrix.
  - passkeys.dev: Related Origins, Terms, Bootstrapping, Client Hints, Known Issues, and the iOS/Android/Windows/macOS references and Libraries pages (https://passkeys.dev/docs/): RP guidance and platform behaviour.
  - web.dev, Related Origin Requests (https://web.dev/articles/webauthn-related-origin-requests): ROR mechanics (support statement stale).
  - Mozilla standards-positions #1052 (https://github.com/mozilla/standards-positions/issues/1052) and Bugzilla 2010193 (https://bugzilla.mozilla.org/show_bug.cgi?id=2010193): Firefox ROR status.
  - Chrome team post to public-webauthn, 2022-11-24 (https://lists.w3.org/Archives/Public/public-webauthn/2022Nov/0135.html): WebAuthn blocked on TLS errors from Chrome 110.
  - CA/Browser Forum internal names guidance (https://cabforum.org/working-groups/server/internal-names/): no public certificates for internal names or reserved IPs.
  - Microsoft Learn, Passkeys in Windows (https://learn.microsoft.com/en-us/windows/security/identity-protection/passkeys/): hybrid needs Bluetooth and internet.
  - Google blog, Sept 2024 GPM passkeys (https://blog.google/innovation-and-ai/technology/safety-security/google-password-manager-passkeys-update-september-2024/): desktop sync.
  - go-webauthn repository: README, CHANGELOG, go.mod, `webauthn/*.go` (https://github.com/go-webauthn/webauthn): library maturity and API.
- Deprioritized:
  - Corbado blog posts: vendor or SEO content; used only for discovery.
  - StackOverflow threads: anecdotal.
  - openpwa.net: third-party, not authoritative.
  - Reddit threads: anecdotal.
  - Apple forum threads: could not be rendered; snippet only, flagged.

## Next steps (research)
- Run the passkeys.dev feature-detect tool (https://tools.passkeys.dev/featuredetect) on the household's actual devices, including an iOS Home Screen install of the SPA.
- Read go-webauthn MIGRATION.md and the package examples (`example_passkey_test.go`) before prototyping, and check its GitHub security advisories directly.
- Verify Signal API support per browser if guest and revoked-passkey cleanup matters.
