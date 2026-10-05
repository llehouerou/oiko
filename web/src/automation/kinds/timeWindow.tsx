import type { StepKind } from '.'
import { Window } from './fields'

// Fires true or false whether the Run's time of day is within its window: the
// handle fired is the verdict, which its evidence spells out.
export const timeWindow: StepKind = {
  label: 'Time window',
  group: 'Condition',
  inputs: ['in'],
  outputs: ['true', 'false'],
  params: () => ({ from: '21:00', to: '06:00' }),
  Form: Window,
  summary: (p) => [`${p.from}–${p.to}`],
  evidence: (r) => [r.fired?.includes('true') ? 'inside the window' : 'outside the window'],
}
