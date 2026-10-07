// Edits to a custom Dashboard's document, each giving the whole Dashboard to save (ADR 0043): its
// Sections moved, sized, added and removed on its Layout with the grid code of an Area's.

import { move, reflow, resize, stored, type Arranged, type Place } from './layout'
import type { AreaSection, CustomDashboard, CustomSection, OwnSection } from './types'

// The key of Section s on its Dashboard's Layout, as dashboard.ts names it.
export const sectionKey = (s: CustomSection) => (s.area !== undefined ? `area:${s.area}` : `own:${s.id}`)

// What a Section is, apart from its place.
export type SectionContent = AreaSection | OwnSection

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

// d with section added at col and row, one column wide and one row tall.
export const addSection = (d: CustomDashboard, section: SectionContent, col: number, row: number) =>
  arranged({ ...d, sections: [...d.sections, { ...section, col, row, width: 1 } as CustomSection] }, (l) => reflow(l, d.columns))

export const moveSection = (d: CustomDashboard, key: string, col: number, row: number) => arranged(d, (l) => move(l, key, col, row, d.columns))

export const resizeSection = (d: CustomDashboard, key: string, size: { width?: number; height?: number }) => arranged(d, (l) => resize(l, key, size, d.columns))

export const removeSection = (d: CustomDashboard, key: string) => ({ ...d, sections: d.sections.filter((s) => sectionKey(s) !== key) })

export const setColumns = (d: CustomDashboard, columns: number) => arranged({ ...d, columns }, (l) => reflow(l, columns))

// d with own Section key changed as change says.
export const updateSection = (d: CustomDashboard, key: string, change: Partial<OwnSection>) => ({
  ...d,
  sections: d.sections.map((s) => (sectionKey(s) === key ? ({ ...s, ...change } as CustomSection) : s)),
})

// d with own Section key in columns, its Tiles pulled in.
export function setSectionColumns(d: CustomDashboard, key: string, columns: number) {
  const s = d.sections.find((s) => sectionKey(s) === key)
  if (!s || s.area !== undefined) return d
  const tileKey = (t: NonNullable<typeof s.tiles>[number]) => t.target ?? `automation ${t.automation}`
  const tiles = s.tiles && placedAs(s.tiles, tileKey, reflow(layoutOf(s.tiles, tileKey), columns))
  return updateSection(d, key, { columns, tiles })
}
