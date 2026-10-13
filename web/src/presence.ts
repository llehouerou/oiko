// A Presence as the web client shows it (ADR 0055): a chip of the Presence row, the stretches of
// its last 24 h, and its Presence sources' live state; for the Home presence, whether someone who
// is none of the Persons is inside, and until when.

import { held, type Point } from './history'
import { rests, roles, turns } from './roles'
import type { Capability, Presence, PresenceSource, Value } from './types'

// The Home presence's id, as in its key presence:home.
export const HOME = 'home'

// persons is the Persons' Presences in presences, by Name, as the row shows them after the Home
// presence.
export const persons = (ps: Presence[]) => ps.filter((p) => p.id !== HOME).sort((a, b) => a.name.localeCompare(b.name))

export type PresenceState = 'home' | 'away' | 'unknown'

const stateOf = (home: Value | undefined): PresenceState => (home === undefined ? 'unknown' : home.data === true ? 'home' : 'away')

// chip is what a Presence's chip shows of home and manual's Values: its face's initial and state,
// whether it is set by hand, and for how long its Value has held.
export function chip(p: Presence, home: Value | undefined, manual: Value | undefined, now: number) {
  return { initial: p.name.charAt(0).toUpperCase(), state: stateOf(home), manual: manual?.data === true, held: home?.since ? heldFor(home.since, now) : '' }
}

// homeChip is what the Home presence's chip says of its Value, given how many Persons are home.
export function homeChip(home: Value | undefined, personsHome: number, now: number) {
  const state = stateOf(home)
  const text = { unknown: 'Home unknown', away: 'Nobody home', home: someoneElse(home, personsHome) ? 'Someone home · not one of you' : 'Someone home' }[state]
  return { state, text, held: home?.since ? heldFor(home.since, now) : '' }
}

// someoneElse is whether someone who is none of the Persons is inside, as far as the Home presence
// tells: it is home while no Person is.
export const someoneElse = (home: Value | undefined, personsHome: number) => home?.data === true && personsHome === 0

// boxUntil is until when the Home presence's box holds someone, as its Entrance doors' and Signs of
// life's Values tell: the exit grace after the last door closed, unless a Sign of life is true now;
// undefined once past, or if nothing will empty it.
// shortcut: a Sign of life that turned true and back since the door closed shows only in its
// History, so the box may hold past the time told; read the History if that misleads.
export function boxUntil(doors: (Value | undefined)[], signs: (Value | undefined)[], exitGrace: number, now: number) {
  if (signs.some((v) => v?.data === true)) return undefined
  const closed = Math.max(...doors.filter((v) => v?.data === false).map((v) => Date.parse(v!.since ?? v!.at)))
  const until = closed + exitGrace * 1000
  return until > now ? until : undefined
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
