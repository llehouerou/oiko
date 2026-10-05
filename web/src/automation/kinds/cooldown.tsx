import { duration } from '../model'
import { clock } from '../runtime'
import type { StepKind } from '.'
import { Check, Row, Seconds, type FormProps } from './fields'

// Lets a Run through at most once per duration, per triggering target or
// not. It remembers when it last did.
export const cooldown: StepKind = {
  label: 'Cooldown',
  group: 'Timing',
  inputs: ['in'],
  outputs: ['out'],
  params: () => ({ duration: 60 }),
  Form: CooldownForm,
  summary: (p) => [`once per ${duration(p.duration ?? 0)}${p.perTarget ? ' per triggering target' : ''}`],
  stateful: () => true,
  badge: (s, p, now) => {
    const opens = (s.passed ?? []).map((x) => Date.parse(x.at) + (p.duration ?? 0) * 1000).filter((t) => t > now.getTime())
    if (!opens.length) return null
    if (p.perTarget) return `closed for ${opens.length} target${opens.length > 1 ? 's' : ''}`
    return `closed — opens ${clock(new Date(opens[0]!).toISOString(), now)}`
  },
}

function CooldownForm({ p, set }: FormProps) {
  return (
    <>
      <Row>
        once per <Seconds value={p.duration} min={0} onChange={(duration) => set({ duration })} />
      </Row>
      <Check checked={!!p.perTarget} onChange={(perTarget) => set({ perTarget: perTarget || undefined })}>
        per triggering target
      </Check>
    </>
  )
}
