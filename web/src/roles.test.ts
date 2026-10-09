import { expect, test } from 'vitest'
import guide from '../../docs/write-a-bridge.md?raw'
import { byReading, co2Level, diagnosticKeys, healthKeys, lightAdjustments, mainControls, presses, readingOrder, roles, stateKeys, turns } from './roles'
import type { Capability } from './types'

const cap = (key: string, type: Capability['type'], more: Partial<Capability> = {}): Capability => ({
  key,
  label: key,
  type,
  access: { observable: true, settable: false, queryable: false },
  category: 'primary',
  ...more,
})
const settable = { access: { observable: true, settable: true, queryable: true } }

// Each Role by its keys, the charted one alone.
const of = (kind: string, ...caps: Capability[]) => {
  const r = roles(kind, caps)
  const keys = (cs: Capability[]) => cs.map((c) => c.key)
  return {
    control: r.control?.key,
    adjustments: keys(r.adjustments),
    state: r.state?.key,
    readings: keys(r.readings),
    events: keys(r.events),
    settings: keys(r.settings),
    health: keys(r.health),
    diagnostics: keys(r.diagnostics),
    charted: r.charted?.key,
  }
}
const none = { adjustments: [], readings: [], events: [], settings: [], health: [], diagnostics: [] }

test('a light: its state, its brightness and colour; extras are settings, color_xy nothing', () => {
  expect(
    of(
      'light',
      cap('state', 'binary', settable),
      cap('brightness', 'numeric', settable),
      cap('color_temp', 'numeric', settable),
      cap('color_hs', 'composite', settable),
      cap('color_xy', 'composite', settable),
      cap('effect', 'enum', settable),
      cap('color_mode', 'enum', { category: 'diagnostic' }),
    ),
  ).toEqual({
    ...none,
    control: 'state',
    adjustments: ['brightness', 'color_temp', 'color_hs'],
    settings: ['effect'],
    diagnostics: ['color_mode'],
    charted: 'state',
  })
})

test('a plug: its state, its power and energy; current and voltage are diagnostics, its child lock a setting', () => {
  expect(
    of(
      'switch',
      cap('state', 'binary', settable),
      cap('power', 'numeric'),
      cap('current', 'numeric'),
      cap('voltage', 'numeric'),
      cap('energy', 'numeric', { counter: true }),
      cap('child_lock', 'binary', settable),
      cap('power_on_behavior', 'enum', { ...settable, category: 'config' }),
    ),
  ).toEqual({
    ...none,
    control: 'state',
    readings: ['power', 'energy'],
    settings: ['child_lock', 'power_on_behavior'],
    diagnostics: ['current', 'voltage'],
    charted: 'power',
  })
})

test('a contact and a motion sensor: a state, charted before any reading', () => {
  expect(of('contact', cap('contact', 'binary'), cap('battery', 'numeric', { category: 'diagnostic' }), cap('tamper', 'binary'))).toEqual({
    ...none,
    state: 'contact',
    health: ['battery', 'tamper'],
    charted: 'contact',
  })
  expect(of('occupancy', cap('illuminance', 'numeric'), cap('occupancy', 'binary'), cap('occupancy_timeout', 'numeric', settable))).toEqual({
    ...none,
    state: 'occupancy',
    readings: ['illuminance'],
    settings: ['occupancy_timeout'],
    charted: 'occupancy',
  })
})

test('a weather station: readings in reading order, the first charted', () => {
  expect(of('rain', cap('rain', 'numeric'), cap('rain_1h', 'numeric'), cap('rain_24h', 'numeric'))).toMatchObject({
    readings: ['rain', 'rain_1h', 'rain_24h'],
    charted: 'rain',
  })
  expect(of('', cap('co2', 'numeric'), cap('humidity', 'numeric'), cap('temperature', 'numeric'))).toMatchObject({
    readings: ['temperature', 'humidity', 'co2'],
    charted: 'temperature',
  })
})

test('a button: its Events, as presses', () => {
  const action = cap('action', 'enum', { stateless: true })
  expect(of('button', action, cap('battery', 'numeric', { category: 'diagnostic' }))).toEqual({
    ...none,
    events: ['action'],
    health: ['battery'],
    charted: 'action',
  })
  expect(presses(action)).toBe(true)
})

test("a Flag's on and a siren's alarm are on/offs; Arlo's arming mode a main control, but no on/off", () => {
  expect(of('flag', cap('on', 'binary', settable))).toEqual({ ...none, control: 'on', charted: 'on' })
  expect(of('alarm', cap('alarm', 'binary', settable), cap('melody', 'enum', settable))).toEqual({
    ...none,
    control: 'alarm',
    settings: ['melody'],
    charted: 'alarm',
  })
  expect(of('arming', cap('mode', 'enum', settable))).toEqual({ ...none, control: 'mode', charted: undefined })
})

test("one main control at most, by precedence; one in the Bridge's config is a setting", () => {
  expect(of('switch', cap('on', 'binary', settable), cap('state', 'binary', settable))).toMatchObject({ control: 'state', settings: ['on'] })
  expect(of('switch', cap('state', 'binary', { ...settable, category: 'config' }))).toMatchObject({ control: undefined, settings: ['state'] })
})

test("a Device's own Capabilities: health, diagnostics and settings", () => {
  expect(
    of(
      '',
      cap('battery', 'numeric', { category: 'diagnostic' }),
      cap('linkquality', 'numeric', { category: 'diagnostic' }),
      cap('power_on_behavior', 'enum', { ...settable, category: 'config' }),
    ),
  ).toEqual({ ...none, health: ['battery'], diagnostics: ['linkquality'], settings: ['power_on_behavior'], charted: undefined })
})

test('a binary Value turns open, occupied or on', () => {
  expect(turns(cap('contact', 'binary'))).toEqual(['open', 'Openings'])
  expect(turns(cap('presence', 'binary'))).toEqual(['occupied', 'Detections'])
  expect(turns(cap('state', 'binary'))).toEqual(['on', 'Times on'])
})

test('readings come in a fixed order, unknown ones last', () => {
  expect(['rain', 'co2', 'humidity', 'temperature'].sort(byReading)).toEqual(['temperature', 'humidity', 'co2', 'rain'])
})

test('CO₂ asks for airing above 1000 ppm, at once above 1500', () => {
  expect(co2Level('co2', 900)).toBeUndefined()
  expect(co2Level('co2', 1200)).toBe('raised')
  expect(co2Level('co2', 1600)).toBe('high')
  expect(co2Level('temperature', 1600)).toBeUndefined()
})

// Authors of a type of Bridge read these keys in the guide's Roles table.
test("the guide's Roles table lists the keys of each Role, in order", () => {
  const row = (role: string) => {
    const line = guide.split('\n').find((l) => l.startsWith(`| ${role} |`)) ?? ''
    return [...(line.split('|')[2] ?? '').matchAll(/`([^`]+)`/g)].map((m) => m[1])
  }
  const table = ['Main control', 'Adjustment', 'State', 'Health', 'Diagnostic', 'Reading']
  expect(Object.fromEntries(table.map((role) => [role, row(role)]))).toEqual({
    'Main control': mainControls,
    Adjustment: lightAdjustments,
    State: stateKeys,
    Health: healthKeys,
    Diagnostic: diagnosticKeys,
    Reading: readingOrder,
  })
})
