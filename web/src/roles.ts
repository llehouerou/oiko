import type { Capability, RoleMap } from './types'

// Roles (GLOSSARY.md): what each Capability is for on its Function. Oiko decides them
// (ADR 0056); every view of a Function, from its Tile to its History, reads them from here.
export interface Roles {
  control?: Capability // the main control, at most one; its on/off when binary
  adjustments: Capability[] // refining the main control: a light's brightness and colour
  state?: Capability // occupied, open, leaking…: active while on
  readings: Capability[] // in reading order; counters included
  events: Capability[]
  settings: Capability[]
  health: Capability[] // shown only when wrong
  diagnostics: Capability[]
  charted?: Capability // what its 24 h chart shows, and so which series the tiles load
}

// What holds Capabilities: a Function, a Device itself, an Aggregate, a Flag or a
// Presence, with the Role Oiko gave each.
export interface Described {
  capabilities: Capability[]
  roles?: RoleMap
}

// roles sorts the Capabilities of o by the Role Oiko gave them; one with none is in none.
export function roles(o: Described): Roles {
  const r: Roles = { adjustments: [], readings: [], events: [], settings: [], health: [], diagnostics: [] }
  const lists = { adjustment: r.adjustments, reading: r.readings, event: r.events, setting: r.settings, health: r.health, diagnostic: r.diagnostics }
  for (const c of o.capabilities) {
    const role = o.roles?.[c.key]
    if (role === 'control') r.control ??= c
    else if (role === 'state') r.state ??= c
    else if (role) lists[role].push(c)
  }
  r.readings.sort((a, b) => byReading(a.key, b.key))
  r.charted =
    r.state ??
    r.readings.find((c) => c.type === 'numeric') ??
    r.readings.find((c) => c.type === 'binary') ??
    (r.control?.type === 'binary' ? r.control : undefined) ??
    r.events[0]
  return r
}

export const readingOrder = ['temperature', 'humidity', 'co2', 'noise', 'pressure', 'illuminance']
const readingRank = (key: string) => (readingOrder.includes(key) ? readingOrder.indexOf(key) : readingOrder.length)
export const byReading = (a: string, b: string) => readingRank(a) - readingRank(b)

// A motion state: while on, motion is now.
export const motion = (c: Capability) => c.key === 'occupancy' || c.key === 'presence'

// What a binary Value is while on, and what its turns on count as.
export const turns = (c: Capability): [on: string, count: string] =>
  c.key === 'contact'
    ? ['open', 'Openings']
    : c.key === 'connected'
      ? ['connected', 'Connections']
      : c.key === 'home'
        ? ['home', 'Arrivals']
        : motion(c)
          ? ['occupied', 'Detections']
          : ['on', 'Times on']

// What a state is while off.
const rest: Record<string, string> = { contact: 'closed', connected: 'not connected', home: 'away' }
export const rests = (c: Capability) => rest[c.key] ?? 'clear'

// A button's Events are presses.
export const presses = (c: Capability) => c.key === 'action'

// CO₂ asks for airing above 1000 ppm, at once above 1500.
export function co2Level(key: string, v: unknown) {
  if (key !== 'co2' || typeof v !== 'number') return undefined
  return v > 1500 ? 'high' : v > 1000 ? 'raised' : undefined
}
