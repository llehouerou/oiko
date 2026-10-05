# Research: how comparable self-hosted apps authenticate dashboards and APIs

Scope: Home Assistant (HA), Immich, Jellyfin, Frigate, Vaultwarden, Pocket ID, and Authelia / authentik used as front doors. Research only: it lists options, trade-offs and evidence and makes no decision for Oiko.

Freshness: sources were read in early October 2026. The HA docs pages served version 2026.9.4. Version and advisory dates are given inline. Labels used below: **[direct]** means the cited source says it, **[interp]** is my reading of a source, **[inference]** is my own reasoning and not stated by any source.

---

## Key findings

1. **Many of these apps give the first admin account to the first visitor, and that window has been a real attack surface.** HA, Immich, Jellyfin and Pocket ID all serve an unauthenticated "create the first admin" endpoint until an account exists. Frigate takes a different route: it generates an admin password and prints it in the logs. Vaultwarden's admin page is protected by a token you set in configuration. authentik serves an unauthenticated `initial-setup` flow.
   - Incidents and CVEs on that window: authentik CVE-2023-46249 (deleting `akadmin` re-opened `initial-setup`, critical); HA CVE-2023-27482 (Supervisor paths left unauthenticated before onboarding, plus path traversal, CVSS 10); Jellyfin 12.0 (2026) fixed "the setup wizard being re-run on a misconfigured server without signing in."
   - Jellyfin's own tracker describes automated deployments "racing" an unauthenticated caller to create the admin.
2. **Recovery always falls back to someone with shell or file access on the host.** HA uses `auth reset` / `hass --script auth`. Immich uses `immich-admin reset-admin-password`. Pocket ID uses `pocket-id one-time-access-token`, which prints a link valid for 1 hour. authentik uses `ak create_recovery_key`. Frigate uses `reset_admin_password: true`, which prints a new password in the logs. Jellyfin writes a PIN file to the data directory. Vaultwarden's admin is just the configured token. HA's last resort for a lost owner password is a new onboarding, which wipes the device unless a backup exists.
3. **Ways to sign in.**
   - Passkeys are first-class only in Pocket ID (passkey-only), Authelia (passkeys since 4.39) and Vaultwarden (WebAuthn as a second factor).
   - HA has TOTP and a "notify" one-time code but no native passkeys; a WebAuthn login provider was not accepted.
   - Immich and Jellyfin have no built-in 2FA and send users to OIDC (Immich) or to plugins and SSO (Jellyfin). Users keep asking for native TOTP in both trackers.
4. **Trusted-network or LAN bypass exists in HA (`trusted_networks`), Authelia (`networks` + `bypass`), Jellyfin (LAN-only users and LAN-trusted endpoints) and Frigate (unauthenticated port 5000).** Each one keeps causing trouble:
   - HA warns that `trusted_networks` without the password provider means "No password is required" for anyone on the LAN. It also cannot trust the proxy's network, MFA is skipped, and refreshing the page with `allow_bypass_login` creates a new login every time.
   - Jellyfin CVE-2025-32012: a spoofed `X-Forwarded-For` made attackers look like LAN clients, allowing unauthenticated restarts and bypassing "remote access disabled".
   - Frigate users who published port 5000 to the internet had their config rewritten to run cryptominers (2025–2026 discussions). The maintainers' answer was "use authentication… prevent any unauthenticated ports from being accessible to the internet."
5. **Long-lived tokens for programs vary widely.**
   - HA: 10-year long-lived access tokens with the full rights of the user.
   - Immich: API keys with fine-grained permissions. The update endpoint once let a key grant itself `all` (GHSA-237r-x578-h5mv, Jan 2026).
   - Jellyfin: admin API keys with no scopes (open "security" issue #13992).
   - Frigate: no API keys at all. Users store a password or open port 5000, and one user asked for scoped long-lived tokens (discussion #19080).
   - Pocket ID: regular API keys plus `STATIC_API_KEY` for declarative setups.
   - Lesson from Immich GHSA-8244-8vpr-vp9c (2026, main-branch-only): an XSS was used to **mint a full API key** that "survives logout, password rotation, and session expiry".
6. **Default session lifetimes run from minutes to years.**

   | Product | Default lifetime |
   |---|---|
   | Authelia | 5 min inactivity, 1 h absolute, "remember me" 1 month |
   | Vaultwarden admin page | 20 min |
   | Pocket ID | 60 min |
   | Frigate | 24 h, refreshed while active |
   | HA | 30 min access tokens; refresh tokens removed after 90 days unused |
   | Immich | session cookie reported hardcoded at 400 days (2024 discussion) |
   | authentik | until the browser closes |

   Complaints go both ways: HA wall-tablet users get logged out with no clear cause, while Immich users say 400 days is absurd for a forgotten public device.
7. **Kiosk mode and guests are mostly unsolved.**
   - HA's only native kiosk is in the iOS Companion app (2026; it hides the sidebar via frontend 2025.2+). Everyone else uses Fully Kiosk plus a dedicated user or `trusted_networks`.
   - Native guest access is a long-standing HA feature request ("Guest Assistant").
   - The good patterns found: Immich public share links with expiry, password and per-link permissions; Jellyfin Quick Connect (a 6-character code approved from a logged-in device); Pocket ID expiring signup tokens and login codes; Frigate per-camera viewer roles (0.17).
8. **Cookie/CSRF and redirect bugs hit auth surfaces repeatedly.**
   - Pocket ID GHSA-252c-qmvj-75c9: cookies had no `SameSite`, there was no CSRF/Origin check, and the JSON binder ignored `Content-Type`, so a cross-site form could create an admin.
   - HA CVE-2023-41895: a `javascript:` redirect URI in `auth_callback`.
   - Immich: a `continue` redirect XSS (2026).
   - Pocket ID: an OTA-token login was accepted as passkey step-up.

---

## Details by product

### Home Assistant (docs version 2026.9.4)

**Onboarding**
- **[direct]** The onboarding user-creation endpoint `POST /api/onboarding/users` is declared with `requires_auth = False`. It refuses with "User step already done" once the step is done. I found no LAN/local restriction in that view. [views.py (dev)](https://raw.githubusercontent.com/home-assistant/core/dev/homeassistant/components/onboarding/views.py)
- **[direct]** "The first user created is designated as the *owner*." [Auth providers](https://www.home-assistant.io/docs/authentication/providers/)
- **[direct]** CVE-2023-27482 / GHSA-2j8f-h4mr-qr25 (CVSS 10): Supervisor proxy paths were deliberately unauthenticated before onboarding finished, and path traversal reached privileged Supervisor APIs. Fixed in Core 2023.3.2 / Supervisor 2023.03.3. [Advisory](https://github.com/home-assistant/core/security/advisories/ghsa-2j8f-h4mr-qr25), [blog](https://www.home-assistant.io/blog/2023/03/08/supervisor-security-disclosure/). *Confidence: medium. I read the search summary, not the full advisory text.*
- **[interp]** Like Immich and Jellyfin, a fresh HA exposed to the internet before onboarding is "first visitor becomes owner". The docs push users to onboard on the LAN. [Onboarding](https://www.home-assistant.io/getting-started/onboarding)

**Credentials**
- **[direct]** Built-in providers are `homeassistant` (username/password, hashed and salted), `trusted_networks` and `command_line` (an external script; can set group `system-admin` / `system-users` and `local_only`). [Providers](https://www.home-assistant.io/docs/authentication/providers/)
- **[direct]** MFA modules: TOTP (loaded automatically) and "notify" (an HMAC one-time password sent through a notify entity). [MFA](https://www.home-assistant.io/docs/authentication/multi-factor-auth/)
- **[direct, via summary]** There is no native passkey login. A 2025 community thread reports that an earlier WebAuthn auth-provider PR was not accepted. A Jan 2026 architecture discussion about WebAuthn selectors only concerns integration config flows. [Feature request](https://community.home-assistant.io/t/passkeys-support-such-as-yubico-or-windows-hello/849953), [discussion #2519](https://github.com/orgs/home-assistant/discussions/2519). *Confidence: medium.*
- **[direct]** Auto-ban: `http.ip_ban_enabled` / `login_attempts_threshold`. Behind a proxy it needs `use_x_forwarded_for` + `trusted_proxies`. [Community answer](https://community.home-assistant.io/t/wth-why-we-can-set-so-weak-passwords/816263/3). *Confidence: medium; this is a community source, see the HTTP integration docs.*

**Recovery**
- **[direct]** "There is only one owner per system. You cannot add a new owner."
- **[direct]** The available paths are:
  - another admin resets the password;
  - on the device console, `auth reset --interactive`;
  - in a container, `hass --script auth --config /config change_password`;
  - otherwise "start a new onboarding process", which erases data without a backup.

  [I'm locked out](https://www.home-assistant.io/docs/locked_out/)

**Trusted networks and LAN bypass**
- **[direct]** `trusted_networks` skips the password for listed CIDRs. `trusted_users` restricts which users can be picked per IP or network. `allow_bypass_login` jumps straight in when only one user can be picked.
- **[direct]** The docs' warnings:
  - "Using only `trusted_networks` without `type: homeassistant` … anyone on your trusted network can log in by selecting a username. No password is required."
  - "The multi-factor authentication module will not participate."
  - "You cannot trust a network that you are using in any trusted_proxies."
  - With bypass, "your cookie will not be stored and every time you refresh the page … a new login will be created."
  - A typo in `auth_providers` can lock everyone out, and "There is no way to recover through the UI."

  [Providers](https://www.home-assistant.io/docs/authentication/providers/)
- **[direct]** A user reports refresh tokens being created on every page reload with `trusted_networks`. [core#59523](https://github.com/home-assistant/core/issues/59523)
- **[direct]** The per-user "Can only log in from the local network" (`local_only`) setting locked an admin out ("Login blocked: User cannot authenticate remotely"). The thread notes HA treats IPv6 addresses as outside the local network. [Community thread, 2022–2025](https://community.home-assistant.io/t/login-blocked-user-cannot-authenticate-remotely/449696)
- **[interp]** I found no CVE where `trusted_networks` itself was bypassed. The recorded problems are misconfiguration: a proxy or broad subnet trusted, the fallback provider missing, IPv6 not counted as local.

**Tokens for programs**
- **[direct]** OAuth2-style flow: the access token has `expires_in: 1800`, the refresh token is revocable via `/auth/revoke`, and "Long-lived access tokens are valid for 10 years". A WebSocket command can create them with a custom `lifespan`, and the token string is not stored server-side. Signed paths (`authSig`) default to 30 s. [Auth API](https://developers.home-assistant.io/docs/auth_api/)
- **[direct]** "Unused refresh tokens are automatically removed … if it has not been used to sign in within 90 days." Users can delete refresh tokens per device. When changing a password, the user is asked whether to sign out other sessions. [Authentication](https://www.home-assistant.io/docs/authentication/)
- **[interp]** Long-lived tokens carry the full rights of the user that created them; I found no per-token scopes.

**Kiosk and wall tablet**
- **[direct]** Native kiosk exists only in the iOS Companion app (requires iOS/iPadOS 2026.7+). It can hide the sidebar and top bar (needs HA 2025.2+), protects its settings with Face ID or a passcode, and the docs recommend iOS Guided Access. Android users are pointed to the "Home App (launcher)" feature. [iOS Kiosk mode](https://companion.home-assistant.io/docs/integrations/ios-kiosk-mode/)
- **[direct, complaint]** Several wall tablets on Fully Kiosk with "keep me logged in" ticked still drop back to the login screen. One poster sees it nightly (2024–2026); there is no resolution in the thread. [Community](https://community.home-assistant.io/t/user-logged-out-and-not-automatically-logged-back-in/714452)
- **[interp]** The common community setup is one HA user per tablet, plus `trusted_users` and `allow_bypass_login` for that tablet's IP, plus the HACS "kiosk-mode" plugin. [kiosk-mode](https://github.com/NemesisRE/kiosk-mode/)

**Guests**
- **[direct]** There is no native time-limited guest access. The request has been open for years ("Guest Assistant", WTH 2022). [Community](https://community.home-assistant.io/t/wth-feature-request-guest-assistant/471141)

**Other auth-relevant advisories**
- **[direct]** GHSA-jvxq-x42r-f7mv / CVE-2023-41895: a `javascript:` redirect URI discovered from the `client_id` page in `auth_callback` allowed account takeover; fixed in 2023.9.0 (Cure53 audit). [Advisory](https://github.com/home-assistant/core/security/advisories/GHSA-jvxq-x42r-f7mv). *Confidence: medium, from the search summary.*
- **[direct]** GHSA-68f4-97mf-f68w (Jul 2026): an Android Companion `homeassistant://invite` deep link started onboarding against an attacker server without showing its URL, enabling credential phishing. Fixed in 2026.6.1. [Advisory](https://github.com/home-assistant/core/security/advisories/GHSA-68f4-97mf-f68w)
- **[direct]** GHSA-7jp2-p2fw-mgvf (May 2026): cross-origin iframe access-token exfiltration through the Companion WebView JS bridge. [Advisory list](https://github.com/home-assistant/core/security/advisories)

### Immich

**Onboarding**
- **[direct]** "The first account registered becomes the Immich administrator." [User management](https://docs.immich.app/administration/user-management/)
- **[direct]** The API exposes `GET /api/server/config` → `isInitialized` and an unauthenticated `POST /api/auth/admin-sign-up` while the server is uninitialized; the PoC in GHSA-237r uses exactly this. [GHSA-237r-x578-h5mv](https://github.com/immich-app/immich/security/advisories/GHSA-237r-x578-h5mv)

**Credentials**
- **[direct]** Password login and OAuth/OIDC, with auto-register, auto-launch, a mobile redirect override, and an option to disable password login. [OAuth](https://docs.immich.app/administration/oauth/)
- **[direct]** No native 2FA. Requests keep coming back (#8175, #12023, #14309, #16933, and #23339 from Oct 2025: "Devs, please listen to us. Please add the ability for TOTP 2FA."). [#23339](https://github.com/immich-app/immich/discussions/23339), [#8175](https://github.com/immich-app/immich/discussions/8175)

**Recovery**
- **[direct]** The `immich-admin` CLI inside the container provides `reset-admin-password` (it asks "Invalidate existing sessions?"), `enable-password-login`, `disable-oauth-login`, `grant-admin` and `revoke-admin`. [Server commands](https://docs.immich.app/administration/server-commands/)

**Tokens for programs**
- **[direct]** API keys with fine-grained permissions such as `asset.read` and `album.create`. [CLI docs](https://docs.immich.app/features/command-line-interface/), [API docs](https://immich.app/blog/immich-api-documentation)
- **[direct]** GHSA-237r-x578-h5mv (High, Jan 2026): `update` did not check that the calling key held the permissions it granted, so a key could escalate itself to `all`. [Advisory](https://github.com/immich-app/immich/security/advisories/GHSA-237r-x578-h5mv)
- **[direct]** GHSA-8244-8vpr-vp9c (Critical, Jun 2026, main-branch builds only, no tagged release): an XSS through the `continue` parameter on the login page could `POST /api/api-keys` with `all` permissions. The advisory notes `HttpOnly` cookies did not help because the injected script just used them same-origin. The resulting key "Survives logout, password rotation, and session expiry". [Advisory](https://github.com/immich-app/immich/security/advisories/GHSA-8244-8vpr-vp9c)

**Sessions**
- **[direct, complaint]** "the session cookie for Immich is hardcoded to have a lifetime of 400 days … an absurdly large amount of time." The request asks for "remember me" and for OIDC lifetimes to be honored. [Discussion #13318](https://github.com/immich-app/immich/discussions/13318)
- **[direct, via summary]** OIDC back-channel logout was added in PR #26235 (merged Apr 2026). [PR](https://github.com/immich-app/immich/pull/26235). *Confidence: medium.*
- **Unverified:** whether the 400-day value is still in place today.

**Guests**
- **[direct]** Public shared links use a random URL that acts as the secret and offer "an expiration date, password protection, allow what actions can be performed". Shared albums give other users editor or viewer rights. [Sharing](https://docs.immich.app/features/sharing)
- **[direct]** A shared-link authorization bypass (GHSA-hvq7-hq9r-8gjr) let a link holder add the owner's other assets into the link. [Advisory](https://github.com/immich-app/immich/security/advisories/GHSA-hvq7-hq9r-8gjr). *Confidence: medium, from the summary.*

### Jellyfin (12.0 released 2026)

**Onboarding**
- **[direct]** Issue #17880 (Sep 2026): the `/Startup/*` endpoints "are reachable without authentication while `IsStartupWizardCompleted` is false … every automated deployment has a window where an unauthenticated caller could create the administrator account". It proposes a non-interactive `--provision-file` mode. [#17880](https://github.com/jellyfin/jellyfin/issues/17880)
- **[direct]** Jellyfin 12.0 security fixes "prevent the setup wizard being re-run on a misconfigured server without signing in". 12.0 also disables the deprecated authorization mechanisms by default. [12.0 post](https://jellyfin.org/posts/jellyfin-release-12.0/)

**Credentials**
- **[direct]** Password login, with optional LDAP and other providers through plugins. Per user: "Allow remote connections" (unchecked means LAN-only logins), a lockout after failed attempts (default 3 for non-admins, 5 for admins; an admin unlocks by re-enabling the user), a device allow-list, and access schedules. [Managing users](https://jellyfin.org/docs/general/server/users/adding-managing-users/)
- **[direct, via summary]** 2FA: feature request #26 is "Planned" but has no timeline; issue #1215 is related. [Issue #1215](https://github.com/jellyfin/jellyfin/issues/1215). *Confidence: medium.*

**Recovery**
- **[interp]** A "forgot password" request writes a PIN file to the server's data directory. GHSA-qcmf-gmhm-rfv9 implies the reset request is limited to the LAN: spoofing a LAN IP allowed attackers to "request password resets (which has no impact unless they can read the created PIN file)". [Advisory](https://github.com/jellyfin/jellyfin/security/advisories/GHSA-qcmf-gmhm-rfv9)

**LAN trust and its incident**
- **[direct]** CVE-2025-32012 / GHSA-qcmf-gmhm-rfv9 (fixed in 10.10.7, Apr 2025):
  - `/System/Restart` was admin-only but also authorized "any device in the same local network".
  - In default configurations, `X-Forwarded-For` could be spoofed to look like a LAN IP, giving unauthenticated DoS and bypassing remote-access-disabled settings and remote bitrate limits.
  - The fix: "`X-Forwarded-For` is now not explicitly trusted when nothing is entered in the Trusted Proxies field".

  [Advisory](https://github.com/jellyfin/jellyfin/security/advisories/GHSA-qcmf-gmhm-rfv9)

**Tokens for programs**
- **[direct]** "Currently the apikey can only grant all perms of the user it belongs to." Issue #13992 is labelled `security` and is still open in 2026. [#13992](https://github.com/jellyfin/jellyfin/issues/13992)

**Second devices and TVs**
- **[direct]** Quick Connect: the new device shows a 6-character code, and an already signed-in device enters it under Settings → Quick Connect. It is enabled by default and can be turned off server-wide. [Quick Connect](https://jellyfin.org/docs/general/server/quick-connect/)

**Sessions**
- **Unverified:** that Jellyfin access tokens have no time-based expiry. A search summary claimed it, but the Kotlin SDK guide only says "the server grants a token … used in future API requests". [SDK guide](https://kotlin-sdk.jellyfin.org/guide/authentication.html)

### Frigate (0.14 added auth; 0.17 current)

**Onboarding**
- **[direct]** "On startup, an admin user and password are generated and printed in the logs." On a new install, the setup wizard's first step offers to change it. [Authentication docs (dev)](https://github.com/blakeblackshear/frigate/blob/dev/docs/docs/configuration/authentication.md)

**Recovery**
- **[direct]** Set `auth.reset_admin_password: true` (or the UI toggle), and the admin password is reset and printed to the logs at the next start. [Docs](https://github.com/blakeblackshear/frigate/blob/dev/docs/docs/configuration/authentication.md)

**Credentials**
- **[direct]** Local passwords only: PBKDF2-SHA256 with 600,000 iterations, minimum length 12. 0.17 tightened the policy. [0.17 release](https://github.com/blakeblackshear/frigate/releases/tag/v0.17.0)
- **[direct]** Login failures are rate-limited with SlowApi (e.g. `1/second;5/minute;20/hour`). The limits reset on restart, and behind a proxy `trusted_proxies` is needed or "a brute force attack will rate limit login attempts from other devices and could temporarily lock you out".
- **[direct]** No native OIDC, SAML or LDAP. Frigate instead accepts proxy headers (`header_map`, `role_map`, `default_role`, optional `X-Proxy-Secret`) and has a fixed allow-list of trusted header names.

**Ports and the bypass incident**
- **[direct]** Port 8971 is authenticated. Port 5000 is "Internal unauthenticated UI and API access" and gives anonymous requests admin rights.
- **[direct, incidents]** Users who published port 5000 to the internet found miners and tor running out of `/config` (#21786), `config.yml` rewritten to fetch and run xmrig (#21897), and an injected `trigger_exec` stream (#22820). The maintainer replied: "ensure you use authentication … and prevent any unauthenticated ports from being accessible to the internet." [#21786](https://github.com/blakeblackshear/frigate/discussions/21786), [#21897](https://github.com/blakeblackshear/frigate/discussions/21897), [#22820](https://github.com/blakeblackshear/frigate/discussions/22820)
- **Unverified:** a 0.17 beta report says port 5000 prompted for login (#21915), with no maintainer answer captured. Current docs still describe 5000 as unauthenticated. [#21915](https://github.com/blakeblackshear/frigate/discussions/21915)

**Sessions and tokens**
- **[direct]** A JWT is stored in a cookie or sent as `Authorization: Bearer`. `session_length` defaults to 86400 s and is refreshed while in use. Tokens are invalidated on password change, and rotating the JWT secret invalidates all of them.
- **[direct]** A request for revocation on user deletion was answered with "use short `session_length` + `refresh_time`". [#12756](https://github.com/blakeblackshear/frigate/issues/12756)
- **[direct, complaint]** There are no API keys. One user's options were storing a full account's password, opening port 5000, adding their own proxy auth, or "Wildly increasing the session_length … for all users", and they asked for scoped long-lived tokens. [#19080](https://github.com/blakeblackshear/frigate/discussions/19080)

**Roles and guests**
- **[direct]** Roles are `admin`, `viewer`, and custom roles limited to selected cameras (0.17). Restrictions are enforced server-side on 8971. [Docs](https://github.com/blakeblackshear/frigate/blob/dev/docs/docs/configuration/authentication.md), [0.17](https://github.com/blakeblackshear/frigate/releases/tag/v0.17.0)

### Vaultwarden (1.37.3, Sep 2026, per releases page summary)

**Onboarding and admin**
- **[direct]** The admin panel is enabled only when `ADMIN_TOKEN` is set. The docs recommend an argon2id PHC hash (`vaultwarden hash`). The admin session is a JWT that lasts 20 min by default (`ADMIN_SESSION_LIFETIME`). "Changing the session lifetime or even the admin token itself won't affect currently logged in users"; to invalidate, delete `rsa_key.pem` and restart. `DISABLE_ADMIN_TOKEN` exists but is discouraged. [Enabling admin page](https://github.com/dani-garcia/vaultwarden/wiki/Enabling-admin-page)
- **[direct, via summary]** User signups are open by default (`SIGNUPS_ALLOWED=true`). `INVITATIONS_ALLOWED` controls org invites. [Disable registration](https://github-wiki-see.page/m/dani-garcia/vaultwarden/wiki/Disable-registration-of-new-users). *Confidence: medium.*

**Credentials**
- **[direct, via summary]** Second factors: TOTP, FIDO2 WebAuthn, YubiKey OTP, Duo and email, plus a recovery code. [README](https://github.com/dani-garcia/vaultwarden/blob/main/README.md)
- **[direct]** OIDC SSO was merged in PR #3899 (`SSO_ENABLED`, `SSO_ONLY`). The master password is still required: "A master password is still required and not controlled by the SSO". [PR #3899](https://github.com/dani-garcia/vaultwarden/pull/3899)

**Advisories**
- **[direct, via summary]**
  - GHSA-f7r5-w49x-gxm3: CSRF to `/admin`, only when `DISABLE_ADMIN_TOKEN` is on; fixed in 1.33.0.
  - GHSA-v6pg-v89r-w8wr / CVE-2026-27801: 2FA bypass on protected actions such as fetching the API key; fixed in 1.35.0.
  - GHSA-c5rv-q295-7w4g: the email-2FA path worked as a password-validity oracle that bypassed the login rate limit; fixed in 1.35.4.

  [Advisories](https://github.com/dani-garcia/vaultwarden/security/advisories). *Confidence: medium; I did not open each one.*

### Pocket ID (passkey-only OIDC provider; v2.x)

**Onboarding**
- **[direct]** "Open `https://id.example.com/setup` … The setup page only works until the first account exists." The admin then adds a passkey. [Installation](https://pocket-id.org/docs/setup/installation). The API behind it is `POST /api/signup/setup`, which returns the session cookie immediately ([GHSA-252c PoC](https://github.com/pocket-id/pocket-id/security/advisories/GHSA-252c-qmvj-75c9)).

**Credentials and recovery**
- **[direct]** Users sign in with passkeys only. New users get a **login code**: one-time, short-lived, shown as a code, QR or link. "A login code is as good as a passkey until it expires." [User management](https://pocket-id.org/docs/setup/user-management)
- **[direct]** If the admin passkey is lost, `pocket-id one-time-access-token <user>` prints a link valid for 1 hour. "Anyone with the link can sign in as that user." [Account recovery](https://pocket-id.org/docs/troubleshooting/account-recovery)
- **[direct]** `EMAIL_ONE_TIME_ACCESS_AS_UNAUTHENTICATED_ENABLED` (default false): "Anyone with access to their email can then sign in as them."
- **[direct]** Other relevant settings: `WEBAUTHN_USER_VERIFICATION` defaults to `required`; `SESSION_DURATION` defaults to 60 minutes; `TRUST_PROXY` defaults to false; `ALLOW_USER_SIGNUPS` is `disabled` / `withToken` / `open`. [Env vars](https://pocket-id.org/docs/configuration/environment-variables)

**Guests and onboarding others**
- **[direct]** Signup tokens with an expiry and a maximum number of uses, which admins can list and revoke. [User management](https://pocket-id.org/docs/setup/user-management)

**Tokens for programs**
- **[direct]** Regular API keys, plus `STATIC_API_KEY`, which has admin rights and is meant for declarative setups. The docs say "Prefer regular API keys where you can." [Env vars](https://pocket-id.org/docs/configuration/environment-variables)

**Advisories (2026)**
- **[direct]** GHSA-252c-qmvj-75c9: cookies were set without `SameSite`, there was no CSRF/Origin/`Sec-Fetch-Site` check, Gin's JSON binder ignored `Content-Type`, and `isAdmin` was client-controlled. A cross-site `text/plain` form could therefore create an admin user (tested on v2.12.0). [Advisory](https://github.com/pocket-id/pocket-id/security/advisories/GHSA-252c-qmvj-75c9)
- **[direct]** GHSA-hp74-gm6m-2qm5: passkey step-up only checked that the JWT was less than 60 s old, so a login through a one-time token or signup token satisfied "re-authenticate with passkey". The `session` cookie was checked for presence only. [Advisory](https://github.com/pocket-id/pocket-id/security/advisories/GHSA-hp74-gm6m-2qm5)
- **[direct, via summary]** GHSA-w6p7-2fxx-4f44: the refresh-token flow ignored revocation, disabled accounts and group limits. [Advisory](https://github.com/pocket-id/pocket-id/security/advisories/GHSA-w6p7-2fxx-4f44)

### Authelia and authentik (front doors)

**Authelia**
- **Onboarding [direct, via summary]:** no wizard. Users live in a YAML/TOML/JSON file with hashed passwords (argon2id by default) or in LDAP. Password reset goes through identity validation (an HMAC-signed JWT, usually sent by email). [File backend](https://www.authelia.com/configuration/first-factor/file/), [Reset password](https://www.authelia.com/configuration/identity-validation/reset-password/)
- **Session defaults [direct]:** `inactivity` 5 minutes, `expiration` 1 hour, `remember_me` 1 month, `same_site` lax. "The Passkey Login Flow currently requires remembering sessions if enabled." [Session](https://www.authelia.com/configuration/session/introduction/)
- **Network rules [direct]:** `networks` "matches against the first address in the `X-Forwarded-For` header, or if there are none … the TCP source IP"; the docs say to configure the proxy correctly. `bypass` "skips all authentication" and cannot be combined with `subject`. The recommended default policy is `deny`. [Access control](https://www.authelia.com/configuration/security/access-control/)
- **Passkeys [direct]:** 4.39 added passkeys and passwordless login, treated as non-MFA by default. [4.39 notes](https://www.authelia.com/blog/4.39-release-notes/)
- **Trusted headers [direct, via summary]:** `Remote-User` and similar headers are unsigned. They are safe only when the backend is reachable solely through the proxy and the proxy strips client copies. Authelia recommends OIDC otherwise. [Threat model](https://www.authelia.com/overview/security/threat-model/)

**authentik**
- **Onboarding [direct]:** visit `/if/flow/initial-setup/` to set the password of `akadmin`. [Install docs](https://github.com/goauthentik/authentik/blob/9a974f14/website/docs/install-config/install/kubernetes.md)
- **CVE-2023-46249 / GHSA-rjvp-29xq-f62w [direct]:** "when the default admin user has been deleted … possible … to set the password of the default admin user without any authentication", because `initial-setup` became available again. Fixed in 2023.8.4 / 2023.10.2. [Advisory](https://github.com/goauthentik/authentik/security/advisories/GHSA-rjvp-29xq-f62w)
- **Recovery [direct]:** `ak create_recovery_key 10 akadmin` prints a link. "This recovery key will give whoever has the link direct access to your instances." [Troubleshooting](https://docs.goauthentik.io/troubleshooting/login/)
- **Sessions [direct, via summary]:** the User Login stage defaults to `session_duration: seconds=0` (browser session) and `remember_me_offset: seconds=0`, so "remember me" is hidden. [User login stage](https://docs.goauthentik.io/add-secure-apps/flows-stages/stages/user_login/)
- **Programs [direct, via summary]:** service accounts with API tokens (Bearer) and app passwords; tokens can expire. [Service accounts](https://docs.goauthentik.io/sys-mgmt/service-accounts/)

---

## Comparison table

| | First admin | Recovery | 2FA / passkeys | LAN bypass | Program tokens | Default session | Guest / temporary |
|---|---|---|---|---|---|---|---|
| Home Assistant | First visitor (unauthenticated onboarding API) | Console `auth reset`, container `hass --script auth`, else re-onboard | TOTP, notify OTP; no passkeys | `trusted_networks`, `allow_bypass_login`, per-user `local_only` | Long-lived 10 y, full user rights | Access 30 min; refresh dropped after 90 d unused | None native |
| Immich | First visitor (`admin-sign-up`) | `immich-admin reset-admin-password` | None native; OIDC | None found | API keys with permissions | Reported 400 d (2024) | Share links with expiry, password, permissions |
| Jellyfin | First visitor (`/Startup/*` until done) | PIN file on disk, request LAN-gated [interp] | None native (planned) | LAN-only users; LAN-trusted endpoints (CVE-2025-32012) | Admin API keys, no scopes | Unverified | Quick Connect code |
| Frigate | Generated password in logs | `reset_admin_password` → logs | None; proxy headers | Port 5000 unauthenticated, admin rights | None (login JWT) | 24 h sliding | Viewer / per-camera roles |
| Vaultwarden | Admin = `ADMIN_TOKEN` in config; open user signup by default | Edit config | TOTP, WebAuthn, YubiKey, Duo, email | None found | Personal API key (not verified here) | Admin 20 min | Not covered |
| Pocket ID | First visitor `/setup` | CLI one-time link (1 h) | Passkeys only | None | API keys, `STATIC_API_KEY` | 60 min | Signup tokens, login codes |
| Authelia | Config file | Email reset / edit file | TOTP, WebAuthn, passkeys, Duo | `networks` + `bypass` | n/a (front door) | 5 m / 1 h / remember 1 M | n/a |
| authentik | First visitor `initial-setup` | `ak create_recovery_key` | Many | Policy-based (not researched) | API tokens, app passwords | Browser session | n/a |

---

## Contradictions and disputed points

- **Session length: short versus forever.** Authelia and Pocket ID default to an hour or less, and Immich users call 400 days "absurd". At the same time, HA wall-tablet owners complain about being logged out at all. **[inference]** The sources point to two separate needs: short sessions for personal browsers and durable device credentials for kiosks. No product solves both cleanly.
- **Frigate port 5000.** Current docs say it is unauthenticated, while a 0.17 beta report says it prompted for login. This is unresolved.
- **HA passkeys.** I found no official statement of refusal, only a community report that a PR was not accepted.

## Missing evidence or not verified

- Jellyfin token expiry; the exact Jellyfin password-reset flow (PIN-file contents, the LAN check in code).
- Whether Immich's 400-day cookie is still current in v2.x/v3.
- Vaultwarden advisories and default signup behavior were read through search summaries, not full pages.
- authentik network or policy-based bypass was not researched.
- Whether HA onboarding has any other guard (e.g. in the frontend or Supervisor). I checked only `views.py`.
- Reddit sources (Frigate PSA, HA incidents) could not be fetched. Only the GitHub discussions are cited.
- No quantitative user-satisfaction data; the praise and complaints above are anecdotal, from threads and issues.

---

## Open points for the decision

1. **How to protect the first-admin window.** The options seen are:
   - first visitor wins (HA, Immich, Jellyfin, Pocket ID; all have been raced or abused);
   - a generated password or setup code printed in the logs (Frigate);
   - a secret set in configuration (Vaultwarden);
   - a CLI or provisioning file (Jellyfin proposal #17880).

   Decide whether the setup endpoint must also require proof of host access, such as a code in the logs, so it stays safe behind a TLS-only proxy before setup is done.
2. **Recovery path for a lost admin.** Every peer relies on host or shell access: a CLI one-time link (Pocket ID, authentik), a log-printed reset (Frigate), or editing files. Decide which, and whether it also revokes sessions (Immich asks).
3. **Credential set.** Passkeys-first (Pocket ID) versus password + TOTP (HA) versus delegating to OIDC (Immich). Decide on recovery codes and on onboarding by one-time login code, the pattern Pocket ID uses for non-technical users.
4. **Whether to trust the LAN at all.** The evidence points to the same failure every time: client IP derived from `X-Forwarded-For` without an explicit trusted-proxy list (Jellyfin CVE), the proxy network trusted (HA forbids this), an unauthenticated port exposed (Frigate). Behind a user-run proxy, every client arrives from the proxy IP. Decide whether to offer any IP-based trust and, if so, require explicit `trusted_proxies` with no default.
5. **Wall tablet and kiosk credential.** Options: a per-device long-lived credential bound to a restricted role (HA community pattern), a pairing code approved from a logged-in phone (Jellyfin Quick Connect), or IP trust. Users complain about random logouts, so durability and revocability per device matter.
6. **Guests.** Options seen: expiring share links with an optional password and per-link permissions (Immich), expiring and usage-limited signup tokens (Pocket ID), restricted viewer roles (Frigate). HA's lack of native guests is a long-standing complaint.
7. **API tokens for programs.**
   - Scoped versus full user rights (Jellyfin and HA are full rights and Jellyfin has an open security issue about it; Frigate has none and users complain).
   - Expiry: HA defaults to 10 years.
   - Whether creating a token needs fresh re-authentication: the Immich XSS minted persistent keys, and the Pocket ID step-up was bypassed.
   - Whether a token may change its own scopes: Immich GHSA-237r says no.
8. **Session defaults and "remember me".** Choose absolute and idle timeouts per client type, and decide whether a password change or role change revokes sessions (Frigate users asked for this, Immich and HA prompt for it).
9. **Browser-surface hardening that peers got wrong.** Set `SameSite` explicitly, add a CSRF / `Origin` / `Sec-Fetch-Site` check, enforce `Content-Type` on JSON (Pocket ID), and allow-list redirect and `continue` URLs (HA 2023, Immich 2026).
10. **Optional OIDC and front-door header trust.** If proxy headers are ever accepted, peers require a shared secret (Frigate `X-Proxy-Secret`) or a strictly isolated backend (Authelia threat model). Decide whether Oiko accepts headers at all or only OIDC.
