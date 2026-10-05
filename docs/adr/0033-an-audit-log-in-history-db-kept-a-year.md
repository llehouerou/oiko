# An Audit log in `history.db`, kept a year, read on request

Sign-in, Sessions, Sign-in links, Kiosk pairing and Tokens need a trail: the identity documents delete a Session or a Sign-in link when it ends (ADR 0032), so without a trail an ended one leaves nothing behind. Oiko keeps an Audit log, an append-only `audit` table in `history.db`, created by its format 2 migration (ADR 0032). It records every refusal (anonymous ones within a budget, ADR 0034) and every change to access. Its entries go through the History's writer but, like a Replace or a Delete (ADR 0016), are never dropped when its queue is full. Each entry is kept one year, then deleted; nothing else deletes or edits one. A year matches a Session's absolute lifetime (ADR 0025), so the sign-in behind any live Session is still in the log.

## Considered Options

- **A database or file of its own** (`audit.db`, JSON Lines): it would survive deleting `history.db`, but it adds a second migration path, which ADR 0032 already declined for credentials. An operator who drops `history.db` to reclaim space loses a trail, not access.
- **The process log only** (journald): nothing to build, but the dashboard cannot show it and the host decides how long it lasts. Writing each entry there as well would store the same fact twice with two retentions, and without client addresses fail2ban or CrowdSec could not act on it; the reverse proxy's access log is the place for that.
- **30 days, like Traces, or indefinitely, like History**: 30 days loses how a year-long Session or a never-expiring Token was obtained; indefinitely keeps internet scanners' refusals forever.
- **Ids only, as in a Command's Origin** (ADR 0031): a Command's Origin is a live reference that follows renames, but an audit entry records the past. After a removal, the case an audit exists for, an id alone would read "a removed Person". Each identity in an entry is therefore stored as its id and its Name at the time.
- **The client address**: behind a TLS-only proxy every client has the proxy's address, since no forwarded header is trusted by default, so the address would mislead and only adds personal data to backups. An entry keeps the browser label that the Session list shows (ADR 0025).
- **Only refusals tied to a known identity**: less noise, but probing with bogus links or Tokens would go unseen. A budget on anonymous refusals bounds the volume instead (ADR 0034).
- **Edits to the home** (Automations, Devices, Areas, Layouts): a general activity log is another feature, with its own volume and display. The Audit log covers access only.
- **Streaming entries as Updates**: every observer sees Updates in the same order, so audit entries would need a second per-observer filter beside the Guest one (ADR 0031). A security log does not need to be live.
- **Erasing a removed identity's entries**: less personal data kept, but removing a compromised account would erase what it did.

## Consequences

- **What is recorded.** Every refusal: an unknown Passkey or one whose signature counter regressed, an invalid, used or expired Sign-in link or Setup link, an invalid Token, a refused or expired Kiosk pairing, and a failed step-up. Every change to access: Persons, Kiosks and Programs created, renamed or removed, Access levels changed, a Guest's end date set, changed or reached, Passkeys added or removed, Tokens generated or revoked, Sign-in links created (by whom, for whom, from the host), Sign-in links, Setup links and pairings used, Sessions started and ended. A successful step-up is not recorded, since the action it guards is.
- **What an entry holds.** Its time, its event, the actor and the subject: a Person, a Kiosk, a Program, the host (`oiko sign-in-link`), Oiko itself (an expiry) or unknown, each as id and Name at the time. A browser label where a browser is involved. No address.
- **Expiries.** A Session, a Sign-in link or a Guest's access that ends by itself is recorded when Oiko notices it (on a request, a sweep or on load), dated when it actually ended.
- **Who reads it.** Admins read all of it, Admin Programs included through the HTTP API. Every signed-in Person, Guests included, reads the entries where they are the actor or the subject. Kiosks read nothing. The log is fetched on request, newest first, filterable by identity and paged. When an identity still exists, its current Name is shown, with the recorded one when they differ.
- **No alerts.** The Audit log only records; nothing is pushed to anyone when an entry is written.
- **Removed identities.** Their entries stay until their year is up, Names included; the documentation says so.
