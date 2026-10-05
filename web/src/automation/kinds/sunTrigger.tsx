import { duration } from '../model'
import type { StepKind } from '.'
import { CatchUp, days, field, Row, Seconds, Weekdays, type FormProps } from './fields'
import { timeTrigger } from './timeTrigger'

// Starts a Run at dawn, sunrise, sunset or dusk plus an offset, on some
// weekdays; it shows its next occurrence as a time trigger does.
export const sunTrigger: StepKind = {
  label: 'Sun',
  group: 'Trigger',
  inputs: [],
  outputs: ['out'],
  params: () => ({ event: 'sunset' }),
  Form: SunForm,
  summary: (p) => [`${p.event}${p.offset ? ` ${p.offset > 0 ? '+' : '−'} ${duration(Math.abs(p.offset))}` : ''}${days(p)}`],
  badge: timeTrigger.badge,
}

function SunForm(props: FormProps) {
  const { p, set } = props
  return (
    <>
      <Row>
        <select value={p.event} onChange={(e) => set({ event: e.target.value })} className={field}>
          {['dawn', 'sunrise', 'sunset', 'dusk'].map((o) => (
            <option key={o}>{o}</option>
          ))}
        </select>
        offset <Seconds value={p.offset} onChange={(offset) => set({ offset: offset || undefined })} />
      </Row>
      <Weekdays {...props} />
      <CatchUp {...props} />
    </>
  )
}
