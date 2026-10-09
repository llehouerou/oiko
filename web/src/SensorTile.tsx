// The Tiles with nothing to command: a button's, a sensor's, and a cell per reading.

import { useState } from 'react'
import { createPortal } from 'react-dom'
import { mdiGauge, mdiGestureTapButton } from '@mdi/js'
import { useLastEvent, useNow } from './store'
import { titleIfTruncated } from './truncated'
import { Svg } from './icons'
import { MiniChart } from './MiniChart'
import { HistorySheet } from './HistorySheet'
import { roles } from './roles'
import type { Part, Shape } from './tiles'
import { readingIcons, ReadingText, ref, useHeld } from './Capabilities'
import { age, More, NameLine, statusSize, useLongPress, type SensorProps } from './TileParts'

// A button's Tile, a bar with no chart: its last Event and how long ago. A tap opens its History, ⋯ or
// a long press its Device's panel.
function EventTile({ name, note, badges, item, dimmed, hideName, onOpen }: SensorProps & { item: Part }) {
  const event = useLastEvent(ref(item))
  const now = useNow()
  const [history, setHistory] = useState(false)
  const longPress = useLongPress(onOpen)
  return (
    <section className={`rounded-xl bg-neutral-900 p-1.5 ${dimmed ? 'opacity-50' : ''}`}>
      <div
        role="button"
        tabIndex={0}
        aria-label={name}
        // its History sheet is portaled out of the bar, but React still bubbles its events here
        onClick={(e) => e.currentTarget.contains(e.target as Node) && setHistory(true)}
        onKeyDown={(e) => e.currentTarget.contains(e.target as Node) && (e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), setHistory(true))}
        {...longPress}
        className="group/card relative flex h-14 cursor-pointer items-center gap-3 rounded-lg bg-neutral-800 px-3 select-none hover:bg-neutral-700/60"
      >
        <Svg path={mdiGestureTapButton} className="size-6 shrink-0 text-neutral-500" />
        <div className="min-w-0 flex-1">
          <NameLine name={name} hidden={hideName} badges={badges} />
          <p className={`truncate ${statusSize(hideName)}`} onMouseEnter={titleIfTruncated}>
            {[event ? `${String(event.data)} · ${age(event.at, now)}` : 'none yet', note].filter(Boolean).join(' · ')}
          </p>
        </div>
        <More onOpen={onOpen} />
      </div>
      {history && createPortal(<HistorySheet target={item.target} onClose={() => setHistory(false)} />, document.body)}
    </section>
  )
}

// A Tile with nothing to command, drawn as its shape says: its last Event, a bar for its state or
// its single reading, or a cell per reading.
export function SensorTile({ shape, ...props }: SensorProps & { shape: Extract<Shape, { kind: 'event' | 'sensor' | 'readings' }> }) {
  if (shape.kind === 'event') return <EventTile {...props} item={shape.event} />
  const icon = readingIcons[shape.readings[0]?.cap.key ?? ''] ?? mdiGauge
  if (shape.kind === 'readings') return <ReadingsTile {...props} icon={icon} readings={shape.readings} />
  if (shape.state) return <StateTile {...props} state={shape.state} readings={shape.readings} />
  return <SensorBar {...props} icon={icon} readings={shape.readings} />
}

// A cell per reading: its value large, its label and its 24 h, each opening its History; ⋯ or a long
// press opens its panel. With three or more, the Tile takes two columns of an automatic grid; in a
// Layout, its place's.
function ReadingsTile({ name, note, badges, icon, readings, dimmed, hideName, onOpen }: SensorProps & { icon: string; readings: Part[] }) {
  const wide = readings.length >= 3
  const longPress = useLongPress(onOpen)
  return (
    <section
      {...longPress}
      className={`@container group/card relative space-y-3 rounded-xl bg-neutral-900 p-3 select-none ${wide ? 'sm:col-span-2' : ''} ${dimmed ? 'opacity-50' : ''}`}
    >
      <More onOpen={onOpen} />
      <div className="flex items-center gap-3 px-1.5">
        <Svg path={icon} className="size-6 shrink-0 text-neutral-500" />
        <div className="flex min-w-0 flex-1 items-center gap-2 text-xs">
          {!hideName && (
            <p className="truncate text-base font-medium" onMouseEnter={titleIfTruncated}>
              {name}
            </p>
          )}
          {badges}
          {note && <span className="text-neutral-500">{note}</span>}
        </div>
      </div>
      {/* three readings side by side only where its place is wide enough for them */}
      <div className={`grid grid-cols-2 gap-2 ${wide ? '@lg:grid-cols-3' : ''}`}>
        {readings.map((r) => (
          <div key={`${r.target}|${r.cap.key}`} className="min-w-0 space-y-1 rounded-lg bg-neutral-800/60 p-2.5">
            <ReadingText {...r} className="block max-w-full truncate text-xl font-medium" />
            <p className="truncate text-xs text-neutral-400">{r.cap.label}</p>
            {/* a reading other than its Function's charted one has no 24 h loaded */}
            {roles(r.fn.kind, r.fn.capabilities).charted === r.cap && (
              <div className="px-1">
                <MiniChart target={r.target} fn={r.fn} />
              </div>
            )}
          </div>
        ))}
      </div>
    </section>
  )
}

function StateTile({ state, ...props }: SensorProps & { state: Part; readings: Part[] }) {
  const held = useHeld(state)
  return <SensorBar {...props} state={state} active={held.active} icon={held.icon} lead={held.text} />
}

// Shaped as a light's Tile: a bar with a big icon, the name and a status line, its chart below. With
// a state, the bar takes its colour while active and the status leads with how long it has held; a
// lone reading instead stands large at the bar's end, or in the name's place, larger, once it is
// hidden. A tap on the bar opens the History of what the chart shows: the state's, or the first
// reading's; ⋯ or a long press, its panel.
function SensorBar({
  name,
  note,
  badges,
  readings,
  dimmed,
  hideName,
  onOpen,
  state,
  active = false,
  icon,
  lead,
}: SensorProps & { readings: Part[]; state?: Part; active?: boolean; icon: string; lead?: string }) {
  const [history, setHistory] = useState(false)
  const longPress = useLongPress(onOpen)
  const charted = state ?? readings[0]
  const lone = !state && readings.length === 1 ? readings[0] : undefined
  const status = [
    lead && (
      <span key="lead" className={active ? 'text-emerald-300' : ''}>
        {lead}
      </span>
    ),
    ...(lone ? [] : readings).map((p) => <ReadingText key={`${p.target}|${p.cap.key}`} {...p} />),
    note && <span key="note">{note}</span>,
  ].filter(Boolean)
  return (
    <section className={`rounded-xl bg-neutral-900 p-1.5 ${dimmed ? 'opacity-50' : ''}`}>
      <div
        role="button"
        tabIndex={0}
        aria-label={name}
        // A reading's History sheet is portaled out of the bar, but React still bubbles its events here.
        onClick={(e) => e.currentTarget.contains(e.target as Node) && setHistory(true)}
        onKeyDown={(e) => e.currentTarget.contains(e.target as Node) && (e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), setHistory(true))}
        {...longPress}
        className={`group/card relative flex h-14 cursor-pointer items-center gap-3 rounded-lg px-3 select-none ${active ? 'bg-emerald-500/20' : 'bg-neutral-800 hover:bg-neutral-700/60'}`}
      >
        <Svg path={icon} className={`size-6 shrink-0 ${active ? 'text-emerald-300' : 'text-neutral-500'}`} />
        <div className="min-w-0 flex-1">
          <NameLine
            name={name}
            hidden={hideName}
            badges={badges}
            className={lone ? 'line-clamp-2 text-sm leading-tight wrap-break-word hyphens-auto' : undefined}
          />
          {lone && hideName && <ReadingText {...lone} unitClassName="ml-1 text-base" className="block text-3xl font-light tabular-nums" />}
          {status.length > 0 && <p className={`truncate ${statusSize(hideName && !lone)}`}>{status.flatMap((s, i) => (i ? [' · ', s] : [s]))}</p>}
        </div>
        {lone && !hideName && <ReadingText {...lone} unitClassName="ml-0.5 text-sm" className="shrink-0 text-2xl font-light tabular-nums" />}
        <More onOpen={onOpen} />
      </div>
      {charted && (
        <div className="mt-2 px-2.5 pb-1 empty:hidden">
          <MiniChart target={charted.target} fn={charted.fn} />
        </div>
      )}
      {/* outside the tile, which an offline Device dims */}
      {history && charted && createPortal(<HistorySheet target={charted.target} onClose={() => setHistory(false)} />, document.body)}
    </section>
  )
}
