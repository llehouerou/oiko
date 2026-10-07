# Dashboards live in one document, and what is deleted leaves them at once

Custom Dashboards are data Oiko owns (ADR 0019), kept like the rest of the configuration (ADR 0003) in one new document, `dashboards.json`, format 1, through `bridge/store`: every shared and personal Dashboard, each Person's ordered list of the Dashboards they see with the ones they hide (ADR 0044), keyed by Person, and each Kiosk's assigned Dashboard, keyed by Kiosk. On a save, Oiko checks the structure (each Section an Area's or an own one, Names of 1 to 100 characters, an Icon by name, a Tile at most once in a Section), the grid at both levels with the one check of ADR 0043, who may save it (an Admin for a shared Dashboard, its owner for a personal one), and that a Kiosk is assigned only the built-in Dashboard or a shared one. A reference to a Device, an Aggregate, a Flag, an Automation or an Area that no longer exists is dropped rather than refused, so a Dashboard edited while something was deleted still saves; a Function's key on a Device that exists is not checked. Whatever is deleted leaves every Dashboard at once, as an Aggregate's members do today: its Tiles leave their Sections and an Area's Section leaves the Dashboard, with the Area's Aggregates.

## Considered Options

- **Each list in `persons.json`, each assignment in `kiosks.json`**: stored next to the Person and the Kiosk, but deleting a Dashboard rewrites three documents, and identity documents (ADR 0032) carry display data.
- **One file per Dashboard**: smaller writes, against ADR 0003's one file per concern, for tens of records.
- **Checking only the shape and the grid**, as ADR 0015 does for an Area: a reference to something already gone would be stored and never cleaned.
- **Refusing a save naming something gone**: an editor open while an Admin deletes a Device loses its work.
- **Leaving what is deleted in place and dropping it on the next save**, as an Area's Layout does (ADR 0015): nothing ties deletions to Dashboards, but a Dashboard nobody edits keeps dead references for good.
- **Dropping a Function's Tile as soon as its key goes**: a Device interviewed again by its Bridge, or Replaced, would erase parts of Dashboards for good.
- **Moving the placements of a Device given up in a Replace to the Device kept**, as its History moves: the only reference in Oiko rewritten on a Replace, where Aggregates drop it and Automations break.
- **A removed Person's personal Dashboards becoming shared, or kept without an owner**: private arrangements appear in everyone's list, or stay as data no one can ever reach, since a Person created again has a new identity.
- **Refusing to delete a shared Dashboard while a Kiosk is assigned it**: no wall tablet changes unexpectedly, at the cost of a step and an error to explain.
- **Recording the clean-ups, in the log or for the editor**: the audit log covers sign-in matters (ADR 0033), changes to Areas and Aggregates are not recorded either, and telling an editor what went needs a memory of removals per Dashboard.

## Consequences

- A Device, an Aggregate, a Flag or an Automation deleted, or the Device given up in a Replace, leaves every own Section that holds it. An Area deleted takes its Section off every Dashboard, its cells left empty, and its Area Aggregates off every own Section. An own Section left without Tiles stays, for its editor to remove; until then it shows to nobody (ADR 0041).
- A Function placed whose key its Device no longer has, or a Device placed with no Function left that has a Tile, stays placed and shows nothing, its cells left empty, like an Area Aggregate without members; it shows again when the key comes back. A Detached Device's Tile stays, marked detached, as on the built-in Dashboard; a Replace leaves it unaffected (ADR 0042).
- A Person removed takes their personal Dashboards and their list with them. A Kiosk removed takes its assignment with it. A shared Dashboard deleted leaves every list, and each Kiosk assigned it shows the built-in Dashboard again.
- Entries left pointing at a removed Person or Kiosk, after a crash between two document writes, are dropped on load, as ADR 0032 does for Sessions.
- Nothing of this is recorded: a Dashboard simply changes, and whoever views it sees the change live.
