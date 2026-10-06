// A Function's History sheet, opened from its tile's mini-chart: its series
// over a range, one panel per unit sharing the time axis, binary and enum
// Values as state bands. At coarse zoom each point is a bucket's mean over
// the band of its min–max, and the sheet says so. A marker strip puts what
// was asked of it (its Commands), the Runs it triggered and its Events over
// the curves, dotted across them. One cursor runs across every panel, and each
// curve tells its values there, or the latest ones. Other Functions'
// default series can join them, and the previous period overlay them,
// dashed. Per period mode sums the Function up per hour, day, week or month
// instead.

import { useEffect, useRef, useState, type ReactNode } from 'react'
import {
  CurvePanel,
  Indented,
  MIN,
  MarkerStrip,
  PAD_L,
  PAD_R,
  Strip,
  TimeAxis,
  cursor,
  hatchGap,
  hatchOffline,
  SelectBox,
  markerStyle,
  useTimeGestures,
  useWidth,
  type Line,
} from './charts'
import { format } from './controls'
import { byWhom, commandKind, runOf, useNames } from './origin'
import {
  DAY,
  HistoryView,
  cluster,
  compareCandidates,
  curve,
  joined,
  shifted,
  type Blanks,
  key,
  measures,
  sheetSeries,
  useHistory,
  type Loaded,
  type Marker,
  type Point,
  targetBlanks,
  type Span,
} from './history'
import { Periods } from './Periods'
import { useCatalogue, useNow } from './store'
import { api, useAllows } from './access'
import type { Capability, CommandRecord, LiveView, Recording, Ref, RunEnd, Target } from './types'
import { watchedFor } from './liveview'
import { cameraOf, describeRecording } from './recordings'
import { RecordingList, useRecordings } from './RecordingList'
import { owner } from './targets'
import type { Document } from './automation/model'
import { runSummary, setting } from './automation/runtime'

const palette = ['#fbbf24', '#38bdf8', '#a78bfa', '#34d399', '#f472b6', '#fb923c', '#a3e635']

const presets: [string, number][] = [
  ['6h', 6 * 60 * MIN],
  ['24h', DAY],
  ['7d', 7 * DAY],
  ['30d', 30 * DAY],
  ['All', Infinity],
]

// A range: its length, Infinity for All, and its end, null while it ends now.
interface Range {
  len: number
  end: number | null
}

// A Guest reads no History: the sheet never opens for them.
export function HistorySheet(props: { target: Target; onClose: () => void }) {
  return useAllows('member') ? <Sheet {...props} /> : null
}

function Sheet({ target, onClose }: { target: Target; onClose: () => void }) {
  const targets = useCatalogue()
  const subject = targets.get(target)
  const dialog = useRef<HTMLDialogElement>(null)
  const pressed = useRef<EventTarget>(null)
  useEffect(() => dialog.current?.showModal(), [])
  const [all, setAll] = useState(false)
  const [preset, setPreset] = useState(DAY)
  const [range, setRange] = useState<Range>({ len: DAY, end: null })
  const [others, setOthers] = useState<Target[]>([]) // the Functions compared with it
  const [previous, setPrevious] = useState(false)
  const prev = previous && Number.isFinite(range.len) // All has no previous period
  const [view] = useState(() => new HistoryView())
  const loaded = useHistory(view)
  const frame = useRef<HTMLDivElement>(null)
  const width = useWidth(frame) - PAD_L - PAD_R // of the time scale
  const points = Math.max(60, Math.round(width))
  // Its series, then each compared Function's default ones; one deleted meanwhile drops out.
  const compared = others.flatMap((t) => targets.get(t) ?? [])
  const series = subject
    ? [
        ...sheetSeries(target, subject.kind, subject.capabilities, subject.ownerCapabilities, all).map((s) => ({ ...s, name: subject.name })),
        ...compared.flatMap((e) => sheetSeries(e.target, e.kind, e.capabilities, [], false).map((s) => ({ ...s, name: e.name }))),
      ]
    : []
  const refs = series.map((s) => s.ref)
  const candidates = subject ? compareCandidates(targets, target, subject.kind, others) : []
  const [mode, setMode] = useState<'curve' | 'period'>('curve')
  const offered = subject ? measures(sheetSeries(target, subject.kind, subject.capabilities, [], false)) : []
  const perPeriod = mode === 'period' && offered.length > 0

  // One call per range, resolution or series; a zoom's gestures settle first.
  // The previous period comes in the same call: the range twice as long, in
  // twice the points, so at the same step, and it slides along while live.
  const refsKey = JSON.stringify(refs)
  useEffect(() => {
    if (!refs.length) return
    const t = setTimeout(() => {
      const end = range.end ?? Date.now()
      const k = prev ? 2 : 1
      void view.load(refs, Number.isFinite(range.len) ? end - k * range.len : 0, end, k * points, range.end === null, true)
    }, 100)
    return () => clearTimeout(t)
  }, [view, refsKey, range, points, prev]) // eslint-disable-line react-hooks/exhaustive-deps -- refs by value
  useEffect(() => () => view.close(), [view])
  useEffect(() => () => cursor.set(null), [])

  // A range that ends now slides with the clock, and at once with a newer point.
  const now = useNow()
  const latest = Math.max(0, ...[...loaded.series.values()].map((ps) => ps.at(-1)?.t ?? 0))
  const to = range.end ?? Math.max(now, latest)
  const from = Number.isFinite(range.len) ? to - range.len : loaded.from
  const len = to - from

  // show shows [f, t], never past now; a range ending about now follows it.
  const show = (f: number, t: number) => {
    const now = Date.now()
    const len = Math.max(10 * MIN, t - f)
    const end = Math.min(t, now)
    setRange(end >= now - len / 100 ? { len, end: null } : { len, end })
  }
  const selecting = useTimeGestures(frame, from, to, show, () => setRange({ len: preset, end: null }))

  // Markers of its own series, not the compared ones', clustered at the width
  // of the time scale; the Recordings of its Device's camera among them.
  const describe = useDescribe()
  const own = series.filter((s) => !others.includes(s.ref.target))
  const camera = cameraOf(targets, target)
  const recordings = useRecordings(camera, range.len, range.end, loaded.from)
  const clusters = cluster(markers(loaded, recordings ?? [], own, describe), from, to, width)
  const marks = clusters.map((ms) => ({ t: ms[0]!.t, color: markerStyle[ms[0]!.kind].color }))

  // Curves by unit, hatched where any of their Functions was offline; a Value
  // without one gets its own panel; Events are markers. The previous period of
  // a curve is dashed, of a band dimmed beneath it: moved onto this one, and
  // read at the cursor.
  const units = new Map<string, { lines: Line[]; blank: Blanks }>()
  const strips: ReactNode[] = []
  series.forEach(({ ref, cap, name }, i) => {
    const color = palette[i % palette.length]!
    const label = compared.length > 0 ? `${name} · ${cap.label}` : cap.label
    const ps = loaded.series.get(key(ref)) ?? []
    const blank = targetBlanks(loaded, ref.target, from, to)
    if (cap.stateless) return
    const was = shifted(ps, len)
    const before = prev ? targetBlanks(loaded, ref.target, from, to, len) : undefined
    if (cap.type === 'numeric') {
      const unit = cap.unit ?? `|${cap.label}`
      const group = units.get(unit)
      const lines = [
        { label, color, data: curve(ps, from, to, blank.all), read: (t: number) => readAt(ps, t, blank.all, cap) },
        ...(before
          ? [
              {
                label: `${label} · previous`,
                color,
                data: curve(was, from, to, before.all),
                read: (t: number) => readAt(was, t, before.all, cap),
                dashed: true,
              },
            ]
          : []),
      ]
      units.set(unit, group ? { lines: [...group.lines, ...lines], blank: joined(group.blank, blank) } : { lines, blank })
    } else {
      const binary = cap.type === 'binary' // an enum's states are written in its band
      strips.push(
        <Indented key={key(ref)} from={from} to={to} label={label}>
          <Strip ps={ps} cap={cap} from={from} to={to} blank={blank} color={color} read={binary ? (t) => readAt(ps, t, blank.all, cap) : undefined} />
          {before && (
            <div className="mt-0.5 opacity-50" title="Previous period">
              <Strip ps={was} cap={cap} from={from} to={to} blank={before} color={color} read={binary ? (t) => readAt(was, t, before.all, cap) : undefined} />
            </div>
          )}
        </Indented>,
      )
    }
  })
  const button = 'rounded px-2 py-0.5 text-xs'

  return (
    <dialog
      ref={dialog}
      onClose={(e) => e.target === e.currentTarget && onClose()} // React passes on the close of a dialog opened from it, such as a Recording's
      onPointerDown={(e) => (pressed.current = e.target)}
      onClick={(e) => e.target === dialog.current && pressed.current === dialog.current && dialog.current.close()} // not a drag released there
      className="m-0 h-full max-h-none w-full max-w-none bg-neutral-900 p-0 text-neutral-100 backdrop:bg-black/60 sm:m-auto sm:h-[94vh] sm:max-h-[94vh] sm:w-[94vw] sm:max-w-7xl sm:rounded-xl"
    >
      <div className="space-y-4 p-4">
        <header className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 className="truncate text-lg font-medium">{subject?.name}</h2>
            <p className="text-xs text-neutral-500">{[subject?.kind, 'History'].filter(Boolean).join(' · ')}</p>
          </div>
          <button onClick={() => dialog.current?.close()} aria-label="Close" className="text-neutral-400 hover:text-white">
            ✕
          </button>
        </header>
        {offered.length > 0 && (
          <div className="flex w-fit gap-1 rounded-lg bg-neutral-800 p-1 text-sm">
            {(
              [
                ['curve', 'Curve'],
                ['period', 'Per period'],
              ] as const
            ).map(([m, label]) => (
              <button
                key={m}
                onClick={() => setMode(m)}
                className={`rounded-md px-3 py-0.5 ${mode === m ? 'bg-neutral-100 text-neutral-900' : 'text-neutral-300 hover:bg-neutral-700'}`}
              >
                {label}
              </button>
            ))}
          </div>
        )}
        {/* Both modes stay mounted while hidden: Per period keeps its place, Curve its gestures and sizes. */}
        {offered.length > 0 && (
          <div hidden={!perPeriod}>
            <Periods
              options={offered}
              active={perPeriod}
              onOpen={(f, t) => {
                setMode('curve')
                show(f, t)
              }}
            />
          </div>
        )}
        <div hidden={perPeriod} className="space-y-4">
          <div className="flex flex-wrap items-center gap-1 text-sm">
            {presets.map(([label, l]) => (
              <button
                key={label}
                onClick={() => {
                  setPreset(l)
                  setRange({ len: l, end: null })
                }}
                className={`${button} ${range.end === null && range.len === l ? 'bg-neutral-100 text-neutral-900' : 'bg-neutral-800 text-neutral-300 hover:bg-neutral-700'}`}
              >
                {label}
              </button>
            ))}
            <button
              onClick={() => show(from - len, from)}
              disabled={!Number.isFinite(range.len)}
              aria-label="Earlier"
              className={`${button} bg-neutral-800 disabled:opacity-30`}
            >
              ◀
            </button>
            <button
              onClick={() => show(to, to + len)}
              disabled={range.end === null}
              aria-label="Later"
              className={`${button} bg-neutral-800 disabled:opacity-30`}
            >
              ▶
            </button>
            <span className="px-1 text-xs text-neutral-500">
              {when(from, len)} → {range.end === null ? 'now' : when(to, len)}
            </span>
            {loaded.bucket && (
              <span
                className="px-1 text-xs text-neutral-500"
                title="Too many points for the width: each point sums up a bucket, a curve its mean over its min–max"
              >
                per {bucketText[loaded.bucket] ?? loaded.bucket} · min/mean/max
              </span>
            )}
            <label className="ml-auto flex items-center gap-1 text-xs text-neutral-400">
              <input type="checkbox" checked={all} onChange={(e) => setAll(e.target.checked)} className="accent-amber-400" /> settings &amp; diagnostics
            </label>
          </div>
          <div className="flex flex-wrap items-center gap-2 text-xs text-neutral-400">
            <label className="flex items-center gap-1">
              <input
                type="checkbox"
                checked={prev}
                disabled={!Number.isFinite(range.len)}
                onChange={(e) => setPrevious(e.target.checked)}
                className="accent-amber-400"
              />{' '}
              vs previous period
            </label>
            {candidates.length > 0 && (
              <select value="" onChange={(e) => setOthers([...others, e.target.value])} className="rounded bg-neutral-800 px-2 py-0.5 text-neutral-300">
                <option value="" disabled>
                  Compare with…
                </option>
                {[...new Set(candidates.map((c) => c.kind))].map((kind) => (
                  <optgroup key={kind} label={kind || 'other'}>
                    {candidates
                      .filter((c) => c.kind === kind)
                      .map((c) => (
                        <option key={c.target} value={c.target}>
                          {c.name}
                        </option>
                      ))}
                  </optgroup>
                ))}
              </select>
            )}
            {compared.map((l) => (
              <span key={l.target} className="flex items-center gap-1.5 rounded bg-neutral-800 py-0.5 pr-1 pl-2 text-neutral-300">
                {l.name}
                <button
                  onClick={() => setOthers(others.filter((t) => t !== l.target))}
                  aria-label={`Stop comparing with ${l.name}`}
                  className="px-1 text-neutral-400 hover:text-white"
                >
                  ✕
                </button>
              </span>
            ))}
          </div>
          <div ref={frame} className="relative space-y-3 select-none" style={{ touchAction: 'pan-y' }} onMouseLeave={() => cursor.set(null)}>
            {[...units].map(([unit, { lines, blank }]) => (
              <CurvePanel
                key={unit + lines.map((l) => l.label + l.color).join()}
                title={
                  <>
                    {lines
                      .filter((l) => !l.dashed)
                      .map((l) => (
                        <span key={l.label} style={lines.length > 1 ? { color: l.color } : undefined}>
                          {l.label}
                        </span>
                      ))}
                    {!unit.startsWith('|') && <span className="text-neutral-400">{unit}</span>}
                    {lines.some((l) => l.dashed) && <span>┄ previous period</span>}
                  </>
                }
                lines={lines}
                marks={marks}
                from={from}
                to={to}
                blank={blank}
                height={240}
                onSelect={show}
              />
            ))}
            {strips}
            {units.size === 0 && series.length > 0 && <TimeAxis from={from} to={to} />}
            {series.length > 0 && <MarkerStrip clusters={clusters} from={from} to={to} />}
            {series.length === 0 && <p className="text-sm text-neutral-500">Nothing to chart here{all ? '' : ' but its settings and diagnostics'}.</p>}
            {selecting && <SelectBox select={selecting} from={from} to={to} />}
          </div>
          {camera && <RecordingList camera={camera} list={recordings} />}
          <p className="flex flex-wrap gap-3 text-[11px] text-neutral-500">
            {Object.values(markerStyle).map((s) => (
              <span key={s.label}>
                <span style={{ color: s.color }}>{s.glyph}</span> {s.label}
              </span>
            ))}
            <span>
              <span className="inline-block h-2 w-3 align-middle" style={{ background: hatchOffline }} /> offline
            </span>
            <span>
              <span className="inline-block h-2 w-3 align-middle" style={{ background: hatchGap }} /> Gap: Oiko recorded nothing
            </span>
            <span className="hidden sm:inline">Drag across a curve or band to zoom, double-click to reset, ctrl+wheel to zoom, shift+wheel to pan.</span>
          </p>
        </div>
      </div>
    </dialog>
  )
}

// What markers tell: what a Command asked, what issued it and, unless
// confirmed, how it ended; what triggered a Run and what it did, with the
// Automations' names.
function useDescribe() {
  const targets = useCatalogue()
  const [names, setNames] = useState<Record<string, string> | null>(null)
  useEffect(() => {
    api('/api/automations')
      .then((r) => (r.ok ? r.json() : []))
      .then(
        (docs: Document[]) => setNames(Object.fromEntries(docs.map((d) => [d.id, d.name]))),
        () => {},
      )
  }, [])
  const automation = (id: string) => names?.[id] ?? (names ? 'a deleted Automation' : 'an Automation')
  const identities = useNames()
  const ended = { pending: 'pending', confirmed: '', failed: 'failed', timed_out: 'no response', superseded: 'superseded' }
  return {
    command: (c: CommandRecord) =>
      [
        Object.entries(c.values ?? {})
          .map(([k, v]) => setting(targets, c.target, k, v))
          .join(', ') || 'Command',
        byWhom(c.origin, identities, automation),
        ended[c.status],
      ]
        .filter(Boolean)
        .join(' · '),
    run: (r: RunEnd) => {
      const { trigger, result } = runSummary(r, targets, (id) => id)
      return `${automation(r.automation)} · ${trigger} → ${result}`
    },
    liveView: (v: LiveView) => ['Live view', byWhom(v.origin, identities, automation), watchedFor(Date.parse(v.end) - Date.parse(v.start))].join(' · '),
  }
}

// markers are the Commands and Runs loaded on the Targets of series, linked to
// their Run's Trace, the Live views and Recordings of the cameras of their
// Devices, and the Events of series: one each, or a bucket's count.
function markers(loaded: Loaded, recordings: Recording[], series: { ref: Ref; cap: Capability }[], describe: ReturnType<typeof useDescribe>): Marker[] {
  const trace = (automation: string, run: string) => `#automations/${automation}/${run}`
  const ofSeries = (t: Target) => series.some((s) => s.ref.target === t)
  return [
    ...loaded.commands
      .filter((c) => ofSeries(c.target))
      .map((c): Marker => {
        const run = runOf(c.origin)
        return { t: Date.parse(c.time), n: 1, kind: commandKind(c), text: describe.command(c), link: run && trace(run.automation, run.run) }
      }),
    ...loaded.runs
      .filter((r) => ofSeries(r.trigger.target))
      .map((r): Marker => ({ t: Date.parse(r.time), n: 1, kind: 'run', text: describe.run(r), link: trace(r.automation, r.run) })),
    ...loaded.liveViews
      .filter((v) => series.some((s) => owner(s.ref.target) === owner(v.target)))
      .map((v): Marker => ({ t: Date.parse(v.start), n: 1, kind: 'liveView', text: describe.liveView(v) })),
    ...recordings.map((r): Marker => ({ t: Date.parse(r.start), n: 1, kind: 'recording', text: describeRecording(r) })),
    ...series
      .filter(({ cap }) => cap.stateless)
      .flatMap(({ ref, cap }) =>
        (loaded.series.get(key(ref)) ?? []).map((p): Marker => {
          const counts = 'counts' in p ? Object.values(p.counts) : [1]
          const what =
            'counts' in p
              ? Object.entries(p.counts)
                  .map(([v, n]) => `${n} ${v}`)
                  .join(', ')
              : 'v' in p
                ? String(p.v)
                : ''
          return { t: p.t, n: counts.reduce((sum, n) => sum + n, 0), kind: 'event', text: `${cap.label}: ${what}` }
        }),
      ),
  ]
}

const bucketText: Record<string, string> = {
  '1m': '1 min',
  '5m': '5 min',
  '15m': '15 min',
  '1h': '1 h',
  '6h': '6 h',
  '1d': '1 day',
  '1w': '1 week',
  '1mo': '1 month',
}

// when tells t, as precisely as a range of len needs.
function when(t: number, len: number) {
  return new Date(t).toLocaleString(
    [],
    len > 2 * DAY
      ? { day: 'numeric', month: 'short', year: len > 300 * DAY ? 'numeric' : undefined }
      : { weekday: 'short', hour: '2-digit', minute: '2-digit' },
  )
}

// readAt tells what ps held at t: a Value, unknown within a blank; a
// bucket's mean and min–max, or share on; the last Event, and when.
export function readAt(ps: Point[], t: number, blank: Span[], cap: Capability) {
  const p = ps.findLast((p) => p.t <= t)
  if (cap.stateless) {
    if (!p) return 'none'
    const what =
      'counts' in p
        ? Object.entries(p.counts)
            .map(([v, n]) => `${n} ${v}`)
            .join(', ')
        : 'v' in p
          ? String(p.v)
          : ''
    return `${what} · ${new Date(p.t).toLocaleString([], { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })}`
  }
  if (blank.some(([s, e]) => t >= s && t < e)) return 'unknown'
  if (!p) return '—'
  if ('mean' in p) return `${format(p.mean, cap.unit)} (${format(p.min)}–${format(p.max)})`
  if ('on' in p) return `${Math.round(p.on * 100)} % on`
  return 'v' in p ? format(p.v, cap.unit) : '—'
}
