# The Public URL is an HTTPS origin in the configuration; the host command finds its socket in the data directory

Sign-in needs the Public URL (ADRs 0024 and 0027), and authentication is the breaking minor of ADR 0019, so what it changes in configuration and packaging is part of the contract. The Public URL is the `publicUrl` key of `config.json`, checked like every other key, and on NixOS the typed option `services.oiko.publicUrl` (default `null`) writes it, because every install has to set it. It is an origin, `https://host[:port]`, with nothing after it: a `__Host-` cookie needs `Path=/`, the RP ID is the host, and the web client is served from `/`. Oiko keeps listening on `:8080` by default. Since the network never authenticates (ADR 0027), the listen address is not a security boundary.

## Considered Options

- **A `-url` flag next to `-listen`**: it describes where Oiko runs, as `-listen` does, but flags are not checked like the configuration, and every command that loads Oiko would need it too.
- **The key only, set on NixOS through `settings`**: one option fewer, but the one value every install needs would be lost in a free-form attrset.
- **A path prefix** (`https://example.org/oiko/`): a proxy could share one host among several apps, but Oiko would lose the `__Host-` cookie prefix, and the web client, the API and the event stream would all need a base path.
- **Listening on loopback by default**, in the module or everywhere: defence in depth, but a proxy on another machine and every Program on the LAN would need the default changed back, and loopback proves nothing behind a proxy on the same host.
- **A wrapper installed by the module**, or an `OIKO_DATA` variable, for the host command: shorter to type, but one more thing the module owns, and a different way from how `homekit pair` is run.
- **Oiko tightening an existing data directory to 0700 at start**: nothing to do by hand, but `-data` may point at a directory shared with something else.

## Consequences

- **Checked at start.** A `publicUrl` that is not `https`, or that has a path, query, fragment or userinfo, stops Oiko from starting and says why. A trailing `/` is accepted. Without the key, Oiko runs as ADR 0027 says. `http://localhost` needs no configuration.
- **Host command.** `oiko sign-in-link` is handled before the configuration is loaded, as `upgrade` is, and needs only `-data`. Its socket is `<data>/sign-in-link.sock`. Oiko replaces a stale socket when it starts, and a data path too long for a socket address fails with that reason. `sign-in-link` becomes a reserved name, which no Bridge may take. On NixOS the command is `sudo -u oiko oiko -data /var/lib/oiko sign-in-link`.
- **Data directory.** Oiko creates a missing data directory 0700 and leaves an existing one as it is. The module sets `StateDirectoryMode = "0700"`. The release notes tell a plain-binary install to `chmod 700` its own directory.
- **Documentation.** The module's `listen` description stops saying there is no authentication. It says instead to put a reverse proxy that terminates TLS in front of Oiko, and that a Token sent over plain HTTP on the LAN can be read by anyone on it. The README gains an "Expose" section: a Public URL is needed even for LAN-only use (a domain, a certificate through DNS-01 or a `ts.net` name, split-horizon DNS); a TLS-only proxy is enough, with no buffering on `/api` for the event stream; the Setup link is in the log; recovery goes through `sign-in-link`; one Caddy example. The README's Backup section says that restoring older identity documents brings back the Sessions and Tokens they contain, that the Audit log lives in `history.db`, and that the Names of removed Persons, Kiosks and Programs stay in it for up to a year.
