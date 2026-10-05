# Long Sessions with step-up, not a daily sign-in

A Session must end at some point, but Oiko is used every day from phones by a household that includes non-technical members and Guests without a Passkey. NIST SP 800-63B-4 recommends, for AAL2, a fresh sign-in every 24 hours and after 1 hour idle; on a phone that is a sign-in prompt every day. Oiko instead keeps a Person's Session for 30 days idle and 1 year at most, and asks for a fresh proof of presence (step-up) right before any action that could grant lasting access, so a stolen or forgotten Session can use the home but cannot turn itself into a new Passkey, a Sign-in link or a new identity.

## Considered Options

- **NIST AAL2 timeouts (24 h absolute, 1 h idle)**: the literal standard, but every Person signs in daily, and a Person without a Passkey needs a new Sign-in link every day.
- **Long Sessions without step-up** (Home Assistant's model): simplest, but a stolen Session cookie can mint a Sign-in link or register a Passkey and keep its access after the Session is revoked.
- **Shorter Sessions for Admins, or for Sessions opened by a Sign-in link**: step-up already guards what Admins do that is hard to undo, and shorter link Sessions would only punish the Persons who declined a Passkey; the strength of a Sign-in link lies in how it was delivered.
- **Step-up on every Admin action, or on editing Code Steps**: Code Steps are sandboxed and reach only their bound targets, so editing one is no worse than editing any Automation, and editing the home is reversible; step-up there would only multiply prompts.
- **Step-up by Passkey only**: a Person without a Passkey could then never create a Sign-in link for another device of theirs.

## Consequences

- **Lifetimes.** A Person's Session ends after 30 days without use or 1 year after sign-in, whatever the Access level and whether it was opened by a Passkey or a Sign-in link. Any authenticated request or open event stream counts as use. The values are fixed in code; a Kiosk's lifetime is decided separately.
- **Server-side Sessions.** A Session is an opaque random value of at least 128 bits in a `__Host-` cookie (`Secure`, `HttpOnly`, `Path=/`, `SameSite`), stored hashed by Oiko, which alone decides when it ends. The cookie's own expiry never enforces a timeout.
- **Step-up.** Creating a Sign-in link (for oneself or another Person), adding or removing a Passkey, and managing Persons, Kiosks, Programs, their credentials and their Access levels require a Session that signed in, or confirmed a Passkey, within the last 10 minutes. A Person with a Passkey just confirms it. A Person without one can do these things only in the 10 minutes after signing in by link; the Passkey offer after a link sign-in falls inside that window. Signing out and revoking one's own Sessions need no step-up: they only protect.
- **Revocation.** Every Person sees their Sessions (a label derived from the browser, how it signed in, when, last use, which one is this device) and can revoke any of them, or all the others at once. After adding or removing a Passkey, they are offered to sign out all other Sessions. An Admin, after step-up, sees and revokes another Person's Sessions and Passkeys; removing a Person ends all their Sessions.
- **Authorization at once.** The Access level is read on every request, never kept in the Session. When it changes, the Person's open event streams are closed so the dashboard reconnects and redraws for the new level; the Sessions stay signed in.
- **Ended Sessions.** When a Session ends (signed out, revoked, expired, Person removed), Oiko closes its event stream at once; the reconnect is refused, and the dashboard shows the sign-in page and returns to the same view afterwards.
- **Departure from NIST.** Oiko's Sessions do not follow NIST's AAL2 reauthentication timeouts, which are recommendations (SHOULD); its sign-in itself stays AAL2 (ADR 0024).
