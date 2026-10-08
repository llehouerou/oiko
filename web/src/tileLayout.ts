// A Section's Tiles rearranged while arranging, an Area's or an own Section's: each change answers
// the Layout to save (ADR 0015, 0043). The Section is as dashboard.ts drew it, so a Tile's height
// nobody set is the one it is drawn with, and never saved.

import type { DashboardSection } from './dashboard'
import { defaultColumns, move, placements, reflow, resize, settle, stored, type Arranged } from './layout'

// A Layout as saved: its columns, and the place of each Tile, a height only where someone set it.
export interface SavedLayout {
  columns: number
  layout: Arranged[]
}

export function tileLayout(section: DashboardSection) {
  const columns = section.columns ?? defaultColumns
  const drawn = placements(section.tiles)
  const saved = (columns: number, layout: Arranged[]): SavedLayout => ({ columns, layout: stored(layout) })
  return {
    // Tile key dropped at col and row; one in its way takes its former place.
    moved: (key: string, col: number, row: number) => saved(columns, move(drawn, key, col, row, columns)),
    // Tile key resized; those in its way move on. The height set is kept from then on.
    sized: (key: string, size: { width?: number; height?: number }) => saved(columns, resize(drawn, key, size, columns)),
    // The Section in n columns: a Tile past the edge is pulled in, those in its way move on.
    columns: (n: number) => saved(n, reflow(drawn, n)),
    // Tile key added in the free cell at col and row, one column wide and one row tall.
    added: (key: string, col: number, row: number) => saved(columns, settle([...drawn, { key, col, row, width: 1, height: 1, auto: true }], columns)),
  }
}
