# Each Person orders their Dashboards; the first one shown opens

Each Person has a list of every Dashboard they see, the built-in one, the shared ones and their own personal ones, in one order they set freely across those kinds, stored with the Person on the server so it follows them to every browser. They may hide any Dashboard in it but the last one still shown; hiding only takes a Dashboard out of the menu, it grants and takes away nothing. The first Dashboard not hidden is the one that opens: there is no default to set apart from the order. A new Person's list starts with the built-in Dashboard; a Dashboard created later, shared or personal, joins the end of each list it belongs to, shown. A Kiosk has no list: it shows the Dashboard an Admin assigned it, the built-in one until then, and cannot switch away from it.

## Considered Options

- **A default each Person marks**, apart from the order: a second setting, and a rule for when the marked one is hidden or deleted. With the first shown one opening, moving a Dashboard to the top is choosing it, and a deleted one is simply no longer first.
- **The last Dashboard viewed opens**, remembered by each browser: a phone and a laptop drift apart, and one look at another Dashboard changes what opens next time.
- **Fixed groups** (the built-in Dashboard, then shared, then personal, by Name): nothing stored, but a Person living in a personal Dashboard could not put it first, and a wall tablet's shared Dashboard would sit in every menu for good (ADR 0041).
- **An Admin orders the shared Dashboards for everyone**: the household's order, not the Person's; who uses which Dashboard differs from one Person to another.
- **The list kept in each browser**, like the charts setting: each phone and laptop to set up again, and lost with the browser's storage.
- **The built-in Dashboard never hidden**: the complete view stays one click away, but a Person who never uses it could not take it out of their menu; it is still reachable at its address.
- **New shared Dashboards arriving hidden**: keeps a wall tablet's Dashboard out of every menu, but nobody else would learn it exists.
- **A Kiosk browsing other Dashboards**, back to its own after some idle time, or several assigned to it: an idle timer and a setting, or a list to manage, against a Kiosk managing nothing (ADR 0029); an Admin reassigns it instead.

## Consequences

- The Home tab of the bar becomes the current Dashboard's Name and opens a menu of the Dashboards shown, in the Person's order, ending with an entry to edit the list: a drag handle and a switch to show or hide each, the hidden ones dimmed. A bar of three tabs fits a phone however many Dashboards there are.
- Each Dashboard has an address in the web client, `#dashboard/<id>`; a bare `#` opens the first one shown. A Dashboard can be bookmarked or linked, and the browser's back button steps between them. A hidden Dashboard still opens at its address; one the viewer does not see, another Person's or a deleted one, opens their first one shown instead.
- A Kiosk ignores the address and shows no menu.
- The list is data Oiko owns (ADR 0019): a Dashboard deleted leaves every list it was in, and a list never holds a Dashboard its Person does not see. The built-in Dashboard is never deleted, so a list is never empty; if a deletion leaves none shown, the first one left is shown again.
