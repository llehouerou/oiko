import { expect, test } from 'vitest'
import { memberLeaves, tileCaps, tileShape, unwell, type Part } from './tiles'
import type { Aggregate, Capability, Fn, Target } from './types'

const agg = (id: string, ...members: Target[]): Aggregate => ({ id, name: id, members, binary: 'any', numeric: 'mean' })

test("an Aggregate's lights are its members, nested Aggregates' included, each once", () => {
  const living = agg('living', 'device:a/light', 'device:b/light')
  const all = agg('all', 'aggregate:living', 'device:b/light', 'device:c/light')
  expect(memberLeaves(all, [living, all])).toEqual(['device:a/light', 'device:b/light', 'device:c/light'])
  expect(memberLeaves(agg('gone', 'aggregate:missing'), [])).toEqual([])
})

const cap = (key: string, settable: boolean, category: Capability['category'] = 'primary'): Capability => ({
  key,
  label: key,
  type: 'binary',
  access: { observable: true, settable, queryable: false },
  category,
})
const fn = (kind: string, ...caps: Capability[]): Fn => ({ key: kind, kind, capabilities: caps })

test('a Tile shows a sensor reading, not its timeout, and a plug its state, not its child lock or health', () => {
  const keys = (f: Fn) => tileCaps(f).map((c) => c.key)
  expect(keys(fn('occupancy', cap('occupancy', false), cap('occupancy_timeout', true)))).toEqual(['occupancy'])
  expect(keys(fn('switch', cap('state', true), cap('child_lock', true), cap('tamper', false)))).toEqual(['state'])
})

const num = (key: string): Capability => ({ ...cap(key, false), type: 'numeric' })
// A Tile's shape, in short: its kind and the keys it draws.
const shape = (...fns: Fn[]) => {
  const s = tileShape(fns.map((f) => ({ target: `device:x/${f.key}`, fn: f })))
  const keys = (ps: Part[]) => ps.map((p) => p.cap.key).join(',')
  if (s.kind === 'bar') return `bar ${s.toggle}`
  if (s.kind === 'event') return `event ${s.event.cap.key}`
  if (s.kind === 'sensor') return `sensor ${s.state?.cap.key ?? '-'} ${keys(s.readings)}`
  if (s.kind === 'readings') return `readings ${keys(s.readings)}`
  if (s.kind === 'camera') return `camera ${s.target} ${s.state?.cap.key ?? '-'}`
  return s.kind
}

test('a lone Function whose main control is an on/off is a bar', () => {
  expect(shape(fn('switch', cap('state', true), num('power'), cap('countdown', true)))).toBe('bar state') // the countdown is a setting
  expect(shape(fn('alarm', cap('alarm', true)))).toBe('bar alarm')
  expect(shape(fn('light', cap('state', true), { ...num('brightness'), access: { observable: true, settable: true, queryable: false } }))).toBe('bar state')
  expect(shape(fn('switch', cap('state', true), cap('on', true)))).toBe('bar state') // one main control, the other a setting
  expect(shape(fn('arming', { ...cap('mode', true), type: 'enum' }))).toBe('controls') // a main control, but no on/off
  expect(shape({ ...fn('switch', cap('state', true)), key: 'l1' }, { ...fn('switch', cap('state', true)), key: 'l2' })).toBe('controls') // two gangs
})

test('a button is its last Event, its battery behind ⋯', () => {
  expect(shape(fn('button', { ...cap('action', false), type: 'enum', stateless: true }, num('battery')))).toBe('event action')
})

test('nothing to command: a bar for a state or a single reading, a cell per reading otherwise', () => {
  expect(shape(fn('contact', cap('contact', false), cap('tamper', false)))).toBe('sensor contact ')
  expect(shape(fn('occupancy', num('illuminance'), cap('occupancy', false), cap('occupancy_timeout', true)))).toBe('sensor occupancy illuminance')
  expect(shape(fn('temperature', num('temperature')))).toBe('sensor - temperature')
  expect(shape(fn('air', num('co2'), num('humidity'), num('temperature')))).toBe('readings temperature,humidity,co2')
  // across a Device's Functions, in reading order
  expect(shape({ ...fn('h', num('humidity')) }, { ...fn('t', num('temperature')) })).toBe('readings temperature,humidity')
})

test('readings beside Events, or nothing on the Tile at all, take controls', () => {
  expect(shape(fn('remote', num('temperature'), { ...cap('action', false), stateless: true }))).toBe('controls')
  expect(shape(fn('settings', cap('led', true, 'config')))).toBe('controls')
})

test("a camera's Tile is its Picture, with its Device's motion beside", () => {
  expect(shape(fn('camera'))).toBe('camera device:x/camera -')
  expect(shape(fn('occupancy', cap('occupancy', false)), fn('camera'))).toBe('camera device:x/camera occupancy')
})

test('health is wrong when low or on', () => {
  expect(unwell(20)).toBe(true)
  expect(unwell(21)).toBe(false)
  expect(unwell(true)).toBe(true)
  expect(unwell(false)).toBe(false)
  expect(unwell(undefined)).toBe(false)
})
