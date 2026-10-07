# Shared Dashboards are an Admin's, personal ones their owner's alone

Besides the built-in Dashboard, which nobody edits, a custom Dashboard is either shared or personal to one Person. Only an Admin creates, edits, renames and deletes a shared one, as they edit Areas and Layouts (ADR 0023), and every Person, a Guest included, sees every shared one. A personal one is seen and edited by its owner alone, an Admin included in neither. A Kiosk is assigned the built-in Dashboard or a shared one, never a personal one. Each viewer gets a Dashboard filtered to what their Access level lets them see: a Tile they may not see is left out without a trace, and an own Section left empty by it is not shown.

## Considered Options

- **Members edit shared Dashboards too**, or Members create shared ones and keep editing them: rearranging what the whole household sees is editing the home, which ADR 0023 keeps an Admin's, and a creator's right over a shared Dashboard is the per-object right ADR 0023 refused.
- **An audience per shared Dashboard** (chosen Persons, or a lowest Access level): per-object access lists to maintain, against ADR 0023's coarse levels; a Dashboard grants nothing anyway, so hiding it from someone protects nothing.
- **Admins see or edit personal Dashboards**: weakens "personal" for a need nobody has; an Admin wanting to set one up for someone builds a shared one, which that Person duplicates.
- **A Dashboard of a Kiosk's own**, or a shared one flagged for Kiosks only: keeps a wall tablet's Dashboard out of Persons' lists, at the cost of a third kind of owner or a flag; two Kiosks on one wall would need two copies.
- **A placeholder for a Tile the viewer may not see**: tells a Guest something is there that they are not allowed.
- **Shared Dashboards holding only what a Guest may see**: an Admin could not put an Automation without a Manual trigger on the household's Dashboard for its Members.

## Consequences

- A Dashboard has a Name, given by whoever edits it.
- Anyone may duplicate a Dashboard they see, the built-in one included (without Others, which cannot be reused), into a new personal one; an Admin may also duplicate one into a new shared one, which is how a personal Dashboard is published. A duplicate is a one-off: its Area Sections follow their Areas live, but it never follows the Dashboard it came from.
- A Dashboard shown at a lower Access level than its editor's is filtered for its viewer, so a Person demoted to Guest keeps their personal Dashboards, showing less. A Kiosk's Dashboard is filtered at the Kiosk's Access level.
- A shared Dashboard meant for a wall tablet shows in every Person's list too.
