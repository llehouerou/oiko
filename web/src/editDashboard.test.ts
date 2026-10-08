import { expect, test } from 'vitest'
import { addSection, layTiles, moveSection, nameSection, removeSection, removeTile, resizeSection, setDashboardColumns, nameTile } from './editDashboard'
import { sectionKey, tileKey } from './dashboard'
import type { CustomDashboard, OwnSection, PlacedTile } from './types'
import type { Arranged, Place } from './layout'

// Oiko's one check of a Layout (home.CheckLayout): 1 to 6 columns, each occupant inside them,
// placed once, apart from the others.
function check(columns: number, layout: (Place & { key: string })[]) {
  expect(columns).toBeGreaterThanOrEqual(1)
  expect(columns).toBeLessThanOrEqual(6)
  const taken = new Set<string>()
  expect(new Set(layout.map((p) => p.key)).size).toBe(layout.length)
  for (const p of layout) {
    expect(p.col >= 0 && p.row >= 0 && p.width >= 1 && p.col + p.width <= columns, `${p.key} inside`).toBe(true)
    for (let r = p.row; r < p.row + Math.max(p.height ?? 1, 1); r++)
      for (let c = p.col; c < p.col + p.width; c++) {
        expect(taken.has(`${c},${r}`), `${p.key} apart`).toBe(false)
        taken.add(`${c},${r}`)
      }
  }
}
function passes(d: CustomDashboard) {
  check(
    d.columns,
    d.sections.map((s) => ({ ...s, key: sectionKey(s) })),
  )
  for (const s of d.sections)
    if (s.area === undefined)
      check(
        s.columns,
        (s.tiles ?? []).map((t) => ({ ...t, key: tileKey(t) })),
      )
  return d
}
// The Sections in short, in their order: each key, col, row, width and height.
const short = (d: CustomDashboard) => d.sections.map((s) => `${sectionKey(s)} ${s.col}${s.row}${s.width}${s.height ?? ''}`)

const empty: CustomDashboard = { id: 'd', owner: 'alice', name: 'Evening', columns: 2, sections: [] }
const own = (id: string, tiles?: PlacedTile[]): OwnSection => ({ id, columns: 2, tiles })

test("a Section added lands in the cell asked, one column wide and one row tall, in the Layout's reading order", () => {
  const living = passes(addSection(empty, { area: 'living' }, 1, 0))
  expect(living.sections).toEqual([{ area: 'living', col: 1, row: 0, width: 1 }])
  const both = passes(addSection(living, own('fav'), 0, 0))
  expect(short(both)).toEqual(['own:fav 001', 'area:living 101'])
  expect(both.sections[0]).toEqual({ id: 'fav', columns: 2, col: 0, row: 0, width: 1 })
  expect(both).toMatchObject({ id: 'd', owner: 'alice', name: 'Evening', columns: 2 })
})

test('a Section added where the stored Layout still holds one it no longer shows takes its cell', () => {
  // the Office is gone: the editor draws its cell free, a + in it
  const d = { ...empty, sections: [{ area: 'office', col: 0, row: 0, width: 1 }] }
  expect(short(passes(addSection(d, { area: 'living' }, 0, 0)))).toEqual(['area:living 001', 'area:office 101'])
})

test('moving, resizing and removing Sections keeps them apart, inside the columns', () => {
  let d = addSection(addSection(addSection(empty, own('fav'), 0, 0), { area: 'living' }, 1, 0), { area: 'office' }, 0, 1)
  // the Section in the way takes the moved one's former place
  d = passes(moveSection(d, 'area:office', 1, 0))
  expect(short(d)).toEqual(['own:fav 001', 'area:office 101', 'area:living 011'])
  // grown wider, a Section pushes those in its way on to the first free cells after it
  d = passes(resizeSection(d, 'own:fav', { width: 2 }))
  expect(short(d)).toEqual(['own:fav 002', 'area:office 011', 'area:living 111'])
  // a height set is stored; a Section's own height, one row, never is
  d = passes(resizeSection(d, 'area:office', { height: 2 }))
  expect(short(d)).toEqual(['own:fav 002', 'area:office 0112', 'area:living 111'])
  d = passes(removeSection(d, 'own:fav'))
  expect(short(d)).toEqual(['area:office 0112', 'area:living 111'])
  // fewer columns pull the Sections in, those in the way moving on
  d = passes(setDashboardColumns(d, 1))
  expect(d.columns).toBe(1)
  expect(short(d)).toEqual(['area:office 0112', 'area:living 031'])
})

test("an own Section's Name and Icon change; an Area's Section has none", () => {
  let d = addSection(addSection(empty, own('fav'), 0, 0), { area: 'living' }, 1, 0)
  d = passes(nameSection(d, 'own:fav', { name: 'Favourites', icon: 'sofa' }))
  expect(d.sections[0]).toMatchObject({ name: 'Favourites', icon: 'sofa' })
  expect(nameSection(d, 'area:living', { name: 'Lounge' }).sections[1]).toEqual({ area: 'living', col: 1, row: 0, width: 1 })
})

// A Tile's place in a Layout as saved: its height only where someone set it.
const at = (key: string, col: number, row: number, more: Partial<Arranged> = {}): Arranged => ({ key, col, row, width: 1, ...more })

test("an own Section's Tiles are laid as their saved Layout says, one added joining them, one removed leaving them", () => {
  let d = addSection(addSection(empty, own('fav', [{ target: 'flag:away', col: 0, row: 0, width: 1 }]), 0, 0), own('more'), 1, 0)
  d = passes(layTiles(d, 'own:fav', { columns: 2, layout: [at('flag:away', 0, 0), at('device:lamp', 1, 0)] }, { target: 'device:lamp' }))
  d = passes(
    layTiles(d, 'own:fav', { columns: 2, layout: [at('flag:away', 0, 0), at('device:lamp', 1, 0), at('automation:night', 0, 1)] }, { automation: 'night' }),
  )
  expect(d.sections[0]).toEqual({
    ...own('fav'),
    col: 0,
    row: 0,
    width: 1,
    tiles: [
      { target: 'flag:away', col: 0, row: 0, width: 1 },
      { target: 'device:lamp', col: 1, row: 0, width: 1 },
      { automation: 'night', col: 0, row: 1, width: 1 },
    ],
  })
  // the same Tile in another Section of the Dashboard
  d = passes(layTiles(d, 'own:more', { columns: 2, layout: [at('device:lamp', 0, 0)] }, { target: 'device:lamp' }))
  expect(d.sections[1]).toMatchObject({ tiles: [{ target: 'device:lamp', col: 0, row: 0, width: 1 }] })
  // the Flag removed, then the others in one column, a height set
  d = passes(removeTile(d, 'own:fav', 'flag:away'))
  d = passes(layTiles(d, 'own:fav', { columns: 1, layout: [at('automation:night', 0, 0, { height: 2 }), at('device:lamp', 0, 2)] }))
  expect(d.sections[0]).toMatchObject({
    columns: 1,
    tiles: [
      { automation: 'night', col: 0, row: 0, width: 1, height: 2 },
      { target: 'device:lamp', col: 0, row: 2, width: 1 },
    ],
  })
  // an Area's Section has no Tiles of its own
  const living = addSection(d, { area: 'living' }, 0, 1)
  expect(layTiles(living, 'area:living', { columns: 1, layout: [at('device:lamp', 0, 0)] }, { target: 'device:lamp' })).toEqual(living)
})

test("a Tile takes a name of its own and hides it, then its own name comes back, the other Tiles' as they were", () => {
  const tiles: PlacedTile[] = [
    { target: 'flag:away', col: 0, row: 0, width: 1 },
    { automation: 'night', name: 'Bedtime', hideName: true, col: 1, row: 0, width: 1 },
  ]
  let d = addSection(empty, own('fav', tiles), 0, 0)
  d = nameTile(d, 'own:fav', 'flag:away', { name: ' Holidays ' })
  d = nameTile(d, 'own:fav', 'flag:away', { hideName: true })
  expect(d.sections[0]).toMatchObject({ tiles: [{ name: 'Holidays', hideName: true }, tiles[1]!] })
  // an empty name is none, a name shown is stored as nothing
  d = nameTile(d, 'own:fav', 'flag:away', { name: ' ', hideName: false })
  expect(d.sections[0]).toMatchObject({ tiles })
  expect(Object.keys((d.sections[0] as OwnSection).tiles![0]!).sort()).toEqual(['col', 'row', 'target', 'width'])
})

test('a Tile placed that shows nothing stays placed, moving on when a Tile shown takes its cell (ADR 0045)', () => {
  // the Fan has no Function with a Tile left: the Section draws the Lamp only
  const tiles: PlacedTile[] = [
    { target: 'device:fan', col: 1, row: 0, width: 1 },
    { target: 'device:lamp', col: 0, row: 0, width: 1 },
  ]
  let d = passes(addSection(empty, own('fav', tiles), 0, 0))
  d = passes(layTiles(d, 'own:fav', { columns: 2, layout: [at('device:lamp', 1, 0)] }))
  expect(d.sections[0]).toMatchObject({
    tiles: [
      { target: 'device:lamp', col: 1, row: 0, width: 1 },
      { target: 'device:fan', col: 0, row: 1, width: 1 },
    ],
  })
  d = passes(layTiles(d, 'own:fav', { columns: 2, layout: [at('device:lamp', 1, 0), at('flag:away', 0, 1)] }, { target: 'flag:away' }))
  expect(d.sections[0]).toMatchObject({
    tiles: [
      { target: 'device:lamp', col: 1, row: 0, width: 1 },
      { target: 'flag:away', col: 0, row: 1, width: 1 },
      { target: 'device:fan', col: 1, row: 1, width: 1 },
    ],
  })
})
