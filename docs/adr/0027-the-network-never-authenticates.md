# Being on the home network never counts as signing in

Signing in from the sofa is a chore some peers remove by trusting the home network, but behind a reverse proxy that adds only TLS every visitor arrives from the proxy's address, and with the proxy on the same host from loopback, so the source address proves nothing unless forwarded headers are configured exactly right; peers that trusted it shipped bypasses (spoofed `X-Forwarded-For`, unauthenticated ports taken over). The home network is also where the least trustworthy machines live: smart TVs, cheap IoT, guests' phones. Oiko never treats where a request comes from as proof of who sent it, at any Access level, and authentication cannot be switched off. Long Sessions (ADR 0025), Kiosks and Programs remove the friction that network trust would have saved.

## Considered Options

- **Guest-level access from a configured trusted range**: no sign-in for viewing and commanding at home, but a compromised LAN device or a misconfigured proxy hands those rights to anyone, and Oiko would need a trusted-proxy setting to get right.
- **Full sign-in from a trusted range as a chosen Person** (Home Assistant's trusted networks): the most convenient, with the same failure modes at full rights.
- **Trusting loopback**: with a reverse proxy on the same host, loopback is the whole internet; the host already has its own channel, the Unix socket of ADR 0026.
- **Authentication off when no public URL is configured**: painless for an Oiko never exposed, but forgetting the setting on an exposed instance opens everything, and nothing lets Oiko tell whether it is exposed.
- **An explicit setting turning authentication off**: never inferred, but it gets copied from forum posts and Oiko still cannot tell when it is dangerous.

## Consequences

- **No trusted networks.** No setting makes an address, a range or loopback count as authentication. Forwarded headers may give a client address for display or rate limiting, never for authorization.
- **A public HTTPS URL for everyone.** Every Oiko needs one to sign anyone in (ADR 0024), even if never exposed: a LAN-only install needs a domain name and a certificate (for instance Let's Encrypt with a DNS-01 challenge, or a Tailscale `ts.net` name), with split-horizon DNS on the LAN.
- **No public URL configured.** Oiko starts and keeps its Bridges and Automations running; only `http://localhost` offers sign-in. Every other dashboard visit shows that a public HTTPS URL must be configured, every other API request is refused with the same reason, and the log says so at start. An upgrade never stops the home and never exposes it.
