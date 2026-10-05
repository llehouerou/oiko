# A Kiosk is paired from its own screen and stays signed in while it is used

A Kiosk (ADR 0022) is a shared screen, such as a wall tablet running a kiosk browser, that must stay signed in for months without anyone typing on it. Its Session has to land in the browser that sits on the wall, and a stolen tablet has to be cut off quickly. A Kiosk is paired from its own screen. Its sign-in page offers to use the screen as a Kiosk and shows a QR code. An Admin scans that code with their signed-in phone, steps up, and approves; the screen then receives the Kiosk's Session. This is the pattern Jellyfin's Quick Connect uses, with the screen showing a code that a signed-in device approves. A Kiosk holds at most one Session. That Session ends after 30 days without use, with no absolute limit, or when an Admin signs it out.

## Considered Options

- **A Sign-in link created for the Kiosk** (ADR 0024): it reuses an existing mechanism, but the link has to be opened in the browser on the wall. Scanning it with the tablet's camera usually opens the default browser instead of the kiosk app, so the Session lands in the wrong browser. The only other way is pasting the link in.
- **Both pairing and a link**: two mechanisms to secure and document, for screens that can display a page anyway.
- **One secret in the QR code**: simpler, but anyone who photographs the screen could claim the Session before the tablet does.
- **Several Sessions per Kiosk**: several screens under one identity, but revoking one stolen tablet then means picking the right Session, and what each screen did is no longer told apart. Two screens are two Kiosks.
- **A Person's lifetimes (30 days idle, 1 year at most)**: consistent with ADR 0025, but the wall would sign out on a date nobody chose, which is the complaint self-hosted peers keep hearing about wall tablets.
- **No expiry at all, like a Program's Token** (ADR 0028): a tablet left in a drawer, or one that walked away unnoticed, would keep its access forever.
- **Members may sign a Kiosk out**: faster when a tablet is stolen, but it stretches ADR 0023, under which only Admins manage access.
- **A Person signing in on top of a Kiosk for a while**: handy for editing the wall's own Layout, but it puts two identities in one browser, and a Person's rights would stay on a shared screen if falling back to the Kiosk failed.
- **A Kiosk accepted only from the home network**: it would make a stolen tablet useless once it leaves the house, but behind a reverse proxy the client's address proves nothing (ADR 0027).

## Consequences

- **Pairing request.** Pairing is offered where sign-in is: on the public HTTPS URL, and on `http://localhost` for development (ADR 0024). The screen creates a pairing request, which Oiko keeps in memory for 10 minutes; once it expires, the screen shows a fresh one. A request carries two secrets of at least 128 bits each. The approval secret is in the QR code, in the URL fragment. The claim secret stays on the screen in a cookie. Only the browser holding the claim secret receives the Session, so a photo of the QR code yields nothing. The number of pending requests is capped and their creation is rate-limited.
- **Approval.** Only an Admin approves a pairing, after step-up (ADR 0025). The approval page says plainly that it enrols a shared screen, and shows the request's age and the browser it came from. The Admin then either creates a Kiosk, giving its Name and its Access level (Guest by default, or Member; never Admin, per ADR 0023), or picks an existing Kiosk. Pairing an existing Kiosk again ends its previous Session; its identity, Name and Access level stay.
- **Lifetime.** A Kiosk's Session is a Session like a Person's (ADR 0025): server-side, in a `__Host-` cookie, stored hashed. It ends after 30 days without use and has no absolute limit; an open event stream counts as use, so a screen that is always on never expires.
- **On the screen.** A Kiosk shows no sign-out. No Person can sign in on top of it. It manages nothing, not even its own Name. When its Session ends, the screen shows the pairing offer again.
- **Admins.** Admins see each Kiosk's Name, its Access level, who paired it and when, and when it was last used. An Admin signs a Kiosk out (it keeps its identity until it is paired again) or removes it, with no step-up: either only takes access away, as revoking one's own Sessions does. Pairing, renaming and changing a Kiosk's Access level need a step-up. An ended Session or a changed Access level closes the Kiosk's event stream at once (ADR 0025).
