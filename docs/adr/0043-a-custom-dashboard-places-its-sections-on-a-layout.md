# A custom Dashboard places its Sections on a Layout, and each own Section its Tiles

The grid of ADR 0015 is used at two levels of a custom Dashboard. Each own Section, a nameless one included, has its columns (1 to 6) and a Layout of its Tiles, stored with it, under the same rules as an Area's. The Dashboard itself has columns (1 to 6, two by default) and a Layout of its Sections: each Section a first cell, a width in columns and a height in rows, a row as tall as its tallest Section, a Section several rows tall letting shorter ones stack beside it, empty cells allowed. A Section not yet placed takes the first free cells, one column wide and one row tall. An Area's Section always uses the Area's Layout, which is arranged on the built-in Dashboard only; on a custom Dashboard the Section moves and resizes as a whole. Where the screen is too narrow for the Dashboard's columns, a phone among others, its Sections stack in reading order and, inside each, a Layout too wide for its width stacks its Tiles, as ADR 0015 already says; nothing is set per screen. The built-in Dashboard keeps its two columns of Sections taken alternately.

## Considered Options

- **An automatic grid for own Sections** (as Others): no gap can be left and where a Tile lands changes with the screen, which ADR 0015 rejected for Areas.
- **A Layout per Dashboard for an Area's Section**: a second Layout to keep in step as Tiles join or leave the Area, the kind of override ADR 0040 refused. A Dashboard wanting an Area arranged differently builds an own Section.
- **Columns of Sections taken alternately**, as on the built-in Dashboard, with a column count per Dashboard and perhaps Sections spanning every column: a strip across the top is possible, but not a tall Area beside two short ones, nor a gap.
- **Sections stacked in columns, masonry style**: no rows to manage, but a spanning Section's place depends on the heights above it, which change as Sections fold and charts load.
- **Fewer columns as the screen narrows**: needs a rule to re-place Sections spanning the columns lost, and what is shown stops being what was arranged.
- **Closing up the place of a Section a viewer cannot see**: a Guest would see another arrangement than the one made.

## Consequences

- One grid model, one check (inside the columns, no overlap, each occupant once) and one editor serve an Area's Tiles, an own Section's Tiles and a Dashboard's Sections.
- The order of a custom Dashboard's Sections is its Layout's reading order: it is what a narrow screen shows, top to bottom.
- On a Dashboard's Layout an Area's Section is named by its Area, and an own Section by an id of its own, which Oiko gives it on its first save and keeps after: moved or renamed, it stays the same occupant, and the fold a browser keeps for it stays.
- An own Section a viewer cannot see, being empty for them (ADR 0041), leaves its cells empty: everyone else keeps the place it was arranged in.
- A Section folded shrinks to its header; its row shrinks with it unless another Section keeps it tall.
- Duplicating the built-in Dashboard lays its Sections out two per row, in their order, for the copy's owner to rearrange.
