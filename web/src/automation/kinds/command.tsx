import { ColorPicker, display, RangeSlider, Switch, type HS } from '../../controls'
import { titleIfTruncated } from '../../truncated'
import { roles } from '../../roles'
import type { Capability, Target } from '../../types'
import { duration, patch, type Params } from '../model'
import type { Catalogue } from '../../targets'
import { CommandHistory } from '../Runs'
import type { StepKind } from '.'
import { field, NumberField, Row, TargetPicker, ValueInput, type FormProps } from './fields'

// Issues one Command per target, then goes on.
export const command: StepKind = {
  label: 'Command',
  group: 'Action',
  inputs: ['in'],
  outputs: ['then'],
  params: () => ({ targets: [], values: {} }),
  Form: CommandForm,
  summary: commandSummary,
}

// Its targets, then what it sets: on/off and a light's adjustments bare, anything
// else after its label, and the transition.
function commandSummary(p: Params, targets: Catalogue) {
  const picked: Target[] = p.targets ?? []
  const chosen = picked.flatMap((t) => targets.get(t) ?? [])
  const caps = chosen.flatMap((e) => e.capabilities)
  const rs = chosen.map((e) => roles(e.kind, e.capabilities))
  const onOff = (key: string) => rs.some((r) => r.control?.key === key && r.control.type === 'binary')
  const bare = (key: string) => onOff(key) || rs.some((r) => r.adjustments.some((c) => c.key === key))
  const first = (key: string) => (onOff(key) ? 0 : 1) // on/off leads
  const entries = Object.entries((p.values ?? {}) as Params).sort(([a], [b]) => first(a) - first(b))
  const values = entries.map(([key, v]) => {
    const cap = caps.find((c) => c.key === key)
    const text = v === 'toggle' ? 'toggle' : key === 'color_hs' ? 'colour' : display(v, cap)
    return bare(key) ? text : `${cap?.label ?? key} ${text}`
  })
  if (p.transition) values.push(`in ${duration(p.transition)}`)
  return [picked.map((t) => targets.name(t)).join(', '), values.join(', ')].filter(Boolean)
}

// One Command per target, with one set of values: on / off / toggle, sliders
// like a Function tile's, each switched on to be sent, and a transition.
function CommandForm({ p, set, targets: catalogue }: FormProps) {
  const targets: Target[] = p.targets ?? []
  const values: Params = p.values ?? {}
  const caps: Capability[] = []
  for (const c of targets.flatMap((t) => catalogue.get(t)?.capabilities ?? [])) {
    if (
      c.access.settable &&
      c.category === 'primary' &&
      (c.type !== 'composite' || c.key === 'color_hs') &&
      c.type !== 'list' &&
      !caps.some((x) => x.key === c.key)
    )
      caps.push(c)
  }
  for (const key of Object.keys(values)) if (!caps.some((c) => c.key === key)) caps.push({ key, label: key } as Capability)
  // A light shows a colour or a white of some temperature, never both (Zigbee's
  // Color Control has a single ColorMode): setting one drops the other.
  const rival: Record<string, string> = { color_hs: 'color_temp', color_temp: 'color_hs' }
  const setValue = (key: string, v: unknown) =>
    set({ values: patch(values, { [key]: v, ...(v !== undefined && rival[key] ? { [rival[key]]: undefined } : {}) }) })
  return (
    <>
      {[...targets, undefined].map((t, i) => (
        <Row key={i}>
          <TargetPicker value={t} targets={catalogue} onChange={(n) => set({ targets: t ? targets.map((x, j) => (j === i ? n : x)) : [...targets, n] })} />
          {t && (
            <button
              onClick={() => set({ targets: targets.filter((_, j) => j !== i) })}
              aria-label="Remove target"
              className="text-neutral-500 hover:text-red-400"
            >
              ✕
            </button>
          )}
          {t && <CommandHistory target={t} />}
        </Row>
      ))}
      {caps.map((c) => {
        const v = values[c.key]
        if (c.key === 'color_hs')
          return (
            <div key={c.key} className="flex items-center justify-between gap-1 text-neutral-400">
              <span className="flex items-center gap-1">
                <Switch small on={v !== undefined} onChange={(on) => setValue(c.key, on ? { hue: 0, saturation: 100 } : undefined)} label="Send Color" />
                Color
              </span>
              <ColorPicker value={v as HS | undefined} onChange={(n) => setValue(c.key, n)} label={c.label} />
            </div>
          )
        return c.type === 'numeric' && c.min != null && c.max != null && (v === undefined || typeof v === 'number') ? (
          <RangeSlider
            key={c.key}
            cap={c}
            value={v}
            onChange={(n) => setValue(c.key, n)}
            lead={<Switch small on={v !== undefined} onChange={(on) => setValue(c.key, on ? c.max : undefined)} label={`Send ${c.label}`} />}
          />
        ) : (
          <Row key={c.key}>
            <span className="w-24 truncate" onMouseEnter={titleIfTruncated}>
              {c.label}
            </span>
            <CommandValue cap={c} value={v} onChange={(n) => setValue(c.key, n)} />
          </Row>
        )
      })}
      <Row>
        <span className="w-24">transition</span>
        <NumberField
          value={p.transition}
          min={0}
          max={6553.5}
          step={0.1}
          onChange={(transition) => set({ transition: transition || undefined })}
          className={`${field} w-20`}
        />
        s
      </Row>
    </>
  )
}

function CommandValue({ cap, value, onChange }: { cap: Capability; value: unknown; onChange: (v: unknown) => void }) {
  if (cap.type === 'binary' || typeof value === 'boolean' || value === 'toggle') {
    return (
      <select
        value={value === undefined ? '' : String(value)}
        onChange={(e) => onChange({ '': undefined, true: true, false: false, toggle: 'toggle' }[e.target.value])}
        className={field}
      >
        <option value="">—</option>
        <option value="true">on</option>
        <option value="false">off</option>
        <option value="toggle">toggle</option>
      </select>
    )
  }
  return <ValueInput cap={cap} value={value} onChange={onChange} unset />
}
