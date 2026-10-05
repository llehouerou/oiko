import { clock, pending } from '../runtime'
import type { StepKind } from '.'
import { CatchUp, days, field, Row, Weekdays, type FormProps } from './fields'

// Starts a Run at a time of day, on some weekdays. Its catch-up mark restarts
// from now on every save, so it is not worth reporting as state.
export const timeTrigger: StepKind = {
  label: 'Time of day',
  group: 'Trigger',
  inputs: [],
  outputs: ['out'],
  params: () => ({ at: '06:00' }),
  Form: TimeForm,
  summary: (p) => [`at ${p.at}${days(p)}`],
  badge: (s, _, now) => {
    const due = pending(s, now)
    return due ? `next ${clock(due, now)}` : null
  },
}

function TimeForm(props: FormProps) {
  const { p, set } = props
  return (
    <>
      <Row>
        at <input type="time" value={p.at ?? ''} onChange={(e) => set({ at: e.target.value })} className={field} />
      </Row>
      <Weekdays {...props} />
      <CatchUp {...props} />
    </>
  )
}
