// Edits to a custom Dashboard's document, each giving the whole Dashboard to save (ADR 0043): its
// Sections, and an own Section's Tiles, moved, sized, added and removed on their Layouts with the
// grid code of an Area's.

import { sectionKey, tileKey } from './dashboard'
import { move, resize, reflow, stored, type Arranged, type Place } from './layout'
import type { AreaSection, CustomDashboard, CustomSection, OwnSection, TileRef } from './types'

// What a Section is, apart from its place.
export type SectionContent = AreaSection | OwnSection

// occupants as a Layout, keyed by key: a height nobody set is the occupant's own, one row.
const layoutOf = <T extends Place>(occupants: T[], key: (o: T) => string): Arranged[] =>
  occupants.map((o) => ({ key: key(o), col: o.col, row: o.row, width: o.width, height: o.height ?? 1, auto: o.height === undefined }))

// occupants each moved to its place in layout, in reading order; those layout lacks dropped.
function placedAs<T extends Place>(occupants: T[], key: (o: T) => string, layout: Arranged[]): T[] {
  return stored(layout)
    .flatMap(({ key: k, ...place }) =>
      occupants.filter((o) => key(o) === k).map(({ col: _c, row: _r, width: _w, height: _h, ...o }) => ({ ...o, ...place }) as T),
    )
    .sort((a, b) => a.row - b.row || a.col - b.col)
}

// d with its Sections where f arranges them, given their Layout.
const arranged = (d: CustomDashboard, f: (layout: Arranged[]) => Arranged[]): CustomDashboard => ({
  ...d,
  sections: placedAs(d.sections, sectionKey, f(layoutOf(d.sections, sectionKey))),
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

// d with own Section key's Tiles in columns, where layout places them; layout is theirs as the
// Section draws them, which leaves out those that show nothing. The Tile added is one of layout's
// that the Section did not hold.
export const layTiles = (d: CustomDashboard, key: string, columns: number, layout: Arranged[], added?: TileRef) =>
  changeOwn(d, key, (s) => ({
    ...s,
    columns,
    tiles: placedAs([...(s.tiles ?? []), ...(added ? [{ ...added, col: 0, row: 0, width: 1 }] : [])], tileKey, layout),
  }))
