# A Tile placed on a Dashboard is a whole Device, or one of its Functions

An own Section holds Tiles, each of a Target or an Automation. A Device placed there gets a Tile showing every one of its Functions that has a Tile, whatever their Areas, and the Tile follows the Device live: a Function it gains appears, one it loses goes. A Device with several such Functions may instead be placed as one of them (`switch/l2` of a two-gang plug), whose Tile shows that Function alone, named after its Device followed by its key (`Kitchen plug · switch/l2`), with the whole Device's health, as the lone-light bar already does. To show two gangs of three, place two Tiles. An Automation's Tile always holds every Manual trigger it has. Aggregates, Area Aggregates included, and Flags are placed as themselves, as on the built-in Dashboard.

## Considered Options

- **A Device only, always whole**: no way to put only one gang of a plug on a Dashboard, short of an Aggregate with one member.
- **A Device with a list of the Function keys it shows**: one Tile can show two gangs of three, but a Function the Device gains never shows unless the list means "all". That makes two modes to explain, and a list to prune when a key goes away.
- **One Tile per Function** (Home Assistant's card per entity): gives up the one Tile per Device of ADR 0014 and of the built-in Dashboard; a multi-sensor becomes four Tiles.
- **A label per placed Tile**: makes two gangs readable, but adds a Name stored per Dashboard, which ADR 0040 refused for an Area's Section, and the same Tile then reads differently on each Dashboard.
- **Only some of an Automation's Manual triggers**: the Dashboard would refer to a Step and need handling when the Step goes away; an Admin can split the Automation instead.

## Consequences

- Nothing new is stored: a Section's Tile is a Target or an Automation, and a Function's Target already exists.
- A Device's identity survives a rename and a Replace, so its Tile is unaffected by either. A Function's Tile survives a Replace when the new hardware has a Function with the same key.
- A Device's Tile and the Tile of one of its Functions are different Targets, so both may sit in one Section.
- The picker offers a Device's Functions only when it has several with a Tile.
