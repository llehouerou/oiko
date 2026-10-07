// Edits to a custom Dashboard's document, each giving the whole Dashboard to save (ADR 0043): its
// Sections moved, sized, added and removed on its Layout with the grid code of an Area's.

import { sectionKey } from './dashboard'
import { move, resize, reflow, stored, type Arranged, type Place } from './layout'
import type { AreaSection, CustomDashboard, CustomSection, OwnSection, PlacedTile } from './types'

// What a Section is, apart from its place.
export type SectionContent = AreaSection | OwnSection

// The key that names Tile t on its own Section's Layout.
export const tileKey = (t: PlacedTile) => t.target ?? `automation ${t.automation}`

// occupants as a Layout, keyed by key: a height nobody set is the occupant's own, one row.
const layoutOf = <T extends Place>(occupants: T[], key: (o: T) => string): Arranged[] =>
  occupants.map((o) => ({ key: key(o), col: o.col, row: o.row, width: o.width, height: o.height ?? 1, auto: o.height === undefined }))

// occupants each moved to its place in layout, in its reading order; those layout lacks dropped.
function placedAs<T extends Place>(occupants: T[], key: (o: T) => string, layout: Arranged[]): T[] {
  return stored(layout).flatMap(({ key: k, ...place }) =>
    occupants.filter((o) => key(o) === k).map(({ col: _c, row: _r, width: _w, height: _h, ...o }) => ({ ...o, ...place }) as T),
  )
}

// d with its Sections where f arranges them, given their Layout, in reading order.
const arranged = (d: CustomDashboard, f: (layout: Arranged[]) => Arranged[]): CustomDashboard => ({
  ...d,
  sections: placedAs(d.sections, sectionKey, f(layoutOf(d.sections, sectionKey))).sort((a, b) => a.row - b.row || a.col - b.col),
})

// d with section added at col and row, one column wide and one row tall; whatever its stored
// Layout still holds there moves on.
export function addSection(d: CustomDashboard, section: SectionContent, col: number, row: number) {
  const added = { ...section, col, row, width: 1 } as CustomSection
  return arranged({ ...d, sections: [added, ...d.sections] }, (l) => move(l, sectionKey(added), col, row, d.columns))
}

export const moveSection = (d: CustomDashboard, key: string, col: number, row: number) => arranged(d, (l) => move(l, key, col, row, d.columns))

export const resizeSection = (d: CustomDashboard, key: string, size: { width?: number; height?: number }) => arranged(d, (l) => resize(l, key, size, d.columns))

export const removeSection = (d: CustomDashboard, key: string) => ({ ...d, sections: d.sections.filter((s) => sectionKey(s) !== key) })

export const setDashboardColumns = (d: CustomDashboard, columns: number) => arranged({ ...d, columns }, (l) => reflow(l, columns))

// d with own Section key as f makes it; an Area's Section has nothing of its own to change.
const changeOwn = (d: CustomDashboard, key: string, f: (s: OwnSection & Place) => OwnSection & Place) => ({
  ...d,
  sections: d.sections.map((s) => (s.area === undefined && sectionKey(s) === key ? f(s) : s)),
})

// d with own Section key's Name or Icon changed.
export const nameSection = (d: CustomDashboard, key: string, change: { name?: string; icon?: string }) => changeOwn(d, key, (s) => ({ ...s, ...change }))

// d with own Section key in columns, its Tiles pulled in.
export const setSectionColumns = (d: CustomDashboard, key: string, columns: number) =>
  changeOwn(d, key, (s) => ({ ...s, columns, tiles: s.tiles && placedAs(s.tiles, tileKey, reflow(layoutOf(s.tiles, tileKey), columns)) }))
