# A Command records who issued it; Guests never see it

Since sign-in is required, every Command comes from a known Person, Kiosk, Program or Automation Run, instead of the anonymous "api" that the dashboard and external programs used to share. A Command's Origin records only the identity that issued it: `{"person": id}`, `{"kiosk": id}`, `{"program": id}`, the Automation form `{"automation", "step", "run"}` unchanged, or `"unknown"`. Names are looked up when the Origin is displayed, so a rename shows everywhere and a removed identity reads as "a removed Person", "a removed Kiosk" or "a removed Program", as a deleted Automation does today. Reading who did what in the home reveals the household's habits, a Member's right (ADR 0023), so an observer below Member never sees an Origin.

## Considered Options

- **The Session too**: it would show which browser issued the Command, but Sessions expire and get revoked, so the reference soon points at nothing, and the audit log is the place for that trail.
- **A snapshot of the Name**: it would still read correctly after the identity is removed, but a Name is a label and never a reference, and the snapshot goes stale on a rename. Keeping a tombstone for every removed identity would cost a removed-but-remembered state on Persons, Kiosks and Programs.
- **Recording the link's creator when an Admin signs in as another Person** (ADR 0026): every such Command would show it was done in someone else's name, at the cost of a second, rarely set field in the contract. The Session label already shows the Person that someone signed in as them.
- **Recording who pressed a Manual trigger on each Command of its Run**: every Command would read "by Alice via Movie night" directly, but the Origin would have two parts and the same fact would be stored twice.
- **Showing Origins to Guests too**: one stream for everyone, but a Guest would watch who in the household does what, live. Hiding only the Persons, Kiosks and Programs while showing Automations would be a second rule for little gain, since a Guest cannot open a Trace anyway.
- **Keeping `"api"` for Commands recorded before sign-in**: it would avoid a migration, but the contract would keep a value that no new Command ever gets.
- **A tagged `{"kind", "id"}` object**: it is uniform, but it would reshape the Automation form, which Programs already read.

## Consequences

- **Manual trigger.** Who started a Run from a Manual trigger is recorded in its Trigger (in the Trace and the end of the Run). The Commands of that Run keep the Run as their Origin.
- **Signed in as someone else.** A Command from a Session opened by a Sign-in link that someone else created records only the Person.
- **Guests.** Updates and responses sent to a Guest, or to a Guest Kiosk, carry the Command without its Origin, and the end of a Run without who started it. The stream is therefore filtered according to each observer's Access level.
- **Migration.** Stored Commands with the `"api"` Origin become `"unknown"` and read as "issuer unknown" (ADR 0019).
- **History.** Commands are described with their Origin ("by Alice", "by Kitchen tablet", "by Node-RED", "by Movie night"). Their markers are of four kinds: by hand (a Person or a Kiosk, and unknown), by a Program, by an Automation, and lost.
- **Aggregates.** The member Commands an Aggregate Command relays keep its Origin, as they do today.
