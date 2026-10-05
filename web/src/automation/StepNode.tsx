// A Step on the canvas: a node whose body is its params form, with its named
// handles on its edges. Only the open Step shows its form, one at a time; the
// others collapse to their header and a summary of their params. Clicking a
// collapsed Step, or the bare part of the open one's header, toggles it.
// Zoomed out, a Step collapses to its Name.

import { Handle, Position, useReactFlow, useStore, useUpdateNodeInternals, type NodeProps } from '@xyflow/react'
import { createContext, useContext, useEffect, type MouseEvent } from 'react'
import { catalogue, handles, type Kind, type StepKind } from './kinds'
import { patch, targetsOf, type Params, type StepNode as Node } from './model'
import { Evidence, Inspect, StateBadge } from './Runs'
import { useCatalogue } from '../store'
import { titleIfTruncated } from '../truncated'

// The Step the Automation is broken at, if any.
export const BrokenStep = createContext<string | undefined>(undefined)

// The one Step whose form is shown, if any.
export const OpenStep = createContext<{ open: string | null; setOpen: (id: string | null) => void }>({ open: null, setOpen: () => {} })

const ZOOM_SUMMARY = 0.5
const groupColor = { Trigger: 'border-violet-500', Condition: 'border-sky-500', Timing: 'border-amber-500', Action: 'border-emerald-500' }

export function StepNode({ id, data }: NodeProps<Node>) {
  const { updateNodeData, deleteElements } = useReactFlow()
  const zoomedOut = useStore((s) => s.transform[2] < ZOOM_SUMMARY)
  const targets = useCatalogue()
  const broken = useContext(BrokenStep) === id
  const { path } = useContext(Inspect)
  const { open, setOpen } = useContext(OpenStep)
  const expanded = open === id
  const deleted = targetsOf(data.params).some((t) => t && !targets.get(t))
  const c: StepKind = catalogue[data.kind]
  const unreached = path && !path.reached.has(id) ? 'opacity-30' : ''
  const frame = `rounded-lg border-t-4 bg-neutral-900 text-xs shadow-lg ${groupColor[c.group]} ${broken || deleted ? 'ring-2 ring-red-500' : ''} ${unreached}`
  const set = (changes: Params) => updateNodeData(id, { params: patch(data.params, changes) })
  // Tall enough, even collapsed, to keep its handles' labels apart.
  const { inputs, outputs } = handles(data.kind, data.params)
  const minHeight = `${Math.max(inputs.length, new Set(outputs.filter(Boolean)).size) * 1.25}rem`

  const width = c.wide ? 'w-[28rem]' : 'w-72'
  if (zoomedOut) {
    return (
      <div className={`${frame} ${width} px-4 py-3 text-3xl`}>
        <Handles id={id} kind={data.kind} params={data.params} labelled={false} />
        <div className="truncate font-medium" onMouseEnter={titleIfTruncated}>
          {data.name || c.label}
        </div>
      </div>
    )
  }
  // A click on a field or a button does its own thing.
  const toggle = (e: MouseEvent) => {
    if (!(e.target as Element).closest('input, button, select, textarea')) setOpen(expanded ? null : id)
  }
  const summary = expanded ? [] : c.summary(data.params, targets)
  return (
    <div className={`${frame} ${width}`} style={{ minHeight }}>
      <Handles id={id} kind={data.kind} params={data.params} labelled />
      <div
        onClick={toggle}
        title={expanded ? 'Collapse' : 'Expand'}
        className={`cursor-pointer rounded-t-md hover:bg-neutral-800/60 ${expanded ? 'border-b border-neutral-800' : 'rounded-b-lg'}`}
      >
        <header className="flex items-center gap-2 px-3 py-1.5">
          <button
            onClick={() => setOpen(expanded ? null : id)}
            aria-expanded={expanded}
            aria-label={expanded ? 'Collapse step' : 'Expand step'}
            className="flex shrink-0 items-center gap-1 text-[10px] tracking-wide text-neutral-500 uppercase hover:text-neutral-300"
          >
            <span className={`transition-transform ${expanded ? 'rotate-90' : ''}`}>▸</span>
            {c.label}
          </button>
          {expanded ? (
            <input
              value={data.name}
              placeholder="name"
              aria-label="Step name"
              onChange={(e) => updateNodeData(id, { name: e.target.value })}
              className="nodrag min-w-0 flex-1 bg-transparent font-medium focus:outline-none"
            />
          ) : (
            <span className="min-w-0 flex-1 truncate font-medium" onMouseEnter={titleIfTruncated}>
              {data.name}
            </span>
          )}
          <StateBadge id={id} params={data.params} badge={c.badge} />
          <button onClick={() => deleteElements({ nodes: [{ id }] })} aria-label="Delete step" className="text-neutral-500 hover:text-red-400">
            ✕
          </button>
        </header>
        {summary.length > 0 && (
          <div className="space-y-0.5 px-3 pb-2 text-neutral-400">
            {summary.map((line, i) => (
              <div key={i} className="truncate" onMouseEnter={titleIfTruncated}>
                {line}
              </div>
            ))}
          </div>
        )}
      </div>
      {expanded && (
        <div className="nodrag nowheel cursor-default space-y-2 px-3 py-2">
          <c.Form id={id} p={data.params} set={set} targets={targets} />
        </div>
      )}
      <Evidence id={id} own={c.evidence} />
    </div>
  )
}

// Inputs on the left, outputs on the right, each named after its handle; a
// Code Step's outputs are handles as soon as they are typed. A handle the
// picked Run fired is ringed.
function Handles({ id, kind, params, labelled }: { id: string; kind: Kind; params: Params; labelled: boolean }) {
  const { inputs, outputs } = handles(kind, params)
  const named = [...new Set(outputs.filter(Boolean))]
  const { path } = useContext(Inspect)
  const update = useUpdateNodeInternals()
  const key = named.join('\n')
  useEffect(() => update(id), [id, key, labelled, update])
  const top = (i: number, n: number) => `${((i + 1) / (n + 1)) * 100}%`
  return (
    <>
      {inputs.map((h, i) => (
        <Handle key={h} id={h} type="target" position={Position.Left} style={{ top: top(i, inputs.length) }} className="h-3! w-3! bg-sky-400!">
          {labelled && <span className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-[10px] text-sky-300">{h}</span>}
        </Handle>
      ))}
      {named.map((h, i) => {
        const fired = path?.fired.has(`${id}.${h}`)
        return (
          <Handle
            key={h}
            id={h}
            type="source"
            position={Position.Right}
            style={{ top: top(i, named.length) }}
            className={`h-3! w-3! ${h === 'false' ? 'bg-rose-500!' : 'bg-emerald-400!'} ${fired ? 'animate-pulse ring-4 ring-amber-300' : ''}`}
          >
            {labelled && (
              <span
                className={`pointer-events-none absolute top-1/2 ${fired ? 'left-5' : 'left-3'} -translate-y-1/2 text-[10px] whitespace-nowrap ${h === 'false' ? 'text-rose-400' : 'text-neutral-300'}`}
              >
                {h}
              </span>
            )}
          </Handle>
        )
      })}
    </>
  )
}
