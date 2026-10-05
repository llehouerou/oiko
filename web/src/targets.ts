import type { Aggregate, Capability, Device, Flag, Fn } from './types'

// A Target's key, as internal/home writes it (ADR 0009): what a Command
// addresses and what a Value belongs to. '' is none.
//   device:<id>             the Device itself
//   device:<id>/<function>  one of its Functions, e.g. device:0193…/switch/l2
//   aggregate:<id>
//   flag:<id>
export type Target = string

export const deviceTarget = (device: string, fn = ''): Target => (fn ? `device:${device}/${fn}` : `device:${device}`)
export const aggregateTarget = (id: string): Target => `aggregate:${id}`
export const flagTarget = (id: string): Target => `flag:${id}`

export interface ParsedTarget {
  kind: 'device' | 'aggregate' | 'flag'
  id: string
  function: string // of a Device's Function, else ''
}

// parseTarget reads a key; undefined if it is none or malformed. Ids never
// contain '/', so a Function key may.
export function parseTarget(t: Target): ParsedTarget | undefined {
  const colon = t.indexOf(':')
  const kind = t.slice(0, colon)
  const rest = t.slice(colon + 1)
  const slash = rest.indexOf('/')
  const id = slash < 0 ? rest : rest.slice(0, slash)
  const fn = slash < 0 ? '' : rest.slice(slash + 1)
  if (colon < 0 || !id || (slash >= 0 && (kind !== 'device' || !fn))) return undefined
  if (kind !== 'device' && kind !== 'aggregate' && kind !== 'flag') return undefined
  return { kind, id, function: fn }
}

// owner is the Target whose Availability t has: a Function's is its Device's.
export function owner(t: Target): Target {
  const p = parseTarget(t)
  return p?.kind === 'device' ? deviceTarget(p.id) : t
}

// What kind of Target t is: a Device's Function, the Device itself, an
// Aggregate or a Flag; undefined if it is none or malformed.
export function targetKind(t: Target) {
  const p = parseTarget(t)
  return p?.kind === 'device' ? (p.function ? 'function' : 'device') : p?.kind
}

// A Target as every view lists it. A Function is named after its Device, with
// its key when the Device has several; a Device itself has kind ''.
// ownerCapabilities are a Function's Device's, [] for any other Target.
export interface Entry {
  target: Target
  name: string
  kind: string
  capabilities: Capability[]
  ownerCapabilities: Capability[]
}

export interface Catalogue {
  list: Entry[] // Functions, Aggregates, Flags, then Devices, by Name within each
  get: (t: Target) => Entry | undefined
  name: (t: Target) => string // 'deleted target' once it is gone
}

// catalogue resolves every Target of the home, the one place a view learns
// what a Target is called and what it can do.
export function catalogue(devices: Device[], aggregates: Aggregate[], flags: Flag[]): Catalogue {
  const byName = (a: Entry, b: Entry) => a.name.localeCompare(b.name)
  const list = [
    ...devices
      .flatMap((d) =>
        (d.functions ?? []).map((fn) => ({
          target: deviceTarget(d.id, fn.key),
          name: (d.functions?.length ?? 0) > 1 ? `${d.name} · ${fn.key}` : d.name,
          kind: fn.kind,
          capabilities: fn.capabilities,
          ownerCapabilities: d.capabilities ?? [],
        })),
      )
      .sort(byName),
    ...aggregates
      .map((a) => ({ target: aggregateTarget(a.id), name: a.name, kind: a.kind ?? '', capabilities: a.capabilities ?? [], ownerCapabilities: [] }))
      .sort(byName),
    ...flags.map((f) => ({ target: flagTarget(f.id), name: f.name, kind: f.kind, capabilities: f.capabilities, ownerCapabilities: [] })).sort(byName),
    ...devices.map((d) => ({ target: deviceTarget(d.id), name: d.name, kind: '', capabilities: d.capabilities ?? [], ownerCapabilities: [] })).sort(byName),
  ]
  const index = new Map(list.map((e) => [e.target, e]))
  return { list, get: (t) => index.get(t), name: (t) => index.get(t)?.name ?? 'deleted target' }
}

// fnOf is an Aggregate's or a Flag's entry as the Function it stands for.
export const fnOf = (e: Entry): Fn => ({ key: '', kind: e.kind, capabilities: e.capabilities })
