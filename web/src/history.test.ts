import { afterEach, expect, test, vi } from 'vitest'
import {
  DAY,
  HistoryView,
  append,
  blanks,
  cluster,
  curve,
  held,
  key,
  lane,
  laneGroup,
  measures,
  timelineSpan,
  shift,
  periodsSpan,
  refreshed,
  sheetSeries,
  compareCandidates,
  joined,
  shifted,
  targetBlanks,
  type Marker,
  type Period,
  type Point,
} from './history'
import { catalogue } from './targets'
import type { Capability, CommandStatus, Device } from './types'

// What the store holds: the lamp's brightness, which the writer may not have committed yet.
vi.mock('./store', () => ({
  follow: () => {},
  current: {
    value: (r: { capability: string }) => (r.capability === 'brightness' ? { data: 25, at: new Date(150).toISOString() } : undefined),
    event: () => undefined,
    availability: () => 'online',
  },
}))

test('a live point is appended as the writer records it', () => {
  const raw: Point[] = [{ t: 10, v: 20 }]
  expect(append(raw, 20, 20, false, DAY)).toBe(raw) // a refresh, or an equal Value
  expect(append(raw, 20, 21, false, DAY)).toEqual([...raw, { t: 20, v: 21 }])
  expect(append(raw, 5, 21, false, DAY)).toEqual([...raw, { t: 10, v: 21 }]) // never back in time
  expect(append([{ t: 10, v: 'single' }], 20, 'single', true, DAY)).toHaveLength(2) // every Event
  const bucket: Point[] = [{ t: 10, mean: 20, min: 19, max: 21 }]
  expect(append(bucket, 10, 20, false, DAY)).toBe(bucket) // summed up in the bucket
  expect(append(bucket, 11, 20, false, DAY)).toEqual([...bucket, { t: 11, v: 20 }]) // raw after it
  const day: Point[] = [
    { t: 0, v: 1 },
    { t: 5, v: 2 },
    { t: 7, v: 3 },
  ]
  expect(append(day, DAY + 6, 4, false, DAY)).toEqual([
    { t: 5, v: 2 }, // still holds a day back
    { t: 7, v: 3 },
    { t: DAY + 6, v: 4 },
  ])
  expect(append(day, DAY + 6, 4, false, Infinity)).toHaveLength(4) // All keeps everything
})

test('nothing is drawn across an offline span or a Gap', () => {
  const availability = [
    { t: 0, v: 'online' },
    { t: 30, v: 'offline' },
    { t: 40, v: 'online' },
  ]
  const b = blanks(
    availability,
    [
      { start: 35, end: 50, lost: 0 },
      { start: 90, end: 200, lost: 0 },
    ],
    10,
    100,
  )
  expect(b.offline).toEqual([[30, 40]])
  expect(b.gaps).toEqual([
    [35, 50],
    [90, 100],
  ])
  expect(b.all).toEqual([
    [30, 50],
    [90, 100],
  ])
  const ps = [
    { t: 0, v: 1 },
    { t: 45, v: 2 }, // replayed within the Gap
    { t: 60, v: 3 },
  ]
  expect(held(ps, 10, 100, b.all).map(({ from, to, p }) => [from, to, p.v])).toEqual([
    [10, 30, 1],
    [50, 60, 2],
    [60, 90, 3],
  ])
  // A curve steps from each piece, breaks before a blank, and ends with the last.
  expect(curve(ps, 10, 100, b.all)).toEqual([
    [10, 30, 50, 60, 90],
    [1, null, 2, 3, 3],
    [1, null, 2, 3, 3],
    [1, null, 2, 3, 3],
  ])
  expect(curve([{ t: 0, mean: 2, min: 1, max: 4 }], 0, 10, [])).toEqual([
    [0, 10],
    [1, 1],
    [4, 4],
    [2, 2],
  ])
})

test('a panel shared by Functions hatches where any of them was offline', () => {
  const gaps = [{ start: 70, end: 80, lost: 0 }]
  const a = blanks(
    [
      { t: 0, v: 'online' },
      { t: 20, v: 'offline' },
      { t: 30, v: 'online' },
    ],
    gaps,
    0,
    100,
  )
  const b = blanks(
    [
      { t: 0, v: 'online' },
      { t: 25, v: 'unknown' },
      { t: 40, v: 'online' },
    ],
    gaps,
    0,
    100,
  )
  expect(joined(a, b)).toEqual({
    offline: [[20, 40]],
    gaps: [[70, 80]],
    all: [
      [20, 40],
      [70, 80],
    ],
  })
})

test('the previous period is drawn as its points and blanks moved onto this one', () => {
  const ps: Point[] = [
    { t: 5, v: 1 },
    { t: 15, mean: 2, min: 1, max: 3 },
  ]
  expect(shifted(ps, 100)).toEqual([
    { t: 105, v: 1 },
    { t: 115, mean: 2, min: 1, max: 3 },
  ])
  const view = new HistoryView()
  view.state = {
    ...view.state,
    series: new Map([
      [
        key({ target: 'device:d', capability: '' }),
        [
          { t: 0, v: 'online' },
          { t: 10, v: 'offline' },
        ],
      ],
    ]),
    gaps: [{ start: 15, end: 20, lost: 0 }],
  }
  // The period [0, 50) moved onto [100, 150).
  expect(targetBlanks(view.state, 'device:d/f', 100, 150, 100)).toEqual({ offline: [[110, 150]], gaps: [[115, 120]], all: [[110, 150]] })
})

afterEach(() => vi.unstubAllGlobals())

function answer(body: unknown) {
  const fetch = vi.fn(async () => new Response(JSON.stringify(body)))
  vi.stubGlobal('fetch', fetch)
  return fetch
}

const lamp = { target: 'device:lamp/light', capability: 'brightness' }

test('a view follows the stream, from the seam on, only while its range ends now', async () => {
  const body = { from: 0, bucket: '', series: [[{ t: 100, v: 20 }]], gaps: [], availability: { 'device:lamp': [{ t: 0, v: 'online' }] } }
  const update = { seq: 1, kind: 'value', ref: lamp, value: { data: 30, at: new Date(200).toISOString() } } as const

  const past = new HistoryView()
  answer(body)
  await past.load([lamp], 0, 1000, 10, false)
  past.receive(update)
  expect(past.state.series.get('device:lamp/light|brightness')).toEqual([{ t: 100, v: 20 }])
  past.close()

  const live = new HistoryView()
  const fetch = answer(body)
  await live.load([lamp], 0, 1000, 10, true)
  const [, init] = fetch.mock.calls[0] as unknown as [string, RequestInit]
  expect(JSON.parse(String(init.body))).toEqual({ from: 0, to: 1000, points: 10, refs: [lamp] })
  live.receive(update)
  live.receive({ seq: 2, kind: 'availability', target: 'device:lamp', availability: 'offline' })
  expect(live.state.series.get('device:lamp/light|brightness')).toEqual([
    { t: 100, v: 20 },
    { t: 150, v: 25 }, // the seam
    { t: 200, v: 30 },
  ])
  expect(live.state.series.get('device:lamp|')?.at(-1)).toMatchObject({ v: 'offline' })
  live.close()
})

test('only the latest load is kept', async () => {
  const v = new HistoryView()
  let first!: (r: Response) => void
  vi.stubGlobal(
    'fetch',
    vi.fn(() => new Promise<Response>((r) => (first = r))),
  )
  const slow = v.load([lamp], 0, 1000, 10, false)
  answer({ from: 500, bucket: '1h', series: [[]], gaps: [], availability: {} })
  await v.load([lamp], 500, 1000, 10, false)
  first(new Response(JSON.stringify({ from: 0, bucket: '', series: [[]], gaps: [], availability: {} })))
  await slow
  expect(v.state).toMatchObject({ from: 500, bucket: '1h' })
  v.close()
})

test('markers follow the Commands and the Runs the Targets triggered', async () => {
  const v = new HistoryView()
  const fetch = answer({ from: 0, bucket: '', series: [[]], gaps: [], availability: {}, commands: [], runs: [] })
  await v.load([lamp], 0, 1000, 10, true, true)
  const [, init] = fetch.mock.calls[0] as unknown as [string, RequestInit]
  expect(JSON.parse(String(init.body))).toMatchObject({ markers: true })
  const command = (id: string, target: string, status: CommandStatus) => v.receive({ seq: 1, kind: 'command', command: { id, target, status, origin: 'api' } })
  command('c1', lamp.target, 'pending')
  command('c1', lamp.target, 'timed_out')
  command('c2', 'device:other/light', 'pending')
  command('c3', lamp.target, 'superseded') // issued before the answer, which holds it
  expect(v.state.commands.map((c) => [c.id, c.status])).toEqual([['c1', 'timed_out']])
  const run = (target: string, capability: string) =>
    v.receive({
      seq: 2,
      kind: 'run',
      run: {
        automation: 'a',
        run: `${target}|${capability}`,
        time: '',
        outcome: 'acted',
        commands: 1,
        trigger: { step: 's', kind: 'k', target, capability, value: 1, time: '' },
      },
    })
  run(lamp.target, 'state')
  run(lamp.target, '') // its Availability
  run('device:other/light', 'state')
  expect(v.state.runs.map((r) => r.run)).toEqual([`${lamp.target}|state`])
  v.close()
})

test('markers closer than a few pixels cluster', () => {
  const m = (t: number): Marker => ({ t, n: 1, kind: 'event', text: '' })
  expect(cluster([m(50), m(0), m(5), m(12), m(30), m(2000)], 0, 1000, 1000).map((ms) => ms.map((x) => x.t))).toEqual([[0, 5], [12], [30], [50]])
})

const cap = (key: string, type: Capability['type'], more: Partial<Capability> = {}): Capability => ({
  key,
  label: key,
  type,
  access: { observable: true, settable: false, queryable: false },
  category: 'primary',
  ...more,
})
const settable = { access: { observable: true, settable: true, queryable: true } }

test('a sheet charts the everyday series, the rest behind a toggle', () => {
  const plug = [
    cap('state', 'binary', settable),
    cap('power', 'numeric'),
    cap('energy', 'numeric'),
    cap('current', 'numeric'),
    cap('voltage', 'numeric'),
    cap('child_lock', 'binary', settable), // primary, yet not the on/off
  ]
  const device = [cap('linkquality', 'numeric', { category: 'diagnostic' })]
  const keys = (all: boolean) => sheetSeries('device:p/plug', 'switch', plug, device, all).map((s) => `${s.ref.target}|${s.ref.capability}`)
  expect(keys(false)).toEqual(['device:p/plug|state', 'device:p/plug|power', 'device:p/plug|energy'])
  expect(keys(true)).toEqual([
    'device:p/plug|state',
    'device:p/plug|power',
    'device:p/plug|energy',
    'device:p/plug|current',
    'device:p/plug|voltage',
    'device:p/plug|child_lock',
    'device:p|linkquality',
  ])
  const lightCaps = [
    cap('state', 'binary', settable),
    cap('brightness', 'numeric', settable),
    cap('color_temp', 'numeric', settable),
    cap('color_hs', 'composite', settable),
    cap('effect', 'enum', settable),
    cap('action', 'enum', { stateless: true }),
  ]
  const light = (all: boolean) => sheetSeries('flag:x', 'light', lightCaps, [], all).map((s) => s.cap.key)
  expect(light(false)).toEqual(['state', 'brightness', 'color_temp', 'action']) // its Events, as ticks
  expect(light(true)).toEqual(['state', 'brightness', 'color_temp', 'action', 'effect'])
})

test('a sheet offers to compare with other Functions that chart something, the same kind first', () => {
  const d = (id: string, name: string, key: string, kind: string, caps = [cap('temperature', 'numeric')]): Device => ({
    id,
    name,
    nativeAddress: id,
    functions: [{ key, kind, capabilities: caps }],
    capabilities: [cap('temperature', 'numeric')], // a Device itself is never offered
  })
  const c = catalogue(
    [
      d('a', 'Living room', 't', 'temperature'),
      d('b', 'Office', 'plug', 'switch', [cap('state', 'binary', settable)]),
      d('c', 'Kitchen', 't', 'temperature'),
      d('d', 'Bedroom', 't', 'temperature'),
      d('e', 'Alpha', 'x', 'other', [cap('timeout', 'numeric', settable)]), // a setting: nothing to chart by default
      d('f', 'Workshop', 'l', 'light', [cap('state', 'binary', settable)]),
      d('g', 'Zinc', 'c', 'contact', [cap('contact', 'binary')]),
    ],
    [],
    [],
  )
  // Then by kind, to be listed under each, and by name.
  expect(compareCandidates(c, 'device:a/t', 'temperature', ['device:d/t']).map((l) => l.name)).toEqual(['Kitchen', 'Zinc', 'Workshop', 'Office'])
})

test('a Function offers time on and counts of its on/off and states, presses, and min/mean/max of its readings', () => {
  const of = (caps: Capability[]) =>
    measures(caps.map((c) => ({ ref: { target: 't', capability: c.key }, cap: c }))).map((m) => [m.ref.capability, m.measure, m.label])
  expect(
    of([
      cap('state', 'binary', settable),
      cap('power', 'numeric', { unit: 'W' }),
      cap('energy', 'numeric', { unit: 'kWh', counter: true }),
      cap('brightness', 'numeric', settable),
    ]),
  ).toEqual([
    ['state', 'time_on', 'Time on'],
    ['state', 'count', 'Times on'],
    ['power', 'stats', 'power min/mean/max'],
    ['energy', 'energy', 'energy'], // a counter's rise by its label, not its min/mean/max
  ])
  expect(
    of([
      cap('contact', 'binary'),
      cap('occupancy', 'binary'),
      cap('action', 'enum', { stateless: true }),
      cap('battery', 'numeric', { category: 'diagnostic' }),
    ]),
  ).toEqual([
    ['contact', 'time_on', 'Time open'],
    ['contact', 'count', 'Openings'],
    ['occupancy', 'time_on', 'Time occupied'],
    ['occupancy', 'count', 'Detections'],
    ['action', 'count', 'Presses'],
  ])
})

test('a Per period view starts as many local periods back', () => {
  const now = new Date(2026, 2, 31, 14, 30).getTime() // a Tuesday
  expect(new Date(shift('hour', now, -23))).toEqual(new Date(2026, 2, 30, 15, 30))
  expect(new Date(shift('day', now, -13))).toEqual(new Date(2026, 2, 18, 14, 30))
  expect(new Date(shift('week', now, -11))).toEqual(new Date(2026, 0, 13, 14, 30))
  expect(new Date(shift('month', now, -11))).toEqual(new Date(2025, 3, 1, 14, 30)) // from the 1st: no 31 February
  expect(new Date(shift('month', now, 1))).toEqual(new Date(2026, 3, 1, 14, 30))
  expect(periodsSpan('day', null, now)).toEqual({ from: shift('day', now, -13), to: now })
  const march = new Date(2026, 2, 1).getTime()
  expect(periodsSpan('day', march - 1, now)).toEqual({ from: shift('day', march - 1, -13), to: march }) // the 14 days before March
})

test('a refreshed current period replaces its own, and a new one slides the window', () => {
  const p = (start: number, value: number): Period => ({ start, value, incomplete: false })
  const old = [p(0, 1), p(10, 2), p(20, 3)]
  expect(refreshed(old, [p(20, 4)])).toEqual([p(0, 1), p(10, 2), p(20, 4)])
  expect(refreshed(old, [p(20, 5), p(30, 0)])).toEqual([p(10, 2), p(20, 5), p(30, 0)])
})

test('a Timeline lane draws an on/off or a binary reading, a reading, Events, and sums each up', () => {
  const of = (caps: Capability[], kind = '') => {
    const l = lane('t', kind, caps)
    return l && { band: l.band?.key, curve: l.curve?.key, events: l.events?.key, summary: l.summary.map((m) => `${m.cap.key} ${m.measure}`) }
  }
  const plug = [
    cap('state', 'binary', settable),
    cap('power', 'numeric'),
    cap('energy', 'numeric', { counter: true }),
    cap('current', 'numeric'),
    cap('voltage', 'numeric'),
    cap('child_lock', 'binary', settable),
  ]
  expect(of(plug)).toEqual({ band: 'state', curve: 'power', events: undefined, summary: ['state time_on', 'energy energy', 'power stats'] })
  expect(of([cap('state', 'binary', settable), cap('brightness', 'numeric', settable)], 'light')).toMatchObject({
    band: 'state',
    curve: undefined,
    summary: ['state time_on'],
  })
  expect(of([cap('on', 'binary', settable)])).toMatchObject({ band: 'on', summary: ['on time_on'] }) // a Flag
  expect(of([cap('alarm', 'binary', settable)])).toMatchObject({ band: 'alarm', summary: ['alarm time_on'] }) // a siren
  expect(of([cap('contact', 'binary')])).toMatchObject({ band: 'contact', summary: ['contact count'] }) // openings
  expect(of([cap('temperature', 'numeric')])).toMatchObject({ curve: 'temperature', summary: ['temperature stats'] })
  expect(of([cap('energy', 'numeric', { counter: true })])).toMatchObject({ curve: 'energy', summary: ['energy energy'] })
  expect(of([cap('action', 'enum', { stateless: true })])).toMatchObject({ events: 'action', summary: ['action count'] })
  // Settings-only Functions draw nothing.
  expect(of([cap('occupancy_timeout', 'numeric', settable)])).toBeUndefined()
  expect(of([cap('led_indication', 'binary', settable)])).toBeUndefined()
  expect(of([cap('melody', 'enum', settable)])).toBeUndefined()
  expect(of([cap('state', 'binary', { ...settable, category: 'config' })])).toBeUndefined()
})

test('a Timeline groups lanes by kind', () => {
  expect(['light', 'switch', 'contact', 'occupancy', 'temperature', 'button', 'flag'].map((k) => laneGroup(k).name)).toEqual([
    'Lights',
    'Plugs',
    'Doors',
    'Motion',
    'Climate',
    'Buttons',
    'Other',
  ])
})

test('a Timeline span is a local day, or a week from Monday', () => {
  const tuesday = new Date(2026, 2, 31, 14, 30).getTime()
  const at = (y: number, m: number, d: number) => new Date(y, m, d).getTime()
  expect(timelineSpan('day', tuesday)).toEqual([at(2026, 2, 31), at(2026, 3, 1)])
  expect(timelineSpan('week', tuesday)).toEqual([at(2026, 2, 30), at(2026, 3, 6)])
  expect(timelineSpan('week', new Date(2026, 3, 5, 23).getTime())).toEqual([at(2026, 2, 30), at(2026, 3, 6)]) // a Sunday
  expect(timelineSpan('day', at(2026, 2, 29))).toEqual([at(2026, 2, 29), at(2026, 2, 30)]) // across DST, where there is one
})

test('a view over a range reaching past now follows the stream, and holds still', async () => {
  const v = new HistoryView()
  const now = Date.now()
  const body = { from: now - DAY, bucket: '', series: [[{ t: now - DAY, v: 20 }]], gaps: [], availability: {} }
  const fetch = answer(body)
  await v.load([lamp], now - DAY, now + DAY, 10, false)
  v.receive({ seq: 1, kind: 'value', ref: lamp, value: { data: 30, at: new Date(now + 1).toISOString() } })
  expect(v.state.series.get(key(lamp))?.map((p) => (p as { v: unknown }).v)).toEqual([20, 25, 30]) // the seam, then the stream
  await v.reload()
  const [, init] = fetch.mock.calls[1] as unknown as [string, RequestInit]
  expect(JSON.parse(String(init.body))).toMatchObject({ from: now - DAY, to: now + DAY })
  v.close()
})
