import type { StepKind } from '.'
import { field, Hint, Row } from './fields'

// Sends a Notification to the home's Telegram chat, then goes on.
export const notify: StepKind = {
  label: 'Notify',
  group: 'Action',
  inputs: ['in'],
  outputs: ['then'],
  params: () => ({ title: '', message: '' }),
  Form: ({ p, set }) => (
    <>
      <Row>
        <input
          value={p.title ?? ''}
          onChange={(e) => set({ title: e.target.value })}
          placeholder="title"
          aria-label="Title"
          className={`${field} min-w-0 flex-1`}
        />
      </Row>
      <textarea
        value={p.message ?? ''}
        onChange={(e) => set({ message: e.target.value })}
        placeholder="message"
        aria-label="Message"
        rows={2}
        className={`${field} w-full`}
      />
      <Hint>{'{name}'} reads the Name of what started the Run.</Hint>
    </>
  ),
  summary: (p) => [p.title, p.message].filter(Boolean),
  evidence: (r) => (r.notification ? [`sent: ${[r.notification.title, r.notification.message].filter(Boolean).join(' — ')}`] : []),
}
