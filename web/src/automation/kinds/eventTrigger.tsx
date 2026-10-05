import { useState } from 'react'
import { emitsEvents, soleCapability } from '../model'
import type { StepKind } from '.'
import { CapabilityPicker, field, Hint, Row, TargetPicker, type FormProps } from './fields'

// Starts a Run when a Capability of its target emits one of its events.
export const eventTrigger: StepKind = {
  label: 'Event',
  group: 'Trigger',
  inputs: [],
  outputs: ['out'],
  params: () => ({ events: [] }),
  Form: EventForm,
  summary: (p, targets) => [p.target && targets.name(p.target), (p.events ?? []).join(', ')].filter(Boolean),
}

function EventForm({ p, set, targets }: FormProps) {
  const known = targets.get(p.target)
  const caps = (known?.capabilities ?? []).filter(emitsEvents)
  const cap = caps.find((c) => c.key === p.capability)
  const events: string[] = p.events ?? []
  const [typed, setTyped] = useState<string | null>(null) // keeps a trailing comma while typing
  return (
    <>
      <Row>
        <TargetPicker
          value={p.target}
          offer={(e) => e.capabilities.some(emitsEvents)}
          targets={targets}
          onChange={(target) => set({ target, capability: soleCapability((targets.get(target)?.capabilities ?? []).filter(emitsEvents))?.key, events: [] })}
        />
      </Row>
      {!p.target && <Hint>Fires when a button or a remote is pressed. For motion, a door or a Flag, use a Value trigger.</Hint>}
      {known && !caps.length && <Hint>This target emits no events. To react to its state, such as motion, use a Value trigger.</Hint>}
      {p.target && (caps.length > 0 || !known) && (
        <Row>
          <CapabilityPicker caps={caps} value={p.capability} onChange={(c) => set({ capability: c.key, events: [] })} />
          is one of
        </Row>
      )}
      {cap?.options ? (
        // The chosen events as chips; picking one of the others adds it.
        <div className="flex flex-wrap items-center gap-1">
          {events.map((o) => (
            <span key={o} className="flex items-center gap-1 rounded bg-neutral-800 py-0.5 pr-1 pl-2 text-neutral-200">
              {o}
              <button onClick={() => set({ events: events.filter((x) => x !== o) })} aria-label={`Remove ${o}`} className="text-neutral-500 hover:text-red-400">
                ✕
              </button>
            </span>
          ))}
          {cap.options.some((o) => !events.includes(o)) && (
            <select value="" onChange={(e) => set({ events: [...events, e.target.value] })} aria-label="Add event" className={field}>
              <option value="" disabled>
                + event…
              </option>
              {cap.options
                .filter((o) => !events.includes(o))
                .map((o) => (
                  <option key={o}>{o}</option>
                ))}
            </select>
          )}
        </div>
      ) : (
        p.capability && (
          <input
            value={typed ?? events.join(', ')}
            placeholder="events, comma-separated: single, double…"
            aria-label="Events"
            onChange={(e) => {
              setTyped(e.target.value)
              set({
                events: e.target.value
                  .split(',')
                  .map((s) => s.trim())
                  .filter(Boolean),
              })
            }}
            onBlur={() => setTyped(null)}
            className={`${field} w-full`}
          />
        )
      )}
    </>
  )
}
