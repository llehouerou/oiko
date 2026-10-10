# Presence shows as a row of chips a Dashboard may carry, each opening its sheet

The web client shows presence as the Presence row: one line of chips at the top of a Dashboard, as tall as the Flag pills. It shows the Home presence's chip first ("Someone home · 5 h", "· not one of you" while it is home with no Person home), then a chip per Person's Presence (a small face with the initial, the Name and how long the Value has held), then one per Aggregate of Presences ("Children 1/2"), which only shows. A face is filled while home, dimmed while away and dashed with no Value. A hand marks it while `manual` is true (ADR 0052). A tap on a Person's chip opens their sheet. It holds the Value and since when, a Home / Away choice that issues the Command, a note while the override holds saying when it ends, the last 24 h with the stretch set by hand hatched, and each Presence source's live state ("not connected · counts as home until 22:12", "offline · ignored"). A tap on the Home presence's chip opens its sheet: the Persons home, whether someone else is inside and until when, and each Entrance door's and Sign of life's state. An Admin edits the bindings in the sheet itself: a Person's Presence sources with their departure delays, the Home presence's Entrance doors, Signs of life and exit grace. Each sheet is saved whole, as `PUT /api/presences/{id}` takes it (ADR 0054).

The Presence row is a setting of each Dashboard. The built-in Dashboard shows it, and an Admin may turn it off while arranging. A custom Dashboard starts without it, and whoever edits it may turn it on. A Kiosk shows the row of the Dashboard it is assigned. A Presence is also a Target with a Tile like any other, placed in an own Section of a custom Dashboard. Its shape follows its Roles (ADR 0014): `home` is its main control (ADR 0052), so its Tile is a bar, a tap toggles it and ⋯ opens its sheet.

A phone's `network` Function (ADR 0050) shows as a state: `connected` joins the state keys, so its Tile is coloured while connected and says how long ("Connected · 2 h"), and its History is in periods, like a contact's.

We chose this because who is home is read at a glance and acted on rarely. A single line costs the Dashboard almost nothing, where a Section of Tiles takes a whole row. Overriding is a decision best taken with the sources in sight, so the chip opens the sheet rather than toggling. Editing the bindings where their live state shows makes a wrong delay or a dead source visible before it is saved. The row stays out of a custom Dashboard unless its editor wants it, and a Tile covers a Dashboard that wants presence somewhere else.

## Considered Options

- **Pills beside the Flags**, a tap toggling: as light, but the Home presence becomes one more pill, and a stray tap overrides a Presence with nothing to tell what the sources say.
- **A Household Section** of bar Tiles above the Areas, the Home presence in its header: it reuses Sections and Tiles as they are, but it takes a row of the Dashboard for four lines of state.
- **A banner of large faces** with names and ages: the same sheets, but more than twice as tall as the row for the same information.
- **A separate settings panel** per Presence, or one for every Presence, as an Aggregate's or a Flag's: it follows the panels an Admin opens from the Dashboard, but it splits the sources from their live state.
- **The row on every Dashboard, always**, as the built-in Dashboard always shows the Flag pills: a Dashboard built for one room or one purpose would carry it anyway.
- **`connected` left a reading**: nothing changes in the Roles, but a Tile ending in "on" says neither connected nor for how long, and its History is a line, not periods.

## Consequences

- A Dashboard gains a setting of its own: whether it shows the Presence row. A custom Dashboard keeps it in its document (ADR 0045); the built-in one, which an Admin has only arranged so far, gets its first setting, which an Admin changes while arranging. Both are new optional fields, absent meaning the default: a minor release (ADR 0019).
- The Roles learn `home` as a Presence's main control and `connected` as a state key; `docs/write-a-bridge.md`'s Roles table lists `connected`, which any type of Bridge may now use.
- A sheet's Home / Away is a Command like a Tile's toggle: anyone who may issue one sees it (ADR 0052, ADR 0053). Only an Admin sees Edit.
- The sheet derives what it says (the Persons home, someone else inside, until when a source counts) from the presences list and the sources' Values, with nothing added to the API.
