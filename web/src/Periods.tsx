// The History sheet's Per period mode: what a measure of a Function sums up
// in each of its local hours, days, weeks or months, as Oiko computes it,
// the latest ones or, with ◀ ▶, earlier ones back to the start of its
// History. A period with a Gap, offline or unknown time is dimmed and marked;
// the current one is drawn so far, against the previous period at equal
// elapsed time, whose total is told alongside. While shown, the current
// period is read again every 10 s. Tapping a period opens it as a curve.

import { useEffect, useState } from 'react'
import { format } from './controls'
import { loadPeriods, periodsShown, periodsSpan, refreshed, shift, type MeasureOption, type Per, type Period, type PeriodValue, type Spread } from './history'

const pers: Per[] = ['hour', 'day', 'week', 'month']
const color = '#fbbf24'
const partColors = ['#34d399', '#38bdf8', '#a78bfa', '#f472b6', '#fb923c', '#facc15']
const soFar = 'repeating-linear-gradient(135deg, #000 0 3px, #0006 3px 6px)' // a mask striping the current period

// Periods keeps its measure, per and window while hidden (active false),
// reading nothing meanwhile.
export function Periods({ options, active, onOpen }: { options: MeasureOption[]; active: boolean; onOpen: (from: number, to: number) => void }) {
  const [picked, setPicked] = useState(0)
  const [per, setPer] = useState<Per>('day')
  const [at, setAt] = useState<number | null>(null) // a time in the last period shown; null: now
  const [data, setData] = useState<{ key: string; end: number; first: number | null; periods: Period[] } | null>(null)
  const [hover, setHover] = useState<number | null>(null)
  const m = options[Math.min(picked, options.length - 1)]!
  const key = JSON.stringify([m.ref, m.measure, per, at])

  useEffect(() => {
    if (!active) return
    const item = { ref: m.ref, measure: m.measure }
    let shown: typeof data = null
    let open = true
    const show = (d: typeof data) => {
      shown = d
      if (open) setData(d)
    }
    const failed = (e: unknown) => console.error('History:', e)
    const { from, to } = periodsSpan(per, at, Date.now())
    const periods = (from: number, to: number) => loadPeriods(from, to, per, [item]).then(({ end, first, items }) => ({ end, first, periods: items[0]! }))
    periods(from, to).then((d) => show({ key, ...d }), failed)
    if (at !== null) return () => void (open = false) // a past window holds still
    const tick = setInterval(() => {
      const last = shown?.periods.at(-1)
      if (!last) return
      periods(last.start, Date.now()).then((d) => shown && show({ ...shown, ...d, periods: refreshed(shown.periods, d.periods) }), failed)
    }, 10_000)
    return () => {
      open = false
      clearInterval(tick)
    }
  }, [key, active]) // eslint-disable-line react-hooks/exhaustive-deps -- key holds the item, per and window

  // Picking another measure or per returns to now.
  const pick = (i: number, p: Per) => {
    setPicked(i)
    setPer(p)
    setAt(null)
    setHover(null)
  }
  const page = (to: number | null) => {
    setAt(to)
    setHover(null)
  }
  const loaded = data?.key === key ? data : null
  const ps = loaded?.periods ?? []
  const end = (k: number) => ps[k + 1]?.start ?? loaded?.end ?? 0
  const atStart = !loaded || loaded.first === null || (ps[0] !== undefined && ps[0].start <= loaded.first)
  const stats = m.measure === 'stats'
  const events = !!m.cap.stateless
  // reach is how high a value draws.
  const reach = (v: PeriodValue) => (v === null ? 0 : stats ? (v as Spread).max : events ? total(v as Counts) : (v as number))
  const values = ps.flatMap((p) => [p.value, ...(p.previous ? [p.previous.value] : [])]).filter((v) => v !== null)
  const top = Math.max(1e-9, ...values.map(reach))
  const bottom = stats ? Math.min(top, ...values.map((v) => (v as Spread).min)) : 0
  const y = (v: number) => `${((v - bottom) / (top - bottom || 1)) * 100}%`
  const text = (v: PeriodValue | undefined) => tell(m, v)
  // Events take a colour each, in the order of their values shown.
  const kinds = events ? [...new Set(values.flatMap((v) => Object.keys(v as Counts)))].sort() : []
  const tint = (value: string) => partColors[kinds.indexOf(value) % partColors.length]
  const every = Math.ceil(ps.length / 12)
  const shown = hover ?? (ps.length ? ps.length - 1 : null)
  const p = shown === null ? undefined : ps[shown]
  const button = 'rounded px-2 py-0.5 text-xs'

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        {options.length > 1 && (
          <select value={picked} onChange={(e) => pick(Number(e.target.value), per)} className="rounded bg-neutral-800 px-2 py-0.5 text-xs">
            {options.map((o, i) => (
              <option key={i} value={i}>
                {o.label}
              </option>
            ))}
          </select>
        )}
        {options.length === 1 && <span className="text-xs text-neutral-300">{m.label}</span>}
        {kinds.map((value) => (
          <span key={value} className="flex items-center gap-1 text-xs text-neutral-400">
            <span className="size-2 rounded-full" style={{ background: tint(value) }} />
            {value}
          </span>
        ))}
        <div className="flex gap-1">
          {pers.map((x) => (
            <button
              key={x}
              onClick={() => pick(picked, x)}
              className={`${button} ${x === per ? 'bg-neutral-100 text-neutral-900' : 'bg-neutral-800 text-neutral-300 hover:bg-neutral-700'}`}
            >
              {x}
            </button>
          ))}
        </div>
        <button
          onClick={() => page((ps[0]?.start ?? 0) - 1)}
          disabled={atStart}
          aria-label="Earlier"
          className={`${button} bg-neutral-800 disabled:opacity-30`}
        >
          ◀
        </button>
        <button
          onClick={() => {
            const next = shift(per, loaded?.end ?? Date.now(), periodsShown[per] - 1)
            page(next >= Date.now() ? null : next)
          }}
          disabled={at === null || !loaded}
          aria-label="Later"
          className={`${button} bg-neutral-800 disabled:opacity-30`}
        >
          ▶
        </button>
      </div>
      <div className="flex h-72 items-end gap-[2px]" onMouseLeave={() => setHover(null)}>
        {ps.map((p, k) => {
          const v = p.value
          const before = p.previous?.value ?? null // the previous period's at the same time, if p is current
          const current = p.previous !== undefined
          return (
            <button
              key={p.start}
              onClick={() => onOpen(p.start, end(k))}
              onMouseEnter={() => setHover(k)}
              title={`${when(p.start, per)}: ${text(v)}`}
              className={`relative h-full min-w-0 flex-1 rounded-t-sm ${k === shown ? 'bg-neutral-800' : 'hover:bg-neutral-800/60'}`}
            >
              <div className="absolute inset-0" style={{ opacity: p.incomplete ? 0.45 : 1, maskImage: current ? soFar : undefined }}>
                {v === null ? null : stats ? (
                  <>
                    <div
                      className="absolute inset-x-[20%] rounded-sm"
                      style={{ bottom: y((v as Spread).min), top: `calc(100% - ${y((v as Spread).max)})`, background: `${color}55` }}
                    />
                    <div className="absolute inset-x-[10%] h-0.5" style={{ bottom: y((v as Spread).mean), background: color }} />
                  </>
                ) : events ? (
                  <div className="absolute inset-x-[12%] bottom-0 flex flex-col-reverse" style={{ height: y(reach(v)) }}>
                    {Object.entries(v as Counts).map(([value, n]) => (
                      <div key={value} style={{ flex: n, background: tint(value) }} />
                    ))}
                  </div>
                ) : (
                  <div className="absolute inset-x-[12%] bottom-0 rounded-t-sm" style={{ height: y(reach(v)), background: color }} />
                )}
              </div>
              {before !== null &&
                (stats ? (
                  <div className="absolute inset-x-[4%] border-t border-dashed border-neutral-300/70" style={{ bottom: y((before as Spread).mean) }} />
                ) : (
                  <div
                    className="absolute inset-x-[4%] bottom-0 rounded-t-sm border border-dashed border-neutral-300/70"
                    style={{ height: y(reach(before)) }}
                  />
                ))}
              {p.incomplete && <span className="absolute top-0 left-1/2 -translate-x-1/2 text-[10px] text-red-400">!</span>}
            </button>
          )
        })}
      </div>
      <div className="flex gap-[2px] text-[10px] text-neutral-500">
        {ps.map((p, k) => (
          <span key={p.start} className="min-w-0 flex-1 overflow-hidden text-center whitespace-nowrap">
            {k % every === 0 ? when(p.start, per, true) : ''}
          </span>
        ))}
      </div>
      <p className="min-h-8 text-sm">
        {!loaded ? (
          <span className="text-neutral-500">Loading…</span>
        ) : (
          p && (
            <>
              <span className="text-neutral-400">{when(p.start, per)}</span> <span className="font-medium">{text(p.value)}</span>
              {p.previous && (
                <span className="text-neutral-400">
                  {' '}
                  so far · the previous {per} at the same time: {text(p.previous.value)}, in all: {text(p.previous.total)}
                </span>
              )}
              {p.incomplete && <span className="text-red-400"> · incomplete: a Gap, offline or unknown time</span>}
            </>
          )
        )}
      </p>
      <p className="flex flex-wrap gap-3 text-[11px] text-neutral-500">
        <span>
          <span className="inline-block h-2 w-3 align-middle" style={{ background: color, maskImage: soFar }} /> so far
        </span>
        <span>
          <span className="inline-block h-2 w-3 border border-dashed border-neutral-300/70 align-middle" /> previous at the same time
        </span>
        <span>
          <span className="text-red-400">!</span> incomplete
        </span>
        <span>Tap a period to open it as a curve.</span>
      </p>
    </div>
  )
}

type Counts = Record<string, number>

export const total = (v: Counts) => Object.values(v).reduce((a, b) => a + b, 0)

// tell tells a period's value of m.
export function tell(m: MeasureOption, v: PeriodValue | undefined) {
  if (v === null || v === undefined) return '—'
  if (m.measure === 'stats') {
    const s = v as Spread
    return `${format(s.mean, m.cap.unit)} (${format(s.min)}–${format(s.max)})`
  }
  if (m.measure === 'time_on') return duration(v as number)
  if (m.measure === 'energy') {
    const e = v as number
    return m.cap.unit === 'kWh' && e < 1 ? format(e * 1000, 'Wh') : format(e, m.cap.unit)
  }
  if (!m.cap.stateless) return String(v)
  const parts = Object.entries(v as Counts).map(([value, n]) => `${n} ${value}`)
  return parts.length > 1 ? `${total(v as Counts)} (${parts.join(', ')})` : (parts[0] ?? '0')
}

export function duration(ms: number) {
  const m = Math.round(ms / 60_000)
  return m < 60 ? `${m} min` : `${Math.floor(m / 60)} h ${String(m % 60).padStart(2, '0')}`
}

// when tells the period starting at t, shortly under its bar.
function when(t: number, per: Per, short = false) {
  const d = new Date(t)
  switch (per) {
    case 'hour':
      return short
        ? d.toLocaleTimeString([], { hour: '2-digit' })
        : d.toLocaleString([], { weekday: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
    case 'day':
      return d.toLocaleDateString([], short ? { weekday: 'narrow', day: 'numeric' } : { weekday: 'long', day: 'numeric', month: 'long' })
    case 'week':
      return short ? d.toLocaleDateString([], { day: 'numeric', month: 'short' }) : `Week of ${d.toLocaleDateString([], { day: 'numeric', month: 'long' })}`
    case 'month':
      return d.toLocaleDateString([], short ? { month: 'short' } : { month: 'long', year: 'numeric' })
  }
}
