import { expect, test } from 'vitest'
import { aggregateTarget, catalogue, deviceTarget, flagTarget, owner, parseTarget, targetKind } from './targets'
import type { Capability } from './types'

// The same keys as internal/home's TestTargetKeyRoundTrips.
test('a Target key round-trips', () => {
  expect(parseTarget('device:d1')).toEqual({ kind: 'device', id: 'd1', function: '' })
  expect(parseTarget('device:d1/light')).toEqual({ kind: 'device', id: 'd1', function: 'light' })
  expect(parseTarget('device:d1/switch/l2')).toEqual({ kind: 'device', id: 'd1', function: 'switch/l2' })
  expect(parseTarget('aggregate:a1')).toEqual({ kind: 'aggregate', id: 'a1', function: '' })
  expect(parseTarget('flag:f1')).toEqual({ kind: 'flag', id: 'f1', function: '' })
  expect(deviceTarget('d1', 'switch/l2')).toBe('device:d1/switch/l2')
  expect(deviceTarget('d1')).toBe('device:d1')
  expect(aggregateTarget('a1')).toBe('aggregate:a1')
  expect(flagTarget('f1')).toBe('flag:f1')
  for (const bad of ['', 'd1', 'device:', 'device:d1/', 'room:r1', 'flag:f1/on', 'aggregate:']) expect(parseTarget(bad), bad).toBeUndefined()
})

test("a Function's Availability is its Device's", () => {
  expect(owner('device:d1/switch/l2')).toBe('device:d1')
  expect(owner('flag:f1')).toBe('flag:f1')
})

test('a Target is a Function, a Device itself, an Aggregate or a Flag', () => {
  expect(['device:d1/light', 'device:d1', 'aggregate:a1', 'flag:f1', 'room:r1'].map(targetKind)).toEqual(['function', 'device', 'aggregate', 'flag', undefined])
})

const cap = (key: string): Capability => ({
  key,
  label: key,
  type: 'binary',
  access: { observable: true, settable: false, queryable: false },
  category: 'primary',
})

test('the catalogue names every Target: a Function after its Device, with its key when it has several', () => {
  const battery = [cap('battery')]
  const c = catalogue(
    [
      {
        id: 'relay',
        name: 'Relay',
        nativeAddress: '',
        functions: [
          { key: 'switch/l1', kind: 'switch', capabilities: [cap('state')] },
          { key: 'switch/l2', kind: 'switch', capabilities: [] },
        ],
        capabilities: battery,
      },
      { id: 'bulb', name: 'Bulb', nativeAddress: '', functions: [{ key: 'light', kind: 'light', capabilities: [] }], capabilities: null },
      { id: 'base', name: 'Arlo base', nativeAddress: '', functions: null, capabilities: null },
    ],
    [{ id: 'empty', name: 'Empty', members: [], binary: 'any', numeric: 'mean' }],
    [{ id: 'away', name: 'Away', kind: 'flag', capabilities: [cap('on')] }],
  )
  // Functions, Aggregates, Flags, then Devices, by Name within each
  expect(c.list.map((e) => [e.target, e.name, e.kind])).toEqual([
    ['device:bulb/light', 'Bulb', 'light'],
    ['device:relay/switch/l1', 'Relay · switch/l1', 'switch'],
    ['device:relay/switch/l2', 'Relay · switch/l2', 'switch'],
    ['aggregate:empty', 'Empty', ''], // no members yet: no kind, no Capabilities
    ['flag:away', 'Away', 'flag'],
    ['device:base', 'Arlo base', ''],
    ['device:bulb', 'Bulb', ''],
    ['device:relay', 'Relay', ''],
  ])
  expect(c.get('device:relay/switch/l1')).toMatchObject({ capabilities: [cap('state')], ownerCapabilities: battery })
  expect(c.get('device:relay')).toMatchObject({ capabilities: battery, ownerCapabilities: [] })
  expect(c.get('aggregate:empty')?.capabilities).toEqual([])
  expect(c.get('flag:gone')).toBeUndefined()
  expect(c.name('flag:away')).toBe('Away')
  expect(c.name('flag:gone')).toBe('deleted target')
})
