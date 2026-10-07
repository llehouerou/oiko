// A Layout: where its occupants sit in a grid of a few columns, gaps included (ADR 0015). An
// occupant is anything with a key: a Tile in an Area's Layout, a Section in a Dashboard's (ADR 0043).

import type { Placement } from './types'

// Where an occupant sits, as a Placement says. Its height is always known once arranged, and auto
// while nobody has set it, so that it stays the occupant's own and is never stored.
export type Place = Omit<Placement, 'tile'> & { auto?: boolean }
// An occupant in its place.
export type Arranged = Place & { key: string }

export const defaultColumns = 3
export const maxColumns = 6
export const maxRows = 6 // the tallest a tile gets

// How many rows p spans.
export const rows = (p: Place) => p.height ?? 1
// The cells p covers in a grid of columns, each as row * columns + col.
export const cellsOf = (p: Place, columns: number) =>
  [...Array(rows(p)).keys()].flatMap((r) => [...Array(p.width).keys()].map((c) => (p.row + r) * columns + p.col + c))

// occupants with their place in a Layout of columns, in reading order. An occupant layout places
// keeps its place, any other takes the next free cells after the last one placed, one column
// wide. One whose height nobody set is rowsOf it, at its width; one that no longer fits where it
// was, under one grown taller, moves on to the next free cells.
export function arrange<T extends { key: string }>(
  occupants: T[],
  columns: number,
  layout: Arranged[] = [],
  rowsOf: (occupant: T, width: number) => number = () => 1,
) {
  const taken = new Set<number>()
  const placed = layout
    .flatMap((p): Arranged[] => {
      const o = occupants.find((o) => o.key === p.key)
      return !o ? [] : p.height === undefined ? [{ ...p, height: rowsOf(o, p.width), auto: true }] : [p]
    })
    .sort(byReading)
    .map((p) => claim(p, taken, columns))
  const next = Math.max(0, ...placed.map((p) => p.row * columns + p.col + p.width))
  return occupants
    .map((o) => {
      const p =
        placed.find((p) => p.key === o.key) ??
        claim({ key: o.key, col: next % columns, row: Math.floor(next / columns), width: 1, height: rowsOf(o, 1), auto: true }, taken, columns)
      const place: Place = { col: p.col, row: p.row, width: p.width, height: p.height, auto: p.auto }
      return { ...o, place }
    })
    .sort((a, b) => byReading(a.place, b.place))
}

// The Layout of occupants as arranged, each in its place.
export const placements = (occupants: { key: string; place?: Place }[]): Arranged[] => occupants.flatMap((o) => (o.place ? [{ key: o.key, ...o.place }] : []))

// The Layout to store: the heights someone set, never an occupant's own.
export const stored = (layout: Arranged[]): Arranged[] => layout.map(({ auto, height, ...p }) => (auto ? p : { ...p, height }))

const byReading = (a: Place, b: Place) => a.row - b.row || a.col - b.col
const overlap = (a: Place, b: Place) => a.row < b.row + rows(b) && b.row < a.row + rows(a) && a.col < b.col + b.width && b.col < a.col + a.width
// p narrowed and pulled left as much as columns need.
const fit = (p: Arranged, columns: number): Arranged => {
  const width = Math.min(p.width, columns)
  return { ...p, width, col: Math.min(p.col, columns - width) }
}

// p at the first cells, from where it wants on, that fit it and are free in taken, which it takes.
function claim(p: Arranged, taken: Set<number>, columns: number): Arranged {
  const f = fit(p, columns)
  const at = (i: number) => ({ ...f, col: i % columns, row: Math.floor(i / columns) })
  let i = f.row * columns + f.col
  while ((i % columns) + f.width > columns || cellsOf(at(i), columns).some((c) => taken.has(c))) i++
  for (const c of cellsOf(at(i), columns)) taken.add(c)
  return at(i)
}

// Places each of wanted in turn in a grid of columns: where it wants if those cells are still
// free, otherwise in the first free ones after, in reading order.
export function settle(wanted: Arranged[], columns: number): Arranged[] {
  const taken = new Set<number>()
  return wanted.map((w) => claim(w, taken, columns))
}

// layout with occupant key moved to col and row; one in its way takes its former place.
export function move(layout: Arranged[], key: string, col: number, row: number, columns: number) {
  const from = layout.find((p) => p.key === key)
  if (!from) return layout
  const to = fit({ ...from, col, row }, columns)
  const others = layout.filter((p) => p !== from).sort(byReading)
  const inWay = others.filter((p) => overlap(p, to))
  return settle([to, ...others.filter((p) => !inWay.includes(p)), ...inWay.map((p) => ({ ...p, col: from.col, row: from.row }))], columns)
}

// layout with occupant key resized, in columns and rows; one in its way moves on to the next free
// cells. The height someone sets is kept from then on.
export function resize(layout: Arranged[], key: string, size: { width?: number; height?: number }, columns: number) {
  const p = layout.find((p) => p.key === key)
  if (!p) return layout
  const resized = {
    ...p,
    width: Math.max(1, size.width ?? p.width),
    height: Math.max(1, size.height ?? rows(p)),
    auto: size.height === undefined && p.auto,
  }
  return settle([resized, ...layout.filter((o) => o !== p).sort(byReading)], columns)
}

// layout in a grid of columns: an occupant past the edge is pulled in, and those in its way move on.
export const reflow = (layout: Arranged[], columns: number) => settle([...layout].sort(byReading), columns)
