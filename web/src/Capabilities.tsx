// The controls and readings of a Capability, shown by the Tiles and the panels.

import { useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import {
  mdiBrightness5,
  mdiDoorClosed,
  mdiDoorOpen,
  mdiGauge,
  mdiMoleculeCo2,
  mdiMotionSensor,
  mdiMotionSensorOff,
  mdiSmokeDetector,
  mdiSmokeDetectorAlert,
  mdiThermometer,
  mdiVolumeHigh,
  mdiWaterAlert,
  mdiWaterOutline,
  mdiWaterPercent,
  mdiWeatherRainy,
} from '@mdi/js'
import { sendCommand, useCommand, useControl, useLastEvent, useNow, useValue } from './store'
import type { Capability, CommandState, Fn, Ref, Target } from './types'
import { ColorPicker, display, format, RangeSlider, Switch, type HS } from './controls'
import { Svg } from './icons'
import { MiniChart } from './MiniChart'
import { HistorySheet } from './HistorySheet'
import { co2Level, motion, roles, turns } from './roles'
import { unwell } from './tiles'
import { age } from './TileParts'

// fade: numeric changes glide over a short transition (a light's primary controls).
interface CapProps {
  target: Target
  cap: Capability
  fade?: boolean
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

// A reading in a status line: its unit says what it is, or else its label does. CO₂ that calls for
// airing takes a colour. A tap opens its own History, not the bar's. unitClassName sets its unit apart.
export function ReadingText(props: CapProps & { className?: string; unitClassName?: string }) {
  const { target, cap, className = '', unitClassName } = props
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
        {value ? <Shown text={display(value.data, cap)} unit={cap.unit} unitClassName={unitClassName} /> : '—'}
      </button>
      {open && createPortal(<HistorySheet target={target} onClose={() => setOpen(false)} />, document.body)}
    </>
  )
}

function Shown({ text, unit, unitClassName }: { text: string; unit?: string; unitClassName?: string }) {
  if (!unitClassName || !unit || !text.endsWith(` ${unit}`)) return text
  return (
    <>
      {text.slice(0, -unit.length - 1)}
      <span className={unitClassName}>{unit}</span>
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
export function useHeld({ target, cap }: CapProps) {
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
export function FunctionControls({ target, fn, heading }: { target: Target; fn: Fn; heading?: (note: ReactNode) => ReactNode }) {
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

export const ref = ({ target, cap }: CapProps): Ref => ({ target, capability: cap.key })

// Health on a Tile, only when wrong: a battery's level once low, a tamper or low flag once on.
export function HealthBadge(props: CapProps) {
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

// A Value's age is only worth showing once it is stale.
const STALE_AFTER = 60 * 60 * 1000
