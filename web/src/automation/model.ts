// The Automation document (internal/automation's Document) and what the
// editor does with it. It maps 1:1 onto React Flow: a Step is a node with the
// same id and position, an edge joins named handles.

import type { Connection, Edge as FlowEdge, Node as FlowNode } from '@xyflow/react'
import type { Target } from '../targets'
import type { Capability } from '../types'
import type { Kind } from './kinds'

export interface Port {
  step: string
  handle: string
}

export interface Edge {
  from: Port
  to: Port
}

// A Step's params are free JSON whose shape depends on its kind.
export type Params = Record<string, any>

export interface Step {
  id: string
  kind: Kind
  name: string
  params: Params
  position: { x: number; y: number }
}

export interface Document {
  id: string
  name: string
  enabled: boolean
  steps: Step[]
  edges: Edge[]
}

// patch applies changes to params; undefined removes a key, so what is left
// at its default is not written.
export function patch(params: Params, changes: Params): Params {
  return Object.fromEntries(Object.entries({ ...params, ...changes }).filter(([, v]) => v !== undefined))
}

// ---- React Flow

export type StepData = Omit<Step, 'id' | 'position'>
export type StepNode = FlowNode<StepData, 'step'>

export const flowEdge = (c: Connection | FlowEdge): FlowEdge => ({
  id: `${c.source}.${c.sourceHandle}>${c.target}.${c.targetHandle}`,
  source: c.source,
  sourceHandle: c.sourceHandle,
  target: c.target,
  targetHandle: c.targetHandle,
})

export function toFlow(doc: Document): { nodes: StepNode[]; edges: FlowEdge[] } {
  return {
    nodes: doc.steps.map(({ id, position, ...data }) => ({ id, type: 'step', position, data })),
    edges: doc.edges.map((e) => flowEdge({ source: e.from.step, sourceHandle: e.from.handle, target: e.to.step, targetHandle: e.to.handle })),
  }
}

// toDocument is doc with the Steps and edges of the graph, in their order:
// fan-out follows edge order.
export function toDocument(doc: Document, nodes: StepNode[], edges: FlowEdge[]): Document {
  return {
    id: doc.id,
    name: doc.name,
    enabled: doc.enabled,
    steps: nodes.map((n) => ({ id: n.id, kind: n.data.kind, name: n.data.name, params: n.data.params, position: { x: n.position.x, y: n.position.y } })),
    edges: edges.map((e) => ({ from: { step: e.source, handle: e.sourceHandle ?? '' }, to: { step: e.target, handle: e.targetHandle ?? '' } })),
  }
}

// spread lays out the Steps of a Document authored without positions, all
// at the origin, in rows. The layout is kept by the next Save of an edit.
export function spread(doc: Document): Document {
  if (doc.steps.length < 2 || doc.steps.some((s) => s.position.x || s.position.y)) return doc
  return { ...doc, steps: doc.steps.map((s, i) => ({ ...s, position: { x: (i % 4) * 360, y: Math.floor(i / 4) * 320 } })) }
}

// unused is the first of prefix1, prefix2… not taken.
export function unused(prefix: string, taken: string[]) {
  let n = 1
  while (taken.includes(`${prefix}${n}`)) n++
  return `${prefix}${n}`
}

// ---- Targets, picked by Name and stored by id (the Catalogue, targets.ts)

// What a watching Step can watch: an Event trigger a Capability that emits
// Events, a Value trigger or condition one with a Value it can compare.
export const emitsEvents = (c: Capability) => !!c.stateless
export const hasValue = (c: Capability) => c.access.observable && !c.stateless && c.type !== 'composite' && c.type !== 'list'

// The Capability to start from among caps: the only one, or else the only
// primary one; none if the occupant has to choose.
export function soleCapability(caps: Capability[]): Capability | undefined {
  if (caps.length === 1) return caps[0]
  const primary = caps.filter((c) => c.category === 'primary')
  return primary.length === 1 ? primary[0] : undefined
}

// The targets a Step's params refer to.
export function targetsOf(params: Params): Target[] {
  return [params.target, ...(params.targets ?? []), ...Object.values(params.bindings ?? {})].filter(Boolean)
}

// ---- Values
// A number as typed, with a decimal point or comma; NaN if it isn't one.
export const parseNumber = (s: string) => (s.trim() === '' ? NaN : Number(s.trim().replace(',', '.')))

// nudge moves n by one step in direction dir, on the grid of step, within min and max.
export function nudge(n: number, dir: 1 | -1, step = 1, min = -Infinity, max = Infinity) {
  const decimals = String(step).split('.')[1]?.length ?? 0
  return Math.min(max, Math.max(min, Number((Math.round(n / step + dir) * step).toFixed(decimals))))
}

export function duration(seconds: number) {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.round((seconds % 60) * 10) / 10
  return [h && `${h} h`, m && `${m} min`, s && `${s} s`].filter(Boolean).join(' ') || '0 s'
}

export const stepLabel = (s: Pick<Step, 'id' | 'name'>) => s.name || s.id
