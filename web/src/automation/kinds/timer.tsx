import { duration } from '../model'
import { clock, pending } from '../runtime'
import type { StepKind } from '.'
import { field, Row, Seconds, type FormProps } from './fields'

// Fires duration after start; a start while it runs restarts it or keeps it.
// It remembers its deadline.
export const timer: StepKind = {
  label: 'Timer',
  group: 'Timing',
  inputs: ['start', 'cancel'],
  outputs: ['fired'],
  params: () => ({ duration: 300, reentry: 'restart' }),
  Form: TimerForm,
  summary: (p) => [`fires after ${duration(p.duration ?? 0)}`, `start while running ${p.reentry === 'keep' ? 'keeps' : 'restarts'} it`],
  stateful: () => true,
  badge: (s, _, now) => {
    const due = pending(s, now)
    return due ? `⏱ running — due ${clock(due, now)}` : null
  },
}

function TimerForm({ p, set }: FormProps) {
  return (
    <>
      <Row>
        fires after <Seconds value={p.duration} min={0} onChange={(duration) => set({ duration })} />
      </Row>
      <Row>
        <select value={p.reentry} onChange={(e) => set({ reentry: e.target.value })} className={field} aria-label="Re-entry">
          <option value="restart">start while running restarts it</option>
          <option value="keep">start while running keeps it</option>
        </select>
      </Row>
    </>
  )
}
