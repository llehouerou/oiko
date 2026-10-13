// A Person's Presence as the web client shows it (ADR 0055): a chip of the Presence row, by Name,
// the stretches of its last 24 h, and its Presence sources' live state.

import { held, type Point } from './history'
import { rests, roles, turns } from './roles'
import type { Capability, Presence, PresenceSource, Value } from './types'

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

// readCapability is what a Presence source is read through, as Oiko reads it (ADR 0049): its main
// control if an on/off, otherwise its binary state; undefined if it has neither.
export function readCapability(kind: string, caps: Capability[]): Capability | undefined {
  const r = roles(kind, caps)
  return r.control?.type === 'binary' ? r.control : r.state
}

// What a Presence source says now: why Oiko ignores it, or what it reads and whether it counts as
// home, until when if only by its departure delay.
export type SourceState = { ignored: 'unavailable' | 'offline' | 'no value' } | { said: string; counts: boolean; until?: number }

// sourceState derives it from what the Presence source is read through (undefined once it vanished), its
// Value, and whether its Device or Bridge is offline or it is Detached.
export function sourceState(source: PresenceSource, cap: Capability | undefined, value: Value | undefined, offline: boolean, now: number): SourceState {
  if (!cap) return { ignored: 'unavailable' }
  if (offline) return { ignored: 'offline' }
  if (typeof value?.data !== 'boolean') return { ignored: 'no value' }
  const state = !cap.access.settable // a state, not a main control
  if (value.data) return { said: state ? turns(cap)[0] : 'on', counts: true }
  const said = state ? rests(cap) : 'off'
  const since = Date.parse(value.since ?? value.at)
  const until = since + (source.departureDelay ?? 0) * 1000
  // now ticks: it may lag a Value just reported
  return until > Math.max(now, since) ? { said, counts: true, until } : { said, counts: false }
}

// sourceText is how a sheet says state, with clock formatting a time of day.
export function sourceText(state: SourceState, clock: (t: number) => string) {
  if ('ignored' in state) return `${state.ignored} · ignored`
  return state.until ? `${state.said} · counts as home until ${clock(state.until)}` : state.said
}
