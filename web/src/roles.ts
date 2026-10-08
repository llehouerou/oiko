import type { Capability } from './types'

// Roles (GLOSSARY.md, ADR 0014): what each Capability is for on its Function, told
// by its key, type, access and category, never by the hardware model. Every view
// of a Function, from its Tile to its History, reads them from here.
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

// In order of precedence: a light's main control is its state.
const mainControls = ['state', 'on', 'alarm', 'mode']
const lightAdjustments = ['brightness', 'color_temp', 'color_hs']
// The Bridge turns a contact's "closed" into off: on is open.
const stateKeys = ['occupancy', 'presence', 'contact', 'water_leak', 'smoke']
const healthKeys = ['battery', 'battpercentage', 'battery_low', 'tamper']
// A plug's current and voltage follow its power.
const diagnosticKeys = ['current', 'voltage']

// roles sorts the Capabilities of a Function of kind, or of a Device itself
// (kind ''), by Role. Each lands in one Role at most: a light's composites
// beside its colour (color_xy is color_hs in other coordinates) in none.
export function roles(kind: string, caps: Capability[]): Roles {
  const light = kind === 'light'
  const r: Roles = { adjustments: [], readings: [], events: [], settings: [], health: [], diagnostics: [] }
  const rank = (c: Capability) => (light ? (c.key === 'state' ? 0 : -1) : mainControls.indexOf(c.key))
  r.control = caps.filter((c) => c.category === 'primary' && c.access.settable && !c.stateless && rank(c) >= 0).sort((a, b) => rank(a) - rank(b))[0]
  for (const c of caps) {
    if (c === r.control) continue
    if (healthKeys.includes(c.key)) r.health.push(c)
    else if (c.category === 'config') r.settings.push(c)
    else if (c.category === 'diagnostic' || diagnosticKeys.includes(c.key)) r.diagnostics.push(c)
    else if (c.stateless) r.events.push(c)
    else if (c.access.settable) {
      if (light && lightAdjustments.includes(c.key)) r.adjustments.push(c)
      else if (!light || c.type !== 'composite') r.settings.push(c)
    } else if (!r.state && c.type === 'binary' && stateKeys.includes(c.key)) r.state = c
    else r.readings.push(c)
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

const readingOrder = ['temperature', 'humidity', 'co2', 'noise', 'pressure', 'illuminance']
const readingRank = (key: string) => (readingOrder.includes(key) ? readingOrder.indexOf(key) : readingOrder.length)
export const byReading = (a: string, b: string) => readingRank(a) - readingRank(b)

// A motion state: while on, motion is now.
export const motion = (c: Capability) => c.key === 'occupancy' || c.key === 'presence'

// What a binary Value is while on, and what its turns on count as.
export const turns = (c: Capability): [on: string, count: string] =>
  c.key === 'contact' ? ['open', 'Openings'] : motion(c) ? ['occupied', 'Detections'] : ['on', 'Times on']

// A button's Events are presses.
export const presses = (c: Capability) => c.key === 'action'

// CO₂ asks for airing above 1000 ppm, at once above 1500.
export function co2Level(key: string, v: unknown) {
  if (key !== 'co2' || typeof v !== 'number') return undefined
  return v > 1500 ? 'high' : v > 1000 ? 'raised' : undefined
}
