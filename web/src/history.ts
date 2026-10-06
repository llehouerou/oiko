// Views of the History: Refs over a range, read in one call, then, while the
// range includes now, followed through the store's stream as Oiko's writer
// records it: a Value or an Availability when it differs from the last point,
// every Event. With markers, a view also holds the Commands on its Targets
// and the Runs their Values or Events triggered, and follows them too. The
// tiles' last 24 h is one view; an open History sheet another, the Timeline a
// third.

import { useSyncExternalStore } from 'react'
import { presses, roles, turns } from './roles'
import { current, follow } from './store'
import { api } from './access'
import { owner, targetKind, type Catalogue, type Entry } from './targets'
import type { CommandKind } from './origin'
import type { Capability, CommandRecord, CommandState, Ref, RunEnd, Snapshot, Target, Update } from './types'

export const DAY = 24 * 60 * 60 * 1000

// Points as /api/history answers them, times in Unix ms: raw, or a bucket of
// a numeric Value, a binary one, or Events.
export interface Raw {
  t: number
  v: unknown
}
export interface Numeric {
  t: number
  mean: number
  min: number
  max: number
}
export interface Share {
  t: number
  on: number
}
export interface Counts {
  t: number
  counts: Record<string, number>
}
export type Point = Raw | Numeric | Share | Counts

export interface Gap {
  start: number
  end: number
  lost: number
}

interface Answer {
  from: number
  bucket: string
  series: Point[][]
  gaps: Gap[]
  availability: Record<Target, Raw[]>
  commands?: CommandRecord[]
  runs?: RunEnd[]
}

// What a view holds; replaced whole on each change. from is where the answer
// starts: no earlier than the History of its series.
export interface Loaded {
  from: number
  bucket: string // '' when the points came raw
  series: ReadonlyMap<string, Point[]> // by Ref; a Ref to Capability '' is its Target's Availability
  gaps: Gap[]
  commands: CommandRecord[] // with markers, the earliest first
  runs: RunEnd[]
}

export const key = (r: Ref) => `${r.target}|${r.capability}`
export const availabilityRef = (target: Target): Ref => ({ target, capability: '' })

const views = new Set<HistoryView>() // those loaded and not closed

export class HistoryView {
  state: Loaded = { from: 0, bucket: '', series: new Map(), gaps: [], commands: [], runs: [] }
  refs: Ref[] = []
  private query = { from: 0, to: 0, points: 0, markers: false }
  private follows = false // whether the range includes now: it follows the stream
  private span: number | null = null // the range's length while it ends now
  private loads = 0 // the latest load's number: an older one's answer is dropped
  private listeners = new Set<() => void>()

  subscribe = (l: () => void) => {
    this.listeners.add(l)
    return () => void this.listeners.delete(l)
  }

  // load reads refs over [from, to] in about points points each, with the
  // markers if asked; with live, the range ends now and slides with it. One
  // reaching past now holds still, and follows the stream too.
  async load(refs: Ref[], from: number, to: number, points: number, live: boolean, markers = false) {
    views.add(this)
    this.refs = refs
    this.query = { from, to, points, markers }
    this.follows = live || to > Date.now()
    this.span = live ? to - from : null
    const n = ++this.loads
    let a: Answer
    try {
      const res = await api('/api/history', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ from, to, points, markers: markers || undefined, refs }),
      })
      if (!res.ok) throw new Error((await res.text()).trim())
      a = await res.json()
    } catch (e) {
      console.error('History:', e)
      return
    }
    if (n !== this.loads) return
    const series = new Map<string, Point[]>()
    refs.forEach((r, i) => series.set(key(r), a.series[i] ?? []))
    for (const [t, ps] of Object.entries(a.availability)) series.set(key(availabilityRef(t)), ps)
    this.state = { from: a.from, bucket: a.bucket, series, gaps: a.gaps, commands: a.commands ?? [], runs: a.runs ?? [] }
    if (this.follows) this.seam(a)
    this.changed()
  }

  // seam adds what the store holds now, which the writer may not have
  // committed when the query read. An Event within the last bucket may
  // already be counted in it, so only one after a raw point is added.
  private seam(a: Answer) {
    const now = Date.now()
    for (const r of this.refs) {
      const value = current.value(r)
      if (value) this.add(r, Date.parse(value.at), value.data, false)
      const event = current.event(r)
      const last = this.state.series.get(key(r))?.at(-1)
      if (event && (!last || ('v' in last && Date.parse(event.at) > last.t))) this.add(r, Date.parse(event.at), event.data, true)
    }
    for (const t of Object.keys(a.availability)) {
      const availability = current.availability(t)
      if (availability) this.add(availabilityRef(t), now, availability, false)
    }
  }

  // reload reads the same again, after a reconnection; a live range ends now.
  reload() {
    const { from, to, points, markers } = this.query
    const now = Date.now()
    return this.span === null ? this.load(this.refs, from, to, points, false, markers) : this.load(this.refs, now - this.span, now, points, true, markers)
  }

  close() {
    views.delete(this)
    this.loads++
  }

  // receive follows a message of the stream while the range includes now.
  receive(msg: Snapshot | Update) {
    if (!this.follows) return
    switch (msg.kind) {
      case 'value':
      case 'refresh': // the writer receives refreshes too: one that differs from the last point is recorded
      case 'event':
        if (!msg.value) return // an Aggregate lost its Value: nothing is synthesized
        if (this.add(msg.ref!, Date.parse(msg.value.at), msg.value.data, msg.kind === 'event')) this.changed()
        return
      case 'availability':
        if (this.add(availabilityRef(msg.target!), Date.now(), msg.availability, false)) this.changed() // Home tells no time: it is now
        return
      case 'command':
        if (this.command(msg.command!)) this.changed()
        return
      case 'run': {
        const r = msg.run!
        if (!this.query.markers || !r.trigger.capability || !this.involves(r.trigger.target)) return
        this.state = { ...this.state, runs: [...this.state.runs, r] }
        return this.changed()
      }
    }
  }

  private involves = (t: Target) => this.refs.some((r) => r.target === t)

  // command follows a Command on a Target of the view: a new one comes in
  // pending, issued now; a known one takes its new status. It says whether
  // anything changed.
  private command(c: CommandState) {
    if (!this.query.markers || !this.involves(c.target)) return false
    const { commands } = this.state
    if (!commands.some((x) => x.id === c.id)) {
      if (c.status !== 'pending') return false // issued before the view's answer, which holds it
      this.state = { ...this.state, commands: [...commands, { ...c, time: new Date().toISOString() }] }
      return true
    }
    this.state = { ...this.state, commands: commands.map((x) => (x.id === c.id ? { ...x, status: c.status } : x)) }
    return true
  }

  // add appends to a loaded series, telling no one; it says whether it did.
  private add(r: Ref, t: number, v: unknown, event: boolean) {
    const ps = this.state.series.get(key(r))
    const next = ps && append(ps, t, v, event, this.span ?? Infinity)
    if (next === ps) return false
    this.state = { ...this.state, series: new Map(this.state.series).set(key(r), next!) }
    return true
  }

  private changed() {
    this.listeners.forEach((l) => l())
  }
}

export const useHistory = (v: HistoryView) => useSyncExternalStore(v.subscribe, () => v.state)

// append is ps with v at t as the writer records it: an Event always, a
// Value only when it differs from the last point, never back in time. After
// a bucket, a Value counts as new only if it came later. Points that no
// longer hold within span are dropped.
export function append(ps: Point[], t: number, v: unknown, event: boolean, span: number): Point[] {
  const last = ps.at(-1)
  if (last && !event && ('v' in last ? JSON.stringify(last.v) === JSON.stringify(v) : t <= last.t)) return ps
  let i = 0
  while (i + 1 < ps.length && ps[i + 1]!.t <= t - span) i++
  return [...ps.slice(i), { t: Math.max(t, last?.t ?? t), v }]
}

// The tiles' last 24 h.

const tiles = new HistoryView()

// The budget of the tiles' call: about a tile chart's width, in pixels.
const POINTS = 240

export const useSeries = (r: Ref) => useSyncExternalStore(tiles.subscribe, () => tiles.state.series.get(key(r)))
export const useGaps = () => useSyncExternalStore(tiles.subscribe, () => tiles.state.gaps)

// What a History sheet charts of a Function: its everyday Roles (main control and
// adjustments, state, readings, Events); with all, its settings and diagnostics
// too, and its Device's (battery, link quality…). Only what draws as a curve, a
// band or ticks: numeric, binary and enum Values, and Events.
export function sheetSeries(target: Target, kind: string, caps: Capability[], deviceCaps: Capability[], all: boolean) {
  const drawn = (c: Capability) => c.stateless || c.type === 'numeric' || c.type === 'binary' || c.type === 'enum'
  const r = roles(kind, caps)
  const everyday = new Set([r.control, ...r.adjustments, r.state, ...r.readings, ...r.events])
  const of = (t: Target) => (cap: Capability) => ({ ref: { target: t, capability: cap.key }, cap })
  // The everyday ones first, so the toggle leaves them as they were.
  const own = caps.filter(drawn)
  return [
    ...own.filter((c) => everyday.has(c)).map(of(target)),
    ...(all ? [...own.filter((c) => !everyday.has(c)).map(of(target)), ...deviceCaps.filter(drawn).map(of(owner(target)))] : []),
  ]
}

// A Function as views of the History list it: a Device's, an Aggregate or a
// Flag; never a Device itself, whose own Capabilities are its Functions' sheets'.
export const listed = (e: Entry) => targetKind(e.target) !== 'device'

// compareCandidates are the Functions a sheet of target, of kind, may add
// beside those picked: any with default series, the same kind first, then by
// kind and name.
export function compareCandidates(c: Catalogue, target: Target, kind: string, picked: Target[]) {
  return c.list
    .filter((e) => listed(e) && e.target !== target && !picked.includes(e.target) && sheetSeries(e.target, e.kind, e.capabilities, [], false).length > 0)
    .sort((a, b) => Number(b.kind === kind) - Number(a.kind === kind) || a.kind.localeCompare(b.kind) || a.name.localeCompare(b.name))
}

function tileRefs(): Ref[] {
  return current
    .catalogue()
    .list.filter(listed)
    .flatMap(({ target, kind, capabilities }) => {
      const c = roles(kind, capabilities).charted
      return c ? [{ target, capability: c.key }] : []
    })
}

function loadTiles() {
  const now = Date.now()
  return tiles.load(tileRefs(), now - DAY, now, POINTS, true)
}

// followHistory keeps every view following the stream. A new Snapshot,
// after a reconnection, reads them again, and other tiles read the tiles'.
export function followHistory() {
  follow((msg) => {
    switch (msg.kind) {
      case 'snapshot':
        for (const v of views) if (v !== tiles) void v.reload()
        return void loadTiles()
      case 'devices':
      case 'aggregates':
      case 'flags':
        if (JSON.stringify(tileRefs()) !== JSON.stringify(tiles.refs)) void loadTiles()
        return
    }
    for (const v of views) v.receive(msg)
  })
}

export type Span = [from: number, to: number]

// A numeric series as uPlot draws it: times, then mins, maxes and means.
export type Curve = [number[], (number | null)[], (number | null)[], (number | null)[]]

// held cuts what each point holds, until the next one, into pieces within
// [from, to) and outside spans (disjoint, the earliest first).
export function held<P extends { t: number }>(ps: P[], from: number, to: number, spans: Span[] = []) {
  const out: { from: number; to: number; p: P }[] = []
  ps.forEach((p, i) => {
    let a = Math.max(p.t, from)
    const b = Math.min(ps[i + 1]?.t ?? to, to)
    for (const [s, e] of spans) {
      if (a >= b || s >= b) break
      if (e <= a) continue
      if (s > a) out.push({ from: a, to: s, p })
      a = e
    }
    if (a < b) out.push({ from: a, to: b, p })
  })
  return out
}

// curve is a numeric series over [from, to) as uPlot data, times, then each
// point's min, max and mean: a raw point's are its Value. Each piece starts a
// step; a null ends one before a blank.
export function curve(ps: Point[], from: number, to: number, spans: Span[]): Curve {
  const out: Curve = [[], [], [], []]
  const push = (t: number, p: Point | null) => {
    const [mean, min, max] = p && 'mean' in p ? [p.mean, p.min, p.max] : p && 'v' in p && typeof p.v === 'number' ? [p.v, p.v, p.v] : [null, null, null]
    out[0].push(t)
    out[1].push(min)
    out[2].push(max)
    out[3].push(mean)
  }
  const pieces = held(ps, from, to, spans)
  pieces.forEach(({ from: s, to: e, p }, i) => {
    push(s, p)
    const next = pieces[i + 1]
    if (!next) push(e, p)
    else if (next.from > e) push(e, null)
  })
  return out
}

// blanks are the spans of [from, to) where nothing is known of a Target: it
// was offline or unknown, or Oiko recorded nothing (a Gap). all merges them.
export type Blanks = ReturnType<typeof blanks>

export function blanks(availability: Raw[], gs: Gap[], from: number, to: number) {
  const offline = held(availability, from, to)
    .filter((x) => x.p.v !== 'online')
    .map((x): Span => [x.from, x.to])
  const gap = gs.map((g): Span => [Math.max(g.start, from), Math.min(g.end, to)]).filter(([s, e]) => s < e)
  return { offline, gaps: gap, all: union([...offline, ...gap]) }
}

// union merges spans into disjoint ones, the earliest first.
function union(spans: Span[]) {
  const out: Span[] = []
  for (const [s, e] of [...spans].sort((x, y) => x[0] - y[0])) {
    const last = out.at(-1)
    if (last && s <= last[1]) last[1] = Math.max(last[1], e)
    else out.push([s, e])
  }
  return out
}

// joined are the blanks of a panel two Functions share, over the same range:
// where either was offline, and the home's Gaps.
export function joined(a: Blanks, b: Blanks): Blanks {
  const offline = union([...a.offline, ...b.offline])
  return { offline, gaps: a.gaps, all: union([...offline, ...a.gaps]) }
}

// shifted is ps moved later by d: the previous period drawn onto this one.
export const shifted = <P extends { t: number }>(ps: P[], d: number) => ps.map((p) => ({ ...p, t: p.t + d }))

// targetBlanks are the blanks of target in a loaded view: its own
// Availability's, a Function's its Device's; with shift, those of the period
// shift earlier, moved onto [from, to).
export const targetBlanks = (loaded: Loaded, target: Target, from: number, to: number, shift = 0) =>
  blanks(
    shifted((loaded.series.get(key(availabilityRef(owner(target)))) ?? []) as Raw[], shift),
    loaded.gaps.map((g) => ({ ...g, start: g.start + shift, end: g.end + shift })),
    from,
    to,
  )

// A marker on the time axis: n Commands, Runs or Events at t, the same kind.
export type MarkerKind = CommandKind | 'run' | 'event'
export interface Marker {
  t: number
  n: number
  kind: MarkerKind
  text: string
  link?: string // to the Trace of the Run it names
}

// cluster gathers the markers within [from, to], px wide, the earliest
// first, each cluster spanning less than gap pixels.
export function cluster(ms: Marker[], from: number, to: number, px: number, gap = 12) {
  const out: Marker[][] = []
  let start = -Infinity
  for (const m of ms.filter((m) => m.t >= from && m.t <= to).sort((a, b) => a.t - b.t)) {
    const x = ((m.t - from) / (to - from)) * px
    if (x - start < gap) out.at(-1)!.push(m)
    else {
      out.push([m])
      start = x
    }
  }
  return out
}

// Per period: what /api/history/periods sums up of a series in each local
// hour, day, week or month, computed by Oiko.

export type Per = 'hour' | 'day' | 'week' | 'month'
export type Measure = 'time_on' | 'count' | 'stats' | 'energy'

export interface Spread {
  min: number
  mean: number
  max: number
}

// ms on, a count, Events by value, a numeric Value's spread, or a counter's
// rise (energy); null when nothing of the period is known and nothing was
// recorded in it.
export type PeriodValue = number | Record<string, number> | Spread | null

// A period from start, Unix ms; the current one, so far, with the previous
// period's value at equal elapsed time and its total.
export interface Period {
  start: number
  value: PeriodValue
  incomplete: boolean // a Gap, offline or unknown time is part of it
  previous?: { value: PeriodValue; total: PeriodValue }
}

export interface MeasureOption {
  ref: Ref
  cap: Capability
  measure: Measure
  label: string
}

// measures are what a Function's series offer per period: time on and
// counts of a binary Value, presses of Events, min/mean/max of a reading,
// the energy of a counter.
export function measures(series: { ref: Ref; cap: Capability }[]): MeasureOption[] {
  return series
    .filter(({ cap }) => cap.category === 'primary')
    .flatMap(({ ref, cap }): MeasureOption[] => {
      if (cap.stateless) return [{ ref, cap, measure: 'count', label: presses(cap) ? 'Presses' : cap.label }]
      if (cap.type === 'binary') {
        const [on, count] = turns(cap)
        return [
          { ref, cap, measure: 'time_on', label: `Time ${on}` },
          { ref, cap, measure: 'count', label: count },
        ]
      }
      if (cap.counter) return [{ ref, cap, measure: 'energy', label: cap.label }]
      return cap.type === 'numeric' && !cap.access.settable ? [{ ref, cap, measure: 'stats', label: `${cap.label} min/mean/max` }] : []
    })
}

// What a Timeline lane draws of a Function: its state, a binary reading or its
// on/off as bands, a reading as a curve (a counter's only when it has no other),
// its Events as ticks; and what it sums up over its span: openings or
// detections, time on, energy, min–max, presses. A Function with only
// settings (a timeout, an LED mode, a melody…) draws nothing.
export function lane(target: Target, kind: string, caps: Capability[]) {
  const r = roles(kind, caps)
  const band = r.state ?? r.readings.find((c) => c.type === 'binary') ?? (r.control?.type === 'binary' ? r.control : undefined)
  const counter = r.readings.find((c) => c.counter)
  const curve = r.readings.find((c) => c.type === 'numeric' && !c.counter) ?? counter
  const events = r.events[0]
  if (!band && !curve && !events) return undefined
  const summed = [...new Set([band, counter, curve, events])].filter((c) => c !== undefined)
  const summary = measures(summed.map((cap) => ({ ref: { target, capability: cap.key }, cap }))).filter(
    (m) => m.cap !== band || m.measure === (band.access.settable ? 'time_on' : 'count'),
  )
  return { band, curve, events, summary }
}

const groups = [
  { name: 'Lights', kinds: ['light'], color: '#fbbf24' },
  { name: 'Plugs', kinds: ['switch'], color: '#fb923c' },
  { name: 'Doors', kinds: ['contact', 'tamper'], color: '#38bdf8' },
  { name: 'Motion', kinds: ['occupancy'], color: '#a78bfa' },
  { name: 'Climate', kinds: ['temperature', 'humidity', 'pressure', 'illuminance'], color: '#34d399' },
  { name: 'Buttons', kinds: ['button'], color: '#34d399' },
  { name: 'Other', kinds: [], color: '#34d399' },
]
export type LaneGroup = (typeof groups)[number]
export const laneGroups = groups
export const laneGroup = (kind: string) => groups.find((g) => g.kinds.includes(kind)) ?? groups.at(-1)!

export type TimelinePer = Extract<Per, 'day' | 'week'>

// timelineSpan is the local day, or week from Monday, t is in.
export function timelineSpan(per: TimelinePer, t: number): Span {
  const d = new Date(t)
  d.setHours(0, 0, 0, 0)
  if (per === 'week') d.setDate(d.getDate() - ((d.getDay() + 6) % 7))
  const from = d.getTime()
  return [from, shift(per, from, 1)]
}

// How many periods a view shows.
export const periodsShown: Record<Per, number> = { hour: 24, day: 14, week: 12, month: 12 }

// shift moves t by n local periods; a month's is then on the 1st, which every
// month has.
export function shift(per: Per, t: number, n: number) {
  const d = new Date(t)
  if (per === 'hour') d.setHours(d.getHours() + n)
  else if (per === 'day') d.setDate(d.getDate() + n)
  else if (per === 'week') d.setDate(d.getDate() + 7 * n)
  else {
    d.setDate(1)
    d.setMonth(d.getMonth() + n)
  }
  return d.getTime()
}

// periodsSpan is the range to ask for the periods shown whose last holds
// at, or now when at is null: Oiko cuts them from the start of the one from
// is in to the end of the one at is in.
export function periodsSpan(per: Per, at: number | null, now: number) {
  const last = at ?? now
  return { from: shift(per, last, 1 - periodsShown[per]), to: at === null ? now : at + 1 }
}

// loadPeriods reads items per period, or over the span as one period.
export async function loadPeriods(from: number, to: number, per: Per | 'span', items: { ref: Ref; measure: Measure }[]) {
  const res = await api('/api/history/periods', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ from, to, per, items: items.map(({ ref, measure }) => ({ ref, measure })) }),
  })
  if (!res.ok) throw new Error((await res.text()).trim())
  return (await res.json()) as { end: number; first: number | null; items: Period[][] }
}

// refreshed is ps with fresh, read again from the current period on: a new
// period pushes the oldest out.
export function refreshed(ps: Period[], fresh: Period[]) {
  const kept = ps.filter((p) => p.start < fresh[0]!.start)
  return [...kept, ...fresh].slice(-ps.length)
}
