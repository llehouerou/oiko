import { duration } from '../model'
import { clock } from '../runtime'
import type { StepKind } from '.'
import { field, NumberField, Row, Seconds, Window, type FormProps } from './fields'

// While enabled, switches on and off at random every day, in blocks drawn
// within its window. It remembers the day's schedule.
export const presenceSimulation: StepKind = {
  label: 'Presence simulation',
  group: 'Timing',
  inputs: ['enable', 'disable'],
  outputs: ['on', 'off'],
  params: () => ({ from: '19:00', to: '23:30', minBlocks: 4, maxBlocks: 6, minDuration: 1800, maxDuration: 2700 }),
  Form: PresenceForm,
  summary: (p) => [`${p.from}–${p.to}`, `${p.minBlocks}–${p.maxBlocks} blocks of ${duration(p.minDuration ?? 0)} to ${duration(p.maxDuration ?? 0)}`],
  stateful: () => true,
  badge: (s, _, now) => {
    if (!s.enabled) return null
    const next = s.schedule?.[0]
    return next ? `enabled — ${next.on ? 'on' : 'off'} at ${clock(next.at, now)}` : `enabled${s.deadline ? ` — draws ${clock(s.deadline, now)}` : ''}`
  },
}

function PresenceForm(props: FormProps) {
  const { p, set } = props
  return (
    <>
      <Window {...props} />
      <Row>
        <NumberField value={p.minBlocks} min={1} onChange={(minBlocks) => set({ minBlocks })} className={`${field} w-12`} />–
        <NumberField value={p.maxBlocks} min={1} onChange={(maxBlocks) => set({ maxBlocks })} className={`${field} w-12`} />
        blocks
      </Row>
      <Row>
        of <Seconds value={p.minDuration} min={0} onChange={(minDuration) => set({ minDuration })} />
      </Row>
      <Row>
        to <Seconds value={p.maxDuration} min={0} onChange={(maxDuration) => set({ maxDuration })} />
      </Row>
    </>
  )
}
