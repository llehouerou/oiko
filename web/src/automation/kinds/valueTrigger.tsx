import { clock, pending } from '../runtime'
import type { StepKind } from '.'
import { ValueForm, valueSummary } from './fields'

// Starts a Run when a Value comes to satisfy its comparison, and has held it
// for heldFor; it remembers that deadline.
export const valueTrigger: StepKind = {
  label: 'Value',
  group: 'Trigger',
  inputs: [],
  outputs: ['out'],
  params: () => ({ op: 'eq', value: true }),
  Form: (props) => <ValueForm {...props} heldFor />,
  summary: valueSummary,
  stateful: (p) => p.heldFor > 0,
  badge: (s, _, now) => {
    const due = pending(s, now)
    return due ? `⏳ held — fires ${clock(due, now)}` : null
  },
}
