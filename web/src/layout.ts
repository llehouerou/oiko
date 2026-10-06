// An Area's Layout: where its tiles sit in a grid of a few columns, gaps included (ADR 0015).

import type { Placement } from './types'

// A tile's Placement as arranged: its height always known, and auto while an Admin has not set
// it, so that it stays the tile's own and is never stored.
export type Arranged = Placement & { auto?: boolean }
// Where a tile sits in its Area's Layout.
export type Place = Omit<Arranged, 'tile'>

export const defaultColumns = 3
export const maxColumns = 6
export const maxRows = 6 // the tallest a tile gets

// How many rows p spans.
export const rows = (p: Place) => p.height ?? 1
// The cells p covers in a grid of columns, each as row * columns + col.
const cellsOf = (p: Place, columns: number) =>
  [...Array(rows(p)).keys()].flatMap((r) => [...Array(p.width).keys()].map((c) => (p.row + r) * columns + p.col + c))

// tiles with their place in a Layout of columns, in reading order. A tile layout places keeps its
// place, any other takes the next free cells after the last one placed, one column wide. A tile
// whose height an Admin never set is rowsOf it, at its width; one that no longer fits where it
// was, under a tile grown taller, moves on to the next free cells.
export function arrange<T extends { key: string }>(
  tiles: T[],
  columns: number,
  layout: Placement[] = [],
  rowsOf: (tile: T, width: number) => number = () => 1,
) {
  const taken = new Set<number>()
  const placed = layout
    .flatMap((p): Arranged[] => {
      const t = tiles.find((t) => t.key === p.tile)
      return !t ? [] : p.height === undefined ? [{ ...p, height: rowsOf(t, p.width), auto: true }] : [p]
    })
    .sort(byReading)
    .map((p) => claim(p, taken, columns))
  const next = Math.max(0, ...placed.map((p) => p.row * columns + p.col + p.width))
  return tiles
    .map((t) => {
      const p =
        placed.find((p) => p.tile === t.key) ??
        claim({ tile: t.key, col: next % columns, row: Math.floor(next / columns), width: 1, height: rowsOf(t, 1), auto: true }, taken, columns)
      const place: Place = { col: p.col, row: p.row, width: p.width, height: p.height, auto: p.auto }
      return { ...t, place }
    })
    .sort((a, b) => byReading(a.place, b.place))
}

// The Layout of tiles as arranged, each in its place.
export const placements = (tiles: { key: string; place?: Place }[]): Arranged[] => tiles.flatMap((t) => (t.place ? [{ tile: t.key, ...t.place }] : []))

// The Layout to store: the heights an Admin set, never a tile's own.
export const stored = (layout: Arranged[]): Placement[] => layout.map(({ auto, height, ...p }) => (auto ? p : { ...p, height }))

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
function settle(wanted: Arranged[], columns: number): Arranged[] {
  const taken = new Set<number>()
  return wanted.map((w) => claim(w, taken, columns))
}

// layout with tile moved to col and row; a tile in its way takes its former place.
export function move(layout: Arranged[], tile: string, col: number, row: number, columns: number) {
  const from = layout.find((p) => p.tile === tile)
  if (!from) return layout
  const to = fit({ ...from, col, row }, columns)
  const others = layout.filter((p) => p !== from).sort(byReading)
  const inWay = others.filter((p) => overlap(p, to))
  return settle([to, ...others.filter((p) => !inWay.includes(p)), ...inWay.map((p) => ({ ...p, col: from.col, row: from.row }))], columns)
}

// layout with tile resized, in columns and rows; a tile in its way moves on to the next free cells.
// The height an Admin sets is kept from then on.
export function resize(layout: Arranged[], tile: string, size: { width?: number; height?: number }, columns: number) {
  const p = layout.find((p) => p.tile === tile)
  if (!p) return layout
  const resized = {
    ...p,
    width: Math.max(1, size.width ?? p.width),
    height: Math.max(1, size.height ?? rows(p)),
    auto: size.height === undefined && p.auto,
  }
  return settle([resized, ...layout.filter((o) => o !== p).sort(byReading)], columns)
}

// layout in a grid of columns: a tile past the edge is pulled in, and those in its way move on.
export const reflow = (layout: Arranged[], columns: number) => settle([...layout].sort(byReading), columns)
