# A Program authenticates with one revocable bearer Token

A Program (ADR 0022), such as Node-RED or a script, calls the HTTP API and its event stream unattended, often from the same host or the home network, and an Automation built on it must not break overnight. A Program authenticates with a Token: an opaque random value sent as `Authorization: Bearer` on every request, which Oiko checks against its stored hash on every request, so it is revoked at once and the Program's Access level is read live (ADR 0025). Only an Admin, after step-up, creates a Program and its Token. A Program holds at most one Token. It never expires, and it is shown once.

## Considered Options

- **OAuth2 client credentials**: standard, but Oiko would become an authorization server and every script would need a refresh step. Short-lived access tokens buy nothing when Oiko checks every request server-side anyway.
- **Signed JWTs**: stateless, but revoking one needs a denylist and the Access level baked into it goes stale. Oiko is a single process, so being stateless gains nothing.
- **Request signing (HMAC)**: the secret never travels, but every client needs custom signing code, and TLS already protects the secret in transit.
- **Several named Tokens per Program**: rotation without downtime, at the cost of more state and UI for a gap of about a minute in a home. Two scripts are two Programs, which also keeps what each does attributed apart.
- **A grace period on rotation**: no downtime, but a leaked Token outlives its revocation.
- **An expiry chosen at creation, or after idle time**: better hygiene by NIST and ASVS standards, but a Token that expires silently breaks an Automation at night, or a script that runs once a season. The last use shown beside each Program makes forgotten ones stand out instead.
- **Retrievable Tokens, stored encrypted**: convenient, but whoever reads the data directory or a backup gets every Token.
- **Members creating Programs up to their own level**: no need to ask an Admin for a personal script, but Members would then manage access, which ADR 0023 keeps for Admins.
- **Tokens accepted only through the public HTTPS URL**: safer on the wire, but the home's automations would depend on the proxy, DNS and certificate, and a client can fake the Host header anyway.

## Consequences

- **Form.** A Token is at least 128 random bits behind a fixed `oiko_` prefix, so secret scanners catch one leaked into a repository or a Node-RED flow export. It is accepted only in the `Authorization` header, never in a URL, on every endpoint including the event stream. Oiko stores a SHA-256 hash of it. The Token's entropy makes a slow hash pointless.
- **Lifecycle.** Generating a Token shows it once and revokes the Program's previous one. Revoking leaves the Program without a Token, with its identity, Name and Access level intact, until a new one is generated. Removing the Program ends its Token. A lost Token is regenerated, never shown again.
- **Ended access.** Revoking or regenerating the Token, removing the Program, or changing its Access level closes its open event stream at once. A reconnect with a revoked Token is refused.
- **Shown to Admins.** Admins see each Program's Name and Access level, who created it and when, when its Token was generated and when it was last used (recorded at most hourly).
- **Any address.** A Token is accepted on every address Oiko listens on, loopback and the home network included: it, never the network, authenticates (ADR 0027). Sending it over plain HTTP across the home network exposes it, and the documentation says so.
- **Without a public URL.** ADR 0027 refuses requests when no public URL is configured because nobody can sign in. That refusal covers only requests without valid credentials: a request bearing a valid Token is served. A LAN-only install can still drive Oiko from Node-RED once an Admin, signed in on `localhost`, has created the Program.
- **Breaking.** Requiring a Token breaks every existing caller of the HTTP API, so it ships in a breaking release (ADR 0019).
