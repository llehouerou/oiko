import type { StepKind } from '.'
import { ValueForm, valueSummary } from './fields'

// Fires true or false whether the last known Value satisfies its comparison,
// and neither when there is none.
export const valueCondition: StepKind = {
  label: 'Value is',
  group: 'Condition',
  inputs: ['in'],
  outputs: ['true', 'false'],
  params: () => ({ op: 'eq', value: true }),
  Form: (props) => <ValueForm {...props} heldFor={false} />,
  summary: valueSummary,
}
