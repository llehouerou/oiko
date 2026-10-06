// The Timeline: what the whole home went through over a local day or week,
// one lane per Function with something to draw, grouped by kind, each group
// shown or hidden by its chip. A lane draws its on/off as bands, its reading
// as a curve over them, its Events as ticks and its Commands as small marks;
// offline time is hatched grey, Gaps red, and the span's future dimmed. It
// ends with what it sums up over the span, or its value at the cursor, which
// runs across every lane. ◀ ▶ or a swipe moves the span, the sheet's gestures
// zoom within it, and tapping a lane's name opens its Function's History
// sheet (a tap on its track places the cursor).
// Lanes follow the stream; summaries are read again every minute while the
// span includes now.

import { useEffect, useRef, useState, type RefObject } from 'react'
import { AppBar } from './AppBar'
import { CursorLine, SelectBox, Ticks, cursor, hatchGap, hatchOffline, markerStyle, useCursor, useTimeGestures, useWidth, MIN } from './charts'
import { format } from './controls'
import { commandKind } from './origin'
import {
  HistoryView,
  curve,
  listed,
  held,
  key,
  lane,
  laneGroup,
  laneGroups,
  loadPeriods,
  targetBlanks,
  timelineSpan,
  useHistory,
  type LaneGroup,
  type Loaded,
  type MeasureOption,
  type Period,
  type PeriodValue,
  type Span,
  type Spread,
  type TimelinePer,
} from './history'
import { HistorySheet, readAt } from './HistorySheet'
import { Sparkline } from './MiniChart'
import { duration, tell, total } from './Periods'
import { useCatalogue, useNow } from './store'
import type { Capability, Target } from './types'

type Lane = NonNullable<ReturnType<typeof lane>> & { target: Target; name: string; group: LaneGroup }

// The lanes of every Flag, Aggregate and Function with something to draw.
function useLanes(): Lane[] {
  return useCatalogue()
    .list.filter(listed)
    .flatMap(({ target, name, kind, capabilities }) => {
      const l = lane(target, kind, capabilities)
      return l ? [{ ...l, target, name, group: laneGroup(kind) }] : []
    })
}

export function Timeline() {
  const lanes = useLanes()
  const now = useNow()
  const [per, setPer] = useState<TimelinePer>('day')
  const [at, setAt] = useState<number | null>(null) // a time in the span shown; null: now, rolling over at midnight
  const span = timelineSpan(per, at ?? now)
  const [zoomed, setZoom] = useState<Span | null>(null)
  const zoom = zoomed && zoomed[0] >= span[0] && zoomed[1] <= span[1] ? zoomed : null // within the span shown
  const [hidden, setHidden] = useState<ReadonlySet<string>>(new Set())
  const [open, setOpen] = useState<Target | null>(null)
  const [from, to] = zoom ?? span
  const current = at === null

  // One call for every lane, per span, zoom or width; a zoom's gestures settle first.
  const [view] = useState(() => new HistoryView())
  const loaded = useHistory(view)
  // The span's past grows with the clock, and at once with a newer point.
  const latest = Math.max(now, ...[...loaded.series.values()].map((ps) => ps.at(-1)?.t ?? 0))
  const frame = useRef<HTMLDivElement>(null)
  const axis = useRef<HTMLDivElement>(null)
  const points = Math.max(60, Math.round(useWidth(axis)))
  const refs = lanes.flatMap((l) => [l.band, l.curve, l.events].flatMap((c) => (c ? [{ target: l.target, capability: c.key }] : [])))
  const refsKey = JSON.stringify(refs)
  useEffect(() => {
    if (!refs.length) return
    const t = setTimeout(() => void view.load(refs, from, to, points, false, true), 100)
    return () => clearTimeout(t)
  }, [view, refsKey, from, to, points]) // eslint-disable-line react-hooks/exhaustive-deps -- refs by value
  useEffect(() => () => view.close(), [view])
  useEffect(() => () => cursor.set(null), [])

  // One call for every summary, per span, read again every minute while the span includes now.
  const items = lanes.flatMap((l) => l.summary)
  const itemsKey = JSON.stringify([items.map((m) => [m.ref, m.measure]), span])
  const [summaries, setSummaries] = useState<{ key: string; periods: Map<string, Period> } | null>(null)
  useEffect(() => {
    if (!items.length) return
    let open = true
    const read = () =>
      loadPeriods(span[0], span[1], 'span', items).then(
        (a) => open && setSummaries({ key: itemsKey, periods: new Map(items.flatMap((m, i) => (a.items[i]?.[0] ? [[summaryKey(m), a.items[i][0]]] : []))) }),
        (e) => console.error('History:', e),
      )
    void read()
    const tick = current ? setInterval(read, 60_000) : undefined
    return () => {
      open = false
      clearInterval(tick)
    }
  }, [itemsKey]) // eslint-disable-line react-hooks/exhaustive-deps -- itemsKey holds the items and span
  const periods = summaries?.key === itemsKey ? summaries.periods : null

  // go shows the span of p t is in.
  const go = (p: TimelinePer, t: number) => {
    setPer(p)
    setAt(timelineSpan(p, t)[1] > Date.now() ? null : t)
    setZoom(null)
  }
  // step moves the span n back or forth, never past today's.
  const step = (n: number) => {
    const t = n < 0 ? span[0] - 1 : span[1]
    if (t <= Date.now()) go(per, t)
  }

  // Gestures zoom and pan within the span; a swipe across all of it moves it.
  const scale = () => axis.current!.getBoundingClientRect()
  const toT = (x: number) => {
    const r = scale()
    return from + ((x - r.left) / r.width) * (to - from)
  }
  const show = (f: number, t: number) => {
    const len = Math.min(span[1] - span[0], Math.max(10 * MIN, t - f))
    const start = Math.min(Math.max(f, span[0]), span[1] - len)
    setZoom(len < span[1] - span[0] ? [start, start + len] : null)
  }
  const selecting = useTimeGestures(
    frame,
    from,
    to,
    show,
    () => setZoom(null),
    () => {
      const f = frame.current!.getBoundingClientRect()
      const r = scale()
      return [r.left - f.left, f.right - r.right]
    },
  )
  useSwipe(frame, step, zoom === null)

  const label =
    per === 'day'
      ? current
        ? 'Today'
        : new Date(span[0]).toLocaleDateString([], { weekday: 'short', day: 'numeric', month: 'short' })
      : `${new Date(span[0]).toLocaleDateString([], { day: 'numeric', month: 'short' })} – ${new Date(span[1] - 1).toLocaleDateString([], { day: 'numeric', month: 'short' })}`
  const grouped = laneGroups.map((g) => [g.name, lanes.filter((l) => l.group === g)] as const).filter(([, ls]) => ls.length)
  const row = 'sm:grid sm:grid-cols-[12rem_1fr_11rem] sm:items-center sm:gap-3'
  const button = 'rounded bg-neutral-800 px-2 py-0.5 disabled:opacity-30'

  return (
    <>
      <AppBar page="#history" />
      <main className="mx-auto max-w-7xl p-4">
        <header className="mb-3 flex flex-wrap items-center gap-3">
          <div className="flex items-center gap-1 text-sm">
            <button onClick={() => step(-1)} aria-label="Earlier" className={button}>
              ◀
            </button>
            <span className="min-w-36 text-center">{label}</span>
            <button onClick={() => step(1)} disabled={current} aria-label="Later" className={button}>
              ▶
            </button>
            {!current && (
              <button onClick={() => go(per, Date.now())} className={`${button} text-xs`}>
                Today
              </button>
            )}
            <div className="ml-2 flex gap-1 rounded-lg bg-neutral-800 p-0.5">
              {(['day', 'week'] as const).map((p) => (
                <button
                  key={p}
                  onClick={() => go(p, at ?? Date.now())}
                  className={`rounded-md px-2 py-0.5 text-xs ${per === p ? 'bg-neutral-100 text-neutral-900' : 'text-neutral-400'}`}
                >
                  {p}
                </button>
              ))}
            </div>
          </div>
        </header>
        <div className="mb-3 flex flex-wrap gap-1">
          {grouped.map(([g]) => (
            <button
              key={g}
              onClick={() => {
                const next = new Set(hidden)
                if (!next.delete(g)) next.add(g)
                setHidden(next)
              }}
              aria-pressed={!hidden.has(g)}
              className={`rounded-full px-2 py-0.5 text-xs ${hidden.has(g) ? 'bg-neutral-900 text-neutral-600 line-through' : 'bg-neutral-800 text-neutral-300'}`}
            >
              {g}
            </button>
          ))}
        </div>
        <div
          ref={frame}
          className="relative select-none"
          style={{ touchAction: 'pan-y' }}
          onMouseMove={(e) => {
            const r = scale()
            cursor.set(e.clientX >= r.left && e.clientX <= r.right ? toT(e.clientX) : null)
          }}
          onMouseLeave={() => cursor.set(null)}
        >
          <div className={`${row} sticky top-[65px] z-10 bg-neutral-950 py-1`}> {/* under the AppBar: h-16 + its 1px border */}
            <div className="hidden sm:block" />
            <div ref={axis}>
              <Ticks from={from} to={to} />
            </div>
            <div className="hidden text-right text-xs text-neutral-400 sm:block">that {per}</div> {/* what the summaries sum up */}
          </div>
          {grouped
            .filter(([g]) => !hidden.has(g))
            .map(([g, ls]) => (
              <section key={g} className="mb-4">
                <h2 className="mb-1 text-xs font-semibold tracking-wide text-neutral-500 uppercase">{g}</h2>
                <div className="space-y-1.5">
                  {ls.map((l) => (
                    <div key={l.target} className={row}>
                      <div className="flex min-w-0 items-baseline justify-between gap-2 sm:block">
                        <button onClick={() => setOpen(l.target)} className="block max-w-full truncate text-left text-sm hover:underline">
                          {l.name}
                        </button>
                        <Summary lane={l} loaded={loaded} from={from} to={to} periods={periods} className="sm:hidden" />
                      </div>
                      <Track lane={l} loaded={loaded} from={from} to={to} now={latest} />
                      <Summary lane={l} loaded={loaded} from={from} to={to} periods={periods} className="hidden text-right sm:block" />
                    </div>
                  ))}
                </div>
              </section>
            ))}
          {selecting && <SelectBox select={selecting} from={from} to={to} />}
          {lanes.length === 0 && <p className="text-sm text-neutral-500">Nothing to draw yet.</p>}
        </div>
        <p className="mt-2 flex flex-wrap gap-3 text-[11px] text-neutral-500">
          {(['hand', 'program', 'automation', 'lost'] as const).map((k) => (
            <span key={k}>
              <span className="inline-block h-2 w-[3px] align-middle" style={{ background: markerStyle[k].color }} /> {markerStyle[k].label}
            </span>
          ))}
          <span>
            <span className="inline-block h-2 w-3 align-middle" style={{ background: hatchOffline }} /> offline
          </span>
          <span>
            <span className="inline-block h-2 w-3 align-middle" style={{ background: hatchGap }} /> Gap: Oiko recorded nothing
          </span>
          <span className="hidden sm:inline">Drag across the lanes to zoom, double-click to reset, ctrl+wheel to zoom, shift+wheel to pan.</span>
        </p>
        {open && <HistorySheet key={open} target={open} onClose={() => setOpen(null)} />}
      </main>
    </>
  )
}

const summaryKey = (m: MeasureOption) => `${key(m.ref)}|${m.measure}`

const series = (loaded: Loaded, target: Target, c: Capability | undefined) => (c ? (loaded.series.get(key({ target, capability: c.key })) ?? []) : [])

// Track draws a lane over [from, to], up to now.
function Track({ lane: l, loaded, from, to, now }: { lane: Lane; loaded: Loaded; from: number; to: number; now: number }) {
  const end = Math.max(from, Math.min(to, now))
  const blank = targetBlanks(loaded, l.target, from, end)
  const at = (t: number) => `${((t - from) / (to - from)) * 100}%`
  const width = (s: number, e: number) => `${((e - s) / (to - from)) * 100}%`
  const color = l.group.color
  const hatched = (spans: Span[], background: string) =>
    spans.map(([s, e]) => <div key={s} className="absolute inset-y-0" style={{ left: at(s), width: width(s, e), minWidth: 2, background }} />)
  return (
    <div className="relative h-5 overflow-hidden rounded-sm bg-neutral-800/50">
      {l.band &&
        held(series(loaded, l.target, l.band), from, end, blank.all).map(({ from: s, to: e, p }) => {
          const on = 'on' in p ? p.on : 'v' in p && p.v === true ? 1 : 0 // a contact is on when open
          return (
            on > 0 && (
              <div
                key={s}
                className="absolute inset-y-0"
                style={{ left: at(s), width: width(s, e), minWidth: 1, opacity: on * (l.curve ? 0.35 : 0.9), background: color }}
              />
            )
          )
        })}
      {hatched(blank.offline, hatchOffline)}
      {hatched(blank.gaps, hatchGap)}
      {l.curve && (
        <div className="absolute inset-0">
          <Sparkline data={curve(series(loaded, l.target, l.curve), from, end, blank.all)} from={from} to={to} stroke="#e5e5e5" />
        </div>
      )}
      {series(loaded, l.target, l.events)
        .filter((p) => p.t >= from && p.t <= to)
        .map((p, i) => (
          <span key={i} className="absolute inset-y-[15%] w-0.5 -translate-x-1/2 rounded bg-emerald-400" style={{ left: at(p.t) }} />
        ))}
      {loaded.commands
        .filter((c) => c.target === l.target)
        .map((c) => (
          <span
            key={c.id}
            className="absolute bottom-0 h-1.5 w-[3px] -translate-x-1/2"
            style={{ left: at(Date.parse(c.time)), background: markerStyle[commandKind(c)].color }}
          />
        ))}
      {to > end && <div className="absolute inset-y-0 right-0 bg-neutral-950/70" style={{ left: at(end) }} />}
      <CursorLine from={from} to={to} />
    </div>
  )
}

// Summary tells what a lane sums up over the span, marked when a Gap, offline
// or unknown time is part of it; or its value at the cursor.
function Summary({
  lane: l,
  loaded,
  from,
  to,
  periods,
  className,
}: {
  lane: Lane
  loaded: Loaded
  from: number
  to: number
  periods: Map<string, Period> | null
  className: string
}) {
  const t = useCursor()
  let text: string
  let incomplete = false
  if (t !== null && t >= from && t <= to) {
    const blank = targetBlanks(loaded, l.target, from, to).all
    text = [l.band, l.curve, l.events].flatMap((c) => (c ? [readAt(series(loaded, l.target, c), t, blank, c)] : [])).join(' · ')
  } else if (periods) {
    const ps = l.summary.map((m) => periods.get(summaryKey(m)))
    text = l.summary.map((m, i) => brief(m, ps[i]?.value ?? null)).join(' · ')
    incomplete = ps.some((p) => p?.incomplete)
  } else text = '…'
  return (
    <span className={`truncate text-xs text-neutral-400 tabular-nums ${className}`}>
      {text}
      {incomplete && (
        <span title="Incomplete: a Gap, offline or unknown time" className="text-red-400">
          {' '}
          !
        </span>
      )}
    </span>
  )
}

// brief tells a span's value of m: time on, openings, presses, energy, min–max.
function brief(m: MeasureOption, v: PeriodValue) {
  if (v === null) return '—'
  switch (m.measure) {
    case 'time_on':
      return `${duration(v as number)} on`
    case 'stats':
      return `${format((v as Spread).min)}–${format((v as Spread).max, m.cap.unit)}`
    case 'count':
      return `${typeof v === 'number' ? v : total(v as Record<string, number>)} ${m.label.toLowerCase()}`
  }
  return tell(m, v)
}

// useSwipe moves the span on a sideways swipe of one finger, while active.
function useSwipe(el: RefObject<HTMLElement | null>, step: (n: number) => void, active: boolean) {
  const latest = useRef({ step, active })
  useEffect(() => {
    latest.current = { step, active }
  })
  useEffect(() => {
    const node = el.current!
    const touches = new Set<number>()
    let start: { x: number; y: number } | null = null
    const down = (e: PointerEvent) => {
      if (e.pointerType !== 'touch') return
      touches.add(e.pointerId)
      start = touches.size === 1 && latest.current.active ? { x: e.clientX, y: e.clientY } : null // a pinch is no swipe
    }
    const up = (e: PointerEvent) => {
      if (!touches.delete(e.pointerId) || !start) return
      const dx = e.clientX - start.x
      const dy = e.clientY - start.y
      start = null
      if (e.type === 'pointerup' && Math.abs(dx) > 60 && Math.abs(dx) > 2 * Math.abs(dy)) latest.current.step(dx > 0 ? -1 : 1)
    }
    node.addEventListener('pointerdown', down)
    node.addEventListener('pointerup', up)
    node.addEventListener('pointercancel', up)
    return () => {
      node.removeEventListener('pointerdown', down)
      node.removeEventListener('pointerup', up)
      node.removeEventListener('pointercancel', up)
    }
  }, [el])
}
