# Identities and credentials live in JSON documents, their secrets only as hashes

Sign-in brings data Oiko owns: Persons with their Passkeys, a pending Sign-in link and a Guest's end date, Kiosks, Programs with their Token, and Sessions. There are tens of records, written rarely except for last-use times, and checked on every request. Like the rest of the configuration (ADR 0003), they are JSON documents in the data directory, kept through `bridge/store`, loaded whole at start and held in memory. There are four of them, one per kind: `persons.json`, `kiosks.json`, `programs.json` and `sessions.json`, which holds both Person and Kiosk Sessions. Each starts at format 1 (ADR 0019). No secret is stored in a form that can be used: Sessions, Sign-in links and Tokens are kept as a SHA-256 hash, and Passkeys as public keys.

## Considered Options

- **An SQLite database of its own**: removing a Person and everything hanging off them would be one transaction, but it adds a second migration path next to `history.db` and still needs an in-memory copy so requests don't wait on SQL. A Session whose Person is gone is refused anyway, so a crash between two document writes is harmless.
- **Tables in `history.db`**: one database, but `history.db` is large, kept indefinitely and something an operator may drop to reclaim space. Credentials would share its fate, and every migration would copy all of it.
- **One document for everything, or one for all identities**: every change would be atomic, but each hourly last-use write would rewrite every Person and Passkey, and kinds ADR 0022 keeps apart would share one format.
- **Hashes keyed with a pepper, or encrypted documents**: the key would live on the same host as the data it protects. A secret of at least 128 random bits cannot be recovered from its SHA-256 hash, so a leaked backup reveals only Names and Access levels. A lost key would sign everyone out or lose the household's identities.
- **WebAuthn ceremony state in a signed cookie**: stateless, but it needs a signing key on disk and still needs replay protection, so state comes back anyway.
- **Last use written within a second, or only on shutdown**: the first rewrites `sessions.json` about every second while any dashboard is open; the second loses every last-use time since start in a crash.
- **A random 64-byte WebAuthn user handle**, as the specification suggests: it hides nothing that a Person's id reveals, since ids are not personal data, and it costs a field and a lookup.
- **Accepting a Passkey whose signature counter went backwards**, or ignoring counters: no lockout from a faulty authenticator, but a cloned device-bound key would keep working. Synced passkeys always report 0 and are never affected.
- **Keeping ended Sessions and Sign-in links, marked ended**: a sign-in history in the store, but it duplicates the audit log and grows without bound.

## Consequences

- **In memory only.** WebAuthn ceremony state is looked up by its challenge, kept 5 minutes and capped in number, like Kiosk pairing requests (ADR 0029) and the Setup link (ADR 0026). A restart drops any ceremony in progress, and the user tries again.
- **Last use.** The last use of a Session, a Kiosk or a Token is exact in memory, which is what Admins and Persons see. It is written to disk at most once an hour and on shutdown, so after a crash a Session can look up to an hour older than it is, which matters little against a 30-day idle limit. A Passkey's signature counter and last use are written at each sign-in.
- **User handle.** A Passkey's user handle is its Person's id. A Passkey left in a password manager after its Person was removed finds no one.
- **Counter regression.** A sign-in whose signature counter is not greater than the stored one, when either is non-zero, is refused. The Person is told to use another Passkey or ask for a Sign-in link, and the event is recorded in the audit log.
- **Ended records.** A Session or Sign-in link is deleted from its document when it ends (used, expired, revoked) and when it is found ended on load. Records left pointing at a removed Person are dropped on load. The audit log keeps the trail.
- **Permissions.** The documents are created mode 0600, as `bridge/store` already does, and Oiko's data directory is now created 0700, as each Bridge's directory already is.
- **Backups.** Restoring an older copy of the documents brings back the Sessions and Tokens it contains, even ones revoked since then. Whoever restores controls the host, which is the root of trust (ADR 0026); the documentation says so.
- **Migration.** `history.db` goes to format 2: in the same transaction, each stored Command whose Origin is `"api"` has it rewritten to `"unknown"` (ADR 0031), after the copy to `history.db.v1` that ADR 0019 requires.
