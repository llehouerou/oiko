import { expect, test } from 'vitest'
import guide from '../../docs/write-a-bridge.md?raw'
import { byReading, co2Level, presses, readingOrder, rests, roles, turns } from './roles'
import type { Capability, Role } from './types'

// Oiko decides the Roles (internal/home/roles.go, ADR 0056): these only check how the
// web client sorts what it announced.

const cap = (key: string, type: Capability['type'], more: Partial<Capability> = {}): Capability => ({
  key,
  label: key,
  type,
  access: { observable: true, settable: false, queryable: false },
  category: 'primary',
  ...more,
})

// Each Role by its keys, the charted one alone, of caps given the Roles in rs.
const of = (rs: Record<string, Role>, ...caps: Capability[]) => {
  const r = roles({ capabilities: caps, roles: rs })
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

test('each Capability lands in the Role Oiko gave it; one given none in none', () => {
  expect(
    of(
      { state: 'control', brightness: 'adjustment', effect: 'setting', color_mode: 'diagnostic', battery: 'health', action: 'event', power: 'reading' },
      cap('state', 'binary'),
      cap('brightness', 'numeric'),
      cap('color_xy', 'composite'),
      cap('effect', 'enum'),
      cap('color_mode', 'enum'),
      cap('battery', 'numeric'),
      cap('action', 'enum', { stateless: true }),
      cap('power', 'numeric'),
    ),
  ).toEqual({
    ...none,
    control: 'state',
    adjustments: ['brightness'],
    readings: ['power'],
    events: ['action'],
    settings: ['effect'],
    health: ['battery'],
    diagnostics: ['color_mode'],
    charted: 'power',
  })
  expect(of({}, cap('state', 'binary'))).toEqual({ ...none, control: undefined, state: undefined, charted: undefined })
})

test('charted: the state, else a numeric reading, a binary reading, an on/off, an Event', () => {
  expect(of({ contact: 'state', temperature: 'reading' }, cap('temperature', 'numeric'), cap('contact', 'binary')).charted).toBe('contact')
  expect(of({ state: 'control', power: 'reading' }, cap('state', 'binary'), cap('power', 'numeric')).charted).toBe('power')
  expect(of({ on: 'control' }, cap('on', 'binary')).charted).toBe('on')
  expect(of({ mode: 'control' }, cap('mode', 'enum')).charted).toBeUndefined()
  expect(of({ action: 'event' }, cap('action', 'enum', { stateless: true })).charted).toBe('action')
})

test('readings come in reading order, unknown ones last, the first charted', () => {
  expect(
    of({ co2: 'reading', humidity: 'reading', temperature: 'reading' }, cap('co2', 'numeric'), cap('humidity', 'numeric'), cap('temperature', 'numeric')),
  ).toMatchObject({ readings: ['temperature', 'humidity', 'co2'], charted: 'temperature' })
  expect(['rain', 'co2', 'humidity', 'temperature'].sort(byReading)).toEqual(['temperature', 'humidity', 'co2', 'rain'])
})

test('a binary Value turns open, occupied, connected, home or on', () => {
  expect(turns(cap('contact', 'binary'))).toEqual(['open', 'Openings'])
  expect(turns(cap('presence', 'binary'))).toEqual(['occupied', 'Detections'])
  expect(turns(cap('connected', 'binary'))).toEqual(['connected', 'Connections'])
  expect(turns(cap('home', 'binary'))).toEqual(['home', 'Arrivals'])
  expect(turns(cap('state', 'binary'))).toEqual(['on', 'Times on'])
})

test("a binary Value at rest is 'closed', 'not connected', 'away' or 'clear'", () => {
  expect(rests(cap('contact', 'binary'))).toBe('closed')
  expect(rests(cap('connected', 'binary'))).toBe('not connected')
  expect(rests(cap('home', 'binary'))).toBe('away')
  expect(rests(cap('occupancy', 'binary'))).toBe('clear')
})

test("a button's Events are presses", () => {
  expect(presses(cap('action', 'enum', { stateless: true }))).toBe(true)
})

test('CO₂ asks for airing above 1000 ppm, at once above 1500', () => {
  expect(co2Level('co2', 900)).toBeUndefined()
  expect(co2Level('co2', 1200)).toBe('raised')
  expect(co2Level('co2', 1600)).toBe('high')
  expect(co2Level('temperature', 1600)).toBeUndefined()
})

// Authors of a type of Bridge read the reading order in the guide's Roles table; Oiko's
// roles_test.go checks the other rows.
test("the guide's Roles table lists the readings in reading order", () => {
  const line = guide.split('\n').find((l) => l.startsWith('| Reading |')) ?? ''
  expect([...(line.split('|')[2] ?? '').matchAll(/`([^`]+)`/g)].map((m) => m[1])).toEqual(readingOrder)
})
