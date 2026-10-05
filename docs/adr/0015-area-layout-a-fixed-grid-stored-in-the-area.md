# An Area's Layout: a grid of fixed columns, stored in the Area

Each Area has a number of columns (1 to 6) and a Layout: for each Tile the occupant placed, its first cell (column, row), its width in columns and its height in rows (until the occupant sets it, one, or for a Tile of readings one per line of its cells, so a tall Tile leaves the cells beside it free from the start). Rows have no fixed height: a row is as tall as its tallest Tile, a Tile several rows tall stretches over them so that shorter Tiles stack beside it, and an empty row keeps a minimum height so a gap the occupant left stays visible. Oiko stores the Layout in the Area, next to the Tiles it hides, and checks only the grid itself (inside the columns, no overlap, each Tile once); which Tiles the Area holds is the dashboard's business. A Tile the Layout does not place, such as a Device just added to the Area, takes the first free cells after the placed ones; a placement whose Tile left the Area is ignored and dropped on the next save. Where the Area is narrower than its columns can fit, the Tiles stack in reading order and the gaps go.

## Considered Options

- **An order only, auto-filled grid** (as before): no gap can be left, and the column count, hence where a Tile lands, changes with the screen.
- **Free width and height, fixed-height rows** (react-grid-layout style): a Tile's content (its chart, its readings) would have to fit the height picked for it, cut or squashed otherwise.
- **One Layout per screen size**: most faithful on a phone, but every arrangement has to be made twice.
- **A Layout per browser**, in local storage: each wall tablet could differ, but a phone, a tablet and a computer would each need arranging, and a new browser starts from nothing.

## Consequences

- A phone shows a Layout as an order, not as a grid: arranging is done on a wide screen.
- Tiles without an Area (Others) keep the automatic grid.
- Areas keep their two-column dashboard; only their order is the occupant's. The columns take them alternately (first left, second right…), never by height, so a chart loading or a section folding never moves an Area to the other column.
