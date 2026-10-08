import { expect, test } from 'vitest'
import { dashboard } from './dashboard'
import { tileLayout, type SavedLayout } from './tileLayout'
import type { Area, Capability, CustomDashboard, Device, Fn } from './types'

const on: Capability = { key: 'state', label: 'State', type: 'binary', access: { observable: true, settable: true, queryable: true }, category: 'primary' }
const fn = (key: string): Fn => ({ key, kind: key, capabilities: [on] })
const reading = (key: string): Fn => ({
  key,
  kind: key,
  capabilities: [{ ...on, key, type: 'numeric', access: { observable: true, settable: false, queryable: false } }],
})
const device = (id: string, area: string | undefined, ...functions: Fn[]): Device => ({ id, name: id, area, nativeAddress: id, functions, capabilities: null })

// Six readings, two to a line: drawn three rows tall while nobody sets its height.
const indoor = device('indoor', 'living', ...['temperature', 'humidity', 'co2', 'noise', 'pressure', 'rain'].map(reading))
const devices = [indoor, device('a', 'living', fn('switch')), device('b', 'living', fn('switch'))]
const living = (more: Partial<Area> = {}) => dashboard(devices, [], [], [{ id: 'living', name: 'Living', columns: 2, ...more }], []).sections[0]!

// The Layout saved in short: its columns, then each Tile's key, col, row, width and the height set.
const short = ({ columns, layout }: SavedLayout) => [columns, ...layout.map((p) => `${p.key} ${p.col}${p.row}${p.width}${p.height ?? ''}`)]

test("a Tile moved takes its cell, the one in its way its former place; a Tile's drawn height is never saved", () => {
  // drawn: the readings at 0,0 three rows tall, a at 1,0, b at 1,1
  expect(short(tileLayout(living()).moved('device:b', 0, 0))).toEqual([2, 'device:b 001', 'device:a 101', 'device:indoor 111'])
})

test('a Tile sized pushes those in its way on, by the height they are drawn with; the height set is saved', () => {
  expect(short(tileLayout(living()).sized('device:a', { width: 2 }))).toEqual([2, 'device:a 002', 'device:indoor 011', 'device:b 111'])
  expect(short(tileLayout(living()).sized('device:a', { height: 2 }))).toEqual([2, 'device:a 1012', 'device:indoor 001', 'device:b 121'])
})

test('a Section in fewer columns pulls a Tile past the edge in', () => {
  // drawn in three columns: a at 1,0 two wide, the readings under it at 0,1, b beside them
  const wide = living({ layout: [{ tile: 'device:a', col: 1, row: 0, width: 2 }], columns: 3 })
  expect(short(tileLayout(wide).columns(2))).toEqual([2, 'device:a 002', 'device:indoor 011', 'device:b 111'])
})

test('a Tile added in an own Section takes its free cell, beside a placement showing nothing, which keeps its own (ADR 0045)', () => {
  const d: CustomDashboard = {
    id: 'evening',
    name: 'Evening',
    columns: 1,
    sections: [
      {
        id: 'fav',
        columns: 2,
        col: 0,
        row: 0,
        width: 1,
        tiles: [
          { target: 'device:a', col: 0, row: 0, width: 1 },
          { target: 'device:gone', col: 1, row: 0, width: 1 },
        ],
      },
    ],
  }
  const [fav] = dashboard(devices, [], [], [], []).custom(d, true)
  expect(short(tileLayout(fav!).added('flag:away', 0, 1))).toEqual([2, 'device:a 001', 'device:gone 101', 'flag:away 011'])
})
