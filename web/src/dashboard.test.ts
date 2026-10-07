import { expect, test } from 'vitest'
import { copied, dashboard, deleting, shown, type DashboardSection } from './dashboard'
import type { Aggregate, Area, AutomationStatus, Capability, CustomDashboard, Device, Fn, Target } from './types'

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

test('a camera has a Tile though it has no Capability', () => {
  const camera: Fn = { key: 'camera', kind: 'camera', capabilities: [] }
  expect(only(dashboard([device('garden', 'living', camera)], [], [], [area('living')], []).sections)).toMatchObject({
    subject: 'device:garden',
    shape: { kind: 'camera', target: 'device:garden/camera' },
  })
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
  const night: AutomationStatus = { id: 'night', name: 'Night', status: 'enabled' } // no Manual trigger: no Tile
  const bedtime: AutomationStatus = { id: 'bedtime', name: 'Bedtime', status: 'enabled', manualTriggers: [{ step: 'go', name: 'Go' }] }
  const empty = dashboard([], [], flags, [area('living')], [night])
  expect(empty.pills.map((f) => f.id)).toEqual(['away'])
  expect(empty.sections.map(short)).toMatchObject([{ area: 'living', tiles: ['flag:guest'] }])
  const full = dashboard([device('lamp', undefined, fn('light'))], [group], flags, [area('living')], [night, bedtime])
  expect(full.sections.map(short).at(-1)).toEqual({
    area: undefined,
    climate: [],
    doors: undefined,
    presence: undefined,
    bar: undefined,
    tiles: ['automation:bedtime', 'aggregate:g', 'device:lamp'],
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

test("a custom Dashboard's Area Sections are the built-in Dashboard's, its own Sections hold what is placed in them", () => {
  const devices = [device('relay', 'living', fn('l1'), fn('l2', 'kitchen')), device('lamp', 'kitchen', fn('light'))]
  const aggregates = [derived('kitchen', 'light', 'device:lamp/light')]
  const flags = [{ id: 'away', name: 'Away', kind: 'flag', capabilities: [{ ...on, key: 'on' }] }]
  const bedtime: AutomationStatus = { id: 'bedtime', name: 'Bedtime', status: 'enabled', manualTriggers: [{ step: 'go', name: 'Go' }] }
  const night: AutomationStatus = { id: 'night', name: 'Night', status: 'enabled' } // no Manual trigger: no Tile
  const home = dashboard(devices, aggregates, flags, [area('living'), area('kitchen')], [bedtime, night])
  const evening: CustomDashboard = {
    id: 'evening',
    owner: 'alice',
    name: 'Evening',
    columns: 2,
    sections: [
      { area: 'kitchen', col: 1, row: 0, width: 1 },
      { area: 'gone', col: 0, row: 1, width: 1 },
      { id: 'empty', columns: 1, col: 1, row: 1, width: 1, tiles: [{ automation: 'night', col: 0, row: 0, width: 1 }] },
      {
        id: 'favourites',
        name: 'Favourites',
        icon: 'sofa',
        columns: 2,
        col: 0,
        row: 0,
        width: 1,
        tiles: [
          { target: 'device:relay', col: 0, row: 0, width: 1 },
          { target: 'aggregate:kitchen-light', col: 1, row: 0, width: 1 },
          { target: 'flag:away', col: 0, row: 1, width: 1 },
          { automation: 'bedtime', col: 1, row: 1, width: 1, height: 2 },
          { automation: 'night', col: 0, row: 2, width: 1 },
          { target: 'device:gone', col: 0, row: 3, width: 1 },
        ],
      },
    ],
  }
  const sections = home.custom(evening)
  // in reading order, each in its place; an Area that is gone, or an own Section with nothing to
  // show, shows nothing
  expect(sections.map((s) => `${s.key} ${s.place?.col}${s.place?.row}`)).toEqual(['own:favourites 00', 'area:kitchen 10'])
  const [own, kitchen] = sections
  expect(kitchen).toMatchObject(home.sections[1]!)
  expect(own).toMatchObject({ own: { name: 'Favourites', icon: 'sofa' }, columns: 2, climate: [] })
  expect(own!.tiles.map((t) => `${t.key} ${t.place?.col}${t.place?.row}${t.place?.height}`)).toEqual([
    'device:relay 001',
    'aggregate:kitchen-light 101',
    'flag:away 011',
    'automation:bedtime 112',
  ])
  // a Device's Tile shows each of its Functions that has one, whatever their Areas
  const relay = own!.tiles[0]!
  expect(relay.kind === 'target' && relay.fns.map((f) => f.target)).toEqual(['device:relay/l1', 'device:relay/l2'])
  // while editing, an own Section with nothing to show is there to edit, and what shows nothing too
  expect(home.custom(evening, true).map((s) => `${s.key} ${s.tiles.length}`)).toEqual(['own:favourites 6', 'area:kitchen 1', 'own:empty 1'])
})

test('an own Section a Guest sees nothing of, its Tiles left out by Oiko, leaves its cells empty', () => {
  const home = dashboard([device('lamp', 'kitchen', fn('light'))], [], [], [area('kitchen')], [])
  const shared: CustomDashboard = {
    id: 'evening',
    shared: true,
    name: 'Evening',
    columns: 2,
    sections: [
      { id: 'automations', name: 'Automations', columns: 1, col: 0, row: 0, width: 1 }, // as Oiko sends it: no Tiles
      { area: 'kitchen', col: 1, row: 0, width: 1 },
      { id: 'lamp', columns: 1, col: 0, row: 1, width: 1, tiles: [{ target: 'device:lamp', col: 0, row: 0, width: 1 }] },
    ],
  }
  expect(home.custom(shared).map((s) => `${s.key} ${s.place?.col}${s.place?.row}`)).toEqual(['area:kitchen 10', 'own:lamp 01'])
})

test("a Function's Tile shows it alone, named after its Device and its key, with the whole Device's health", () => {
  const battery = reading('battery')
  const plug: Device = { ...device('plug', 'kitchen', fn('switch/l1'), fn('switch/l2'), fn('switch/l3')), name: 'Kitchen plug', capabilities: [battery] }
  const home = dashboard([plug], [], [], [area('kitchen')], [])
  const tiles = (...targets: string[]) => ({
    id: 'mine',
    owner: 'alice',
    name: 'Mine',
    columns: 1,
    sections: [{ id: 'own', columns: 3, col: 0, row: 0, width: 1, tiles: targets.map((target, col) => ({ target, col, row: 0, width: 1 })) }],
  })
  // two gangs of three: two Tiles, each driving its own gang; beside them, the Device's own Tile
  const [own] = home.custom(tiles('device:plug/switch/l1', 'device:plug/switch/l3', 'device:plug', 'device:plug/gone'))
  expect(own!.tiles).toMatchObject([
    {
      key: 'device:plug/switch/l1',
      label: 'Kitchen plug · switch/l1',
      name: 'Kitchen plug · switch/l1',
      subject: 'device:plug',
      health: [{ target: 'device:plug', cap: battery }],
      shape: { kind: 'bar', target: 'device:plug/switch/l1', toggle: 'state' },
    },
    { key: 'device:plug/switch/l3', name: 'Kitchen plug · switch/l3', shape: { kind: 'bar', target: 'device:plug/switch/l3' } },
    { key: 'device:plug', name: 'Kitchen plug' },
  ])
  expect(own!.tiles[2]!.kind === 'target' && own!.tiles[2]!.fns.length).toBe(3)
})

test('a placement showing nothing leaves its cells empty, but in the editor: a card naming it, saying why it is empty', () => {
  const plug: Device = { ...device('plug', 'kitchen', fn('switch/l1')), name: 'Kitchen plug' }
  const bare: Device = { ...device('bare', 'kitchen'), name: 'Bare' }
  const night: AutomationStatus = { id: 'night', name: 'Night', status: 'enabled' }
  const home = dashboard([plug, bare], [], [], [area('kitchen')], [night])
  const placed = ['device:plug/switch/l2', 'device:bare', 'automation:night', 'device:gone']
  const mine: CustomDashboard = {
    id: 'mine',
    owner: 'alice',
    name: 'Mine',
    columns: 2,
    sections: [
      { id: 'kept', columns: 1, col: 0, row: 0, width: 1, tiles: [{ target: 'device:plug/switch/l1', col: 0, row: 0, width: 1 }] },
      {
        id: 'dormant',
        columns: 4,
        col: 1,
        row: 0,
        width: 1,
        tiles: placed.map((key, col) =>
          key.startsWith('automation:') ? { automation: key.slice('automation:'.length), col, row: 0, width: 1 } : { target: key, col, row: 0, width: 1 },
        ),
      },
    ],
  }
  // viewers see none of it, nor the Section it leaves empty
  expect(home.custom(mine).map((s) => s.key)).toEqual(['own:kept'])
  // the editor shows each in its cells, named, with why
  const [, dormant] = home.custom(mine, true)
  expect(dormant!.tiles.map((t) => (t.kind === 'dormant' ? `${t.key} ${t.place?.col} ${t.label}: ${t.why}` : t.key))).toEqual([
    'device:plug/switch/l2 0 Kitchen plug · switch/l2: Its device has no function switch/l2 with a tile now. It shows again if the function comes back.',
    'device:bare 1 Bare: Its device has no function with a tile now. It shows again if one comes back.',
    'automation:night 2 Night: Its automation has no manual trigger now. It shows again if one comes back.',
    'device:gone 3 Unavailable: What it showed is gone, or hidden from you.',
  ])
})

test("the Tiles to pick for an own Section: the home's, grouped by Area, searched by name, those it holds greyed out", () => {
  const nothing: Fn = { key: 'config', kind: 'config', capabilities: [{ ...on, category: 'config' }] }
  const devices = [
    device('lamp', 'living', fn('light')),
    device('fan', 'kitchen', fn('switch')),
    device('config', 'living', nothing),
    device('radio', undefined, fn('switch')),
    device('plug', 'kitchen', fn('l1'), fn('l2', 'living'), nothing),
  ]
  const aggregates: Aggregate[] = [
    { ...derived('living', 'light', 'device:lamp/light'), name: 'Living room lights' },
    { id: 'down', name: 'Downstairs', members: [], binary: 'any', numeric: 'mean' },
  ]
  const flags = [{ id: 'guest', name: 'Guest', kind: 'flag', area: 'kitchen', capabilities: [{ ...on, key: 'on' }] }]
  const bedtime: AutomationStatus = { id: 'bedtime', name: 'Bedtime', status: 'enabled', manualTriggers: [{ step: 'go', name: 'Go' }] }
  const night: AutomationStatus = { id: 'night', name: 'Night', status: 'enabled' } // no Manual trigger: no Tile
  const areas = [area('living', { name: 'Living room' }), area('office', { name: 'Office' }), area('kitchen', { name: 'Kitchen' })]
  const home = dashboard(devices, aggregates, flags, areas, [bedtime, night])
  const short = (groups: ReturnType<typeof home.choices>) =>
    groups.map(
      (g) =>
        `${g.area?.name ?? '-'}: ${g.tiles
          .map(
            (t) =>
              `${t.name}${t.taken ? ' (in it)' : ''}${t.functions ? ` [${t.functions.map((f) => `${f.name}${f.taken ? ' (in it)' : ''}`).join(', ')}]` : ''}`,
          )
          .join(', ')}`,
    )
  // in the Areas' order, those without one last; a Device without a Tile is not there; a Device
  // with several Functions that have a Tile offers each under it, in its Device's Area
  expect(short(home.choices('', ['device:lamp', 'automation:bedtime', 'device:plug/l2']))).toEqual([
    'Living room: lamp (in it), Living room lights',
    'Kitchen: fan, Guest, plug [plug · l1, plug · l2 (in it)]',
    '-: Bedtime (in it), Downstairs, radio',
  ])
  expect(short(home.choices(' LI ', []))).toEqual(['Living room: Living room lights'])
  // a Function is searched by its name, its Device's included
  expect(short(home.choices('l2', []))).toEqual(['Kitchen: plug [plug · l2]'])
  expect(short(home.choices('plug', []))).toEqual(['Kitchen: plug [plug · l1, plug · l2]'])
  expect(home.choices('l1', [])[0]!.tiles[0]!.functions![0]).toMatchObject({ key: 'device:plug/l1', tile: { target: 'device:plug/l1' }, kind: 'Function' })
  expect(home.choices('bed', [])[0]!.tiles[0]).toMatchObject({ key: 'automation:bedtime', tile: { automation: 'bedtime' }, kind: 'Automation' })
  expect(home.choices('guest', [])[0]!.tiles[0]).toMatchObject({ key: 'flag:guest', tile: { target: 'flag:guest' }, kind: 'Flag' })
})

test('#dashboard/<id> opens a Dashboard the Person sees, hidden or not; any other address their first one shown', () => {
  const mine = [
    { id: 'evening', owner: 'alice', name: 'Evening', columns: 2, sections: [] },
    { id: 'night', shared: true as const, name: 'Night', columns: 2, sections: [] },
  ]
  const list = [{ id: 'evening', hidden: true as const }, { id: 'night' }, { id: 'builtin' }]
  expect(shown('#dashboard/evening', mine, list)?.name).toBe('Evening')
  expect(shown('#dashboard/builtin', mine, list)).toBeUndefined()
  for (const hash of ['', '#', '#dashboard/someone-elses', '#dashboard/']) expect(shown(hash, mine, list)?.name, hash).toBe('Night')
  // the built-in Dashboard first, or no list at all, opens it
  expect(shown('#', mine, [{ id: 'builtin' }, { id: 'night' }])).toBeUndefined()
  expect(shown('#', mine, [])).toBeUndefined()
})

test('a Kiosk shows the Dashboard it is assigned, whatever the address', () => {
  const night = { id: 'night', shared: true as const, name: 'Night', columns: 2, sections: [] }
  for (const hash of ['#', '#dashboard/builtin', '#dashboard/evening']) {
    expect(shown(hash, [night], [], true), hash).toBe(night)
    expect(shown(hash, [], [], true), hash).toBeUndefined() // the built-in one
  }
})

test('deleting a Dashboard names the Kiosks showing it', () => {
  expect(deleting('Evening', [])).toBe('Delete "Evening"?')
  expect(deleting('Evening', ['Hall tablet'])).toBe('Delete "Evening"? Hall tablet shows it: it will show Home, the built-in Dashboard, instead.')
  expect(deleting('Evening', ['Hall tablet', 'Kitchen tablet'])).toBe(
    'Delete "Evening"? Hall tablet and Kitchen tablet show it: they will show Home, the built-in Dashboard, instead.',
  )
})

test('a copy of the built-in Dashboard holds its Areas two per row, in order, without Others; of a custom one, what its viewer sees', () => {
  expect(copied(undefined, [area('living'), area('kitchen'), area('office')])).toEqual({
    columns: 2,
    sections: [
      { area: 'living', col: 0, row: 0, width: 1 },
      { area: 'kitchen', col: 1, row: 0, width: 1 },
      { area: 'office', col: 0, row: 1, width: 1 },
    ],
  })
  const evening: CustomDashboard = {
    id: 'evening',
    shared: true,
    name: 'Evening',
    columns: 3,
    sections: [
      { area: 'kitchen', col: 2, row: 0, width: 1, height: 2 },
      {
        id: 'own',
        name: 'Lights',
        icon: 'sofa',
        columns: 2,
        col: 0,
        row: 0,
        width: 2,
        tiles: [{ target: 'device:lamp', col: 1, row: 0, width: 1, height: 2 }],
      },
    ],
  }
  expect(copied(evening, [])).toEqual({ columns: 3, sections: evening.sections })
})
