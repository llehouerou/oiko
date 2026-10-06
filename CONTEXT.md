# Oiko

Local home automation platform: observe and control the devices of a home in real time.

## Language

### Hardware

**Bridge**:
External system that makes devices known to Oiko and relays their messages: a zigbee2mqtt instance, Oiko's own HomeKit controller, an Arlo account in Arlo's cloud, or a Netatmo account in Netatmo's cloud. Oiko may have several, each known by a name; each is online or offline on its own, and while one is offline, the Availability of its Devices is unknown. Each is of a type compiled into Oiko: one of the four built in, or one a build of Oiko adds; several may share a type.
_Avoid_: Integration, Adapter, Coordinator, Plugin

**Build**:
What an Oiko executable is made of: Oiko's version and each type of Bridge compiled into it, built in or added, with the Go package registering it and the module and version that package comes from, as Go records them in the executable. A version is unknown for a development build or a module taken from a directory; an untagged commit has a pseudo-version.
_Avoid_: recipe, manifest (reserved for a Bridge's), bill of materials
**Install**:
How an Oiko runs on its host: from Oiko's NixOS module, in a Docker image, or as a plain binary, as `OIKO_INSTALL` tells (`nixos`, `docker`, unset). It decides how a Release is applied: the dashboard tells what to change for each, and a plain binary rebuilds itself with `oiko upgrade`.
_Avoid_: deployment, distribution, install method
**Manifest**:
What an added type of Bridge's module tells the catalogue about itself, in `oiko-bridge.json` at its root: each type its root package registers, with a description and an example of its section of the configuration. Its versions and the Oiko each needs are not in it: the module proxy and its `go.mod` say them (ADR 0020).
_Avoid_: descriptor, plugin.json
**Release**:
A version of Oiko, or of the module of an added type of Bridge, that the Go module proxy lists: neither a pre-release nor a pseudo-version. Oiko tells when one newer than its Build's exists, one that cannot break it or a breaking one (a minor during v0, a major after), and never applies it; a human does (ADR 0019).
_Avoid_: update (reserved for Oiko's numbered records), upgrade (the act of applying one)
**Replay**:
What a Bridge hands Oiko on connecting so that its Devices' current state is known: zigbee2mqtt's retained messages, each paired HomeKit accessory's first reading (an accessory out of reach has nothing to replay), Arlo's report on connecting, which ends with the location's mode, or Netatmo's first poll. Replayed Values are state, not changes: they fire no Value trigger. The home is known once every Bridge has replayed; the automation engine's clock waits for it, at most about 10 s.
_Avoid_: sync, warm-up, boot

**Device**:
Physical device known through a Bridge, created automatically when the Bridge reports it. Its identity in Oiko is stable and survives hardware replacement.
_Avoid_: Node, Accessory, Thing

**Detached**:
State of a Device whose hardware is no longer known to its Bridge. It keeps its Name, Area, Functions and last Native Address, so the same hardware coming back attaches to it again, until it is replaced or deleted.
_Avoid_: orphan, removed, dead

**Replace**:
Handing a Detached Device the Native Address of newly paired hardware, so that hardware takes over its identity, Name, Icon and Areas; the Device created for the new hardware disappears. Functions and Capabilities whose keys match are kept.
_Avoid_: re-pair, migrate

**Native Address**:
Identifier of a Device within its Bridge (IEEE address for Zigbee, device ID for HomeKit), unique within that Bridge only. A replaceable attribute of the Device, never its identity.
_Avoid_: device_id, IEEE as identifier

**Availability**:
Reachability of a Device as last reported by its Bridge: online, offline or unknown; an Aggregate's is derived from its members'. May lag reality, so it never blocks a Command. Separate from Values, never mixed with them.
_Avoid_: unavailable (as a state value)

### Functional

**Function**:
What a Device does in the home (light, switch, cover, thermostat, occupancy, temperature, contact, button, camera…). The unit of display and control; belongs to exactly one Device, unless it is a Group, an Aggregate or a Flag. A sensor measuring several quantities provides one Function per quantity, so each can be named, placed and aggregated on its own. Identified within its Device by its kind and endpoint (e.g. `switch/l2`).
_Avoid_: Entity, Endpoint, Service, Channel

**Group**:
Function provided by a Bridge rather than a Device, controlling several member Functions at once in a single transmission. Its members and its Values are owned by the Bridge.
_Avoid_: Scene, Zone

**Aggregate**:
Function computed by Oiko from member Functions of the same kind. A member may itself be an Aggregate: members resolve to the underlying Functions, each counted once, and an Aggregate never contains itself. Each of its Values is derived from the members' Values by a rule (any or all for binary Capabilities; mean, min, max or sum for numeric ones); members without a Value are ignored. A settable numeric Value (a light's brightness) counts only the members switched on, all of them when none is. Online if any member is, otherwise offline if any is, otherwise unknown. Only counted members take part: a member whose Device is Detached, or whose Function vanished from its Device, stays configured but is ignored until its hardware returns. Its energy over a period is not read from its own History but summed from its current members' counters, Detached ones included, so a member joining or leaving changes no past total. A Command on an Aggregate is relayed to each member and is confirmed once every member has confirmed; one that only adjusts (a brightness, no state) goes to the members switched on, all of them when none is, so it turns none on.
_Avoid_: Group (reserved for the Bridge's), helper, virtual device

**Flag**:
Binary Function held by Oiko itself, with no Device or Bridge: on or off, set by a Command from anyone (a Person, a Kiosk, a Program, an Automation) and remembered across restarts. Created, renamed and deleted by an Admin; starts off. Always online. Has no Area unless assigned one, and may be a member of an Aggregate of Flags. State private to a single automation is not a Flag.
_Avoid_: Mode, helper, input_boolean, variable, virtual switch

**Capability**:
Typed property of a Function (on, brightness, temperature…), or of the Device itself for configuration and diagnostic properties (battery, link quality, power-on behavior…). Has a type, unit, bounds, access (observable, settable, queryable) and category (primary, configuration, diagnostic). Identified within its owner by its property name (e.g. `brightness`). A numeric Capability may be a counter: a running total that only rises, except when the device resets it (energy in kWh); what was used over a period is its rise above its highest reading, a drop of more than 1 % counting as a restart from zero and a smaller one as the device's rounding.
_Avoid_: Attribute, Feature, Characteristic, Expose

**Role**:
What a Capability is for on its Function, told by its key, type, access and category, never by the hardware model: the main control (at most one per Function, its on/off when binary), an adjustment refining it (a light's brightness and colour), a state (occupied, open, leaking…), a reading, an Event, a setting, health (battery, tamper: shown only when wrong) or a diagnostic (link quality, a plug's current and voltage beside its power). Every view of a Function, from its Tile to its History, reads the same Roles.
_Avoid_: purpose, meaning, usage

**Value**:
Typed value of a Capability, with the time the device reported it and the time it took that value (how long it has held, recalled from the History across a restart; an Aggregate's from its members'). No Value means "unknown"; the last known Value is kept along with its age. A report identical to the current Value is a refresh (its age resets, not how long it has held), not a change.
_Avoid_: State (in the Home Assistant sense)

**Event**:
Momentary occurrence emitted by a stateless Capability, such as a button press. An Event has no Value; Oiko only remembers the last occurrence of each, to tell when it last happened.
_Avoid_: action, click

**Picture**:
The latest still image a camera Function has, with when it was taken, read without waking the camera. Never a Value: it is fetched when shown and never recorded.
_Avoid_: snapshot, still, thumbnail, last image

**Live view**:
A camera Function's video as it happens, opened on demand by a Person, a Kiosk or a Program and shared by everyone watching it at once. Recorded in the camera's History, with its Origin and how long it lasted; never recorded as video.
_Avoid_: stream, live feed, live stream, video

**Recording**:
A clip a camera's own system recorded by itself, on motion or any trigger of its own, with a thumbnail and what triggered it. Kept by that system, never by Oiko, which lists them and plays them for a Member or an Admin; one deleted there is gone.
_Avoid_: clip, event, video, library, capture

**Command**:
Request to set one or more Capabilities of a single Function or Device, or of an Aggregate, optionally with a transition: the duration over which the device fades to the new values. Pending until a reported Value confirms it; otherwise failed or timed out. A newer Command on the same target supersedes a pending one. A binary Capability may be asked to toggle: the Command turns it off if the pending Command or, failing one, the current Value has it on, otherwise it turns it on with the other requested values. Values outside a Capability's bounds are refused, never clamped. A Command on an Aggregate is relayed as one Command per counted member, refused as a whole if any member would refuse it; it is confirmed once all of them are, and failed, timed out or superseded as soon as one of them is. Every Command records its Origin; a relayed Command inherits the Origin of the Aggregate Command that relayed it. Accepted Commands are kept with their outcome indefinitely, alongside the History.
_Avoid_: Service call, action

**Origin**:
What issued a Command: a Person, a Kiosk, a Program or a Run of an Automation, by identity, never by Name; unknown for a Command recorded before sign-in was required. Who starts a Run from a Manual trigger is recorded with its trigger, not on the Run's Commands. Only a Member or an Admin sees it.
_Avoid_: source, author, actor, issuer

**Target**:
What a Command addresses and what a Value belongs to: a Function (a Device's, a Group, an Aggregate or a Flag) or a Device itself, by stable identity, never by Name. Every kind of Target is read, watched and commanded the same way; only how a Command reaches it differs.
_Avoid_: entity, member (when not in an Aggregate), address

**Update**:
Numbered record that something happened in Oiko: a new Value, an Event, an Availability transition, a Command status change, a Target deleted, a Device replaced, a change of an Automation's status or the end of a Run. Every observer sees Updates in the same order.
_Avoid_: Event (reserved for button presses), Notification (reserved for an Automation's messages), message, change

**History**:
Recorded past of a Target: its Values, Events and Availability, and a camera's Live views, kept indefinitely from the first thing Oiko records of it until the Target is deleted. Every Value is recorded whole on each change, never on a refresh, and holds until the next one is recorded; a Replayed Value only when it differs from the last one recorded. It never invents a Value: what Oiko did not record is a Gap, and a Device offline or unknown shows in its Availability. Replace carries the new hardware's History over to the kept Device; a Capability that disappears keeps its History, which no longer grows. A Trace tells what one Run did; a History tells what a Target went through.
_Avoid_: Recorder, Log, Series

**Gap**:
Span of time over which the whole home's History is incomplete: Oiko was stopped, or recorded points were lost. Not a Device being offline, which is recorded as its Availability.
_Avoid_: outage, hole

### Automation

**Automation**:
Graph of Steps, created and named by an Admin, that reacts to what happens in the home by issuing Commands. Refers to Targets by identity, never by Name, and may watch any of them. Enabled or disabled; broken while a Target it refers to is gone, and runaway once it runs too often in a short time. A broken Automation never runs until edited, a runaway one until re-enabled.
_Avoid_: Flow, Rule, Scenario, Script

**Step**:
One box of an Automation's graph: a trigger, a condition, a timer or an action, joined to other Steps through named handles. Its kind (Event trigger, Timer, Cooldown, Code…) sets its handles, its params, what it does when a Run reaches it or its deadline comes due, and what it remembers. Some Steps keep state between Runs, such as a timer's deadline; it is remembered across restarts and cleared when the Automation is disabled or broken.
_Avoid_: Node, Block

**Code Step**:
Step running a Starlark `run(trigger, state)` written by an Admin. It reaches targets only through aliases bound to them in its params, and fires the output handles it declares. A call that fails issues nothing, fires nothing and keeps its previous state; the error goes into the Trace.
_Avoid_: Function node, Script

**Manual trigger**:
Trigger Step started from the dashboard rather than by the home: a button named after the Step, on the dashboard's tile for its Automation, or in the editor. Its Run is like any other, with a Trace; only an enabled Automation, neither broken nor runaway, runs.
_Avoid_: Scene, Script, inject, button (reserved for a Device's)

**Run**:
One pass through an Automation, from the trigger that started it, executed at once and never waiting. Reads the home as of the Update that triggered it; each Step input acts at most once per Run.
_Avoid_: Execution, Instance

**Trace**:
Record of one Run: its trigger, each Step it went through with the evidence for the handles it fired (the Value a condition read, a timer's deadline, a refusal, a Code error), and the Commands it issued. Every Run leaves one, even when it issues nothing; kept for 30 days.
_Avoid_: Log, Execution, History (reserved for a Target's past)

**Notification**:
Message an Automation sends to the home's one Telegram chat, never to a Person: a title and a text, which may name the Target that started its Run. Sent once, never retried, after the Run: its Trace shows what was sent, and one Telegram refuses is only logged. Without Telegram configured, it fails its Run.
_Avoid_: alert, message, push

### Home

**Area**:
Room or zone of the home, created and named by an Admin, in an order of their choosing. Areas are flat: none contains another. A Device has an Area or none; each of its Functions inherits it unless assigned another one. A Flag or an Aggregate may be assigned one too. Deleting an Area leaves what it held without one. The dashboard shows one section per Area, in their order, and everything without one last.

**Area Aggregate**:
Aggregate Oiko derives for an Area and an aggregated kind (lights, occupancy, doors, temperature, humidity, CO2) from the Functions of that kind in the Area, under a rule fixed per kind: a room's temperature and humidity are its mean, its CO2 its highest. Its members are never stored, its Name is derived, and it exists while it has at least one member, coming back under the same identity when it has one again.
_Avoid_: Room, Zone

**Tile**:
A Device's, Aggregate's, Flag's or Automation's box on the dashboard; an Automation's holds a button for each of its Manual triggers. A Target's Tile shows at most one control, its main control; every other settable Capability is a setting, behind its ⋯. Its shape (control bar, state, readings or event) follows from its Capabilities' Roles, never from the hardware model; a camera's shows its Picture. Battery and tamper show on it only when something is wrong.
_Avoid_: card, widget, entity row

**Layout**:
Where an Area's Tiles sit on the dashboard: a grid of a few columns, and for each Tile placed, its cell and how many columns and rows it spans, with empty cells wherever they are left. A row is as tall as its tallest Tile; a Tile several rows tall fills them, leaving the cells beside it to others. Until it is set, a Tile is one row tall, a Tile of readings one per line of its cells. A Tile not placed takes the first free cells after the placed ones. A screen too narrow for the columns shows the Tiles one under another in reading order, without the empty cells. Tiles without an Area have no Layout.
_Avoid_: arrangement, position, grid (as the term)

**Name**:
Display label of a Device, an Aggregate, a Flag, an Area, a Person, a Kiosk or a Program. A new Device takes its Bridge's label; afterwards it is freely editable in Oiko, never used as a reference, and never written back to the Bridge. A Function has no Name of its own: it shows its Device's, followed by its key when the Device has several (e.g. `Kitchen plug · switch/l2`).
_Avoid_: entity_id, slug as identifier

**Icon**:
Picture the dashboard shows for a Device or an Aggregate, picked by an Admin among the dashboard's own, known to Oiko by its name only. Without one, a light shows a bulb and an Aggregate of lights several. Like a Name, a label: never a reference, never written back to the Bridge.
_Avoid_: symbol, image

### Access

**Person**:
A human known to Oiko: a member of the household or a temporary guest, under an identity that survives a change of Name or of credentials. May sign in to the dashboard, or never do so (a child presence will later track). Created by an Admin, who invites them by creating a Sign-in link for them.
_Avoid_: User, account, member (as the term), occupant, resident

**Kiosk**:
A shared screen, such as a wall tablet, signed in to the dashboard as itself rather than as a Person. What is done from it is the Kiosk's doing, and it is revoked on its own. Holds at most one Session, which ends only when an Admin signs it out or after 30 days without use; it shows no sign-out and manages nothing, not even its Name.
_Avoid_: panel, display, shared device, household account

**Kiosk pairing**:
How a screen becomes a Kiosk: its sign-in page shows a QR code, which an Admin scans from a signed-in phone and approves as a new Kiosk or an existing one, ending that Kiosk's previous Session. Only the screen that showed the code receives the Session.
_Avoid_: enrolment, device code, Quick Connect, pairing (alone: reserved for a Bridge's hardware)

**Program**:
External software, such as Node-RED or a script, calling Oiko's HTTP API under its own identity rather than a Person's. It outlives whoever created it, and what it does is its own doing.
_Avoid_: Integration, API client, service account, app, token (its credential, not its identity)

**Access level**:
What a Person, a Kiosk or a Program may do in Oiko: Guest, Member or Admin, a list fixed by Oiko, each allowing everything the one below it does; whoever holds one is called by it (a Guest, an Admin). A Guest observes the home as it is now, issues Commands except on configuration Capabilities, and starts Manual triggers, seeing of the Automations only the Tiles of those with a Manual trigger, never why one does not run, nor any Run; a Member also reads the home's past (History, Traces, the Commands issued) and how its Automations are built; an Admin also edits the home (Devices, configuration Capabilities, Areas, Layouts, Aggregates, Flags, Automations), sees the Build and Releases, and manages access: Persons, Kiosks, Programs, their credentials and Access levels. A Kiosk is a Guest or a Member, never an Admin; a Program may be an Admin but never manages access, which only a Person does; the last Admin Person can be neither demoted nor removed. Everyone signed in manages their own credentials and Name, never their own Access level. A Guest may have an end date set by an Admin, after which they can no longer sign in while the Person stays; promotion to Member clears it.
_Avoid_: role (reserved for a Capability's), permission, group, right

**Passkey**:
A Person's lasting credential for signing in, kept by their device or password manager and bound to this Oiko. A Person may hold several and needs no username or password.
_Avoid_: password, key, token, login

**Sign-in link**:
A single-use link, or its QR code, that signs one Person in on one device. It expires 24 hours after an Admin creates it for another Person, and 15 minutes after a Person creates it for themselves or a command on Oiko's host creates it. A Person has at most one unused link: a new one revokes the previous one.
_Avoid_: magic link, login link, login code, token, invitation

**Setup link**:
The single-use link a fresh Oiko prints to its log while it has no Admin; whoever opens it becomes its first Admin. It stays valid until used or until Oiko restarts.
_Avoid_: setup code, claim link, invitation

**Public URL**:
The HTTPS address at which a household reaches its Oiko, through a reverse proxy, and the only one where anyone signs in (`http://localhost` aside); its host is what the Passkeys are bound to. Every Oiko needs one to sign anyone in, even one never exposed to the internet.
_Avoid_: base URL, external URL, origin, domain

**Session**:
A browser signed in to the dashboard as a Person or a Kiosk, from sign-in until it is signed out, revoked or expires. One device may hold several, one per browser.
_Avoid_: login, signed-in device (as the term), token, cookie

**Token**:
A Program's credential: a secret it sends with every request to the HTTP API, shown once when an Admin generates it, valid until it is revoked or replaced, never expiring. A Program holds at most one.
_Avoid_: API key, access token, secret, password

**Audit log**:
Record of who signed in or was refused, and of every change to access: Persons, Kiosks and Programs, their Access levels, credentials and Sessions, Sign-in links, Kiosk pairings, and what ended by itself. Each entry names who acted and whom it concerns as they were named then. Kept one year; never edited. Admins read all of it, every Person the entries that concern them.
_Avoid_: security log, activity log, journal, History (reserved for a Target's past)
