# Three fixed, nested Access levels; access is managed only by a Person

Exposing Oiko to the internet means not everyone signed in may do everything. Oiko has three Access levels, fixed in its code and nested: a Guest observes the home as it is now, issues Commands and starts Manual triggers; a Member also reads the home's past (History, Traces, the Commands issued), which reveals the household's habits; an Admin also edits the home and manages access. Every Person, Kiosk and Program holds exactly one. Access levels stay coarse: nothing is granted per Area, Target or Automation.

## Considered Options

- **Members edit the home too, Admin only manages access**: one fewer thing for the household to ask an Admin, but anyone signed in as a Member could break or rewrite Automations and Code Steps.
- **Admin and Member only, a guest being a Member with an expiry**: a guest would read History and Traces, the household's occupancy patterns.
- **An Owner above Admin**: guards against lockout by demotion; refusing to demote or remove the last Admin Person does the same with one tier fewer.
- **Levels an Admin composes from permissions**: fine-grained permissions under another name, and every permission name a contract (ADR 0019).
- **Every settable Capability commanded alike**: but a configuration Capability (a light's power-on behaviour, a thermostat's calibration) changes how a Device behaves, which is editing the home; it is an Admin's, decided by the Capability's category, never per Target.

## Consequences

- A Kiosk is a Guest or a Member, never an Admin: a screen left on a wall must not be an admin console.
- A Program may be an Admin (configuration as code) but never manages access: Persons, Kiosks, Programs, credentials and Access levels are managed only by a signed-in Person, so a leaked token can damage the home's configuration but never mint identities or lock the household out.
- Oiko refuses to demote or remove the last Admin Person.
- Everyone signed in manages their own credentials, sessions and Name, never their own Access level; inviting anyone, a Guest included, is an Admin's.
- The Build and the notice of a newer Release are shown to Admins only.
- Adding, removing or splitting a level later migrates every stored identity (ADR 0019).
