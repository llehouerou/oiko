import { describe, expect, it } from 'vitest'
import { chip, heldFor, ordered, stretches } from './presence'
import type { Presence, Value } from './types'

const presence = (id: string, name: string): Presence => ({ id, name, kind: 'presence', capabilities: [] })
const value = (data: boolean, since = '2026-01-01T10:00:00Z'): Value => ({ data, at: since, since })
const now = Date.parse('2026-01-01T12:30:00Z')

describe('the Presence row', () => {
  it('shows a chip per Person, by Name', () => {
    const ps = [presence('b', 'bob'), presence('c', 'Chloe'), presence('a', 'Alice')]
    expect(ordered(ps).map((p) => p.name)).toEqual(['Alice', 'bob', 'Chloe'])
    expect(ps.map((p) => p.name)).toEqual(['bob', 'Chloe', 'Alice']) // left as it was
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
