import { parseTarget } from './targets'
import { byReading, roles, type Roles } from './roles'
import type { Aggregate, Capability, Fn, Target } from './types'

// The Functions an Aggregate stands for: its members, nested Aggregates resolved to theirs, each
// once; as Oiko counts them.
export function memberLeaves(a: Aggregate, aggregates: Aggregate[], seen = new Set<string>([a.id])): Target[] {
  const leaves = a.members.flatMap((m) => {
    const p = parseTarget(m)
    if (p?.kind !== 'aggregate') return [m]
    const nested = aggregates.find((n) => n.id === p.id)
    if (!nested || seen.has(nested.id)) return []
    seen.add(nested.id)
    return memberLeaves(nested, aggregates, seen)
  })
  return [...new Set(leaves)]
}

// What a Tile shows of a Function (ADR 0014): its main control and what refines it,
// its state, readings and Events; settings, health and diagnostics are behind ⋯.
export function tileCaps(fn: Fn) {
  const r = roles(fn.kind, fn.capabilities)
  return [r.control, ...r.adjustments, r.state, ...r.readings, ...r.events].filter((c) => c !== undefined)
}

// One Capability on a Tile, with the Function it belongs to and its Target.
export interface Part {
  target: Target
  fn: Fn
  cap: Capability
}

// A Tile's shape (ADR 0014), from its Functions' Roles:
// - bar: a lone Function whose main control is an on/off, which a tap toggles; a
//   light's bar also drags its brightness;
// - event: nothing but Events, the last one shown;
// - sensor: nothing to command or press, with a state or a single reading: a bar
//   leading with how long the state has held, the readings beside;
// - readings: several readings and no state, a cell each;
// - camera: a camera's Picture (ADR 0036), with the state of its Device's other Functions, if any;
// - controls: anything else, each Function's controls.
export type Shape =
  | { kind: 'camera'; target: Target; state?: Part }
  | { kind: 'bar'; target: Target; fn: Fn; toggle: string }
  | { kind: 'event'; event: Part }
  | { kind: 'sensor'; state?: Part; readings: Part[] }
  | { kind: 'readings'; readings: Part[] }
  | { kind: 'controls' }

export function tileShape(fns: { target: Target; fn: Fn }[]): Shape {
  const all = fns.map(({ target, fn }) => ({ target, fn, r: roles(fn.kind, fn.capabilities) }))
  const parts = (pick: (r: Roles) => (Capability | undefined)[]) =>
    all.flatMap(({ target, fn, r }) => pick(r).flatMap((cap) => (cap ? [{ target, fn, cap }] : [])))
  const camera = all.find(({ fn }) => fn.kind === 'camera')
  if (camera) return { kind: 'camera', target: camera.target, state: parts((r) => [r.state])[0] }
  const [only] = all
  if (all.length === 1 && only?.r.control?.type === 'binary') return { kind: 'bar', target: only.target, fn: only.fn, toggle: only.r.control.key }
  const controls = parts((r) => [r.control, ...r.adjustments])
  const events = parts((r) => r.events)
  const states = parts((r) => [r.state])
  const readings = parts((r) => r.readings).sort((a, b) => byReading(a.cap.key, b.cap.key))
  const [event] = events
  if (event && !controls.length && !states.length && !readings.length) return { kind: 'event', event }
  if (controls.length || events.length || (!states.length && !readings.length)) return { kind: 'controls' }
  return states[0] || readings.length < 2 ? { kind: 'sensor', state: states[0], readings } : { kind: 'readings', readings }
}

// Health shows on a Tile only when something is wrong: a low battery, a tamper or low flag on.
export const unwell = (data: unknown) => (typeof data === 'number' ? data <= 20 : data === true)

// A lone control labels itself, and so do readings; a toggle has no label of its own,
// and several controls together need their Function named.
export function titled(fn: Fn) {
  const r = roles(fn.kind, fn.capabilities)
  const settable = [r.control, ...r.adjustments].filter((c) => c !== undefined)
  return settable.some((c) => c.type === 'binary') || (tileCaps(fn).length > 1 && settable.length > 0)
}

// Names the rules that apply to the Aggregate's Capabilities, e.g. "temperature · mean of 2".
export function aggregateSummary(a: Aggregate) {
  const types = new Set((a.capabilities ?? []).map((c) => c.type))
  const rules = [types.has('binary') && a.binary, types.has('numeric') && a.numeric].filter(Boolean).join('/')
  return [a.kind, `${rules} of ${a.members.length}`.trim()].filter(Boolean).join(' · ')
}
