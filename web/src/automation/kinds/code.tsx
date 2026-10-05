import type { Target } from '../../types'
import { unused } from '../model'
import type { StepKind } from '.'
import { field, Row, TargetPicker, type FormProps } from './fields'

// Runs def run(trigger, state) in Starlark, on the targets bound to its
// aliases, and fires the outputs it declares. It remembers its state.
export const code: StepKind = {
  label: 'Code',
  group: 'Action',
  inputs: ['run'],
  outputs: (p) => p.outputs ?? [],
  params: () => ({ source: 'def run(trigger, state):\n    return None\n', outputs: [], bindings: {} }),
  Form: CodeForm,
  summary: (p, targets) => Object.entries((p.bindings ?? {}) as Record<string, Target>).map(([alias, t]) => `${alias} → ${t ? targets.name(t) : ''}`),
  wide: true,
  stateful: () => true,
  badge: (s) => {
    const text = JSON.stringify(s.state)
    return text && text !== '{}' ? `state ${text.length > 40 ? `${text.slice(0, 39)}…` : text}` : null
  },
}

function CodeForm({ p, set, targets }: FormProps) {
  const bindings = Object.entries((p.bindings ?? {}) as Record<string, Target>)
  const outputs: string[] = p.outputs ?? []
  const setBindings = (entries: [string, Target][]) => set({ bindings: Object.fromEntries(entries) })
  const label = 'text-[10px] font-semibold tracking-wide text-neutral-500 uppercase'
  return (
    <>
      <div className={label}>Bindings</div>
      {bindings.map(([alias, t], i) => (
        <Row key={i}>
          <input
            value={alias}
            aria-label="Alias"
            onChange={(e) => setBindings(bindings.map((b, j) => (j === i ? [e.target.value, t] : b)))}
            className={`${field} w-24 font-mono`}
          />
          →
          <TargetPicker value={t} targets={targets} onChange={(n) => setBindings(bindings.map((b, j) => (j === i ? [alias, n] : b)))} />
          <button onClick={() => setBindings(bindings.filter((_, j) => j !== i))} aria-label="Remove binding" className="text-neutral-500 hover:text-red-400">
            ✕
          </button>
        </Row>
      ))}
      <button
        onClick={() =>
          setBindings([
            ...bindings,
            [
              unused(
                't',
                bindings.map(([a]) => a),
              ),
              '',
            ],
          ])
        }
        className="text-amber-400 hover:text-amber-300"
      >
        + binding
      </button>
      <div className={label}>Outputs</div>
      <Row>
        {outputs.map((o, i) => (
          <span key={i} className="flex items-center">
            <input
              value={o}
              aria-label="Output"
              onChange={(e) => set({ outputs: outputs.map((x, j) => (j === i ? e.target.value : x)) })}
              className={`${field} w-20 font-mono`}
            />
            <button
              onClick={() => set({ outputs: outputs.filter((_, j) => j !== i) })}
              aria-label="Remove output"
              className="px-0.5 text-neutral-500 hover:text-red-400"
            >
              ✕
            </button>
          </span>
        ))}
        <button onClick={() => set({ outputs: [...outputs, ''] })} className="text-amber-400 hover:text-amber-300">
          + output
        </button>
      </Row>
      <textarea
        value={p.source ?? ''}
        onChange={(e) => set({ source: e.target.value })}
        spellCheck={false}
        aria-label="Source"
        rows={Math.max(6, (p.source ?? '').split('\n').length + 1)}
        className="w-full rounded bg-neutral-950 p-2 font-mono text-emerald-200"
      />
      <p className="text-neutral-500">
        <code>value(alias, capability)</code> · <code>command(alias, transition=None, **capabilities)</code> · <code>now</code> · return an output name, a list
        of them, or None
      </p>
    </>
  )
}
