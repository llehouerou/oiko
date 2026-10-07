# A Dashboard is a flat list of Sections; an Area is reused live, never copied

A custom Dashboard is an ordered list of Sections, and nothing else. Sections never nest. A Section is either an Area's or one of the Dashboard's own. An Area's Section refers to the Area by identity and shows exactly what the built-in Dashboard shows of it, now: its header (Name, Icon, climate, presence, doors, light bar) and its Tiles minus those the Area hides, with no override on this Dashboard. An own Section holds Tiles placed one by one (Devices, Aggregates, Area Aggregates included, Flags, Automations with a Manual trigger), under an optional Name and Icon; without a Name it has no header, which is how Tiles are placed loose. A Tile appears at most once in a Section, but may appear in several Sections of one Dashboard. Flags are placed as Tiles, so a custom Dashboard has no pill row, and Others, the built-in Dashboard's catch-all, cannot be reused.

## Considered Options

- **Copy an Area's Tiles when it is added** (openHAB's "Add from Model", Home Assistant's "Take control"): the copy goes stale as Devices join or leave the Area, which openHAB documents and its users trip over.
- **A live Area with overrides per Dashboard** (Home Assistant's area view, with its own `hidden` and `order`): a second hide list to keep in step with the Area's. A Dashboard wanting the Area differently builds an own Section.
- **Loose Tiles beside Sections at the top level**: two kinds of item to arrange. A Section without a Name gives the same result with one kind.
- **Nested Sections**: a second level of layout and of folding, which no peer offers below its view.
- **A Tile at most once per Dashboard**: forbids a "Favourites" Section above the Areas its Tiles live in.

## Consequences

- Every Section is a grid of Tiles under a foldable header, unless it is an own Section without a Name. The built-in Dashboard is then the Sections of every Area, in their order, followed by Others.
- Areas gain an Icon, which an Admin picks and their Section shows on every Dashboard. A header without an Icon shows its Name alone.
- Each browser remembers which Sections it folds per Dashboard, so folding an Area on one Dashboard leaves it open on another.
