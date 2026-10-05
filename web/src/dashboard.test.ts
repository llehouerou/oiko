import { expect, test } from 'vitest'
import type { Document } from './automation/model'
import { dashboard, type DashboardSection } from './dashboard'
import type { Aggregate, Area, Capability, Device, Fn, Target } from './types'

const on: Capability = { key: 'state', label: 'State', type: 'binary', access: { observable: true, settable: true, queryable: true }, category: 'primary' }
const fn = (key: string, area?: string): Fn => ({ key, kind: key, area, capabilities: [on] })
const device = (id: string, area: string | undefined, ...functions: Fn[]): Device => ({ id, name: id, area, nativeAddress: id, functions, capabilities: null })
const derived = (area: string, kind: string, ...members: Target[]): Aggregate => ({
  id: `${area}-${kind}`,
  name: kind,
  area,
  derived: true,
  kind,
  members,
  binary: 'any',
  numeric: 'mean',
  capabilities: [on],
})
const area = (id: string, more: Partial<Area> = {}): Area => ({ id, name: id, ...more })

// Each Section in short: its Area, header and the keys of its tiles.
const short = (s: DashboardSection) => ({
  area: s.area?.id,
  climate: s.climate.map((a) => a.id),
  doors: s.doors?.id,
  presence: s.presence?.id,
  bar: s.bar && `${s.bar.fns.map((f) => f.target)} ${s.bar.shape.kind}`,
  tiles: s.tiles.map((t) => t.key),
})

test("an Area's lone light is its bar, not a tile; two lights make their Aggregate the bar", () => {
  const devices = [device('lamp', 'living', fn('light')), device('a', 'kitchen', fn('light')), device('b', 'kitchen', fn('light'))]
  const aggregates = [
    derived('living', 'light', 'device:lamp/light'),
    derived('kitchen', 'light', 'device:a/light', 'device:b/light'),
    derived('living', 'temperature', 'device:t/temperature'),
    derived('living', 'contact', 'device:d/contact'),
  ]
  const { sections } = dashboard(devices, aggregates, [], [area('living'), area('kitchen')], [])
  expect(sections.map(short)).toEqual([
    { area: 'living', climate: ['living-temperature'], doors: 'living-contact', presence: undefined, bar: 'device:lamp/light bar', tiles: [] },
    { area: 'kitchen', climate: [], doors: undefined, presence: undefined, bar: 'aggregate:kitchen-light bar', tiles: ['device:a', 'device:b'] },
  ])
})

test('an Area hiding its lights Aggregate has no bar, its lone light a tile', () => {
  const { sections } = dashboard(
    [device('lamp', 'living', fn('light'))],
    [derived('living', 'light', 'device:lamp/light')],
    [],
    [area('living', { hiddenAggregates: ['light'] })],
    [],
  )
  expect(sections.map(short)).toMatchObject([{ bar: undefined, tiles: ['device:lamp'] }])
})

test("a Device's tile in each Area gathers its Functions there; one with nothing to show has none", () => {
  const nothing: Fn = { key: 'config', kind: 'config', capabilities: [{ ...on, category: 'config' }] }
  const relay = device('relay', 'living', fn('l1'), fn('l2', 'kitchen'), nothing)
  const { sections } = dashboard([relay], [], [], [area('living'), area('kitchen')], [])
  const fns = sections.map((s) => s.tiles.flatMap((t) => (t.kind === 'target' ? t.fns.map((f) => f.fn.key) : [])))
  expect(fns).toEqual([['l1'], ['l2']])
})

const only = (sections: DashboardSection[]) => {
  const [t] = sections.flatMap((s) => s.tiles)
  if (t?.kind !== 'target') throw new Error('no Target tile')
  return t
}
const reading = (key: string): Capability => ({ ...on, key, type: 'numeric', access: { observable: true, settable: false, queryable: false } })

test("a Device's Tile is dimmed while detached, its health from all its Functions, its lone Function's kind on its header", () => {
  const battery = reading('battery')
  const relay: Device = { ...device('relay', 'living', fn('switch')), detached: true, capabilities: [battery] }
  expect(only(dashboard([relay], [], [], [area('living')], []).sections)).toMatchObject({
    subject: 'device:relay',
    detached: true,
    health: [{ target: 'device:relay', cap: battery }],
    shape: { kind: 'bar', target: 'device:relay/switch', toggle: 'state' },
  })
  const mode: Capability = { ...on, key: 'mode', type: 'enum', options: ['heat', 'off'] }
  const thermostat = device('t', 'living', { key: 'thermostat', kind: 'thermostat', capabilities: [mode, reading('temperature')] })
  expect(only(dashboard([thermostat], [], [], [area('living')], []).sections)).toMatchObject({ summary: 'thermostat', shape: { kind: 'controls' } })
})

test("an Aggregate of lights' Tile is a bar over its lights, with its rule on its header", () => {
  const lights: Aggregate = {
    id: 'g',
    name: 'Lights',
    area: 'living',
    kind: 'light',
    members: ['device:a/light', 'device:b/light'],
    binary: 'any',
    numeric: 'mean',
    capabilities: [on],
  }
  expect(only(dashboard([], [lights], [], [area('living')], []).sections)).toMatchObject({
    subject: 'aggregate:g',
    members: ['device:a/light', 'device:b/light'],
    summary: 'light · any of 2',
    shape: { kind: 'bar' },
  })
})

test("a Flag's Tile is a bar that toggles it", () => {
  const guest = { id: 'guest', name: 'Guest', kind: 'flag', area: 'living', capabilities: [{ ...on, key: 'on' }] }
  expect(only(dashboard([], [], [guest], [area('living')], []).sections)).toMatchObject({
    subject: 'flag:guest',
    health: [],
    shape: { kind: 'bar', target: 'flag:guest', toggle: 'on' },
  })
})

test('Flags without an Area are pills; Others opens with the manual Automations and shows only if it holds anything', () => {
  const flags = [
    { id: 'away', name: 'Away', kind: 'flag', capabilities: [on] },
    { id: 'guest', name: 'Guest', kind: 'flag', area: 'living', capabilities: [on] },
  ]
  const group: Aggregate = { id: 'g', name: 'G', members: [], binary: 'any', numeric: 'mean' }
  const manual = [{ id: 'bedtime', name: 'Bedtime' } as Document]
  const empty = dashboard([], [], flags, [area('living')], [])
  expect(empty.pills.map((f) => f.id)).toEqual(['away'])
  expect(empty.sections.map(short)).toMatchObject([{ area: 'living', tiles: ['flag:guest'] }])
  const full = dashboard([device('lamp', undefined, fn('light'))], [group], flags, [area('living')], manual)
  expect(full.sections.map(short).at(-1)).toEqual({
    area: undefined,
    climate: [],
    doors: undefined,
    presence: undefined,
    bar: undefined,
    tiles: ['bedtime', 'aggregate:g', 'device:lamp'],
  })
})

test("an Area's tiles come in its Layout's reading order", () => {
  const devices = [device('a', 'living', fn('switch')), device('b', 'living', fn('switch'))]
  const { sections } = dashboard(devices, [], [], [area('living', { columns: 2, layout: [{ tile: 'device:b', col: 0, row: 0, width: 1 }] })], [])
  expect(sections[0]).toMatchObject({ columns: 2, tiles: [{ key: 'device:b' }, { key: 'device:a', place: { col: 1, row: 0 } }] })
})

test('a tile of readings takes a row per line of its cells, unless its height is set', () => {
  const reading = (key: string): Fn => ({
    key,
    kind: key,
    capabilities: [{ ...on, key, type: 'numeric', access: { observable: true, settable: false, queryable: false } }],
  })
  const indoor = device('indoor', 'living', ...['temperature', 'humidity', 'co2', 'noise', 'pressure'].map(reading))
  const devices = [indoor, device('a', 'living', fn('switch')), device('b', 'living', fn('switch')), device('c', 'living', fn('switch'))]
  const places = (layout: Area['layout']) =>
    dashboard(devices, [], [], [area('living', { columns: 2, layout })], []).sections[0]!.tiles.map(
      (t) => `${t.key} ${t.place?.col}${t.place?.row} ${t.place?.height}`,
    )
  // two readings to a line: three rows, the switches beside
  expect(places([])).toEqual(['device:indoor 00 3', 'device:a 10 1', 'device:b 11 1', 'device:c 12 1'])
  // two columns wide, three to a line: two rows
  expect(places([{ tile: 'device:indoor', col: 0, row: 0, width: 2 }])[0]).toBe('device:indoor 00 2')
  expect(places([{ tile: 'device:indoor', col: 0, row: 0, width: 1, height: 1 }])[0]).toBe('device:indoor 00 1')
})
