# One Person per human; Kiosks and Programs are identities of their own

Authenticating to Oiko needs an identity model, and presence detection will later attach to the same humans. Oiko has one concept per human, the Person, whether a member of the household or a temporary guest: the identity that signs in is the one presence and per-person Notifications will address, and signing in is optional, so a Person who never does is still one. A shared screen is a Kiosk and an external program is a Program, each an identity of its own, never a Person and never a credential of one.

## Considered Options

- **A User that signs in, linked to a Person presence tracks** (Home Assistant's split): allows a login without a human or a human without a login, but two lifecycles to keep in step and a link to maintain; an optional credential on a Person covers the second case, and Kiosks and Programs the first.
- **Only a User now, a Person when presence comes**: smallest today, but the later split migrates every stored identity (ADR 0019).
- **A guest as a lighter identity** (a share link with no Person behind it): a guest later needing a Notification or seen by presence would have nothing to attach to, and becoming a member would be a migration instead of a role change.
- **A token or long-lived login of a Person for programs and tablets** (Home Assistant's long-lived tokens): fewer concepts, but removing that Person silently breaks the program or logs out the tablet, and what they do reads as that Person's doing.
- **One non-human identity for Kiosks and Programs**: fewer terms, but a Kiosk uses the dashboard in a browser session and a Program the API with a token; they differ in enrolment, lifetime and role, and a shared term would only mean "not a Person".

## Consequences

- A Person's identity is stable and never its Name or a credential, like a Device's (ADR 0001).
- Removing a Person leaves the Kiosks and Programs they created working.
- No term yet covers "a Person, a Kiosk or a Program"; roles and Command attribution name one if they need it.
