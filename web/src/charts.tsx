// What History charts draw with, on one time scale [from, to]: curves with
// uPlot, state bands, Event ticks and markers as plain divs, a cursor every
// chart of a view follows, and the gestures that move the range. Offline time
// is hatched grey and Gaps red.

import { useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore, type ReactNode, type RefObject } from 'react'
import { createPortal } from 'react-dom'
import uPlot from 'uplot'
import 'uplot/dist/uPlot.min.css'
import { format } from './controls'
import { DAY, held, type Blanks, type Curve, type Marker, type MarkerKind, type Point, type Span } from './history'
import type { Capability } from './types'

export const MIN = 60 * 1000
export const HOUR = 60 * MIN
export const PAD_L = 48 // the curves' y axis: bands are indented as much to share the time scale
export const PAD_R = 12

const offlineColor = 'rgba(163,163,163,.35)'
const gapColor = 'rgba(248,113,113,.5)'
export const hatchOffline = `repeating-linear-gradient(135deg, ${offlineColor} 0 2px, transparent 2px 6px)`
export const hatchGap = `repeating-linear-gradient(135deg, ${gapColor} 0 2px, transparent 2px 6px)`
const enumColors = ['#38bdf8', '#a78bfa', '#34d399', '#f472b6', '#fb923c', '#facc15']

// A step line from each point to the next, never interpolated.
export const stepped = uPlot.paths.stepped!({ align: 1, alignGaps: 1 })

// pct is t's place within [from, to], as a CSS length.
const pct = (t: number, from: number, to: number) => `${((t - from) / (to - from)) * 100}%`

// The cursor: a time every chart follows; src is the uPlot that set it.
let cursorAt: number | null = null
let cursorSrc: uPlot | null = null
const cursorListeners = new Set<() => void>()
export const cursor = {
  set(t: number | null, src: uPlot | null = null) {
    cursorAt = t
    cursorSrc = src
    cursorListeners.forEach((l) => l())
  },
  subscribe(l: () => void) {
    cursorListeners.add(l)
    return () => void cursorListeners.delete(l)
  },
}
export const useCursor = () => useSyncExternalStore(cursor.subscribe, () => cursorAt)

export function useWidth(el: RefObject<HTMLElement | null>) {
  const [w, setW] = useState(600)
  useLayoutEffect(() => {
    const node = el.current!
    const ro = new ResizeObserver(() => node.clientWidth && setW(node.clientWidth)) // not while hidden
    ro.observe(node)
    return () => ro.disconnect()
  }, [el])
  return w
}

// A span being dragged across: its ends a and b, the inset of the time scale
// within el, and the pointer's height in el.
export interface Selecting {
  a: number
  b: number
  inset: [left: number, right: number]
  y: number
}

// useTimeGestures moves the range of the charts within el through show:
// ctrl+wheel (a trackpad's pinch) zooms, shift+wheel or a sideways swipe
// pans, a double-click resets, a mouse dragging from anywhere over the time
// scale but a control, a curve (it selects its own) or [data-no-drag]
// selects a span (held by el till released, and within the time scale); on
// touch, one finger pans, two pinch, a tap places the cursor. inset tells how
// far within el the time scale starts and ends. It returns the span being
// selected, for a SelectBox.
export function useTimeGestures(
  el: RefObject<HTMLElement | null>,
  from: number,
  to: number,
  show: (f: number, t: number) => void,
  reset: () => void,
  inset: () => [left: number, right: number] = () => [PAD_L, PAD_R],
) {
  const [selecting, setSelecting] = useState<Selecting | null>(null)
  const latest = useRef({ from, to, show, reset, inset })
  useEffect(() => {
    latest.current = { from, to, show, reset, inset }
  })
  useEffect(() => {
    const node = el.current!
    const perPx = () => {
      const [l, r] = latest.current.inset()
      return (latest.current.to - latest.current.from) / (node.clientWidth - l - r)
    }
    const toT = (clientX: number) => latest.current.from + (clientX - node.getBoundingClientRect().left - latest.current.inset()[0]) * perPx()
    const zoom = (at: number, k: number) => {
      const { from, to, show } = latest.current
      show(at - (at - from) * k, at + (to - at) * k)
    }
    const pan = (d: number) => latest.current.show(latest.current.from + d, latest.current.to + d)
    const onWheel = (e: WheelEvent) => {
      if (e.ctrlKey) {
        e.preventDefault()
        zoom(toT(e.clientX), Math.exp(e.deltaY * 0.01))
      } else if (e.shiftKey || Math.abs(e.deltaX) > Math.abs(e.deltaY)) {
        e.preventDefault()
        pan((e.shiftKey ? e.deltaY || e.deltaX : e.deltaX) * perPx())
      }
    }
    const touches = new Map<number, number>()
    let moved = false
    let downAt = 0
    let drag: { a: number; inset: [number, number] } | null = null
    const within = (clientX: number) => Math.min(Math.max(toT(clientX), latest.current.from), latest.current.to)
    const y = (e: PointerEvent) => e.clientY - node.getBoundingClientRect().top
    const onDown = (e: PointerEvent) => {
      if (e.pointerType === 'mouse') {
        const target = e.target as Element
        const t = toT(e.clientX)
        const { from, to } = latest.current
        if (e.button !== 0 || t < from || t > to || target.closest('button, a, input, select, .uplot, [data-no-drag]')) return
        node.setPointerCapture(e.pointerId)
        drag = { a: within(e.clientX), inset: latest.current.inset() }
        setSelecting({ ...drag, b: drag.a, y: y(e) })
      }
      if (e.pointerType !== 'touch') return
      touches.set(e.pointerId, e.clientX)
      moved = false
      downAt = e.timeStamp
    }
    const onMove = (e: PointerEvent) => {
      if (drag) {
        const b = within(e.clientX)
        cursor.set(b)
        setSelecting({ ...drag, b, y: y(e) })
      }
      const before = touches.get(e.pointerId)
      if (before === undefined) return
      if (touches.size === 1) {
        if (Math.abs(e.clientX - before) <= 3 && !moved) return
        moved = true
        pan((before - e.clientX) * perPx())
      } else if (touches.size === 2) {
        moved = true
        const other = [...touches].find(([id]) => id !== e.pointerId)![1]
        zoom(toT((e.clientX + other) / 2), Math.abs(before - other) / Math.max(20, Math.abs(e.clientX - other)))
      }
      touches.set(e.pointerId, e.clientX)
    }
    const onUp = (e: PointerEvent) => {
      if (drag && e.pointerType === 'mouse') {
        const [a, b] = [drag.a, within(e.clientX)].sort((p, q) => p - q) as [number, number]
        drag = null
        setSelecting(null)
        if (e.type === 'pointerup' && b - a > 4 * perPx()) latest.current.show(a, b)
      }
      if (!touches.delete(e.pointerId)) return
      if (!moved && e.timeStamp - downAt < 300) cursor.set(toT(e.clientX))
    }
    const onDouble = () => latest.current.reset()
    node.addEventListener('wheel', onWheel, { passive: false })
    node.addEventListener('pointerdown', onDown)
    node.addEventListener('pointermove', onMove)
    node.addEventListener('pointerup', onUp)
    node.addEventListener('pointercancel', onUp)
    node.addEventListener('dblclick', onDouble)
    return () => {
      node.removeEventListener('wheel', onWheel)
      node.removeEventListener('pointerdown', onDown)
      node.removeEventListener('pointermove', onMove)
      node.removeEventListener('pointerup', onUp)
      node.removeEventListener('pointercancel', onUp)
      node.removeEventListener('dblclick', onDouble)
    }
  }, [el])
  return selecting
}

const pill =
  'pointer-events-none absolute -translate-x-1/2 rounded bg-amber-400 px-1.5 py-0.5 text-xs font-medium whitespace-nowrap text-neutral-900 tabular-nums'

// SelectBox shows a span being selected across its parent, which holds the
// time scale [from, to] within its inset, and its ends by the pointer.
export function SelectBox({ select: { a, b, inset, y }, from, to }: { select: Selecting; from: number; to: number }) {
  const [s, e] = [Math.min(a, b), Math.max(a, b)]
  return (
    <div className="pointer-events-none absolute inset-y-0" style={{ left: inset[0], right: inset[1] }}>
      <div
        className="absolute inset-y-0 bg-amber-400/15 shadow-[inset_1px_0_#fbbf24,inset_-1px_0_#fbbf24]"
        style={{ left: pct(s, from, to), width: `${((e - s) / (to - from)) * 100}%` }}
      />
      <div className={pill} style={{ top: y + 16, left: `clamp(80px, ${pct((s + e) / 2, from, to)}, calc(100% - 80px))` }}>
        {stamp(s)} → {stamp(e)}
      </div>
    </div>
  )
}

// An 8 px hatch tile per colour, for canvas fills.
const tiles = new Map<string, HTMLCanvasElement>()
function hatchTile(color: string) {
  let c = tiles.get(color)
  if (!c) {
    c = document.createElement('canvas')
    c.width = c.height = 8
    const g = c.getContext('2d')!
    g.strokeStyle = color
    g.lineWidth = 2
    g.beginPath()
    g.moveTo(0, 8)
    g.lineTo(8, 0)
    g.stroke()
    tiles.set(color, c)
  }
  return c
}

export interface Line {
  label: string
  color: string
  data: Curve
  read: (t: number) => string // what it held at t, as its bubble tells it
  dashed?: boolean // the previous period, shifted onto this one: its mean only
}

// A dotted line across the curves, where markers are.
export interface Mark {
  t: number
  color: string
}

// CurvePanel draws lines of one unit as step lines over the band of their
// buckets' min–max (a dashed one without), blanks hatched under them, marks
// dotted across. Dragging across it selects a span to show: the pointer is
// held by the panel till released, and the span stops at its edges. Its
// lines are fixed: key it by them.
export function CurvePanel({
  title,
  lines,
  marks,
  from,
  to,
  blank,
  height,
  onSelect,
}: {
  title: ReactNode
  lines: Line[]
  marks: Mark[]
  from: number
  to: number
  blank: Blanks
  height: number
  onSelect: (f: number, t: number) => void
}) {
  const el = useRef<HTMLDivElement>(null)
  const [plot, setPlot] = useState<uPlot | null>(null)
  const [drawn, setDrawn] = useState(0) // redraws the overlay with the plot
  const live = useRef({ from, to, blank, marks, onSelect })
  useEffect(() => {
    const node = el.current!
    const axis = { stroke: '#a3a3a3', grid: { stroke: '#262626', width: 1 }, ticks: { stroke: '#262626', width: 1 }, font: '11px system-ui' }
    const line = { paths: stepped, points: { show: false } }
    const drawn = () => setDrawn((n) => n + 1)
    let held = false
    const u = new uPlot(
      {
        width: node.clientWidth,
        height,
        ms: 1,
        padding: [8, PAD_R, 0, 0],
        legend: { show: false },
        cursor: {
          drag: { x: true, y: false, setScale: false },
          points: { show: false },
          y: false,
          move: (u, l, t) => (held ? [Math.min(Math.max(l, 0), u.over.clientWidth), t] : [l, t]),
        },
        scales: {
          x: { time: true, range: () => [live.current.from, live.current.to] },
          y: {
            range: (_u, min, max) => {
              if (min == null || max == null) return [0, 1]
              const pad = Math.max((max - min) * 0.08, Math.abs(max) * 0.01, 0.5)
              return [min - pad, max + pad]
            },
          },
        },
        axes: [
          { ...axis, size: 28 },
          { ...axis, size: PAD_L, values: (_u, vs) => vs.map((v) => format(v)) },
        ],
        series: [
          {},
          ...lines.flatMap((l) => [
            { ...line, stroke: 'transparent' },
            { ...line, stroke: 'transparent' },
            { ...line, stroke: l.color, width: l.dashed ? 1.2 : 1.6, dash: l.dashed ? [4, 4] : undefined },
          ]),
        ],
        bands: lines.flatMap((l, i): uPlot.Band[] => (l.dashed ? [] : [{ series: [2 + 3 * i, 1 + 3 * i], fill: `${l.color}33` }])),
        hooks: {
          drawClear: [
            (u) => {
              const { offline, gaps } = live.current.blank
              for (const [spans, color] of [
                [offline, offlineColor],
                [gaps, gapColor],
              ] as const) {
                u.ctx.fillStyle = u.ctx.createPattern(hatchTile(color), 'repeat')!
                for (const [s, e] of spans) {
                  const x0 = u.valToPos(s, 'x', true)
                  u.ctx.fillRect(x0, u.bbox.top, Math.max(1, u.valToPos(e, 'x', true) - x0), u.bbox.height)
                }
              }
            },
          ],
          draw: [
            (u) => {
              const { ctx, bbox } = u
              ctx.save()
              ctx.setLineDash([2, 3])
              for (const m of live.current.marks) {
                const x = Math.round(u.valToPos(m.t, 'x', true)) + 0.5
                ctx.strokeStyle = `${m.color}99`
                ctx.beginPath()
                ctx.moveTo(x, bbox.top)
                ctx.lineTo(x, bbox.top + bbox.height)
                ctx.stroke()
              }
              ctx.restore()
            },
            drawn,
          ],
          setSelect: [
            (u) => {
              if (u.select.width > 4) live.current.onSelect(u.posToVal(u.select.left, 'x'), u.posToVal(u.select.left + u.select.width, 'x'))
              u.setSelect({ left: 0, top: 0, width: 0, height: 0 }, false)
              drawn()
            },
          ],
          setCursor: [
            (u) => {
              const l = u.cursor.left
              cursor.set(l != null && l >= 0 ? u.posToVal(l, 'x') : null, u)
            },
          ],
        },
      },
      [[], ...lines.flatMap(() => [[], [], []])],
      node,
    )
    setPlot(u)
    // Held, the pointer moves nothing else and releasing it, wherever, ends the drag.
    u.over.addEventListener('pointerdown', (e) => {
      if (e.pointerType !== 'mouse' || e.button !== 0) return
      held = true
      u.over.setPointerCapture(e.pointerId)
    })
    u.over.addEventListener('lostpointercapture', () => (held = false))
    const off = cursor.subscribe(() => {
      if (cursorSrc === u) return
      const t = cursorAt
      u.setCursor({ left: t === null ? -10 : u.valToPos(t, 'x'), top: t === null ? -10 : 10 }, false)
    })
    const resized = new ResizeObserver(() => node.clientWidth && u.setSize({ width: node.clientWidth, height }))
    resized.observe(node)
    return () => {
      off()
      resized.disconnect()
      u.destroy()
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps -- the panel is keyed by its lines
  useEffect(() => {
    live.current = { from, to, blank, marks, onSelect }
    plot?.batch(() => {
      plot.setData(uPlot.join(lines.map((l) => l.data)))
      plot.setScale('x', { min: from, max: to })
    })
  }, [plot, lines, from, to, blank, marks, onSelect]) // not on its own redraws
  return (
    <div>
      <div className="mb-1 flex flex-wrap items-baseline gap-x-3 text-xs text-neutral-500" style={{ paddingLeft: PAD_L }}>
        {title}
      </div>
      <div ref={el} />
      {plot && <Overlay u={plot} drawn={drawn} lines={lines} from={from} to={to} />}
    </div>
  )
}

const stamp = (t: number) => new Date(t).toLocaleString([], { weekday: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })

// at is the mean c held at t.
function at(c: Curve, t: number) {
  return c[3][c[0].findLastIndex((x) => x <= t)] ?? null
}

// Overlay tells, over u, what its lines held at the cursor, or latest
// without one: a bubble by it and a dot on each line. While dragging, it
// tells the span selected instead. It reads u as last drawn: drawn counts
// its draws.
function Overlay({ u, lines, from, to }: { u: uPlot; drawn: number; lines: Line[]; from: number; to: number }) {
  'use no memo' // u is read as it is now, not memoized
  const c = useCursor()
  const w = u.over.clientWidth
  const { left, width } = u.select
  if (width > 0)
    return createPortal(
      <div className={`${pill} top-full mt-1`} style={{ left: `clamp(80px, ${left + width / 2}px, calc(100% - 80px))` }}>
        {stamp(u.posToVal(left, 'x'))} → {stamp(u.posToVal(left + width, 'x'))}
      </div>,
      u.over,
    )
  const t = c !== null && c >= from && c <= to ? c : to
  const x = u.valToPos(t, 'x')
  return createPortal(
    <>
      {lines.map((l) => {
        const v = at(l.data, t)
        return (
          v !== null && (
            <span
              key={l.label}
              className="pointer-events-none absolute size-2.5 -translate-1/2 rounded-full border-2 border-neutral-900"
              style={{ left: x, top: u.valToPos(v, 'y'), background: l.color }}
            />
          )
        )
      })}
      <div
        className="pointer-events-none absolute top-1 rounded-md border border-neutral-700 bg-neutral-900/90 px-2 py-1 whitespace-nowrap shadow-lg"
        style={x > w / 2 ? { right: w - x + 10 } : { left: x + 10 }}
      >
        <div className="text-xs text-neutral-400">
          {c === null && 'Latest · '}
          {stamp(t)}
        </div>
        {lines.map((l) => (
          <div key={l.label} className={`flex items-center gap-1.5 ${l.dashed ? 'opacity-60' : ''}`}>
            <span className="size-2 shrink-0 rounded-full" style={{ background: l.color }} />
            {lines.length > 1 && <span className="max-w-48 truncate text-xs text-neutral-400">{l.label}</span>}
            <span className="text-base font-semibold tabular-nums">{l.read(t)}</span>
          </div>
        ))}
      </div>
    </>,
    u.over,
  )
}

// Indented lines a strip up with the curves' time scale; hovering it sets the cursor.
export function Indented({ from, to, label, children }: { from: number; to: number; label?: ReactNode; children: ReactNode }) {
  return (
    <div style={{ paddingLeft: PAD_L, paddingRight: PAD_R }}>
      {label && <div className="truncate text-[11px] text-neutral-500">{label}</div>}
      <div
        onMouseMove={(e) => {
          const rect = e.currentTarget.getBoundingClientRect()
          cursor.set(from + ((e.clientX - rect.left) / rect.width) * (to - from))
        }}
      >
        {children}
      </div>
    </div>
  )
}

// Strip draws ps as a state band: a binary Value's on time, as opaque as its
// buckets' share on; an enum's states, each its colour; or Events as ticks.
// Blanks are hatched over it. With read, it tells its value at the cursor.
export function Strip({
  ps,
  cap,
  from,
  to,
  blank,
  color,
  read,
}: {
  ps: Point[]
  cap: Capability
  from: number
  to: number
  blank: Blanks
  color: string
  read?: (t: number) => string
}) {
  const at = (t: number) => pct(t, from, to)
  const width = (s: number, e: number) => `${((e - s) / (to - from)) * 100}%`
  const hatched = (spans: Span[], background: string) =>
    spans.map(([s, e]) => <div key={s} className="pointer-events-none absolute inset-y-0" style={{ left: at(s), width: width(s, e), background }} />)
  let marks: ReactNode
  if (cap.stateless) {
    marks = ps
      .filter((p) => p.t >= from && p.t <= to)
      .map((p, i) => (
        <span
          key={i}
          title={
            'counts' in p
              ? Object.entries(p.counts)
                  .map(([v, n]) => `${n} ${v}`)
                  .join(', ')
              : 'v' in p
                ? String(p.v)
                : ''
          }
          className="absolute inset-y-0 w-0.5 -translate-x-1/2 bg-emerald-400"
          style={{ left: at(p.t) }}
        />
      ))
  } else {
    marks = held(ps, from, to, blank.all).map(({ from: s, to: e, p }) => {
      if (cap.type === 'binary') {
        const on = 'on' in p ? p.on : 'v' in p && p.v === true ? 1 : 0 // a contact is on when open
        return on > 0 && <div key={s} className="absolute inset-y-0" style={{ left: at(s), width: width(s, e), minWidth: 1, opacity: on, background: color }} />
      }
      const v = 'v' in p ? String(p.v) : ''
      return (
        <div
          key={s}
          title={v}
          className="absolute inset-y-0 overflow-hidden px-1 text-[10px] leading-4 whitespace-nowrap text-neutral-900"
          style={{
            left: at(s),
            width: width(s, e),
            minWidth: 1,
            opacity: 0.75,
            background: cap.options?.includes(v) ? enumColors[cap.options.indexOf(v) % enumColors.length] : '#a3a3a3', // grey: outside the Options
          }}
        >
          {v}
        </div>
      )
    })
  }
  return (
    <div className="relative h-4 overflow-hidden rounded-sm bg-neutral-800/60">
      {marks}
      {hatched(blank.offline, hatchOffline)}
      {hatched(blank.gaps, hatchGap)}
      <CursorLine from={from} to={to} read={read} />
    </div>
  )
}

// CursorLine marks the cursor across its parent; with read, labelled with its
// value, on the side with more room.
export function CursorLine({ from, to, read }: { from: number; to: number; read?: (t: number) => string }) {
  const t = useCursor()
  if (t === null || t < from || t > to) return null
  const at = (t - from) / (to - from)
  return (
    <>
      <div className="pointer-events-none absolute inset-y-0 w-px bg-white/60" style={{ left: pct(t, from, to) }} />
      {read && (
        <span
          className="pointer-events-none absolute inset-y-0 rounded-sm bg-neutral-950/85 px-1 text-[10px] leading-4 font-medium whitespace-nowrap text-white"
          style={at > 0.5 ? { right: `calc(${(1 - at) * 100}% + 4px)` } : { left: `calc(${at * 100}% + 4px)` }}
        >
          {read(t)}
        </span>
      )}
    </>
  )
}

// How each kind of marker shows.
export const markerStyle: Record<MarkerKind, { glyph: string; color: string; label: string }> = {
  hand: { glyph: '▲', color: '#38bdf8', label: 'Command by hand' },
  program: { glyph: '▲', color: '#fbbf24', label: 'by a Program' },
  automation: { glyph: '▲', color: '#a78bfa', label: 'by an Automation' },
  lost: { glyph: '▲', color: '#f87171', label: 'no response' },
  run: { glyph: '◆', color: '#e879f9', label: 'Run' },
  event: { glyph: '●', color: '#34d399', label: 'Event' },
  liveView: { glyph: '■', color: '#fb923c', label: 'Live view' },
  recording: { glyph: '►', color: '#f472b6', label: 'Recording' },
}

// MarkerStrip draws clusters of markers on the time scale: one alone as its
// glyph, several as their count. Tapping one tells what it holds, and links a
// Run to its Trace.
export function MarkerStrip({ clusters, from, to }: { clusters: Marker[][]; from: number; to: number }) {
  const [open, setOpen] = useState<Marker[] | null>(null)
  return (
    <div>
      <Indented from={from} to={to} label="Commands, Runs, Events">
        <div className="relative h-4 rounded-sm bg-neutral-800/60">
          {clusters.map((ms, i) => {
            const n = ms.reduce((sum, m) => sum + m.n, 0)
            const kind = ms.every((m) => m.kind === ms[0]!.kind) ? ms[0]!.kind : null
            const style = kind && markerStyle[kind]
            return (
              <button
                key={i}
                onClick={() => setOpen(ms)}
                title={n === 1 ? ms[0]!.text : `${n} markers`}
                className="absolute top-0 h-4 -translate-x-1/2 text-[10px] leading-4"
                style={{ left: pct(ms[0]!.t, from, to), color: style?.color ?? '#e5e5e5' }}
              >
                {n === 1 ? style!.glyph : <span className="rounded-full bg-neutral-700 px-1">{n}</span>}
              </button>
            )
          })}
          <CursorLine from={from} to={to} />
        </div>
      </Indented>
      {open && (
        <div
          data-no-drag
          className="mt-1 max-h-48 space-y-0.5 overflow-y-auto rounded-lg bg-neutral-800 p-2 text-xs"
          style={{ marginLeft: PAD_L, marginRight: PAD_R }}
        >
          <button onClick={() => setOpen(null)} aria-label="Close" className="float-right text-neutral-400 hover:text-white">
            ✕
          </button>
          {open.slice(0, 50).map((m, i) => (
            <div key={i} className="flex min-w-0 gap-2">
              <span style={{ color: markerStyle[m.kind].color }}>{markerStyle[m.kind].glyph}</span>
              <span className="shrink-0 text-neutral-400 tabular-nums">
                {new Date(m.t).toLocaleString([], { weekday: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' })}
              </span>
              <span className="truncate">{m.text}</span>
              {m.link && (
                <a href={m.link} className="shrink-0 text-amber-400 hover:text-amber-300">
                  Trace ›
                </a>
              )}
            </div>
          ))}
          {open.length > 50 && <div className="text-neutral-500">…and {open.length - 50} more: zoom in</div>}
        </div>
      )}
    </div>
  )
}

// A time axis for strips with no curve above them.
export function TimeAxis({ from, to }: { from: number; to: number }) {
  return (
    <Indented from={from} to={to}>
      <Ticks from={from} to={to} />
    </Indented>
  )
}

// Ticks label [from, to] across their parent, on local time, and tag the
// cursor's time over them.
export function Ticks({ from, to }: { from: number; to: number }) {
  const c = useCursor()
  const step = [15 * MIN, HOUR, 3 * HOUR, 6 * HOUR, 12 * HOUR, DAY, 7 * DAY, 30 * DAY].find((s) => (to - from) / s <= 8) ?? 365 * DAY
  const offset = new Date().getTimezoneOffset() * MIN // ticks fall on local time
  const ticks: number[] = []
  for (let t = Math.ceil((from - offset) / step) * step + offset; t <= to; t += step) ticks.push(t)
  return (
    <div className="relative h-5 text-xs leading-5 text-neutral-400">
      {ticks.map((t) => (
        <span key={t} className="absolute -translate-x-1/2 whitespace-nowrap" style={{ left: pct(t, from, to) }}>
          {step >= DAY
            ? new Date(t).toLocaleDateString([], { day: 'numeric', month: 'short' })
            : new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
        </span>
      ))}
      {c !== null && c >= from && c <= to && (
        <span
          className="absolute -translate-x-1/2 rounded bg-neutral-100 px-1.5 font-semibold whitespace-nowrap text-neutral-900 tabular-nums"
          style={{ left: `clamp(3rem, ${pct(c, from, to)}, calc(100% - 3rem))` }}
        >
          {to - from > DAY ? stamp(c) : new Date(c).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
        </span>
      )}
    </div>
  )
}
