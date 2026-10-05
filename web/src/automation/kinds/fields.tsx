// The fields Step kinds' params forms are made of, shared between kinds.

import { useState, type InputHTMLAttributes, type ReactNode } from 'react'
import type { Capability, Target } from '../../types'
import { display } from '../../controls'
import { duration, hasValue, nudge, parseNumber, soleCapability, type Params } from '../model'
import { targetKind, type Catalogue, type Entry } from '../../targets'

export type FormProps = { id: string; p: Params; set: (changes: Params) => void; targets: Catalogue }

export const field = 'rounded bg-neutral-800 px-1.5 py-0.5 aria-invalid:text-red-400 aria-invalid:ring-1 aria-invalid:ring-red-500'

export function Row({ children }: { children: ReactNode }) {
  return <div className="flex flex-wrap items-center gap-1 text-neutral-400">{children}</div>
}

export function Check({ checked, onChange, children }: { checked: boolean; onChange: (v: boolean) => void; children: ReactNode }) {
  return (
    <label className="flex items-center gap-1 text-neutral-400">
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} className="accent-amber-400" />
      {children}
    </label>
  )
}

export function Hint({ children }: { children: ReactNode }) {
  return <p className="text-neutral-500">{children}</p>
}

// A number that can be typed freely, with a decimal point or comma: it shows
// what is typed until it loses focus. The arrow keys move it by step, within
// min and max; out of them, or not a number, it is marked invalid.
export function NumberField({
  value,
  onChange,
  min,
  max,
  step,
  ...props
}: { value?: number; onChange: (v: number | undefined) => void; min?: number; max?: number; step?: number } & Omit<
  InputHTMLAttributes<HTMLInputElement>,
  'value' | 'onChange' | 'min' | 'max' | 'step'
>) {
  const [typed, setTyped] = useState<string | null>(null)
  const text = typed ?? (value === undefined ? '' : String(value))
  const n = parseNumber(text)
  const invalid = text.trim() !== '' && (Number.isNaN(n) || n < (min ?? -Infinity) || n > (max ?? Infinity))
  const put = (t: string) => {
    setTyped(t)
    const x = parseNumber(t)
    onChange(Number.isNaN(x) ? undefined : x)
  }
  return (
    <input
      inputMode="decimal"
      {...props}
      value={text}
      aria-invalid={invalid}
      onChange={(e) => put(e.target.value)}
      onKeyDown={(e) => {
        if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return
        e.preventDefault()
        put(String(nudge(Number.isNaN(n) ? (min ?? 0) : n, e.key === 'ArrowUp' ? 1 : -1, step, min, max)))
      }}
      onBlur={() => setTyped(null)}
    />
  )
}

export function Seconds({ value, onChange, min }: { value?: number; onChange: (v: number | undefined) => void; min?: number }) {
  return (
    <>
      <NumberField value={value} min={min} onChange={onChange} className={`${field} w-16`} />s
      {!!value && Math.abs(value) >= 60 && <span className="text-neutral-500">({duration(Math.abs(value))})</span>}
    </>
  )
}

// A time window [from, to), for a time window and a presence simulation.
export function Window({ p, set }: FormProps) {
  return (
    <Row>
      from <input type="time" value={p.from ?? ''} onChange={(e) => set({ from: e.target.value })} className={field} />
      to <input type="time" value={p.to ?? ''} onChange={(e) => set({ to: e.target.value })} className={field} />
      {p.from > p.to && <span className="text-neutral-500">wraps midnight</span>}
    </Row>
  )
}

const weekdays = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun']

// The days a time or sun trigger fires on.
export function Weekdays({ p, set }: FormProps) {
  const on: string[] = p.weekdays ?? []
  return (
    <div className="flex gap-0.5" title="None: every day">
      {weekdays.map((d) => (
        <button
          key={d}
          onClick={() => {
            const next = weekdays.filter((w) => (w === d) !== on.includes(w))
            set({ weekdays: next.length ? next : undefined })
          }}
          className={`rounded px-1 py-0.5 ${on.includes(d) ? 'bg-amber-400 text-neutral-900' : 'bg-neutral-800 text-neutral-400'}`}
        >
          {d}
        </button>
      ))}
    </div>
  )
}

export function CatchUp({ p, set }: FormProps) {
  return (
    <Check checked={!!p.catchUp} onChange={(catchUp) => set({ catchUp: catchUp || undefined })}>
      catch up: fire once if missed while Oiko was down
    </Check>
  )
}

const groups = { function: 'Functions', aggregate: 'Aggregates', flag: 'Flags', device: 'Devices' }

// Picked by Name, stored by id. A deleted target stays, marked as such.
// Watching Steps offer only a target with a Capability that fits the Step.
export function TargetPicker({
  value,
  onChange,
  targets,
  offer,
}: {
  value?: Target
  onChange: (t: Target) => void
  targets: Catalogue
  offer?: (e: Entry) => boolean
}) {
  const key = value ?? ''
  const deleted = key !== '' && !targets.get(key)
  const offered = offer ? targets.list.filter((e) => e.target === key || offer(e)) : targets.list
  return (
    <select value={key} onChange={(e) => onChange(e.target.value)} aria-label="Target" className={`${field} min-w-0 flex-1 ${deleted ? 'text-red-400' : ''}`}>
      <option value="" disabled>
        target…
      </option>
      {deleted && <option value={key}>⚠ {targets.name(key)}</option>}
      {Object.entries(groups).map(([kind, label]) => (
        <optgroup key={kind} label={label}>
          {offered
            .filter((e) => targetKind(e.target) === kind)
            .map((e) => (
              <option key={e.target} value={e.target}>
                {e.name}
              </option>
            ))}
        </optgroup>
      ))}
    </select>
  )
}

// For a collapsed Step's summary: the weekdays a trigger fires on, a Value's
// comparison. A deleted target reads as such, and rings its Step red.
export const days = (p: Params) => (p.weekdays?.length ? ` · ${p.weekdays.join(' ')}` : '')

export function valueSummary(p: Params, targets: Catalogue) {
  const cap = targets.get(p.target)?.capabilities.find((c) => c.key === p.capability)
  const op = ops[p.op as keyof typeof ops] ?? p.op
  const compared = p.capability && `${cap?.label ?? p.capability} ${op} ${display(p.value, cap)}${p.heldFor ? ` for ${duration(p.heldFor)}` : ''}`
  return [p.target && targets.name(p.target), compared].filter(Boolean)
}

export function CapabilityPicker({ caps, value, onChange }: { caps: Capability[]; value?: string; onChange: (c: Capability) => void }) {
  return (
    <select value={value ?? ''} onChange={(e) => onChange(caps.find((c) => c.key === e.target.value)!)} aria-label="Capability" className={field}>
      <option value="" disabled>
        capability…
      </option>
      {value && !caps.some((c) => c.key === value) && <option value={value}>{value}</option>}
      {caps.map((c) => (
        <option key={c.key} value={c.key}>
          {c.label}
        </option>
      ))}
    </select>
  )
}

// A value of Capability cap, or of an unknown one; undefined unsets it.
export function ValueInput({ cap, value, onChange, unset }: { cap?: Capability; value: unknown; onChange: (v: unknown) => void; unset?: boolean }) {
  if (cap?.type === 'binary' || typeof value === 'boolean') {
    return (
      <select
        value={value === undefined ? '' : String(value)}
        onChange={(e) => onChange(e.target.value === '' ? undefined : e.target.value === 'true')}
        className={field}
      >
        {unset && <option value="">—</option>}
        <option value="true">on</option>
        <option value="false">off</option>
      </select>
    )
  }
  if (cap?.type === 'enum') {
    return (
      <select value={value === undefined ? '' : String(value)} onChange={(e) => onChange(e.target.value || undefined)} className={field}>
        <option value="" disabled={!unset}>
          —
        </option>
        {cap.options?.map((o) => (
          <option key={o}>{o}</option>
        ))}
      </select>
    )
  }
  if (cap?.type === 'numeric' || typeof value === 'number') {
    return (
      <>
        <NumberField value={value as number | undefined} min={cap?.min} max={cap?.max} step={cap?.step} onChange={onChange} className={`${field} w-20`} />
        {cap?.unit}
      </>
    )
  }
  return <input value={String(value ?? '')} onChange={(e) => onChange(e.target.value || undefined)} className={`${field} w-24`} />
}

// A value a comparison starts from once its Capability is picked.
function defaultValue(c: Capability): unknown {
  return c.type === 'binary' ? true : c.type === 'numeric' ? (c.min ?? 0) : c.type === 'enum' ? c.options?.[0] : ''
}

const ops = { eq: '=', ne: '≠', lt: '<', le: '≤', gt: '>', ge: '≥' }

// A Value compared, for a value trigger and a value condition.
export function ValueForm({ p, set, targets, heldFor }: FormProps & { heldFor: boolean }) {
  const caps = (targets.get(p.target)?.capabilities ?? []).filter(hasValue)
  const cap = caps.find((c) => c.key === p.capability)
  const numeric = cap?.type === 'numeric' || typeof p.value === 'number'
  // What picking Capability c sets: a comparison it starts from.
  const compare = (c?: Capability) => (c ? { capability: c.key, value: defaultValue(c), op: 'eq' } : { capability: undefined })
  return (
    <>
      <Row>
        <TargetPicker
          value={p.target}
          offer={(e) => e.capabilities.some(hasValue)}
          targets={targets}
          onChange={(target) => set({ target, ...compare(soleCapability((targets.get(target)?.capabilities ?? []).filter(hasValue))) })}
        />
      </Row>
      {!p.target && heldFor && <Hint>Fires when a state changes: motion, a door, a temperature, a Flag… For a button, use an Event trigger.</Hint>}
      <Row>
        <CapabilityPicker caps={caps} value={p.capability} onChange={(c) => set(compare(c))} />
        <select value={p.op} onChange={(e) => set({ op: e.target.value })} aria-label="Comparison" className={field}>
          {Object.entries(ops)
            .filter(([op]) => numeric || op === 'eq' || op === 'ne')
            .map(([op, sign]) => (
              <option key={op} value={op}>
                {sign}
              </option>
            ))}
        </select>
        <ValueInput cap={cap} value={p.value} onChange={(value) => set({ value })} />
      </Row>
      {heldFor && (
        <Row>
          held for <Seconds value={p.heldFor} min={0} onChange={(heldFor) => set({ heldFor: heldFor || undefined })} />
        </Row>
      )}
    </>
  )
}
