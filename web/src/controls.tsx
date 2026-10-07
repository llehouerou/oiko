// Controls shared by the Function tiles and the automation editor.

import type { ReactNode } from 'react'
import type { Capability } from './types'

export function Switch({
  on,
  onChange,
  label,
  small,
  disabled,
}: {
  on: boolean
  onChange: (on: boolean) => void
  label: string
  small?: boolean
  disabled?: boolean
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label={label}
      title={label}
      disabled={disabled}
      onClick={() => onChange(!on)}
      className={`shrink-0 rounded-full transition-colors disabled:opacity-40 ${small ? 'h-4 w-7' : 'h-7 w-12'} ${on ? 'bg-amber-400' : 'bg-neutral-700'}`}
    >
      <span
        className={`block rounded-full bg-white transition-transform ${small ? 'size-3' : 'size-5'} ${on ? (small ? 'translate-x-3.5' : 'translate-x-6') : small ? 'translate-x-0.5' : 'translate-x-1'}`}
      />
    </button>
  )
}

// A colour as zigbee2mqtt's color_hs holds it: hue in degrees, saturation in %.
export type HS = { hue: number; saturation: number }

// The native colour picker over a colour's hue and saturation. How dark the
// picked colour is has no say: brightness is a Capability of its own. Without
// a value it is greyed out; dim, it is faded but still usable.
export function ColorPicker({ value, onChange, label, dim }: { value?: HS; onChange: (v: HS) => void; label: string; dim?: boolean }) {
  return (
    <input
      type="color"
      aria-label={label}
      value={toHex(value ?? { hue: 0, saturation: 0 })}
      disabled={!value}
      onChange={(e) => onChange(toHS(e.target.value))}
      className={`h-6 w-10 shrink-0 cursor-pointer rounded bg-transparent disabled:cursor-default disabled:opacity-30 ${dim ? 'opacity-30' : ''}`}
    />
  )
}

// The colour at full value (HSV), as #rrggbb.
export function toHex({ hue, saturation }: HS) {
  const s = saturation / 100
  const channel = (n: number) => {
    const k = (n + hue / 60) % 6
    return Math.round(255 * (1 - s * Math.max(0, Math.min(k, 4 - k, 1))))
  }
  return `#${[5, 3, 1].map((n) => channel(n).toString(16).padStart(2, '0')).join('')}`
}

// The hue and saturation (HSV) of #rrggbb.
export function toHS(hex: string): HS {
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255) as [number, number, number]
  const max = Math.max(r, g, b)
  const d = max - Math.min(r, g, b)
  const h = d === 0 ? 0 : max === r ? ((g - b) / d) % 6 : max === g ? (b - r) / d + 2 : (r - g) / d + 4
  return { hue: Math.round((h * 60 + 360) % 360), saturation: max === 0 ? 0 : Math.round((d / max) * 100) }
}

// A slider over a numeric Capability's range, its value shown in people's
// units. Without a value it is greyed out; dim, it is faded and shows no
// value, but is still usable. lead goes before the label.
export function RangeSlider({
  cap,
  value,
  onChange,
  onDragging,
  lead,
  dim,
  className = '',
}: {
  cap: Capability
  value?: number
  onChange: (v: number) => void
  onDragging?: (dragging: boolean) => void
  lead?: ReactNode
  dim?: boolean
  className?: string
}) {
  // Color temperature: mireds on the wire, Kelvin and a warm→cool gradient for people.
  // Kelvin falls as mireds rise, so the slider runs right-to-left.
  const temperature = cap.unit === 'mired'
  const brightness = cap.key === 'brightness'
  const shown = value ?? cap.max!
  return (
    <label className={`block ${className}`}>
      <span className={`flex items-center justify-between gap-1 text-neutral-400 ${lead ? 'mb-1.5' : ''}`}>
        <span className="flex items-center gap-1">
          {lead}
          {cap.label}
        </span>
        <span>{value === undefined || dim ? '—' : display(value, cap)}</span>
      </span>
      <input
        type="range"
        min={brightness ? Math.max(1, cap.min!) : cap.min} // brightness 0 switches the light off: that is the toggle's job
        max={cap.max}
        step={cap.step ?? 1}
        dir={temperature ? 'rtl' : undefined}
        value={shown}
        disabled={value === undefined}
        onPointerDown={() => onDragging?.(true)}
        onPointerUp={() => onDragging?.(false)}
        onChange={(e) => onChange(Number(e.target.value))}
        className={`${temperature ? temperatureTrack : brightness ? brightnessTrack : 'w-full accent-amber-400'} ${off} ${dim ? 'opacity-30' : ''}`}
        style={brightness ? { background: brightnessFill((shown - cap.min!) / (cap.max! - cap.min!)) } : undefined}
      />
    </label>
  )
}

const track = 'h-2 w-full cursor-pointer appearance-none rounded-full '
const thumb =
  '[&::-webkit-slider-thumb]:size-4 [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-white [&::-webkit-slider-thumb]:shadow [&::-webkit-slider-thumb]:ring-2 [&::-webkit-slider-thumb]:ring-neutral-900 ' +
  '[&::-moz-range-thumb]:size-4 [&::-moz-range-thumb]:rounded-full [&::-moz-range-thumb]:border-2 [&::-moz-range-thumb]:border-neutral-900 [&::-moz-range-thumb]:bg-white'
const temperatureTrack = `${track}bg-linear-to-r from-orange-400 via-amber-50 to-sky-200 ${thumb}`
const brightnessTrack = `${track}${thumb}`
// Without a value there is no position to show: no thumb.
const off = 'disabled:opacity-30 disabled:[&::-webkit-slider-thumb]:opacity-0 disabled:[&::-moz-range-thumb]:opacity-0'

// The filled part of the brightness bar glows brighter as the light gets brighter.
function brightnessFill(fraction: number) {
  const p = fraction * 100
  return `linear-gradient(to right, color-mix(in oklch, #fff4d6 ${p}%, #5a3a10) 0 ${p}%, #262626 ${p}% 100%)`
}

// Brightness as a percentage of its range; rounding never hides the ends:
// a light that is not fully dimmed shows at least 1 %, one not at full at most 99 %.
function percent(value: number, cap: Capability) {
  const p = Math.round(((value - cap.min!) / (cap.max! - cap.min!)) * 100)
  return `${value <= cap.min! ? 0 : value >= cap.max! ? 100 : Math.min(99, Math.max(1, p))} %`
}

function kelvin(mireds: number) {
  const k = Math.round(1_000_000 / mireds / 100) * 100
  return `${k} K · ${k < 3000 ? 'warm' : k < 4500 ? 'neutral' : 'cool'}`
}

const number = new Intl.NumberFormat('en', { maximumFractionDigits: 1 })

export function format(data: unknown, unit?: string) {
  const text =
    typeof data === 'number' ? number.format(data) : typeof data === 'boolean' ? (data ? 'on' : 'off') : typeof data === 'string' ? data : JSON.stringify(data)
  return unit ? `${text} ${unit}` : text
}

// A value as people read it: Kelvin for a colour temperature, a percentage for
// brightness, otherwise the number or text with its unit.
export function display(data: unknown, cap?: Capability) {
  if (typeof data === 'number' && cap?.unit === 'mired') return kelvin(data)
  if (typeof data === 'number' && cap?.key === 'brightness' && cap.min != null && cap.max != null) return percent(data, cap)
  return format(data, cap?.unit)
}
