# Dashboards reach the web client in its event stream, and are saved whole

The web client reads Dashboards only from its event stream (`GET /api/updates`): the snapshot carries what its identity sees, a Person's Dashboards and their list, a Kiosk its assigned Dashboard, an Admin also each Kiosk's assignment, and a `dashboards` message resends all of it, for that identity, whenever any of it changes: an edit, a clean-up after a deletion (ADR 0045), a reassignment, a list saved in another browser. Like `releases`, it is a message per identity, not a numbered Update: personal Dashboards differ from one Person to another, and Updates come to every observer in the same order. Oiko leaves out what the viewer may not see (ADR 0041) from a Dashboard they cannot edit, a shared one for anyone but an Admin and a Kiosk's; a Dashboard they edit comes whole, so a Person demoted to Guest who saves their own does not lose what they no longer see. Each change in the editor saves the whole Dashboard; the last save wins, as for Areas, Layouts and Automations. Programs get none of it: these endpoints serve the web client, not an API designed for Programs.

## Considered Options

- **A `GET /api/dashboards`, the stream only telling that something changed**: two paths to the same data and a round trip on every change, for tens of records the stream can carry whole.
- **Dashboards in Home's numbered Updates**: Home would learn Persons and Dashboards, and one Person's personal Dashboards would take numbers in the order every observer shares.
- **The web client leaving out what its viewer may not see**: a Guest would still receive the ids of Automations it may not see, the hint ADR 0041 refused as a placeholder.
- **Oiko filtering every Dashboard, edited ones included**: a demoted Person saving their personal Dashboard would erase what they placed, for good.
- **One endpoint per change** (move a Section, add a Tile, rename): smaller requests, a dozen endpoints each checking part of a document ADR 0045 checks whole.
- **A revision, a stale save refused**: nothing overwritten unseen, but the first such check in Oiko, firing on two drags a second apart while each save already reaches the other editor at once.
- **Duplicating on the server**: Oiko would have to derive the built-in Dashboard, which only the web client does.
- **Step-up to assign a Kiosk its Dashboard**: assigning grants nothing; step-up guards what grants lasting access (ADR 0025).

## Consequences

- The endpoints, each for a signed-in Person only (a Kiosk or a Program is refused), none asking for step-up: `POST /api/dashboards` creates one from `shared` and a whole document, answering its id; `PUT /api/dashboards/{id}` saves one whole; `DELETE /api/dashboards/{id}` deletes one; `PUT /api/me/dashboards` saves the Person's list, its order and hidden ones; `PUT /api/kiosks/{id}/dashboard`, an Admin's, assigns a Kiosk its Dashboard. Who may save or delete which Dashboard is checked as ADR 0041 and ADR 0045 say.
- The built-in Dashboard has the reserved id `builtin`, in lists, in Kiosk assignments and in its address `#dashboard/builtin`.
- A duplicate is built by the web client from what its viewer sees and created like any new Dashboard: a Guest's copy of a shared Dashboard holds only what the Guest sees. Whether a Dashboard is shared or personal is set when it is created; a personal one is published by duplicating it into a shared one (ADR 0041).
- A list saved is reconciled, as ADR 0045 does for references: entries for Dashboards the Person does not see are dropped, and a Dashboard missing from it is added at the end, shown, so a list edited while a Dashboard is created or deleted still saves. Only a list with none shown is refused.
- An Access level changed already closes the identity's stream (ADR 0025); the reconnect brings its Dashboards filtered for the new level.
- An Area's Icon (ADR 0040) is set with its Name: `PUT /api/areas/{id}` takes both, and comes back, like the rest of the Area, in its Updates.
