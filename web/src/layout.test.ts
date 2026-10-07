import { expect, test } from 'vitest'
import { arrange, move, placements, reflow, resize, stored, type Arranged } from './layout'

const at = (key: string, col: number, row: number, width = 1): Arranged => ({ key, col, row, width })
// A layout in short, in reading order: each occupant, col, row and width.
const short = (layout: Arranged[]) => [...layout].sort((a, b) => a.row - b.row || a.col - b.col).map((p) => `${p.key}${p.col}${p.row}${p.width}`)
test('a Layout keeps its gaps; a tile it does not place takes the next cell after the last one placed', () => {
  const tiles = ['a', 'b', 'c', 'd'].map((key) => ({ key }))
  const layout = [
    { key: 'b', col: 1, row: 0, width: 2 },
    { key: 'a', col: 0, row: 2, width: 1 },
    { key: 'left', col: 1, row: 2, width: 1 }, // its tile left the Area
  ]
  const placed = arrange(tiles, 3, layout)
  expect(placed.map((t) => [t.key, t.place.col, t.place.row, t.place.width])).toEqual([
    ['b', 1, 0, 2],
    ['a', 0, 2, 1],
    ['c', 1, 2, 1],
    ['d', 2, 2, 1],
  ])
  expect(arrange([{ key: 'x' }, { key: 'y' }], 1).map((t) => t.place.row)).toEqual([0, 1])
})

test('a tile moves into a gap, or swaps places with the tile in its way; never past the edge', () => {
  const layout = [at('a', 0, 0), at('b', 1, 0), at('c', 0, 1, 2)]
  expect(short(move(layout, 'a', 2, 2, 3))).toEqual(['b101', 'c012', 'a221'])
  expect(short(move(layout, 'a', 1, 0, 3))).toEqual(['b001', 'a101', 'c012'])
  // c is two wide: dropped on the last column, it is pulled left, and b takes its place
  expect(short(move(layout, 'c', 2, 0, 3))).toEqual(['a001', 'c102', 'b011'])
  // what no longer fits where it goes moves on to the next free cells
  expect(short(move(layout, 'a', 1, 1, 3))).toEqual(['b101', 'a111', 'c022'])
})

test('a tile widened pushes the tiles in its way on; one on the edge widens leftwards', () => {
  const layout = [at('a', 0, 0), at('b', 1, 0), at('c', 2, 0)]
  expect(short(resize(layout, 'a', { width: 2 }, 3))).toEqual(['a002', 'b201', 'c011'])
  expect(short(resize(layout, 'c', { width: 2 }, 3))).toEqual(['a001', 'c102', 'b011'])
  expect(short(resize(layout, 'a', { width: 9 }, 3))).toEqual(['a003', 'b011', 'c111'])
})

test('a tile several rows tall leaves the cells beside it to others', () => {
  const tall = { key: 't', col: 0, row: 0, width: 1, height: 3 }
  const placed = arrange(
    ['t', 'a', 'b', 'c', 'd'].map((key) => ({ key })),
    2,
    [tall],
  )
  expect(placed.map((t) => `${t.key}${t.place.col}${t.place.row}`)).toEqual(['t00', 'a10', 'b11', 'c12', 'd03'])
  // made taller, it pushes the tile under it beside it
  const layout = [at('a', 0, 0), at('b', 0, 1), at('c', 1, 0)]
  const taller = resize(layout, 'a', { height: 2 }, 2)
  expect(short(taller)).toEqual(['a001', 'c101', 'b111'])
  expect(taller.find((p) => p.key === 'a')?.height).toBe(2)
  // a tile dropped on its lower part swaps places with it
  expect(short(move([tall, at('a', 1, 0)], 'a', 0, 2, 2))).toEqual(['t101', 'a021'])
})

test("only the heights an Admin set are stored; a tile's own follows its readings", () => {
  const rowsOf = (t: { key: string }) => (t.key === 'station' ? 3 : 1)
  const tiles = ['station', 'lamp'].map((key) => ({ key }))
  const arranged = placements(arrange(tiles, 2, [], rowsOf))
  // moving the lamp stores no height: the station keeps its own, three rows
  expect(stored(move(arranged, 'lamp', 1, 2, 2))).toEqual([
    { key: 'lamp', col: 1, row: 2, width: 1 },
    { key: 'station', col: 0, row: 0, width: 1 },
  ])
  // setting the station's height stores it
  expect(stored(resize(arranged, 'station', { height: 1 }, 2))[0]).toEqual({ key: 'station', col: 0, row: 0, width: 1, height: 1 })
})

test('fewer columns pull the tiles past the edge in, narrowed if need be', () => {
  const layout = [at('a', 0, 0), at('b', 1, 0), at('c', 2, 0), at('d', 0, 1, 3)]
  expect(short(reflow(layout, 2))).toEqual(['a001', 'b101', 'c011', 'd022'])
})

test("a Dashboard's Sections take their places on its Layout as Tiles do on an Area's", () => {
  const sections = [{ key: 'area:living', name: 'Living room' }, { key: 'own:favourites' }, { key: 'area:office', name: 'Office' }]
  const placed = arrange(sections, 2, [{ key: 'own:favourites', col: 0, row: 0, width: 2 }], (s) => (s.key === 'area:living' ? 2 : 1))
  expect(placed.map((s) => `${s.key} ${s.place.col}${s.place.row} ${s.place.height}`)).toEqual(['own:favourites 00 1', 'area:living 01 2', 'area:office 11 1'])
  expect(placed[1]).toMatchObject({ name: 'Living room' })
  const layout = placements(placed)
  // the Section in its way takes its former place, or the first free cells after it
  expect(short(move(layout, 'area:office', 0, 0, 2))).toEqual(['area:office001', 'area:living011', 'own:favourites032'])
  expect(short(resize(layout, 'area:office', { width: 2 }, 2))).toEqual(['own:favourites002', 'area:office012', 'area:living021'])
  expect(short(reflow(layout, 1))).toEqual(['own:favourites001', 'area:living011', 'area:office031'])
  expect(stored(layout).map((p) => p.key)).toEqual(['own:favourites', 'area:living', 'area:office'])
})
