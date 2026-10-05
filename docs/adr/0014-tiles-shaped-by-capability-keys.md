# Tiles shaped by what their Capabilities mean, never by the hardware model

A dashboard Tile shows at most one control: its main control, picked by Capability key (`state`, `on`, `alarm`, `mode`, and a light's brightness). Every other settable Capability is a setting and lives behind ⋯, whatever category the Bridge gave it. Battery and tamper show only when something is wrong. The Tile's shape (control bar, state, readings, event) follows from the same keys: a known control gives a bar, a known state (`occupancy`, `contact`, `water_leak`, `smoke`) gives a state, a stateless Capability an event, and anything else readings. Bridges already name Capabilities alike across brands, so the lists are short and hold for hardware not bought yet; an unknown reading still shows.

## Considered Options

- **Trust the Bridge's category alone**: zigbee2mqtt marks some settings `config`, but many drivers leave an occupancy timeout or a LED switch `primary`, so tiles fill up with settings.
- **Correct the category per model**: a list that never ends and that each new device breaks.
- **Let the occupant pick what each Tile shows**: works for any device, but every Device needs setting up before the dashboard is usable. Kept for later, should the rules get a real device wrong.

## Consequences

- The rule lives in the web client: Automations still see every Capability, whatever the Tile shows.
- A device whose main control has an unlisted key shows no control on its Tile until the key is added to the list.
- An Aggregate shows as a Device of its kind would.
