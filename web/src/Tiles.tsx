// The Tiles of a Dashboard, drawn as dashboard.ts resolves them, and the controls and readings
// they and the panels show of a Capability.

import { useEffect, useRef, useState, type CSSProperties, type PointerEvent, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import {
  mdiAlarmLight,
  mdiAlarmLightOutline,
  mdiBrightness5,
  mdiCctv,
  mdiCogOutline,
  mdiDoorClosed,
  mdiDoorOpen,
  mdiFlag,
  mdiFlagOutline,
  mdiGauge,
  mdiGestureTapButton,
  mdiMoleculeCo2,
  mdiMotionSensor,
  mdiMotionSensorOff,
  mdiPlay,
  mdiPowerPlug,
  mdiPowerPlugOutline,
  mdiSmokeDetector,
  mdiSmokeDetectorAlert,
  mdiThermometer,
  mdiVolumeHigh,
  mdiWaterAlert,
  mdiWaterOutline,
  mdiWaterPercent,
  mdiWeatherRainy,
} from '@mdi/js'
import { runManual, sendCommand, useAvailability, useCommand, useControl, useLastEvent, useNow, useOnCount, useValue } from './store'
import type { AutomationStatus, Capability, CommandState, Flag, Fn, Ref, Target } from './types'
import { flagTarget } from './targets'
import { titleIfTruncated } from './truncated'
import { ColorPicker, display, format, RangeSlider, Switch, type HS } from './controls'
import { LightIcon, Svg } from './icons'
import { MiniChart } from './MiniChart'
import { HistorySheet } from './HistorySheet'
import { LiveViewer } from './LiveViewer'
import { Panel } from './Panel'
import { co2Level, motion, roles, turns } from './roles'
import { titled, unwell, type Part, type Shape } from './tiles'
import type { TargetTile } from './dashboard'

const clamp = (f: number) => Math.min(1, Math.max(0, f))

// A bar's icon, off and on, outside lights: those have their own, picked per Device.
const barIcons: Record<string, [off: string, on: string]> = {
  switch: [mdiPowerPlugOutline, mdiPowerPlug],
  alarm: [mdiAlarmLightOutline, mdiAlarmLight],
  flag: [mdiFlagOutline, mdiFlag],
}

// A light's, a plug's or a siren's tile is a single control, a bar: a tap toggles it, a sideways
// drag sets a light's brightness from where it was, a vertical one scrolls the page. ⋯ or a long
// press opens its other controls and readings, and from there its Device's or Aggregate's panel,
// which goes back to them. In an Area's header on a phone, the bar is a square: its icon alone, its
// level filling it from the bottom.
function BarTile({
  name,
  icon,
  members,
  note,
  badges,
  target,
  toggle: key,
  fn,
  dimmed,
  compact = false,
  onSettings,
}: {
  name: string
  icon?: string
  members?: Target[] // an Aggregate's lights
  note: string // detached, offline…
  badges?: ReactNode // its health, when wrong
  target: Target
  toggle: string // its main control, an on/off
  fn: Fn
  dimmed: boolean
  compact?: boolean // in an Area's header: the bar alone, its name only on its sheet; a square on a phone
  onSettings?: (back: () => void) => void // an Admin's: its Device's or Aggregate's panel
}) {
  const cap = fn.capabilities.find((c) => c.key === 'brightness' && c.access.settable && c.min != null && c.max != null)
  const on = useValue({ target, capability: key })?.data === true
  const powerCap = fn.capabilities.find((c) => c.key === 'power')
  const power = useValue({ target, capability: 'power' })?.data // a plug's, while on
  const command = useCommand(target)
  const { value: picked, set, picking, hold } = useControl<number>(target, 'brightness', { fade: true })
  const brightness = typeof picked === 'number' ? picked : undefined
  const [sheet, setSheet] = useState(false)
  const gesture = useRef<{ x: number; y: number; from: number; width: number; moving: boolean; press: number } | null>(null)
  const tapped = useRef(false) // the gesture that just ended neither dimmed nor scrolled
  const lit = on || picking
  const group = members !== undefined
  const litCount = useOnCount((members ?? []).map((m) => ({ target: m, capability: 'state' })))
  const min = Math.max(1, cap?.min ?? 1) // brightness 0 switches the light off: that is the tap's job
  const max = cap?.max ?? 1
  const level = !lit ? 0 : cap && brightness !== undefined ? clamp((brightness - min) / (max - min)) : 1
  const toggle = () => sendCommand(target, { [key]: 'toggle' })
  const dim = (f: number) => {
    if (!cap) return
    set(Math.round(min + clamp(f) * (max - min)))
  }

  const down = (e: PointerEvent<HTMLDivElement>) => {
    const press = window.setTimeout(() => ((gesture.current = null), hold(false), setSheet(true)), 500)
    // a drag across the phone's square still dims over a finger's travel
    gesture.current = { x: e.clientX, y: e.clientY, from: level, width: Math.max(e.currentTarget.clientWidth, 160), moving: false, press }
  }
  const end = () => {
    const g = gesture.current
    if (g) clearTimeout(g.press)
    gesture.current = null
    hold(false)
    return g
  }
  const move = (e: PointerEvent<HTMLDivElement>) => {
    const g = gesture.current
    if (!g) return
    const dx = e.clientX - g.x
    if (!g.moving) {
      if (Math.abs(e.clientY - g.y) > 10) return void end() // a scroll
      if (Math.abs(dx) < 8) return
      g.moving = true
      clearTimeout(g.press)
      e.currentTarget.setPointerCapture(e.pointerId)
      hold(true)
    }
    dim(g.from + dx / g.width)
  }
  const up = () => {
    const g = end()
    tapped.current = !!g && !g.moving // not after a long press either: it ended the gesture
  }
  // The toggle waits for the click, not the pointerup: a mobile browser sends no
  // click for a touch that scrolled or stopped a fling, so scrolling never switches a light.
  const click = () => tapped.current && ((tapped.current = false), toggle())

  const status = (
    <>
      {[
        lit && group && `${litCount}/${members.length}`,
        !lit ? 'off' : cap && brightness !== undefined ? display(brightness, cap) : 'on',
        lit && powerCap && power !== undefined && display(power, powerCap),
        note,
      ]
        .filter(Boolean)
        .join(' · ')}
      <CommandNote command={command} />
    </>
  )
  return (
    <>
      <section
        title={compact ? name : undefined}
        className={`${compact ? 'flex w-14 shrink-0 sm:w-56' : 'rounded-xl bg-neutral-900 p-1.5'} ${dimmed ? 'opacity-50' : ''}`}
      >
        <div
          role="switch"
          aria-checked={lit}
          aria-label={name}
          tabIndex={0}
          onPointerDown={down}
          onPointerMove={move}
          onPointerUp={up}
          onPointerCancel={() => (end(), (tapped.current = false))}
          onContextMenu={(e) => e.preventDefault()}
          onClick={click}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') toggle()
            else if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') dim(level + (e.key === 'ArrowRight' ? 0.1 : -0.1))
            else return
            e.preventDefault()
          }}
          className={`relative ${compact ? 'min-h-12 flex-1' : 'h-14'} cursor-pointer touch-pan-y overflow-hidden rounded-lg bg-neutral-800 select-none`}
        >
          <div
            className={`absolute bg-amber-400/25 ${compact ? 'inset-x-0 bottom-0 h-(--level) sm:inset-x-auto sm:inset-y-0 sm:left-0 sm:h-auto sm:w-(--level)' : 'inset-y-0 left-0 w-(--level)'}`}
            style={{ '--level': `${lit ? Math.max(level, 0.04) * 100 : 0}%` } as CSSProperties}
          />
          <div className={`relative flex h-full items-center ${compact ? 'justify-center gap-2.5 sm:justify-start sm:pl-3' : 'gap-3 pl-3'}`}>
            {fn.kind === 'light' ? (
              <LightIcon icon={icon} group={group} lit={lit} className={`size-6 shrink-0 ${lit ? 'text-amber-300' : 'text-neutral-500'}`} />
            ) : (
              <Svg path={(barIcons[fn.kind] ?? barIcons.switch!)[lit ? 1 : 0]} className={`size-6 shrink-0 ${lit ? 'text-amber-300' : 'text-neutral-500'}`} />
            )}
            {compact ? (
              <p className="hidden min-w-0 flex-1 truncate text-sm text-neutral-300 sm:block">{status}</p>
            ) : (
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2 text-xs">
                  <p className="truncate text-base font-medium" onMouseEnter={titleIfTruncated}>
                    {name}
                  </p>
                  {badges}
                </div>
                <p className="truncate text-xs text-neutral-400">{status}</p>
              </div>
            )}
            <button
              onPointerDown={(e) => e.stopPropagation()}
              onKeyDown={(e) => e.stopPropagation()}
              onClick={() => setSheet(true)}
              aria-label="More"
              className={`self-stretch ${compact ? 'hidden px-3 sm:block' : 'px-4'} text-lg text-neutral-500 hover:text-white`}
            >
              ⋯
            </button>
          </div>
        </div>
        {!compact && (
          <div className="mt-2 px-2.5 pb-1 empty:hidden">
            <MiniChart target={target} fn={fn} />
          </div>
        )}
      </section>
      {sheet && (
        <Panel
          title={
            <>
              <h2 className="truncate text-lg font-medium" onMouseEnter={titleIfTruncated}>
                {name}
              </h2>
              <p className="text-xs text-neutral-500">{status}</p>
            </>
          }
          onClose={() => setSheet(false)}
        >
          <FunctionControls target={target} fn={fn} />
          {onSettings && (
            <div className="mt-6 flex justify-end border-t border-neutral-800 pt-4">
              <button
                onClick={() => (setSheet(false), onSettings(() => setSheet(true)))}
                className="flex items-center gap-1.5 rounded-full bg-neutral-800 py-1.5 pr-3.5 pl-2.5 text-sm text-neutral-300 hover:bg-neutral-700 hover:text-white"
              >
                <Svg path={mdiCogOutline} className="size-4" />
                Settings
              </button>
            </div>
          )}
        </Panel>
      )}
    </>
  )
}

// A Flag without an Area, as a pill under the page header: a tap toggles it, ⋯ opens its panel, to an Admin.
export function FlagPill({ flag, onOpen }: { flag: Flag; onOpen?: () => void }) {
  const target = flagTarget(flag.id)
  const on = useValue({ target, capability: 'on' })?.data === true
  const command = useCommand(target)
  return (
    <div className={`flex items-center rounded-full text-sm ${on ? 'bg-amber-400/25 text-amber-200' : 'bg-neutral-800 text-neutral-300'}`}>
      <button
        role="switch"
        aria-checked={on}
        onClick={() => sendCommand(target, { on: 'toggle' })}
        className="flex items-center gap-1.5 py-1.5 pr-1 pl-3 hover:text-white"
      >
        <Svg path={on ? mdiFlag : mdiFlagOutline} className="size-4" />
        {flag.name}
        <CommandNote command={command} />
      </button>
      {onOpen && (
        <button onClick={onOpen} aria-label={`${flag.name} settings`} className="self-stretch pr-3 pl-1 text-neutral-500 hover:text-white">
          ⋯
        </button>
      )}
    </div>
  )
}

// An Automation's Tile with Manual triggers: a button for each, named after it.
// Only an enabled Automation runs; otherwise the tile is dimmed and says why.
export function ManualTile({ automation, onResult }: { automation: AutomationStatus; onResult: (r: { text: string; error?: boolean }) => void }) {
  const off = automation.status !== 'enabled' ? automation.status : null
  return (
    <section className={`space-y-3 rounded-xl bg-neutral-900 p-4 ${off ? 'opacity-50' : ''}`}>
      <div className="min-w-0">
        <p className="truncate font-medium" onMouseEnter={titleIfTruncated}>
          {automation.name}
        </p>
        <p className="text-xs text-neutral-500">{['automation', off].filter(Boolean).join(' · ')}</p>
      </div>
      <div className="flex flex-wrap gap-2">
        {automation.manualTriggers?.map((s) => (
          <button
            key={s.step}
            disabled={!!off}
            onClick={async () => {
              const r = await runManual(automation.id, s.step)
              onResult({ ...r, text: `${automation.name} · ${s.name || 'Run'}: ${r.text}` })
            }}
            className="rounded bg-neutral-800 px-3 py-1 hover:bg-neutral-700 disabled:cursor-not-allowed disabled:hover:bg-neutral-800"
          >
            {s.name || 'Run'}
          </button>
        ))}
      </div>
    </section>
  )
}

// fade: numeric changes glide over a short transition (a light's primary controls).
interface CapProps {
  target: Target
  cap: Capability
  fade?: boolean
}

// A Target's Tile, as dashboard.ts resolves it, drawn as its shape says: a bar, a sensor's, or its
// Functions' controls. A lone Function shares the Tile's header; a Function's kind or key shows as
// a heading only where it adds something. Dimmed while its Device is detached or it is offline.
export function Tile({ tile, compact, onOpen }: { tile: TargetTile; compact?: boolean; onOpen?: (back?: () => void) => void }) {
  const offline = useAvailability(tile.subject) === 'offline'
  const { shape, name, fns } = tile
  const dimmed = tile.detached || offline
  const note = [tile.detached && 'detached', offline && 'offline'].filter(Boolean).join(' · ')
  const badges = tile.health.map((h) => <HealthBadge key={`${h.target}|${h.cap.key}`} {...h} />)
  if (shape.kind === 'bar')
    return (
      <BarTile
        name={name}
        icon={tile.icon}
        members={tile.members}
        note={note}
        badges={badges}
        target={shape.target}
        fn={shape.fn}
        toggle={shape.toggle}
        dimmed={dimmed}
        compact={compact}
        onSettings={onOpen}
      />
    )
  if (shape.kind === 'camera')
    return <CameraTile target={shape.target} state={shape.state} name={name} note={note} badges={badges} dimmed={dimmed} onOpen={onOpen && (() => onOpen())} />
  if (shape.kind !== 'controls') return <SensorTile shape={shape} name={name} note={note} badges={badges} dimmed={dimmed} onOpen={onOpen && (() => onOpen())} />
  const header = (command?: ReactNode) => (
    <div className="min-w-0">
      {onOpen ? (
        <button onClick={() => onOpen()} onMouseEnter={titleIfTruncated} className="block max-w-full truncate text-left font-medium hover:underline">
          {name}
        </button>
      ) : (
        <p onMouseEnter={titleIfTruncated} className="truncate font-medium">
          {name}
        </p>
      )}
      <p className="flex items-center gap-2 text-xs text-neutral-500">
        <span>
          {[tile.summary, note].filter(Boolean).join(' · ')}
          {command}
        </span>
        {badges}
      </p>
    </div>
  )
  const heading = (fn: Fn) =>
    titled(fn)
      ? (command: ReactNode) => (
          <span className="text-sm text-neutral-400">
            {fn.capabilities.length === 1 ? fn.capabilities[0]?.label : fn.key}
            <span className="text-xs text-neutral-500">{command}</span>
          </span>
        )
      : undefined
  const [only] = fns
  return (
    <section className={`space-y-3 rounded-xl bg-neutral-900 p-4 ${dimmed ? 'opacity-50' : ''}`}>
      {fns.length === 1 && only ? (
        <FunctionControls {...only} heading={header} />
      ) : (
        <>
          {header()}
          {fns.map((f) => (
            <FunctionControls key={f.target} {...f} heading={heading(f.fn)} />
          ))}
        </>
      )}
    </section>
  )
}

// A button's Tile, a bar with no chart: its last Event and how long ago. A tap opens its History, ⋯ its
// Device's panel.
function EventTile({ name, note, badges, item, dimmed, onOpen }: SensorProps & { item: Part }) {
  const event = useLastEvent(ref(item))
  const now = useNow()
  const [history, setHistory] = useState(false)
  return (
    <section className={`rounded-xl bg-neutral-900 p-1.5 ${dimmed ? 'opacity-50' : ''}`}>
      <div
        role="button"
        tabIndex={0}
        aria-label={name}
        // its History sheet is portaled out of the bar, but React still bubbles its events here
        onClick={(e) => e.currentTarget.contains(e.target as Node) && setHistory(true)}
        onKeyDown={(e) => e.currentTarget.contains(e.target as Node) && (e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), setHistory(true))}
        className="flex h-14 cursor-pointer items-center gap-3 rounded-lg bg-neutral-800 pl-3 select-none hover:bg-neutral-700/60"
      >
        <Svg path={mdiGestureTapButton} className="size-6 shrink-0 text-neutral-500" />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 text-xs">
            <p className="truncate text-base font-medium" onMouseEnter={titleIfTruncated}>
              {name}
            </p>
            {badges}
          </div>
          <p className="truncate text-xs text-neutral-400" onMouseEnter={titleIfTruncated}>
            {[event ? `${String(event.data)} · ${age(event.at, now)}` : 'none yet', note].filter(Boolean).join(' · ')}
          </p>
        </div>
        <More onOpen={onOpen} />
      </div>
      {history && createPortal(<HistorySheet target={item.target} onClose={() => setHistory(false)} />, document.body)}
    </section>
  )
}

// How often a camera's Tile reads its Picture again while its page shows (ADR 0036).
const PICTURE_EVERY = 5 * 60 * 1000

// A camera's Picture: its URL, new each time it is read, so that its image loads again, and when
// it was taken; undefined until first read, null when there is none. A HEAD tells when it was
// taken, which an image cannot. Read when shown, when its page shows again, and every
// PICTURE_EVERY while it shows.
function usePicture(target: Target) {
  const [picture, setPicture] = useState<{ src: string; taken: string | null } | null>()
  useEffect(() => {
    let gone = false
    const read = async () => {
      if (document.hidden) return
      const src = `/api/picture?target=${encodeURIComponent(target)}&read=${Date.now()}`
      const res = await fetch(src, { method: 'HEAD' }).catch(() => null)
      if (!gone) setPicture(res?.ok ? { src, taken: res.headers.get('Last-Modified') } : null)
    }
    read()
    const every = setInterval(read, PICTURE_EVERY)
    document.addEventListener('visibilitychange', read)
    return () => {
      gone = true
      clearInterval(every)
      document.removeEventListener('visibilitychange', read)
    }
  }, [target])
  return picture
}

// A camera's Tile: its Picture and how old it is, its Device's state beside its name; a camera
// drawn instead until it has a Picture, or once it cannot load it. A tap on it opens its Live view,
// never by itself: watching may wake a camera on battery (ADR 0036).
function CameraTile({ target, state, name, note, badges, dimmed, onOpen }: SensorProps & { target: Target; state?: Part }) {
  const picture = usePicture(target)
  const [broken, setBroken] = useState<string>() // the src that failed to load
  const [live, setLive] = useState(false)
  const now = useNow()
  const shown = picture && picture.src !== broken ? picture : undefined
  const status = [shown ? shown.taken && age(shown.taken, now) : picture === undefined ? 'loading' : 'no picture', note].filter(Boolean).join(' · ')
  return (
    <section className={`space-y-1.5 rounded-xl bg-neutral-900 p-1.5 ${dimmed ? 'opacity-50' : ''}`}>
      <button
        onClick={() => setLive(true)}
        aria-label={`Watch ${name} live`}
        className="group relative block aspect-video w-full overflow-hidden rounded-lg bg-neutral-800"
      >
        {shown ? (
          <img src={shown.src} alt={`${name}, its latest picture`} onError={() => setBroken(shown.src)} className="size-full object-cover" />
        ) : (
          <Svg path={mdiCctv} className="absolute inset-0 m-auto size-10 text-neutral-600" />
        )}
        <span className="absolute right-2 bottom-2 rounded-full bg-black/60 p-1.5 text-white group-hover:bg-amber-500 group-hover:text-black">
          <Svg path={mdiPlay} className="size-5" />
        </span>
      </button>
      {live && createPortal(<LiveViewer target={target} name={name} onClose={() => setLive(false)} />, document.body)}
      <div className="flex items-center gap-3 pl-3">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 text-xs">
            <p className="truncate text-base font-medium" onMouseEnter={titleIfTruncated}>
              {name}
            </p>
            {badges}
          </div>
          <div className="flex items-center gap-3 text-xs text-neutral-400">
            <span className="truncate">{status}</span>
            {state && <StateText {...state} />}
          </div>
        </div>
        <More onOpen={onOpen} />
      </div>
    </section>
  )
}

interface SensorProps {
  name: string
  note: string // detached, offline…
  badges?: ReactNode // its health, when wrong
  dimmed: boolean
  onOpen?: () => void // an Admin's: its Device's or Aggregate's panel
}

// A sensor bar's ⋯, opening its panel, if anything does.
function More({ onOpen }: { onOpen?: () => void }) {
  if (!onOpen) return null
  return (
    <button
      onClick={(e) => (e.stopPropagation(), onOpen())}
      onKeyDown={(e) => e.stopPropagation()}
      aria-label="More"
      className="self-stretch px-4 text-lg text-neutral-500 hover:text-white"
    >
      ⋯
    </button>
  )
}

// A Tile with nothing to command, drawn as its shape says: its last Event, a bar for its state or
// its single reading, or a cell per reading.
function SensorTile({ shape, ...props }: SensorProps & { shape: Extract<Shape, { kind: 'event' | 'sensor' | 'readings' }> }) {
  if (shape.kind === 'event') return <EventTile {...props} item={shape.event} />
  const icon = readingIcons[shape.readings[0]?.cap.key ?? ''] ?? mdiGauge
  if (shape.kind === 'readings') return <ReadingsTile {...props} icon={icon} readings={shape.readings} />
  if (shape.state) return <StateTile {...props} state={shape.state} readings={shape.readings} />
  return <SensorBar {...props} icon={icon} readings={shape.readings} />
}

// A cell per reading: its value large, its label and its 24 h, each opening its History. With three
// or more, the Tile takes two columns of an automatic grid; in a Layout, its place's.
function ReadingsTile({ name, note, badges, icon, readings, dimmed, onOpen }: SensorProps & { icon: string; readings: Part[] }) {
  const wide = readings.length >= 3
  return (
    <section className={`@container space-y-3 rounded-xl bg-neutral-900 p-3 ${wide ? 'sm:col-span-2' : ''} ${dimmed ? 'opacity-50' : ''}`}>
      <div className="flex items-center gap-3 px-1.5">
        <Svg path={icon} className="size-6 shrink-0 text-neutral-500" />
        <div className="flex min-w-0 flex-1 items-center gap-2 text-xs">
          <p className="truncate text-base font-medium" onMouseEnter={titleIfTruncated}>
            {name}
          </p>
          {badges}
          {note && <span className="text-neutral-500">{note}</span>}
        </div>
        {onOpen && (
          <button onClick={onOpen} aria-label="More" className="px-2 text-lg text-neutral-500 hover:text-white">
            ⋯
          </button>
        )}
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

export const readingIcons: Record<string, string> = {
  temperature: mdiThermometer,
  humidity: mdiWaterPercent,
  co2: mdiMoleculeCo2,
  noise: mdiVolumeHigh,
  pressure: mdiGauge,
  illuminance: mdiBrightness5,
  rain: mdiWeatherRainy,
}

// Shaped as a light's Tile: a bar with a big icon, the name and a status line, its chart below. With
// a state, the bar takes its colour while active and the status leads with how long it has held. A
// tap on the bar opens the History of what the chart shows: the state's, or the first reading's.
function SensorBar({
  name,
  note,
  badges,
  readings,
  dimmed,
  onOpen,
  state,
  active = false,
  icon,
  lead,
}: SensorProps & { readings: Part[]; state?: Part; active?: boolean; icon: string; lead?: string }) {
  const [history, setHistory] = useState(false)
  const charted = state ?? readings[0]
  const status = [
    lead && (
      <span key="lead" className={active ? 'text-emerald-300' : ''}>
        {lead}
      </span>
    ),
    ...readings.map((p) => <ReadingText key={`${p.target}|${p.cap.key}`} {...p} />),
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
        className={`flex h-14 cursor-pointer items-center gap-3 rounded-lg pl-3 select-none ${active ? 'bg-emerald-500/20' : 'bg-neutral-800 hover:bg-neutral-700/60'}`}
      >
        <Svg path={icon} className={`size-6 shrink-0 ${active ? 'text-emerald-300' : 'text-neutral-500'}`} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 text-xs">
            <p className="truncate text-base font-medium" onMouseEnter={titleIfTruncated}>
              {name}
            </p>
            {badges}
          </div>
          <p className="truncate text-xs text-neutral-400">{status.flatMap((s, i) => (i ? [' · ', s] : [s]))}</p>
        </div>
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

// A reading in a status line: its unit says what it is, or else its label does. CO₂ that calls for
// airing takes a colour. A tap opens its own History, not the bar's.
export function ReadingText(props: CapProps & { className?: string }) {
  const { target, cap, className = '' } = props
  const value = useValue(ref(props))
  const [open, setOpen] = useState(false)
  const level = co2Level(cap.key, value?.data)
  return (
    <>
      <button
        onClick={(e) => (e.stopPropagation(), setOpen(true))}
        onKeyDown={(e) => e.stopPropagation()}
        title={`${cap.label} history`}
        className={`-my-1 py-1 text-left underline-offset-2 hover:text-white hover:underline ${level === 'high' ? 'text-red-400' : level === 'raised' ? 'text-amber-300' : ''} ${className}`}
      >
        {!cap.unit && `${cap.label} `}
        {value ? display(value.data, cap) : '—'}
      </button>
      {open && createPortal(<HistorySheet target={target} onClose={() => setOpen(false)} />, document.body)}
    </>
  )
}

const stateIcons: Record<string, [idle: string, active: string]> = {
  occupancy: [mdiMotionSensorOff, mdiMotionSensor],
  presence: [mdiMotionSensorOff, mdiMotionSensor],
  contact: [mdiDoorClosed, mdiDoorOpen],
  water_leak: [mdiWaterOutline, mdiWaterAlert],
  smoke: [mdiSmokeDetector, mdiSmokeDetectorAlert],
}

// A state: whether it is active, its icon, and how long it has held, as Home tells it (an
// Aggregate's from its members'). Motion that is on is "now".
function useHeld({ target, cap }: CapProps) {
  const value = useValue({ target, capability: cap.key })
  const now = useNow()
  const active = value?.data === true
  const [idle, lit] = stateIcons[cap.key] ?? [mdiMotionSensorOff, mdiMotionSensor]
  const text = active && motion(cap) ? 'now' : value?.since ? age(value.since, now) : '?'
  return { active, icon: active ? lit : idle, text, title: `${cap.label} · ${value ? format(value.data) : 'unknown'}` }
}

// A state as an Area's header tells its presence or doors: its icon, what it is, how long it has
// held, lit while active. A tap opens its History.
export function StateText(props: CapProps) {
  const held = useHeld(props)
  const [open, setOpen] = useState(false)
  return (
    <>
      <button onClick={() => setOpen(true)} title={held.title} className="flex items-center gap-1 whitespace-nowrap hover:underline">
        <Svg path={held.icon} className={`size-4 ${held.active ? 'text-emerald-300' : 'text-neutral-500'}`} />
        <span className={held.active ? 'text-emerald-300' : ''}>{held.active ? turns(props.cap)[0] : props.cap.key === 'contact' ? 'closed' : 'clear'}</span>
        <span className="text-neutral-500">{held.text}</span>
      </button>
      {open && createPortal(<HistorySheet target={props.target} onClose={() => setOpen(false)} />, document.body)}
    </>
  )
}

// The controls of one Function: its main control and adjustments, its state,
// readings and Events. Its toggles sit beside its heading, which shows the
// Function's pending Command; settings and diagnostics live in the DevicePanel.
function FunctionControls({ target, fn, heading }: { target: Target; fn: Fn; heading?: (note: ReactNode) => ReactNode }) {
  const command = useCommand(target)
  const fade = fn.kind === 'light'
  const r = roles(fn.kind, fn.capabilities)
  const controls = [r.control, ...r.adjustments].filter((c) => c !== undefined)
  const toggles = controls.filter((c) => c.type === 'binary')
  const sliders = controls.filter((c) => c.type === 'numeric' && c.min != null && c.max != null)
  const colors = controls.filter((c) => c.type === 'composite') // a light's color_hs
  const selects = controls.filter((c) => c.type === 'enum')
  const readings = [r.state, ...r.readings].filter((c) => c !== undefined)
  const events = r.events

  return (
    <>
      {heading && (
        <div className="flex items-start justify-between gap-3">
          {heading(<CommandNote command={command} />)}
          {toggles.map((c) => (
            <Toggle key={c.key} target={target} cap={c} />
          ))}
        </div>
      )}
      {readings.map((c) => (
        <Reading key={c.key} target={target} cap={c} />
      ))}
      {sliders.map((c) => (
        <Slider key={c.key} target={target} cap={c} fade={fade} />
      ))}
      {colors.map((c) => (
        <Color key={c.key} target={target} cap={c} />
      ))}
      {selects.map((c) => (
        <Select key={c.key} target={target} cap={c} />
      ))}
      {events.map((c) => (
        <LastEvent key={c.key} target={target} cap={c} />
      ))}
      <MiniChart target={target} fn={fn} />
    </>
  )
}

// Only a Command that ended badly is worth mentioning.
export function CommandNote({ command }: { command?: CommandState }) {
  switch (command?.status) {
    case 'timed_out':
      return <span className="text-amber-400"> · no response</span>
    case 'failed':
      return <span className="text-red-400"> · {command.error ?? 'failed'}</span>
  }
  return null
}

export function Control(props: CapProps) {
  const { cap } = props
  if (cap.access.settable && cap.type === 'binary') {
    return (
      <div className="flex items-center justify-between gap-2 text-sm">
        <span className="text-neutral-400">{cap.label}</span>
        <Toggle {...props} />
      </div>
    )
  }
  if (cap.access.settable && cap.type === 'numeric' && cap.min != null && cap.max != null) return <Slider {...props} />
  if (cap.access.settable && cap.type === 'enum') return <Select {...props} />
  return <Reading {...props} />
}

const ref = ({ target, cap }: CapProps): Ref => ({ target, capability: cap.key })

// Health on a Tile, only when wrong: a battery's level once low, a tamper or low flag once on.
function HealthBadge(props: CapProps) {
  const data = useValue(ref(props))?.data
  if (!unwell(data)) return null
  if (typeof data !== 'number') return <span className="text-red-400">{props.cap.label}</span>
  const level = Math.max(0, Math.min(100, data))
  return (
    <span title={props.cap.label} className="flex items-center gap-1 text-red-400">
      <svg viewBox="0 0 24 12" className="h-2.5 w-5" aria-hidden>
        <rect x="0.5" y="0.5" width="20" height="11" rx="2" fill="none" stroke="currentColor" />
        <rect x="21.5" y="3.5" width="2" height="5" rx="1" fill="currentColor" />
        <rect x="2" y="2" width={(17 * level) / 100} height="8" rx="1" fill="currentColor" />
      </svg>
      {Math.round(level)}%
    </span>
  )
}

function Toggle(props: CapProps) {
  const { target, cap } = props
  const on = useValue(ref(props))?.data === true
  return <Switch on={on} onChange={(v) => sendCommand(target, { [cap.key]: v })} label={cap.label} />
}

// Which of a colour light's modes it shows, if it tells: hs, xy or color_temp.
// The other mode's control shows a value derived from it, so it is dimmed.
function useColorMode(target: Target) {
  return useValue({ target, capability: 'color_mode' })?.data as string | undefined
}

function Slider(props: CapProps) {
  const { target, cap, fade } = props
  const { value, set, picking, hold } = useControl<number>(target, cap.key, { fade })
  const mode = useColorMode(target)
  return (
    <RangeSlider
      cap={cap}
      value={typeof value === 'number' ? value : cap.min!}
      dim={cap.key === 'color_temp' && mode !== undefined && mode !== 'color_temp' && !picking}
      onDragging={hold}
      onChange={set}
      className="text-sm"
    />
  )
}

function Color(props: CapProps) {
  const { target, cap } = props
  const { value, set, picking } = useControl<HS>(target, cap.key)
  const mode = useColorMode(target)
  return (
    <div className="flex items-center justify-between gap-2 text-sm">
      <span className="text-neutral-400">Color</span>
      <ColorPicker value={value ?? { hue: 0, saturation: 0 }} dim={mode === 'color_temp' && !picking} onChange={set} label={cap.label} />
    </div>
  )
}

function Select(props: CapProps) {
  const { target, cap } = props
  const value = useValue(ref(props))
  const current = typeof value?.data === 'string' ? value.data : ''
  return (
    <label className="flex items-center justify-between gap-2 text-sm">
      <span className="text-neutral-400">{cap.label}</span>
      <select value={current} onChange={(e) => sendCommand(target, { [cap.key]: e.target.value })} className="rounded bg-neutral-800 px-2 py-1">
        <option value="" disabled>
          —
        </option>
        {current !== '' && !cap.options?.includes(current) && (
          <option disabled>{current}</option> // reported, but not one a Command may set
        )}
        {cap.options?.map((o) => (
          <option key={o}>{o}</option>
        ))}
      </select>
    </label>
  )
}

export function Reading(props: CapProps) {
  const { cap } = props
  const value = useValue(ref(props))
  const now = useNow()
  return (
    <div className="flex items-baseline justify-between gap-2 text-sm">
      <span className="text-neutral-400">{cap.label}</span>
      <span>
        {value ? format(value.data, cap.unit) : '—'}
        {value && now - new Date(value.at).getTime() > STALE_AFTER && <span className="ml-2 text-xs text-amber-400/80">{age(value.at, now)}</span>}
      </span>
    </div>
  )
}

function LastEvent(props: CapProps) {
  const { cap } = props
  const event = useLastEvent(ref(props))
  const now = useNow()
  return (
    <div className="flex items-baseline justify-between gap-2 text-sm">
      <span className="text-neutral-400">{cap.label}</span>
      {event ? (
        <span>
          {String(event.data)}
          <span className="ml-2 text-xs text-neutral-500">{age(event.at, now)}</span>
        </span>
      ) : (
        <span className="text-neutral-500">none yet</span>
      )}
    </div>
  )
}

const relative = new Intl.RelativeTimeFormat('en', { numeric: 'auto', style: 'narrow' })

// A Value's age is only worth showing once it is stale.
const STALE_AFTER = 60 * 60 * 1000

function age(at: string, now: number) {
  const seconds = Math.round((new Date(at).getTime() - now) / 1000)
  if (seconds > -10) return 'just now'
  if (seconds > -3600) return relative.format(Math.round(seconds / 60), 'minute')
  if (seconds > -86400) return relative.format(Math.round(seconds / 3600), 'hour')
  return relative.format(Math.round(seconds / 86400), 'day')
}
