import { duration } from '../model'
import { clock, pending } from '../runtime'
import type { StepKind } from '.'
import { targetKind } from '../../targets'
import { field, Hint, Row, Seconds, TargetPicker, type FormProps } from './fields'

// Starts a Run when a Device's, an Aggregate's or a Flag's Availability comes
// to be, or not to be, one state, and has stayed so for heldFor; it remembers
// that deadline.
export const availabilityTrigger: StepKind = {
  label: 'Availability',
  group: 'Trigger',
  inputs: [],
  outputs: ['out'],
  params: () => ({ op: 'ne', availability: 'online' }),
  Form: AvailabilityForm,
  summary: (p, targets) =>
    [p.target && targets.name(p.target), `${p.op === 'ne' ? 'not ' : ''}${p.availability}${p.heldFor ? ` for ${duration(p.heldFor)}` : ''}`].filter(Boolean),
  stateful: (p) => p.heldFor > 0,
  badge: (s, _, now) => {
    const due = pending(s, now)
    return due ? `⏳ held — fires ${clock(due, now)}` : null
  },
}

function AvailabilityForm({ p, set, targets }: FormProps) {
  return (
    <>
      <Row>
        {/* A Function's Availability is its Device's. */}
        <TargetPicker value={p.target} targets={targets} offer={(e) => targetKind(e.target) !== 'function'} onChange={(target) => set({ target })} />
      </Row>
      {!p.target && <Hint>Fires when a Device goes offline, or comes back… Leaving unknown, as a Bridge connects, fires nothing.</Hint>}
      <Row>
        <select value={p.op} onChange={(e) => set({ op: e.target.value })} aria-label="Comparison" className={field}>
          <option value="eq">is</option>
          <option value="ne">is not</option>
        </select>
        <select value={p.availability} onChange={(e) => set({ availability: e.target.value })} aria-label="Availability" className={field}>
          <option>online</option>
          <option>offline</option>
          <option>unknown</option>
        </select>
      </Row>
      <Row>
        held for <Seconds value={p.heldFor} min={0} onChange={(heldFor) => set({ heldFor: heldFor || undefined })} />
      </Row>
    </>
  )
}
