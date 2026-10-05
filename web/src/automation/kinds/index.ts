// The Step kinds, one module each, as internal/automation has them: their
// handles, the params a new Step starts with, their params form, and how the
// editor shows what they remember and what they did in a Run.

import type { Connection, Edge as FlowEdge } from '@xyflow/react'
import type { ReactNode } from 'react'
import type { Reached, StepState } from '../../types'
import { flowEdge, stepLabel, unused, type Document, type Params, type Step, type StepNode } from '../model'
import type { Catalogue } from '../../targets'
import { availabilityTrigger } from './availabilityTrigger'
import { code } from './code'
import { command } from './command'
import { cooldown } from './cooldown'
import { eventTrigger } from './eventTrigger'
import type { FormProps } from './fields'
import { manualTrigger } from './manualTrigger'
import { notify } from './notify'
import { presenceSimulation } from './presenceSimulation'
import { sunTrigger } from './sunTrigger'
import { timer } from './timer'
import { timeTrigger } from './timeTrigger'
import { timeWindow } from './timeWindow'
import { valueCondition } from './valueCondition'
import { valueTrigger } from './valueTrigger'

type Group = 'Trigger' | 'Condition' | 'Timing' | 'Action'

export interface StepKind {
  label: string
  group: Group
  inputs: string[]
  // A Code Step declares its outputs in its params.
  outputs: string[] | ((p: Params) => string[])
  params: () => Params
  Form: (props: FormProps) => ReactNode
  // What its params say, in a line or two, shown while it is collapsed.
  summary: (p: Params, targets: Catalogue) => string[]
  wide?: boolean
  // Whether a Step of this kind remembers something between Runs worth
  // reporting when a save keeps or drops it.
  stateful?: (p: Params) => boolean
  // What it remembers now, or null if nothing worth showing.
  badge?: (s: StepState, p: Params, now: Date) => string | null
  // What its evidence in a Run says besides what every Step's does.
  evidence?: (r: Reached) => string[]
}

export const catalogue = {
  eventTrigger,
  valueTrigger,
  availabilityTrigger,
  timeTrigger,
  sunTrigger,
  manualTrigger,
  valueCondition,
  timeWindow,
  timer,
  cooldown,
  presenceSimulation,
  command,
  notify,
  code,
} satisfies Record<string, StepKind>

export type Kind = keyof typeof catalogue
export const kinds = Object.keys(catalogue) as Kind[]
export const groups: Group[] = ['Trigger', 'Condition', 'Timing', 'Action']

// A trigger has no input: it starts a Run.
export const isTrigger = (k: Kind) => catalogue[k].inputs.length === 0

export function handles(kind: Kind, params: Params): { inputs: string[]; outputs: string[] } {
  const { inputs, outputs }: StepKind = catalogue[kind]
  return { inputs, outputs: typeof outputs === 'function' ? outputs(params) : outputs }
}

// newStep is a Step of kind at position, with an id no other Step has.
export function newStep(kind: Kind, taken: string[], position: { x: number; y: number }): StepNode {
  return { id: unused(`${kind}-`, taken), type: 'step', position, data: { kind, name: '', params: catalogue[kind].params() } }
}

// connectionError says why an edge can't be drawn, or null if it can.
// Cycles are fine: each input acts at most once per Run.
export function connectionError(c: Connection | FlowEdge, nodes: { id: string; data: { kind: Kind } }[], edges: FlowEdge[]): string | null {
  if (c.source === c.target) return 'A Step cannot join itself.'
  if (edges.some((e) => flowEdge(e).id === flowEdge(c).id)) return 'These handles are already joined.'
  const to = nodes.find((n) => n.id === c.target)
  if (to && isTrigger(to.data.kind)) return 'A trigger has no input: it starts a Run.'
  return null
}

const stateful = (s: Step) => (catalogue[s.kind] as StepKind).stateful?.(s.params) ?? false

// reloadReport tells what saving after over before does to Step state: a
// Step keeps it if its id and kind are unchanged, whatever its params.
export function reloadReport(before: Document, after: Document) {
  const same = (s: Step, o: Step) => o.id === s.id && o.kind === s.kind
  const kept: string[] = []
  const empty: string[] = []
  for (const s of after.steps.filter(stateful)) (before.steps.some((o) => same(s, o)) ? kept : empty).push(stepLabel(s))
  const dropped = before.steps.filter((o) => stateful(o) && !after.steps.some((s) => same(s, o))).map(stepLabel)
  return { kept, dropped, empty }
}
