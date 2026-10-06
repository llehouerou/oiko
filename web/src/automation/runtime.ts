// What the editor shows of an Automation at runtime: a picked Run's path and
// evidence, each Step's current state, and the Runs and Commands as they come.

import { roles } from '../roles'
import type { Capability, CommandRecord, CommandState, Reached, RunEnd, RunTrigger, StepState, Target, Trace } from '../types'
import type { Catalogue } from '../targets'
import { duration } from './model'

// The Steps a Run reached, and the handles it fired as `step.handle`.
export interface Path {
  reached: Set<string>
  fired: Set<string>
}

export function pathOf(t: Trace): Path {
  return {
    reached: new Set(t.steps.map((r) => r.step)),
    fired: new Set(t.steps.flatMap((r) => (r.fired ?? []).map((h) => `${r.step}.${h}`))),
  }
}

// An edge is on the path if the Run fired its handle: every edge out of it
// was followed, into a Step the Run reached.
export const onPath = (p: Path, e: { source: string; sourceHandle?: string | null; target: string }) =>
  p.fired.has(`${e.source}.${e.sourceHandle}`) && p.reached.has(e.target)

const ms = (iso: string) => new Date(iso).getTime()

// A time as a clock reads it, with the day if it isn't the day of ref.
export function clock(iso: string, ref = new Date(), seconds = false) {
  const d = new Date(iso)
  const time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: seconds ? '2-digit' : undefined })
  return d.toDateString() === ref.toDateString() ? time : `${d.toLocaleDateString([], { weekday: 'short', day: 'numeric', month: 'short' })} ${time}`
}

// A Value as it reads: a binary one on or off, a number with its unit.
function show(v: unknown, c?: Capability) {
  if (typeof v === 'boolean') return v ? 'on' : 'off'
  if (typeof v === 'number') return c?.unit ? `${v} ${c.unit}` : String(v)
  return typeof v === 'string' ? v : JSON.stringify(v)
}

// A Capability of t set to v, as it reads: "Effect speed 0.5"; an on/off (a
// light's, a plug's, a Flag's) reads "on" alone.
export function setting(targets: Catalogue, t: Target, key: string, v: unknown) {
  const e = targets.get(t)
  const c = e?.capabilities.find((x) => x.key === key)
  const control = e && roles(e.kind, e.capabilities).control
  return control?.key === key && control.type === 'binary' ? show(v, c) : `${c?.label ?? key} ${show(v, c)}`
}

// What happened to start a Run, if it happened to a target: its Value, the
// Event it emitted, or its Availability.
function happened(tg: RunTrigger, targets: Catalogue): string | undefined {
  if (tg.target && tg.kind === 'availabilityTrigger') return `${targets.name(tg.target)}: ${show(tg.value)}`
  if (!tg.target || !tg.capability) return undefined
  const what = tg.kind === 'eventTrigger' ? show(tg.value) : setting(targets, tg.target, tg.capability, tg.value)
  return `${targets.name(tg.target)}: ${what}`
}

// What a Run shows in the drawer: what started it, and what it did.
export function runSummary(r: RunEnd, targets: Catalogue, stepName: (id: string) => string) {
  const did = { acted: r.commands ? `${r.commands} command${r.commands > 1 ? 's' : ''}` : 'notified', nothing: 'nothing to do', error: 'error' }
  return { trigger: happened(r.trigger, targets) ?? stepName(r.trigger.step), result: did[r.outcome] }
}

// evidence is what Step r shows for Run t, one line per fact: what happened
// and when, on its trigger, and who started it (by, for a Manual trigger);
// what each Command asked and how it went. own is what its kind adds.
export function evidence(r: Reached, t: Trace, targets: Catalogue, own: string[] = [], by?: string): string[] {
  const ref = new Date(t.time)
  const lines: string[] = []
  if (r.step === t.trigger.step)
    lines.push(`${clock(t.trigger.time, ref, true)} ${happened(t.trigger, targets) ?? 'fired'}${by ? `, ${by}` : ''}${t.trigger.catchUp ? ', caught up' : ''}`)
  if (r.unknown) lines.push('read unknown')
  else if (r.read !== undefined)
    lines.push(r.at ? `read ${show(r.read)}, ${duration(Math.max(0, Math.round((ms(t.time) - ms(r.at)) / 1000)))} old` : `read ${show(r.read)}`)
  lines.push(...own)
  if (r.action) lines.push(r.until ? `${r.action} — until ${clock(r.until, ref)}` : r.action)
  for (const c of r.commands ?? []) {
    const asked = c.refused
      ? `refused, ${c.refused}`
      : `${
          Object.entries(c.values ?? {})
            .map(([k, v]) => setting(targets, c.target, k, v))
            .join(', ') || 'command'
        }${c.status ? ` — ${c.status.replace('_', ' ')}` : ''}`
    lines.push(`${clock(t.time, ref, true)} ${targets.name(c.target)}: ${asked}`)
  }
  if (r.error) lines.push(`error: ${r.error}`)
  if (r.print) lines.push(`print: ${r.print}`)
  return lines
}

// pending is a Step's deadline, unless already past: the Run it started
// refreshes the state.
export const pending = (s: StepState, now: Date) => (s.deadline && ms(s.deadline) > now.getTime() ? s.deadline : undefined)

// mergeRuns lists the Runs announced live with those fetched, the latest
// first, each once.
export function mergeRuns(live: RunEnd[], fetched: RunEnd[]): RunEnd[] {
  const seen = new Set<string>()
  return [...live, ...fetched].filter((r) => !seen.has(r.run) && seen.add(r.run)).sort((a, b) => ms(b.time) - ms(a.time))
}

// mergeCommands lays the latest Command announced live on a target over its
// kept history: the history is written a little after the announcement.
export function mergeCommands(kept: CommandRecord[], live?: CommandState): (CommandState & { time?: string })[] {
  if (!live?.id) return kept
  if (!kept.some((c) => c.id === live.id)) return [live, ...kept]
  return kept.map((c) => (c.id === live.id ? { ...c, status: live.status } : c))
}
