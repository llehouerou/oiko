// A tile's last 24 h, under its Function's controls: a sensor's reading as a
// sparkline with its min–max, an on/off as a state band, Events as ticks with
// their count. Offline time is hatched grey and Gaps red; nothing is drawn
// across either. Tapping it opens the Function's History sheet. ChartsShown
// hides every one beneath it; a Guest, who reads no History, sees none.

import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import uPlot from 'uplot'
import 'uplot/dist/uPlot.min.css'
import { hatchGap, hatchOffline, stepped } from './charts'
import { format } from './controls'
import { DAY, blanks, curve, held, useGaps, useSeries, type Raw, type Span } from './history'
import { HistorySheet } from './HistorySheet'
import { useNow } from './store'
import { presses, roles } from './roles'
import { owner } from './targets'
import type { Capability, Fn, Target } from './types'
import { useAllows } from './access'

const color = '#fbbf24'

export const ChartsShown = createContext(true)

// It charts the Function's charted Role, the series the tiles load.
export function MiniChart({ target, fn }: { target: Target; fn: Fn }) {
  const shown = useContext(ChartsShown)
  const past = useAllows('member')
  const cap = roles(fn.kind, fn.capabilities).charted
  const [open, setOpen] = useState(false)
  if (!cap || !shown || !past) return null
  return (
    <>
      <button onClick={() => setOpen(true)} aria-label="History" className="-m-1 block w-[calc(100%+0.5rem)] rounded-md p-1 text-left hover:bg-neutral-800">
        <Chart target={target} cap={cap} />
      </button>
      {/* outside the tile, which an offline Device dims */}
      {open && createPortal(<HistorySheet target={target} onClose={() => setOpen(false)} />, document.body)}
    </>
  )
}

function Chart({ target, cap }: { target: Target; cap: Capability }) {
  const ps = useSeries({ target, capability: cap.key })
  const availability = (useSeries({ target: owner(target), capability: '' }) ?? []) as Raw[] // never bucketed
  const gaps = useGaps()
  const now = useNow()
  // The window slides with the clock, and at once with a newer point.
  const to = Math.max(now, ps?.at(-1)?.t ?? 0, availability.at(-1)?.t ?? 0)
  const from = to - DAY
  const blank = blanks(availability, gaps, from, to)
  const at = (t: number) => `${((t - from) / DAY) * 100}%`
  // Ticks and band pieces hang left of their end, so the latest stays in sight at the right edge.
  const before = (t: number) => `${((to - t) / DAY) * 100}%`
  const width = (s: number, e: number) => `${((e - s) / DAY) * 100}%`
  const hatch = (spans: Span[], background: string) =>
    spans.map(([s, e]) => <div key={s} className="pointer-events-none absolute inset-y-0" style={{ left: at(s), width: width(s, e), background }} />)

  let chart: ReactNode = null
  let caption = '24 h'
  if (ps && cap.stateless) {
    const shown = ps.filter((p) => p.t >= from && p.t <= to)
    const n = shown.reduce((sum, p) => sum + ('counts' in p ? Object.values(p.counts).reduce((a, c) => a + c, 0) : 1), 0)
    const [one, many] = presses(cap) ? ['press', 'presses'] : ['event', 'events']
    caption += ` · ${n} ${n === 1 ? one : many}`
    chart = shown.map((p, i) => <span key={i} className="absolute inset-y-0 w-0.5 bg-emerald-400" style={{ right: before(p.t) }} />)
  } else if (ps && cap.type === 'binary') {
    chart = held(ps, from, to, blank.all).map(({ from: s, to: e, p }) => {
      const on = 'on' in p ? p.on : 'v' in p && p.v === true ? 1 : 0 // a contact is on when open
      return (
        on > 0 && <div key={s} className="absolute inset-y-0" style={{ right: before(e), width: width(s, e), minWidth: 1, opacity: on, background: color }} />
      )
    })
  } else if (ps) {
    const [xs, lows, highs, means] = curve(ps, from, to, blank.all)
    const known = (vs: (number | null)[]) => vs.filter((v) => v !== null)
    if (known(lows).length) caption += ` · ${format(Math.min(...known(lows)))}–${format(Math.max(...known(highs)), cap.unit)}`
    chart = <Sparkline data={[xs, lows, highs, means]} from={from} to={to} />
  }

  const strip = !cap.stateless && cap.type !== 'binary' ? 'h-7' : 'h-2 rounded-sm bg-neutral-800/60'
  return (
    <div>
      <div className={`relative overflow-hidden ${strip}`}>
        {chart}
        {hatch(blank.offline, hatchOffline)}
        {hatch(blank.gaps, hatchGap)}
      </div>
      <p className="mt-0.5 text-[10px] text-neutral-500">{caption}</p>
    </div>
  )
}

// A step line over the band of its buckets' min–max, filling its parent.

export function Sparkline({ data, from, to, stroke = color }: { data: uPlot.AlignedData; from: number; to: number; stroke?: string }) {
  const el = useRef<HTMLDivElement>(null)
  const plot = useRef<uPlot | null>(null)
  const range = useRef<uPlot.Range.MinMax>([from, to])
  useEffect(() => {
    const node = el.current!
    const line = { paths: stepped, points: { show: false } }
    const u = new uPlot(
      {
        width: node.clientWidth,
        height: node.clientHeight,
        padding: [2, 0, 2, 0],
        legend: { show: false },
        cursor: { show: false },
        scales: {
          x: { time: false, range: () => range.current },
          y: { range: (_u, min, max) => (min === max ? [min - 1, max + 1] : [min, max]) },
        },
        axes: [{ show: false }, { show: false }],
        series: [{}, { ...line, stroke: 'transparent' }, { ...line, stroke: 'transparent' }, { ...line, stroke, width: 1.5 }],
        bands: [{ series: [2, 1], fill: `${stroke}33` }],
      },
      [[], [], [], []],
      node,
    )
    plot.current = u
    const resized = new ResizeObserver(() => u.setSize({ width: node.clientWidth, height: node.clientHeight }))
    resized.observe(node)
    return () => {
      resized.disconnect()
      u.destroy()
    }
    // oxlint-disable-next-line react-hooks/exhaustive-deps -- made once: a Sparkline's stroke never changes while it shows
  }, [])
  useEffect(() => {
    range.current = [from, to]
    plot.current?.setData(data)
  }, [data, from, to])
  return <div ref={el} className="absolute inset-0" />
}
