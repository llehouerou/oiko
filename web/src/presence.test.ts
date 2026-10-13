import { describe, expect, it } from 'vitest'
import { boxUntil, chip, heldFor, homeChip, persons, readCapability, someoneElse, sourceState, sourceText, stretches } from './presence'
import type { Capability, Presence, Value } from './types'

const presence = (id: string, name: string): Presence => ({ id, name, kind: 'presence', capabilities: [], sources: [] })
const value = (data: boolean, since = '2026-01-01T10:00:00Z'): Value => ({ data, at: since, since })
const now = Date.parse('2026-01-01T12:30:00Z')

describe('the Presence row', () => {
  it('shows a chip per Person, by Name, after the Home presence', () => {
    const ps = [presence('home', 'Home'), presence('b', 'bob'), presence('c', 'Chloe'), presence('a', 'Alice')]
    expect(persons(ps).map((p) => p.name)).toEqual(['Alice', 'bob', 'Chloe'])
    expect(ps.map((p) => p.name)).toEqual(['Home', 'bob', 'Chloe', 'Alice']) // left as it was
  })

  it('tells each chip home, away or unknown, and set by hand', () => {
    expect(chip(presence('a', 'alice'), value(true), undefined, now)).toEqual({ initial: 'A', state: 'home', manual: false, held: '2 h' })
    expect(chip(presence('a', 'Alice'), value(false, '2026-01-01T12:25:00Z'), value(true), now)).toEqual({
      initial: 'A',
      state: 'away',
      manual: true,
      held: '5 min',
    })
    expect(chip(presence('a', 'Alice'), value(true), value(false), now).manual).toBe(false)
    expect(chip(presence('a', 'Alice'), undefined, undefined, now)).toEqual({ initial: 'A', state: 'unknown', manual: false, held: '' })
  })

  it("tells the Home presence's chip someone home, not one of you, or nobody", () => {
    expect(homeChip(value(true), 1, now)).toEqual({ state: 'home', text: 'Someone home', held: '2 h' })
    expect(homeChip(value(true), 0, now).text).toBe('Someone home · not one of you')
    expect(homeChip(value(false), 0, now).text).toBe('Nobody home')
    expect(homeChip(undefined, 0, now)).toEqual({ state: 'unknown', text: 'Home unknown', held: '' })
  })
})

describe("the Home presence's sheet", () => {
  it('tells someone else inside while it is home and no Person is', () => {
    expect(someoneElse(value(true), 0)).toBe(true)
    expect(someoneElse(value(true), 2)).toBe(false)
    expect(someoneElse(value(false), 0)).toBe(false)
    expect(someoneElse(undefined, 0)).toBe(false)
  })

  it('tells until when the box holds: the exit grace after the last door closed, unless a Sign of life is on', () => {
    const closed = (since: string) => value(false, since)
    const doors = [closed('2026-01-01T12:20:00Z'), closed('2026-01-01T12:28:00Z'), value(true, '2026-01-01T12:29:00Z'), undefined]
    expect(boxUntil(doors, [value(false), undefined], 300, now)).toBe(Date.parse('2026-01-01T12:33:00Z'))
    expect(boxUntil(doors, [value(true)], 300, now)).toBeUndefined() // still there
    expect(boxUntil([closed('2026-01-01T12:20:00Z')], [], 300, now)).toBeUndefined() // past
    expect(boxUntil([value(true)], [], 300, now)).toBeUndefined() // nothing closed
  })
})

describe('heldFor', () => {
  it('says how long a Value has held, roughly', () => {
    const ago = (ms: number) => heldFor(new Date(now - ms).toISOString(), now)
    expect(ago(20_000)).toBe('now')
    expect(ago(59 * 60_000)).toBe('59 min')
    expect(ago(47 * 3600_000)).toBe('47 h')
    expect(ago(72 * 3600_000)).toBe('3 d')
  })
})

describe('stretches', () => {
  it('cuts a series into what each point held, on in shares of the range', () => {
    const ps = [
      { t: 0, v: false },
      { t: 25, v: true },
      { t: 50, on: 0.2 }, // a bucket, mostly off
    ]
    expect(stretches(ps, 10, 110)).toEqual([
      { left: 0, width: 15, on: false },
      { left: 15, width: 25, on: true },
      { left: 40, width: 60, on: false },
    ])
  })
})

const cap = (key: string, settable: boolean, type: Capability['type'] = 'binary'): Capability => ({
  key,
  label: key,
  type,
  access: { observable: true, settable, queryable: false },
  category: 'primary',
})

describe('a Presence source', () => {
  it('is read through its main control if an on/off, otherwise its binary state', () => {
    expect(readCapability('network', [cap('connected', false)])?.key).toBe('connected')
    expect(readCapability('flag', [cap('on', true)])?.key).toBe('on')
    expect(readCapability('light', [cap('brightness', true, 'numeric'), cap('state', true)])?.key).toBe('state')
    expect(readCapability('arming', [cap('mode', true, 'enum'), cap('contact', false)])?.key).toBe('contact')
    expect(readCapability('temperature', [cap('temperature', false, 'numeric')])).toBeUndefined()
  })

  const phone = { target: 'device:p/network', departureDelay: 900 }
  const connected = cap('connected', false)
  const at = (data: boolean, since: string): Value => ({ data, at: '2026-01-01T12:29:00Z', since })

  it('counts as home while on, and until its departure delay has passed since it went off', () => {
    expect(sourceState(phone, connected, at(true, '2026-01-01T08:00:00Z'), false, now)).toEqual({ said: 'connected', counts: true })
    const left = sourceState(phone, connected, at(false, '2026-01-01T12:20:00Z'), false, now)
    expect(left).toEqual({ said: 'not connected', counts: true, until: Date.parse('2026-01-01T12:35:00Z') })
    expect(sourceText(left, () => '12:35')).toBe('not connected · counts as home until 12:35')
    const gone = sourceState(phone, connected, at(false, '2026-01-01T12:15:00Z'), false, now)
    expect(gone).toEqual({ said: 'not connected', counts: false })
    expect(sourceText(gone, () => '')).toBe('not connected')
    expect(sourceState({ target: 'flag:f' }, cap('on', true), at(false, '2026-01-01T12:29:00Z'), false, now)).toEqual({ said: 'off', counts: false })
    // No delay: off at once, even reported after now last ticked.
    expect(sourceState({ target: 'flag:f' }, cap('on', true), at(false, '2026-01-01T12:30:05Z'), false, now)).toEqual({ said: 'off', counts: false })
  })

  it('is ignored offline, with no Value, or once it vanished', () => {
    const offline = sourceState(phone, connected, at(true, '2026-01-01T08:00:00Z'), true, now)
    expect(sourceText(offline, () => '')).toBe('offline · ignored')
    expect(sourceState(phone, connected, undefined, false, now)).toEqual({ ignored: 'no value' })
    expect(sourceState(phone, undefined, undefined, false, now)).toEqual({ ignored: 'unavailable' })
  })
})
