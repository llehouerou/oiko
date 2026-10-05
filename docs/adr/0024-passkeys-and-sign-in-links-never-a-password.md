# A Person signs in with a Passkey or a Sign-in link, never a password

Exposing Oiko to the internet means every Person must prove who they are, and an account is only as strong as its weakest way in. A Person signs in either with a Passkey or with a Sign-in link: a single-use link that is valid for 15 minutes and that a signed-in device creates for a Person. Oiko has no password, no TOTP and no username. A household Oiko has no mail server or support desk, so a password would bring hashing, brute-force defence and lockout, and it would add a phishable path that undercuts the Passkey. Sign-in is username-less: the device's passkey sheet picks the account, so no identifier needs managing and none can be enumerated.

## Considered Options

- **A password with TOTP as an optional fallback**: covers devices or origins where passkeys fail, but that phishable path then sets the account's real security level, and Oiko would carry password storage, rate limiting and lockout.
- **A password alone**: the weakest option (AAL1, phishable, brute-forceable).
- **Device-bound passkeys required for Admins**: resists takeover through someone's Apple or Google account, but each Admin then needs hardware keys and a backup key. Synced passkeys with user verification already reach AAL2.
- **Attestation required**: would lock out iCloud Keychain, most Google Password Manager passkeys and third-party password managers, which covers most household devices.
- **A short typed code instead of a link**: easier to read aloud, but it has low entropy, needs rate limiting, and is easy to phish ("read me the code").
- **A configurable RP ID (a parent domain)**: survives a move between subdomains, but every app on that domain could then request assertions for Oiko's passkeys, and it adds one more setting to get wrong.

## Consequences

- **Passkeys.** Oiko accepts any Passkey, synced or not, at every Access level. Attestation is `none`; the AAGUID only names the provider in a Person's list of Passkeys. User verification is required, so a Passkey is multi-factor.
- **Origin and RP ID.** Sign-in only works on the configured public HTTPS URL, and `http://localhost` stays available for development. Reached by IP or plain http, Oiko offers no sign-in, only a pointer to that URL. On the LAN, split-horizon DNS gives the same name and certificate. The RP ID is the host of that URL. Changing the hostname makes every Passkey useless, so everyone re-enrols through Sign-in links.
- **Sign-in links.** A Sign-in link carries a secret of at least 128 bits in the URL fragment, so it never reaches the proxy's or Oiko's logs; the page posts it to Oiko. It can also be shown as a QR code. Oiko stores the secret hashed, and an unused link can be revoked.
- **Self-service.** Any signed-in Person can create a Sign-in link for themselves to sign in another device. Cross-device sign-in with a phone's Passkey (hybrid) needs nothing from Oiko.
- **Passkey offer.** After signing in with a Sign-in link, or from another device, a Person is offered a Passkey for the current device and may decline. A Person who never creates one signs in by link each time a session ends.
