# Research: What NIST SP 800-63B-4 and OWASP ASVS 5.0 require of sessions and API tokens

Scope: what the standards say, with requirement identifiers, for an application aiming at the strongest practical assurance (Oiko: Go stdlib `net/http`, React SPA, JSON API, SSE `/api/updates`, behind a user-run TLS-terminating reverse proxy). This is research only. It lays out options and trade-offs and leaves the decisions to the project.

Sources read for this report (fetched directly, not from search snippets):

- NIST SP 800-63B-4, *Authentication and Authenticator Management*. CSRC lists it as **"Date Published: July 2025"**; it supersedes SP 800-63B (03/02/2020) ([CSRC](https://csrc.nist.gov/pubs/sp/800/63/b/4/final)). Sections were read on the HTML edition at pages.nist.gov ([AAL / full doc](https://pages.nist.gov/800-63-4/sp800-63b.html), [authenticators](https://pages.nist.gov/800-63-4/sp800-63b/authenticators/), [session](https://pages.nist.gov/800-63-4/sp800-63b/session/), [syncable, Appendix B](https://pages.nist.gov/800-63-4/sp800-63b/syncable/), [events](https://pages.nist.gov/800-63-4/sp800-63b/events/)).
- OWASP ASVS **5.0.0**, from the `v5.0.0` tag of the GitHub repository ([repo](https://github.com/OWASP/ASVS/tree/v5.0.0/5.0/en)). The repository README says 5.0.0 came out in May 2025 at OWASP Global AppSec EU Barcelona; this report takes that date from a search summary of the README and did not check it separately.
- Supporting sources (not normative for this question): [OWASP Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html) (living document, read 2026), [RFC 6750](https://www.rfc-editor.org/rfc/rfc6750.txt) (Oct 2012), [GitHub token-format engineering post](https://github.blog/engineering/behind-githubs-new-authentication-token-formats/) (2021), and [GitHub PAT docs](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens).

How to read NIST keywords: NIST uses **SHALL** (requirement), **SHOULD** (recommendation) and **MAY**. Most of SP 800-63B-4's *numbers* (timeouts) are SHOULDs. NIST has no numbered requirement IDs, so this report cites the section number and anchor (e.g. §2.2.3 `#aal2reauth`). ASVS requirements are cited as `V<chapter>.<section>.<n>` with their level (L1/L2/L3). Here "strongest" means ASVS L3 and NIST AAL3.

---

## Key findings

1. **Passkeys and AALs (NIST):**
   - **Synced passkeys can reach AAL2 at most.** NIST says syncable authenticators "**SHALL NOT** be used at AAL3" because the private key is exportable (§2.3.2; Appendix B).
   - **A passkey with User Verification (UV) counts as a *multi-factor* cryptographic authenticator.** Without UV it counts as single-factor. Verifiers "SHALL indicate that UV is preferred and SHALL inspect responses" (Appendix B).
   - **Only device-bound passkeys can reach AAL3.** The key must be non-exportable and held in a hardware-protected environment, and AAL3 adds FIPS 140 Level 1 validation of the authenticator (§2.3.2).
   - **WebAuthn counts as phishing-resistant** through "verifier name binding" (§3.2.5.2). OTP, out-of-band codes, look-up secrets and passwords do not.
2. **NIST requires a phishing-resistant *option* at AAL2** ("Verifiers SHALL offer at least one phishing-resistant authentication option at AAL2", §2.2.2) and **requires phishing resistance at AAL3**.
3. **Reauthentication timeouts (NIST).** A definite overall timeout **SHALL** exist at every AAL. The values themselves are SHOULDs:

   | AAL | Overall timeout | Inactivity timeout | Notes |
   |---|---|---|---|
   | AAL1 | ≤ 30 days | optional | |
   | AAL2 | ≤ 24 h | ≤ 1 h | After an inactivity timeout, reauthentication MAY use only a password or biometric plus the session secret |
   | AAL3 | ≤ 12 h (this one is a **SHALL**) | ≤ 15 min | Reauthentication = full AAL3 authentication |

   Sources: §2.1.3, §2.2.3, §2.3.3, Table 1.
4. **ASVS 5.0 sets no timeout numbers.** It requires both timeouts to exist and be documented, and any deviation from NIST to be justified: V7.1.1 (L2), V7.3.1 and V7.3.2 (L2).
5. **Step-up and reauthentication (ASVS):**
   - Full reauthentication before changing authentication-related attributes: **V7.5.1 (L2)**.
   - At least one factor or a secondary verification before "highly sensitive transactions": **V7.5.3 (L3)**.
   - A new session token on every authentication, including reauthentication: **V7.2.4 (L1)**.
   - NIST ties binding a new authenticator to the AAL the authenticator will be used at, and requires an independent notification (§4.1.2).
6. **Revocation (ASVS):**
   - Logout and expiry must make the session unusable server-side; self-contained tokens need a deny-list, a per-user "not before" time or per-user key rotation: **V7.4.1 (L1)**.
   - All sessions end when an account is disabled or deleted: **V7.4.2 (L1)**.
   - Offer "terminate other sessions" after any factor change: **V7.4.3 (L2)**.
   - Admins can kill sessions: **V7.4.5 (L2)**.
   - Users can list and kill their sessions after reauthenticating: **V7.5.2 (L2)**.
   - Authorization changes apply immediately: **V8.3.2 (L3)**.
   - *Inference:* in practice these push toward server-side (stateful) sessions.
7. **Cookies:**
   - NIST §5.1.1 for session cookies: Secure **SHALL**; minimal host and path **SHALL**; HttpOnly **SHOULD**; `__Host-` prefix with `Path=/` **SHOULD**; `SameSite=Lax` or `Strict` **SHOULD**; opaque value **SHOULD**; no cleartext personal information **SHALL**. Cookie expiry "**SHALL NOT** be relied upon to enforce session timeouts".
   - ASVS: Secure plus `__Host-` or `__Secure-` (**V3.3.1, L1**); SameSite set by purpose (**V3.3.2, L2**); `__Host-` unless deliberately shared (**V3.3.3, L2**); HttpOnly, and the token delivered only via `Set-Cookie` (**V3.3.4, L2**).
8. **Session secret strength:**
   - NIST: ≥ 64 bits from an approved RBG (§5.1, item 2).
   - ASVS: ≥ 128 bits from a CSPRNG for reference tokens (**V7.2.3, L1**) and for any non-guessable value (**V11.5.1, L2**).
   - NIST also: bearer session secrets **SHOULD NOT** persist across an app restart or reboot; "remember my browser" **SHALL NOT** replace authentication except for AAL2 reauthentication after an inactivity timeout. Proof-of-possession sessions (DBSC) **MAY** persist (§5.1).
9. **API tokens for programs:**
   - **Neither standard has a dedicated "personal API token" chapter.**
   - Directly relevant: no static API secrets as *session* tokens (**V7.2.2, L1**); no tokens in URLs or query strings (**V14.2.1, L1**; RFC 6750 §2.3 "SHOULD NOT"); ≥ 128-bit CSPRNG values (**V11.5.1**); secrets expire and rotate per documentation (**V13.3.4, L3**); service-to-service authentication avoids "unchanging credentials such as … API keys" (**V13.2.1, L2**, written for backend components).
   - NIST: the presence of an access token **SHALL NOT** be taken as proof that the user is present (§5.1.2).
   - Hashed storage, prefix and checksum formats, and "shown once" come from *non-normative* guidance and industry practice (OWASP cheat sheet, RFC 6819 cited there, GitHub), not from ASVS or NIST.
10. **Rate limiting and lockout:**
    - NIST: no more than **100 consecutive failed attempts** per authenticator per account; when exceeded, that authenticator is disabled and must be re-bound (§3.2.2). Lower limits MAY be set. Mitigations that avoid locking out the legitimate user are MAY items: bot challenges, increasing delays (e.g. 30 s up to 1 h), risk signals.
    - ASVS: document the anti-brute-force and anti-stuffing controls *and how they prevent malicious lockout* (**V6.1.1, L1**) and implement them (**V6.3.1, L1**). No user enumeration through error messages, status codes or timing (**V6.3.8, L3**). General anti-automation (**V2.4.1, L2**).

---

## Details

### 1. Authenticator assurance levels and where passkeys land

**NIST SP 800-63B-4, §2 (AALs)** ([source](https://pages.nist.gov/800-63-4/sp800-63b.html)), quoted:

- **AAL1:** single-factor or multi-factor, any listed type. "Verifiers **SHOULD** make multi-factor authentication options available at AAL1."
- **AAL2 (§2.2.1–2.2.2):** "authentication **SHALL** use either a multi-factor authenticator or a combination of two separate authentication factors." "At least one authenticator used at AAL2 **SHALL** be replay-resistant." "Authentication at AAL2 **SHOULD** demonstrate authentication intent." "Verifiers **SHALL** offer at least one phishing-resistant authentication option at AAL2 … verifiers **SHOULD** encourage the use of phishing-resistant authentication at AAL2 whenever practical."
- **AAL3 (§2.3.1–2.3.2):** permitted combinations are multi-factor cryptographic, or single-factor cryptographic plus a password or biometric. "The cryptographic authenticator used at AAL3 **SHALL** have a non-exportable private key and **SHALL** provide phishing resistance." "All authentication and reauthentication processes at AAL3 **SHALL** demonstrate authentication intent." "Single-factor and multi-factor authenticators used at AAL3 **SHALL** be validated to meet the requirements of [FIPS140] Level 1 or higher overall." "Since syncable authenticators … require the private key to be exportable, syncable authenticators **SHALL NOT** be used at AAL3."
- Table 1 (non-normative summary): phishing resistance is "Not required" at AAL1, "Recommended; Must be available" at AAL2, and "Required" at AAL3. Key exportability is "Permitted" at AAL1 and AAL2 and "Prohibited" at AAL3.

**Phishing resistance, §3.2.5** ([source](https://pages.nist.gov/800-63-4/sp800-63b/authenticators/#verifimpers)): "Phishing resistance requires single- or multi-factor cryptographic authentication. Authenticators that involve the manual entry of an authenticator output (e.g., out-of-band and OTP authenticators) **SHALL NOT** be considered phishing-resistant." WebAuthn/FIDO2 is named as giving phishing resistance "through verifier name binding". The verifier identifier must be the authenticated hostname or a parent domain at least one level below the public suffix. Passwords and look-up secrets are explicitly "not phishing-resistant" (§3.1.1, §3.1.2).

**Syncable (passkey) authenticators, Appendix B (normative)** ([source](https://pages.nist.gov/800-63-4/sp800-63b/syncable/)):

- Synced keys have to meet several conditions: stored in the sync fabric only in encrypted form (≥ 112-bit key); private-key operations done on the local device; access to the sync fabric protected by "AAL2-equivalent MFA". "Authentication at AAL2 may be supported subject to the above requirements. However, syncing violates the non-exportability requirements of AAL3."
- WebAuthn flags:
  - **UP:** verifiers SHOULD check it.
  - **UV:** "Verifiers **SHALL** indicate that UV is preferred and **SHALL** inspect responses … If the user is not verified, agencies **SHALL** treat the authenticator as a single-factor cryptographic authenticator."
  - **BE (Backup Eligible):** verifiers MAY use it to restrict syncable authenticators; it is the flag that tells device-bound credentials from syncable ones.
  - **BS (Backup State):** "Agencies **SHOULD NOT** condition acceptance based on this flag for public-facing applications due to user experience concerns."
- Attestation: "The unavailability of attestations **SHOULD NOT** block the use of syncable authenticators for broad public-facing applications."
- Sharing: some passkey providers let users share keys between people, and RPs "should assume that all syncable authenticators may be subject to sharing."
- Revocation: NIST notes that there is no central revocation for WebAuthn keys. The RP removes them from the account.

**Where each passkey type lands.** *Source interpretation*, combining §2.2, §2.3 and Appendix B:

| Authenticator | NIST category | Max AAL | Phishing-resistant |
|---|---|---|---|
| Synced passkey (iCloud Keychain, Google Password Manager, third-party managers), UV performed | Multi-factor cryptographic | AAL2 | Yes |
| Synced passkey, no UV | Single-factor cryptographic | AAL2 only when combined with a password (or biometric) | Yes, for the crypto factor |
| Device-bound passkey on a hardware security key or a non-syncing platform authenticator, UV performed | Multi-factor cryptographic | AAL3, if the key is non-exportable, hardware-protected and FIPS 140 L1 validated | Yes |
| Password + TOTP | Two single-factor authenticators | AAL2 | No |

**OWASP ASVS 5.0** ([V6](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x15-V6-Authentication.md)):

- V6.3 section text: "L2 applications must force the use of multi-factor authentication (MFA). L3 applications must use hardware-based authentication, performed in an attested and trusted execution environment (TEE). This could include device-bound passkeys, eIDAS Level of Assurance (LoA) High enforced authenticators, authenticators with NIST Authenticator Assurance Level 3 (AAL3) assurance, or an equivalent mechanism."
- **V6.3.3 (L2):** "Verify that either a multi-factor authentication mechanism or a combination of single-factor authentication mechanisms, must be used in order to access the application. For L3, one of the factors must be a hardware-based authentication mechanism which provides compromise and impersonation resistance against phishing attacks while verifying the intent to authenticate by requiring a user-initiated action (such as a button press on a FIDO hardware key or a mobile phone). Relaxing any of the considerations in this requirement requires a fully documented rationale and a comprehensive set of mitigating controls."
- Related requirements:
  - **V6.3.6 (L3):** email is not an authentication factor.
  - **V6.6.1 (L2):** SMS and phone only with caveats; *not available at L3*.
  - **V6.5.6 (L3):** any factor can be revoked.
  - **V6.7.2 (L3):** challenge nonce ≥ 64 bits.
  - **V6.3.5, V6.3.7 (L3):** notify users of suspicious attempts and of changes to authentication details.
  - **V6.8.4 (L2):** when an external IdP is used (optional OIDC), check `acr`, `amr` and `auth_time`, or fall back to assuming the weakest method was used.

### 2. Session lifetimes, reauthentication, step-up

**NIST §2.x.3 and §5.2** ([AAL](https://pages.nist.gov/800-63-4/sp800-63b.html), [session](https://pages.nist.gov/800-63-4/sp800-63b/session/#sessionreauthn)):

- AAL1: "A definite reauthentication overall timeout **SHALL** be established, which **SHOULD** be no more than 30 days at AAL1. An inactivity timeout **MAY** be applied but is not required."
- AAL2: overall timeout **SHALL** exist and **SHOULD** be ≤ 24 hours. "The inactivity timeout **SHOULD** be no more than 1 hour. When the inactivity timeout has occurred but the overall timeout has not yet occurred, the verifier **MAY** allow the subscriber to reauthenticate using only a successful password or biometric comparison in conjunction with the session secret."
- AAL3: "the overall timeout for reauthentication **SHALL** be no more than 12 hours. The inactivity timeout **SHOULD** be no more than 15 minutes … AAL3 reauthentication requirements are the same as for initial authentication at AAL3."
- §5.2: "When either timeout expires, the session **SHALL** be terminated. Session activity **SHALL** reset the inactivity timeout, and successful reauthentication during a session **SHALL** reset both timeouts." The RP MAY warn the user before expiry. Limits depend on the AAL, the environment, the endpoint type and whether the device is managed, and "**MAY** also be extended when higher security session maintenance technologies (e.g., device-bound mechanisms) are used." Agencies "**SHALL** establish and document" the limits.
- §5.1: "A session **SHOULD** inherit the AAL properties of the authentication event … **SHALL NOT** be considered at a higher AAL than the authentication event."
- Federation: "the RP **SHALL** be authoritative as to whether the reauthentication requirements have been met" (§5.2). Relevant if OIDC is added.
- §5.3: session monitoring (continuous authentication) is a MAY. Its signals have privacy implications, which **SHALL** be covered by the privacy risk assessment.

**NIST on step-up.** The term "step-up" is not used in the sections read. The closest requirements are:

- Binding an additional authenticator requires authentication "at either the maximum AAL currently available in the subscriber account or the maximum AAL at which the new authenticator will be used, whichever is lower", and "the CSP **SHALL** notify the subscriber via a mechanism independent of the transaction" (§4.1.2, [events](https://pages.nist.gov/800-63-4/sp800-63b/events/)).
- Account recovery always triggers notifications (§4.2).

**ASVS 5.0 V7** ([source](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x16-V7-Session-Management.md)):

- **V7.1.1 (L2):** "the user's session inactivity timeout and absolute maximum session lifetime are documented, are appropriate in combination with other controls, and that the documentation includes justification for any deviations from NIST SP 800-63B re-authentication requirements."
- **V7.1.2 (L2):** document how many concurrent sessions are allowed per account and what happens when the limit is reached.
- **V7.3.1 / V7.3.2 (L2):** an inactivity timeout and an absolute maximum lifetime are enforced "according to risk analysis and documented security decisions."
- **V7.2.4 (L1):** "generates a new session token on user authentication, including re-authentication, and terminates the current session token."
- **V7.5.1 (L2):** "requires full re-authentication before allowing modifications to sensitive account attributes which may affect authentication such as email address, phone number, MFA configuration, or other information used in account recovery."
- **V7.5.3 (L3):** "requires further authentication with at least one factor or secondary verification before performing highly sensitive transactions or operations."
- **V7.6.2 (L2):** creating a session requires user consent or an explicit action. This matters for silent SSO.
- **V8.1.4 / V8.2.4 (L3):** document and implement adaptive controls (allow, challenge, deny, step-up) based on context, applied both at session start and during a session ([V8](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x17-V8-Authorization.md)).

**OWASP Session Management Cheat Sheet** (informative, [source](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)): "Common idle timeouts ranges are 2-5 minutes for high-value applications and 15-30 minutes for low risk applications." The absolute timeout depends on usage, e.g. "between 4 and 8 hours" for a full office day. Timeouts must be enforced server-side.

### 3. Session binding and revocation

**NIST §5.1 Session Bindings** ([source](https://pages.nist.gov/800-63-4/sp800-63b/session/#bindings)):

- Session secrets: established at or right after authentication; from an approved RBG; "at least 64 bits in length"; erased or invalidated on logout; sent only over an authenticated protected channel; time out per the AAL limits; "unavailable to intermediaries between the host and the subscriber's endpoint."
- "Session secrets that are used as bearer tokens for session management **SHOULD NOT** be persistent (i.e., retained across a restart of the associated application or a reboot of the host device)."
- "Cookies and similar 'remember my browser' features **SHALL NOT** be used instead of authentication except as provided for reauthentication at AAL2 … when the inactivity limit has been exceeded but the time limit has not."
- Device Bound Session Credentials (DBSC), an emerging W3C/browser specification, is cited as a proof-of-possession option: "Session secrets used with such proof of possession techniques **MAY** persist. However, RPs and CSPs **SHALL** ensure that the session lifetime limits … are enforced."
- "They **SHOULD NOT** be placed in insecure locations (e.g., HTML5 Local Storage)."
- "Following authentication, authenticated sessions **SHALL NOT** fall back to an insecure transport."
- "POST/PUT content **SHALL** contain a session identifier that the RP **SHALL** verify to protect against cross-site request forgery (CSRF)."
- Logout: sessions **SHOULD** provide a readily accessible logout, "particularly if the endpoint might be accessible to others."
- §3.2.1: browser cookies "do not satisfy" device-identity authentication "except as short-term secrets for session maintenance (not authentication)."

**ASVS V7.2 and V7.4** ([source](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x16-V7-Session-Management.md)):

- **V7.2.1 (L1):** all session token verification happens in a trusted backend service.
- **V7.2.2 (L1):** "uses either self-contained or reference tokens that are dynamically generated for session management, i.e. not using static API secrets and keys."
- **V7.2.3 (L1):** reference tokens are unique, come from a CSPRNG and have "at least 128 bits of entropy."
- **V7.4.1 (L1):** after termination (logout or expiry) the session can no longer be used. "For reference tokens or stateful sessions, this means invalidating the session data at the application backend. Applications using self-contained tokens will need a solution such as maintaining a list of terminated tokens, disallowing tokens produced before a per-user date and time or rotating a per-user signing key."
- **V7.4.2 (L1):** all active sessions end when an account is disabled or deleted.
- **V7.4.3 (L2):** offer to end all other sessions after any factor change or removal, including a password reset.
- **V7.4.4 (L2):** a visible logout on every authenticated page.
- **V7.4.5 (L2):** admins can end sessions for one user or for all users.
- **V7.5.2 (L2):** users can view their sessions and, after authenticating again with at least one factor, end any or all of them.
- **V14.3.1 (L1):** authenticated data is cleared from client storage when the session ends; `Clear-Site-Data` is mentioned ([V14](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x23-V14-Data-Protection.md)).

**Server-side token storage (informative).** The OWASP cheat sheet says that if read-only disclosure of the session store is in the threat model, the store should hold a one-way verifier: an identifier plus a verifier, with the full SHA-256 of the verifier stored and compared in constant time. It adds that "A fast hash is sufficient for these random verifiers; password hashing algorithms are unnecessary." It recommends ≥ 128 bits of entropy and prefers ≥ 160, citing RFC 6749 §10.10 and RFC 6819 §4.3.2 ([source](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html#server-side-session-token-storage)). ASVS itself does not require hashing of session tokens; this is cheat-sheet guidance.

### 4. Cookies, and server-side sessions vs self-contained JWT

**NIST §5.1.1 Browser Cookies** ([source](https://pages.nist.gov/800-63-4/sp800-63b/session/#sesscookies)), quoted. Cookies used for session maintenance:

1. "**SHALL** be tagged to be accessible only on secure (i.e., HTTPS) sessions."
2. "**SHALL** be accessible to the minimum practical hostnames and paths."
3. "**SHOULD** be tagged as inaccessible via JavaScript (i.e., HttpOnly)."
4. "**SHOULD** be tagged to expire at or soon after the session's validity period … but **SHALL NOT** be relied upon to enforce session timeouts."
5. "**SHOULD** have the '__Host-' prefix and set 'Path=/'."
6. "**SHOULD** set 'SameSite=Lax' or 'SameSite=Strict'."
7. "**SHOULD** contain only an opaque string (e.g., a session identifier) and **SHALL NOT** contain cleartext personal information."

**ASVS V3.3 Cookie Setup** ([source](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x12-V3-Web-Frontend-Security.md)):

- **V3.3.1 (L1):** "cookies have the 'Secure' attribute set, and if the '__Host-' prefix is not used for the cookie name, the '__Secure-' prefix must be used."
- **V3.3.2 (L2):** "each cookie's 'SameSite' attribute value is set according to the purpose of the cookie."
- **V3.3.3 (L2):** "cookies have the '__Host-' prefix for the cookie name unless they are explicitly designed to be shared with other hosts."
- **V3.3.4 (L2):** HttpOnly when scripts don't need the value (e.g. a session token), and "the same value … must only be transferred to the client via the 'Set-Cookie' header field."
- **V3.3.5 (L3):** cookie name plus value ≤ 4096 bytes.

Related V3 requirements:

- **V3.4.1 (L1):** HSTS with max-age ≥ 1 year (and includeSubDomains for L2 and up).
- **V3.4.3 (L2):** CSP; per-response nonces or hashes at L3.
- **V3.5.1–3.5.3 (L1):** CSRF defence by anti-forgery tokens, non-safelisted headers, or reliance on CORS preflight with Origin and Content-Type checks; no "safe" methods for sensitive operations, or strict `Sec-Fetch-*` validation.
- **V3.7.4 (L3):** HSTS preload of the top-level domain.

**OWASP cheat sheet (informative).** `__Host-` requires `Secure`, no `Domain` and `Path=/`, and is "Recommended for session IDs". Its example is `Set-Cookie: __Host-SessionID=<value>; Secure; HttpOnly; SameSite=Strict; Path=/`. It treats SameSite as defence in depth, "not as a replacement for a CSRF token". It warns that browsers with session restore can keep session cookies after the browser closes, and that tokens should never go in `localStorage` or `sessionStorage`.

**Self-contained tokens (JWT): ASVS V9** ([source](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x18-V9-Self-contained-Tokens.md)):

- **V9.1.1 (L1):** check the signature or MAC before trusting the contents.
- **V9.1.2 (L1):** algorithm allowlist, never `none`.
- **V9.1.3 (L1):** keys only from trusted, pre-configured sources (`jku`, `x5u` and `jwk` allowlisted).
- **V9.2.1 (L1):** `nbf` and `exp` enforced.
- **V9.2.2–9.2.4 (L2):** token type, purpose and audience checked.

ASVS V7 intro: "Regardless of whether a stateful or 'stateless' session mechanism is chosen, the analysis must be complete and documented to demonstrate that the selected solution is capable of satisfying all relevant security requirements."

*Source interpretation:* ASVS does not ban JWT sessions. But V7.4.1 (server-side termination), V7.4.2/7.4.5 (admin and account-disable kill), V7.5.2 (list and kill sessions) and V8.3.2 (authorization changes apply immediately, L3) all need server-side state. A self-contained session token would therefore still need a server-side lookup, such as a deny-list, a per-user "not before" time or a session record. NIST §5.1.1 item 7 recommends an "opaque string" in the cookie.

### 5. API tokens for external programs

**What the standards say directly:**

- **ASVS V7.2.2 (L1).** Session management must not use "static API secrets and keys". This is aimed at *user sessions*. *Interpretation:* a long-lived API token for a program is a different credential type from a session. V7.2.2 does not forbid such tokens, but they should not stand in for browser sessions.
- **ASVS V13.2.1 (L2).** Communication between backend components "must use individual service accounts, short-term tokens, or certificate-based authentication and not unchanging credentials such as passwords, API keys, or shared accounts with privileged access" ([V13](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x22-V13-Configuration.md)). *Interpretation:* this is written for the application's *own* backend components, not for third-party clients of its public API. Applying it to user-issued program tokens is a stretch, but it shows the direction: short-lived or rotating credentials over static keys.
- **ASVS V13.3.4 (L3).** "secrets are configured to expire and be rotated based on the application's documentation." **V13.1.4 (L3):** document critical secrets and a rotation schedule. **V13.3.1 (L2):** use a secrets-management solution for *backend* secrets, hardware-backed at L3. This applies to Oiko's own secrets, such as signing keys, rather than to tokens issued to users.
- **ASVS V14.2.1 (L1).** "the URL and query string do not contain sensitive information, such as an API key or session token" ([V14](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x23-V14-Data-Protection.md)).
- **ASVS V11.5.1 (L2).** Non-guessable values come from a CSPRNG with ≥ 128 bits; "UUIDs do not respect this condition" ([V11](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x20-V11-Cryptography.md)).
- **Analogy for hashed storage: ASVS V6.5.2 (L2), written for look-up secrets.** Secrets with < 112 bits of entropy need a salted password hash; "A standard hash function can be used if the secret has 112 bits of entropy or more." NIST §3.1.2.2 says the same for look-up secrets. *Interpretation:* applied by analogy, a high-entropy random API token can be stored as a plain SHA-256 hash. Neither standard states this for API tokens specifically.
- **Scoping.** ASVS V8.2.1/V8.2.2 (L1) and V8.2.3 (L2): function-, data- and field-level access only with explicit permissions. V9.2.3 and RFC 6750 §5.2 ("Restricting the use of the token to a specific scope is also RECOMMENDED"). Neither standard prescribes a scope model.
- **NIST §5.1.2 Access Tokens.** "The RP **SHALL NOT** interpret the presence of an access token as an indicator of the subscriber's presence in the absence of other signals." *Interpretation:* a program token must not satisfy "recent reauthentication" or step-up checks.
- **RFC 6750 (Bearer Token Usage, 2012):**
  - Clients SHOULD send the token in the `Authorization: Bearer` header.
  - The URI query method "SHOULD NOT be used unless it is impossible to transport the access token in the 'Authorization' request header field or the HTTP request entity-body" (§2.3).
  - "Token servers SHOULD issue short-lived (one hour or less) bearer tokens" (§5.3), a recommendation aimed at OAuth access tokens.
  - When TLS terminates in front of the resource server, "sufficient measures MUST be employed to ensure confidentiality of the token between the front-end and back-end servers" (§5.2). *Inference:* this applies to the proxy-to-Oiko hop.
  - "Implementations that do store bearer tokens in cookies MUST take precautions against cross-site request forgery" (§5.3).

**Industry practice (not normative):**

- **Prefix and checksum format.** GitHub tokens use an identifiable prefix (e.g. `ghp_`) with `_` as separator, "A 32 bit checksum in the last 6 digits" (CRC32, Base62), and about 178 bits of entropy. The stated goal is accurate secret scanning ("virtually eliminates false positives … offline") ([GitHub blog, 2021](https://github.blog/engineering/behind-githubs-new-authentication-token-formats/)).
- **Expiry.** GitHub docs: "we highly recommend adding an expiration to your personal access tokens". GitHub removes tokens left unused for a year; infinite lifetimes are allowed but can be blocked by policy ([GitHub docs](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens)).
- **"Shown once".** Widespread practice, but **not verified** in a primary source during this run, and not required by ASVS or NIST.

### 6. Rate limiting and lockout

**NIST §3.2.2 Rate Limiting (Throttling)** ([source](https://pages.nist.gov/800-63-4/sp800-63b/authenticators/#throttle)):

- "the verifier **SHALL** limit consecutive failed authentication attempts using a specific authenticator on a single subscriber account to no more than 100 by disabling that authenticator. If more than one authenticator is involved … both authenticators **SHALL** be disabled. Authenticators that have been disabled **SHALL** be required to rebind to the subscriber account."
- "The limit of 100 attempts is an upper bound, and agencies **MAY** impose lower limits."
- Techniques to avoid locking out the legitimate user (MAY): bot challenges; "Requiring the claimant to wait after a failed attempt for a period of time that increases as the subscriber account approaches its maximum allowance … (e.g., 30 seconds up to an hour)"; risk-based signals such as IP, geolocation, timing and browser metadata.
- After a successful authentication, the verifier SHOULD disregard or reset previous failures. A reset "**SHALL** not exceed the AAL of the session from which it is being reset."
- Per-authenticator rules: passwords "**SHALL** implement a rate-limiting mechanism" (§3.1.1.2). OOB codes shorter than 64 bits need rate limiting, and "Generating a new authentication secret **SHALL NOT** reset the failed authentication count" (§3.1.3). Saved recovery codes are subject to throttling (§4.2).
- Passwords (§3.1.1.2), for context:
  - Single-factor passwords ≥ 15 characters; passwords used only inside MFA ≥ 8.
  - A blocklist check **SHALL** be done.
  - No composition rules (**SHALL NOT**), no periodic rotation (**SHALL NOT**).
  - Password managers and autofill **SHALL** be allowed.
  - Salted password hashing **SHALL** be used.

**ASVS:**

- **V6.1.1 (L1):** "application documentation defines how controls such as rate limiting, anti-automation, and adaptive response, are used to defend against attacks such as credential stuffing and password brute force. The documentation must make clear how these controls are configured and prevent malicious account lockout."
- **V6.3.1 (L1):** those controls are implemented as documented.
- **V6.3.8 (L3):** "valid users cannot be deduced from failed authentication challenges, such as by basing on error messages, HTTP response codes, or different response times. Registration and forgot password functionality must also have this protection."
- **V6.6.3 (L2):** rate-limit code-based OOB authentication. **V6.6.4 (L3):** rate-limit push notifications.
- **V2.4.1 (L2):** anti-automation against excessive calls in general ([V2](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x11-V2-Validation-and-Business-Logic.md)).
- ASVS sets **no numeric lockout threshold**.

*Inference for a deployment behind a reverse proxy:* IP-based throttling depends on a trustworthy client address. If the proxy adds nothing but TLS, `X-Forwarded-For` is either absent or untrusted. That makes per-account counters, which NIST §3.2.2 is written around, the dependable control. Per-IP limits only work if a trusted-proxy setting exists.

---

## Contradictions and tensions

- **Session secret entropy.** NIST §5.1 asks for ≥ 64 bits; ASVS V7.2.3 and V11.5.1 ask for ≥ 128 bits; the OWASP cheat sheet says ≥ 64 is the minimum, ≥ 128 when generating your own, and prefers 160. These don't conflict, since meeting 128 satisfies all three. The strictest is ASVS.
- **Timeouts.** NIST gives numeric SHOULDs (AAL2: 24 h overall and 1 h idle; AAL3: 12 h overall, a SHALL, and 15 min idle). ASVS only requires documented, risk-based values and a justification for deviating from NIST (V7.1.1). The OWASP cheat sheet suggests 2–5 min idle for "high-value" applications, which is stricter than NIST AAL3's 15 min. All three diverge from daily phone use and an always-on wall tablet; how to reconcile this is a project decision.
- **Persistent sessions.** NIST says bearer session secrets **SHOULD NOT** survive an app restart or reboot (§5.1). Real phone and tablet use usually wants a persistent login. NIST allows persistence only for proof-of-possession sessions (DBSC) and still requires the timeouts to be enforced.
- **Attestation at the top level.** The ASVS V6.3 text describes L3 as "hardware-based authentication, performed in an attested and trusted execution environment". NIST Appendix B says missing attestation **SHOULD NOT** block syncable authenticators in public-facing applications, and attestation is mainly a federal-enterprise SHOULD (§3.2.4). The two standards target different levels (ASVS L3 vs NIST AAL2 for synced passkeys), so this is a tension, not a direct conflict.
- **FIPS 140 at AAL3.** §2.3.2 says AAL3 authenticators "**SHALL** be validated to meet … FIPS140 Level 1", with no federal qualifier. Table 1 labels the FIPS row "(Government Verifiers and Authenticators)". Whether a non-government RP must check FIPS validation to claim AAL3 is ambiguous. An RP can only learn this through attestation.
- **Static API keys.** ASVS V13.2.1 rejects "unchanging credentials such as … API keys" for backend components, while user-issued long-lived program tokens are common practice and not addressed directly by either standard (see Missing evidence).

## Missing evidence / not verified

- No ASVS 5.0 or NIST 800-63B-4 requirement specifically addresses **user-issued API tokens for third-party programs**: their format, hashed storage, "shown once", maximum lifetime or scope model. The statements above are analogies (V6.5.2, V11.5.1, V13.3.4) or industry practice. NIST 800-63 is about authenticating human subscribers; non-person credentials are out of its scope (*researcher inference from the document scope; not quoted*).
- "Shown once at creation" was not verified in a primary source in this run.
- The exact ASVS 5.0.0 release date came from a search-engine summary of the GitHub README, not a direct read. CSRC gives SP 800-63B-4 as "July 2025". Some secondary sources say August 2025; this report did not reconcile the two.
- NIST SP 800-63C-4 (federation) was not read beyond the cross-references in 63B §5.2. It matters if optional OIDC is added (ASVS V6.8, V7.6, V10 OAuth/OIDC were also not reviewed in depth).
- The DBSC specification status and browser support were not checked; NIST only cites it as "emerging".
- WebAuthn Level 3 flag semantics were taken from NIST Appendix B, not from the W3C specification itself.
- `source_check` was not run; every quote above comes from directly fetched primary text (NIST HTML pages and the ASVS `v5.0.0` Markdown files).

## Open points for the decision

1. **Target level per surface.** Possible targets: ASVS L2 or L3 overall; NIST AAL2 everywhere, with AAL3 only for admin or sensitive actions; or something else. This sets whether synced passkeys are enough (AAL2, ASVS L2) or device-bound hardware keys are needed for some roles (AAL3, ASVS L3 V6.3.3).
2. **Phishing-resistant option.** NIST AAL2 requires one to be *offered*. Choices: passkeys only, passkeys plus a password+TOTP fallback, or passwords allowed at all for household members. Also: should the BE/BS flags restrict anything? NIST advises against conditioning on BS for public-facing applications.
3. **Timeout policy and the wall tablet.**
   - Options: follow NIST AAL2 numbers (24 h / 1 h); document deviations under V7.1.1 (e.g. longer absolute lifetimes for a low-privilege "kiosk/tablet" role); or use different numbers per role.
   - Related choice: rely on the NIST AAL2 "password or biometric plus session secret" reauthentication after idle.
   - Whether persistent cookies are acceptable even though NIST §5.1 says bearer secrets SHOULD NOT persist, and whether to track DBSC.
4. **Which actions count as "sensitive"** for V7.5.1 (account and factor changes, full reauthentication) and V7.5.3 (L3, at least one factor). Candidates: managing users and roles, creating API tokens, editing Code Steps or Automations, changing security-relevant settings. Also how fresh `auth_time` must be.
5. **Stateful vs self-contained sessions.**
   - Requirements that push toward server-side state: V7.4.1/7.4.2/7.4.5, V7.5.2 and V8.3.2. Whether to store a hashed verifier (cheat sheet) rather than the raw token, and the concurrent-session policy (V7.1.2).
   - The SSE stream: `EventSource` cannot set an `Authorization` header (*inference; not verified in this run*), and V14.2.1 rules out tokens in the URL. Together these point to cookie authentication for `/api/updates`. How should long-lived SSE connections react to session expiry or revocation?
6. **CSRF approach.** NIST §5.1 requires a verified session identifier in POST/PUT content. ASVS V3.5.1/3.5.2 accept anti-forgery tokens *or* non-safelisted headers *or* CORS preflight with Origin and Content-Type checks. `SameSite` alone is not enough under the cheat sheet. Also decide between `SameSite=Strict` and `Lax`.
7. **API token design** (no normative source; project choice):
   - prefix and checksum format (GitHub-style) for secret scanning;
   - ≥ 128-bit randomness (V11.5.1);
   - SHA-256-hashed storage (by analogy with V6.5.2);
   - mandatory or default expiry, and whether "never expires" is allowed (V13.3.4 at L3 leans toward expiry and rotation);
   - scope model mapped to the coarse roles (V8.2.x);
   - display once;
   - per-token revocation and a last-used timestamp;
   - tokens never satisfy reauthentication or step-up (NIST §5.1.2);
   - header-only transport (V14.2.1, RFC 6750 §2.1).
8. **Rate-limit and lockout design.** A per-account, per-authenticator failure counter with a hard cap ≤ 100 that disables the authenticator (NIST §3.2.2) needs a recovery path. Progressive delays avoid malicious lockout (V6.1.1). Decide whether per-IP limits exist at all, given the proxy may not supply a trustworthy client IP. Responses must not reveal which usernames exist (V6.3.8, L3).
9. **Notifications.** NIST requires independent notification on authenticator binding and on account recovery (§4.1.2, §4.2, §4.6, which asks for ≥ 2 notification addresses), and ASVS asks for them at L3 (V6.3.5, V6.3.7). A self-hosted hub may have no email or SMS channel, so how would these notifications be delivered?
10. **Admin and household recovery.** NIST disables authenticators after too many failures and requires re-binding. ASVS V6.4.6 (L3) says admins can start a reset but cannot choose the password. How does a non-technical household recover without weakening AAL?

```acceptance-report
{
  "criteriaSatisfied": [
    {
      "id": "criterion-1",
      "status": "satisfied",
      "evidence": "Report written to /tmp/wf-auth/research/auth-session-token-standards.md with Key findings, Details (AALs/passkeys, timeouts/reauth/step-up, binding/revocation, cookies/JWT, API tokens, rate limiting), Contradictions, Missing evidence, Open points. Every NIST/ASVS requirement quoted from directly fetched primary text (pages.nist.gov 800-63-4 HTML; OWASP/ASVS v5.0.0 markdown) with section/requirement IDs."
    }
  ],
  "changedFiles": [
    "/tmp/wf-auth/research/auth-session-token-standards.md"
  ],
  "testsAddedOrUpdated": [],
  "commandsRun": [
    {
      "command": "fetch_content / get_search_content on NIST SP 800-63B-4 pages and OWASP ASVS v5.0.0 chapter files",
      "result": "passed",
      "summary": "Primary texts retrieved and quoted"
    },
    {
      "command": "source_check",
      "result": "not-run",
      "summary": "Claims verified by direct reading of fetched primary sources instead"
    }
  ],
  "validationOutput": [
    "All requirement quotes cross-checked against fetched primary source text; no secondary sources used for normative claims"
  ],
  "residualRisks": [
    "No normative source covers user-issued API tokens for programs; those recommendations are analogies or industry practice",
    "'Shown once' token practice not verified in a primary source",
    "ASVS 5.0.0 release date taken from a search summary of the README; NIST date per CSRC metadata (July 2025) may differ from secondary sources (August 2025)",
    "SP 800-63C-4 (federation), ASVS V10 (OAuth/OIDC), DBSC status and EventSource header limitation not reviewed in depth",
    "FIPS 140 applicability to non-government AAL3 claims is ambiguous in the source"
  ],
  "noStagedFiles": true,
  "diffSummary": "New research report file outside the repo; no repo changes",
  "reviewFindings": [
    "no blockers"
  ],
  "manualNotes": "Research only, no decisions made. Report contains no personal data. NIST has no numbered requirement IDs, so section numbers/anchors are cited instead."
}
```
