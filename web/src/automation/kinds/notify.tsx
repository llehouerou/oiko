import type { StepKind } from '.'
import { Check, field, Hint, Row } from './fields'

// Sends a Notification to the home's Telegram chat, then goes on; with video, as the video of the
// Recording whose Event started the Run (ADR 0039).
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
      <Check checked={!!p.video} onChange={(video) => set({ video: video || undefined })}>
        with the video of the Recording that started the Run
      </Check>
      <Hint>{'{name}'} reads the Name of what started the Run. A video needs a camera's recording Event as the trigger.</Hint>
    </>
  ),
  summary: (p) => [p.title, p.message, p.video && '+ video'].filter(Boolean),
  evidence: (r) =>
    r.notification ? [`sent: ${[r.notification.title, r.notification.message].filter(Boolean).join(' — ')}${r.notification.recording ? ' + video' : ''}`] : [],
}
