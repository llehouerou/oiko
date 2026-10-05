# A fresh Oiko is claimed through a Setup link in its log; the host is the last way back in

A fresh Oiko has no Admin, and on an instance already exposed to the internet, letting its first visitor claim it is a race an attacker can win (peers have shipped exactly that bug). While no Admin Person exists, Oiko prints a Setup link to its log at each start, and only whoever opens it becomes the first Admin; whoever can read the log already controls the host and its data directory. When a Person loses every Passkey, an Admin creates a Sign-in link for them; when the last Admin does, a command on the host creates one through a Unix socket in the data directory. Host access is the root of trust, so recovery adds no secret for the household to keep.

## Considered Options

- **The first visitor claims it, within a time window after the first start**: matches the setup-wizard habit, but the race stays open for the length of the window.
- **The first visitor claims it from the LAN only**: relies on the client address, which Oiko does not trust behind a proxy, and a LAN bypass is what peers keep getting wrong.
- **A Setup link valid 15 minutes, or stored and valid across restarts**: the first races the operator looking for the log; the second leaves every old log line a live credential until the claim.
- **Recovery codes**: a household loses them, and they add a typed secret below the strength of a Passkey that Oiko would store and rate-limit.
- **Admins create Sign-in links only for Persons below Admin**: limits impersonation between Admins, but Admins are equals and an Admin who loses everything would need the host.
- **The host command on a TCP port, or run while Oiko is stopped**: behind a reverse proxy on the same host, localhost is every visitor; offline, recovery means downtime.

## Consequences

- **Setup link.** Its secret (at least 128 bits, in the URL fragment of the public URL) is kept in memory only: it is valid until the first Admin exists or Oiko restarts, and each start without an Admin prints a fresh one. Opening it asks for a Name, creates the first Person as an Admin, signs them in and offers a Passkey, which they may decline like anyone (ADR 0024). Until then, the dashboard tells every visitor to look in Oiko's log.
- **Sign-in link for another Person.** Any Admin, after step-up (ADR 0025), may create one for any Person, another Admin included. The Session it opens is labelled with the Admin who created the link in that Person's list of Sessions, so signing in as someone else is visible to them.
- **Host command.** `oiko sign-in-link` connects to a Unix socket in the data directory, mode 0600 and owned by Oiko's user, never a TCP port. Without an argument it lists the Persons (Name, Access level, id); given an id, it prints a Sign-in link for that Person, whatever their Access level, and the Session it opens is labelled as created from the host. The socket does nothing else: the Admin it lets back in does the rest from the dashboard. Oiko refuses to remove the last Admin (ADR 0023), so there is always one to sign in as.
- **The command and its socket are a contract** under ADR 0019, like the HTTP API.
