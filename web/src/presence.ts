// A Person's Presence as the web client shows it (ADR 0055): a chip of the Presence row, by Name,
// and the stretches of its last 24 h.

import { held, type Point } from './history'
import type { Presence, Value } from './types'

// ordered is presences by Name, as the row shows them.
export const ordered = (ps: Presence[]) => [...ps].sort((a, b) => a.name.localeCompare(b.name))

export type PresenceState = 'home' | 'away' | 'unknown'

// chip is what a Presence's chip shows of home and manual's Values: its face's initial and state,
// whether it is set by hand, and for how long its Value has held.
export function chip(p: Presence, home: Value | undefined, manual: Value | undefined, now: number) {
  const state: PresenceState = home === undefined ? 'unknown' : home.data === true ? 'home' : 'away'
  return { initial: p.name.charAt(0).toUpperCase(), state, manual: manual?.data === true, held: home?.since ? heldFor(home.since, now) : '' }
}

// heldFor is how long since since, roughly: 'now', minutes, hours up to two days, then days.
export function heldFor(since: string, now: number) {
  const m = Math.floor((now - Date.parse(since)) / 60_000)
  if (m < 1) return 'now'
  if (m < 60) return `${m} min`
  if (m < 48 * 60) return `${Math.floor(m / 60)} h`
  return `${Math.floor(m / 1440)} d`
}

// stretches cuts a binary series into what each point held within [from, to), in percent of it;
// a bucket counts as on when mostly on.
export const stretches = (ps: Point[], from: number, to: number) =>
  held(ps, from, to).map(({ from: a, to: b, p }) => ({
    left: ((a - from) / (to - from)) * 100,
    width: ((b - a) / (to - from)) * 100,
    on: 'v' in p ? p.v === true : 'on' in p && p.on > 0.5,
  }))
