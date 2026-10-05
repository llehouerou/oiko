import { describe, expect, test } from 'vitest'
import flow5a from '../../../internal/automation/testdata/flow5a-veranda.json'
import flow6c from '../../../internal/automation/testdata/flow6c-night-mode.json'
import { catalogue } from '../targets'
import type { Capability } from '../types'
import { connectionError, newStep, reloadReport, type Kind } from './kinds'
import { emitsEvents, hasValue, flowEdge, nudge, parseNumber, patch, soleCapability, toDocument, toFlow, type Document, type StepNode } from './model'

const handWritten = (d: typeof flow5a | typeof flow6c): Document => ({
  id: 'a',
  ...d,
  steps: d.steps.map((s, i) => ({ ...s, kind: s.kind as Kind, position: { x: i * 100, y: 0 } })),
})

const json = (v: unknown) => JSON.parse(JSON.stringify(v))

describe('the graph maps 1:1 to the document', () => {
  test.each([flow5a, flow6c])('$name', (d) => {
    const doc = handWritten(d)
    const { nodes, edges } = toFlow(doc)
    expect(json(toDocument(doc, nodes, edges))).toEqual(json(doc))
  })
})

// Builds d from an empty graph as the editor does: each Step dropped from the
// palette, its params set field by field, and each edge drawn. The result
// equals d apart from Step ids, names and positions.
test.each([flow5a, flow6c])('$name can be built from scratch', (d) => {
  const doc = handWritten(d)
  const nodes: StepNode[] = []
  const ids = new Map<string, string>()
  for (const s of doc.steps) {
    const n = newStep(
      s.kind,
      nodes.map((n) => n.id),
      s.position,
    )
    for (const [k, v] of Object.entries(s.params)) n.data.params = patch(n.data.params, { [k]: v })
    ids.set(s.id, n.id)
    nodes.push(n)
  }
  let edges = toFlow({ ...doc, steps: [], edges: [] }).edges
  for (const e of doc.edges) {
    const c = { source: ids.get(e.from.step)!, sourceHandle: e.from.handle, target: ids.get(e.to.step)!, targetHandle: e.to.handle }
    expect(connectionError(c, nodes, edges)).toBeNull()
    edges = [...edges, flowEdge(c)]
  }
  const built = toDocument(doc, nodes, edges)
  const strip = (x: Document, id: (s: string) => string) => ({
    ...json(x),
    steps: x.steps.map(({ id: s, kind, params }) => json({ id: id(s), kind, params })),
    edges: x.edges.map((e) => ({ from: { ...e.from, step: id(e.from.step) }, to: { ...e.to, step: id(e.to.step) } })),
  })
  expect(strip(built, (s) => s)).toEqual(strip(doc, (s) => ids.get(s)!))
})

test('params left at their default are not written', () => {
  expect(patch({ targets: [], values: { on: true }, transition: 2 }, { transition: undefined })).toEqual({ targets: [], values: { on: true } })
})

describe('connections', () => {
  const { nodes, edges } = toFlow(handWritten(flow6c))
  const refused = (source: string, sourceHandle: string, target: string, targetHandle: string) =>
    connectionError({ source, sourceHandle, target, targetHandle }, nodes, edges)

  test('a self-join is refused', () => expect(refused('delay', 'fired', 'delay', 'start')).toMatch(/itself/))
  test('a duplicate is refused', () => expect(refused('lights-off', 'out', 'night-window', 'in')).toMatch(/already joined/))
  test('an edge into a trigger is refused', () => expect(refused('night-on', 'then', 'morning', 'in')).toMatch(/trigger/))
  test('a cycle is allowed', () => expect(refused('still-night', 'false', 'night-window', 'in')).toBeNull())
})

test("editing a running Timer's duration keeps its state", () => {
  const before = handWritten(flow6c)
  const after = { ...before, steps: before.steps.map((s) => (s.id === 'delay' ? { ...s, params: { ...s.params, duration: 600 } } : s)) }
  expect(reloadReport(before, after)).toEqual({ kept: ['15 min'], dropped: [], empty: [] })
})

test('a removed Step or a changed kind drops its state; a new Step starts empty', () => {
  const before = handWritten(flow5a)
  const after = {
    ...before,
    steps: [
      ...before.steps.filter((s) => s.id !== 'long').map((s) => (s.id === 'short' ? { ...s, kind: 'cooldown' as Kind, params: { duration: 60 } } : s)),
      newStep('timer', [], { x: 0, y: 0 }),
    ].map((s) => ('data' in s ? { id: s.id, position: s.position, ...s.data } : s)),
  }
  expect(reloadReport(before, after)).toEqual({ kept: [], dropped: ['15 min', '5 min'], empty: ['5 min', 'timer-1'] })
})

describe('a watching Step lists the targets it can watch', () => {
  const cap = (key: string, extra: Partial<Capability> = {}): Capability => ({
    key,
    label: key,
    type: 'binary',
    access: { observable: true, settable: false, queryable: false },
    category: 'primary',
    ...extra,
  })
  const occupancy = cap('occupancy')
  const battery = cap('battery', { type: 'numeric', category: 'diagnostic' })
  const action = cap('action', { type: 'enum', stateless: true, options: ['single', 'double'] })
  const options = catalogue(
    [{ id: 'remote', name: 'Remote', nativeAddress: '', functions: [{ key: 'button', kind: 'button', capabilities: [action, battery] }], capabilities: null }],
    [{ id: 'motion', name: 'Motion veranda', members: [], binary: 'any', numeric: 'mean', capabilities: [occupancy, battery] }],
    [],
  )
  const names = (fits: (c: Capability) => boolean) => options.list.filter((o) => o.capabilities.some(fits)).map((o) => o.name)

  test('an Event trigger only those emitting events, a Value trigger those with a Value', () => {
    expect(names(emitsEvents)).toEqual(['Remote'])
    expect(names(hasValue)).toEqual(['Remote', 'Motion veranda'])
  })
  test('the Capability is picked when the choice is obvious', () => {
    expect(soleCapability([action])).toBe(action)
    expect(soleCapability([occupancy, battery])).toBe(occupancy) // the only primary one
    expect(soleCapability([occupancy, cap('contact')])).toBeUndefined()
  })
})

describe('a number field', () => {
  test('takes a decimal comma as well as a point', () => {
    expect(parseNumber('0,5')).toBe(0.5)
    expect(parseNumber(' 0.25 ')).toBe(0.25)
    expect(parseNumber('')).toBeNaN()
    expect(parseNumber('abc')).toBeNaN()
  })
  test('moves by its step, within its range, without float noise', () => {
    expect(nudge(0.5, 1, 0.01, 0, 1)).toBe(0.51)
    expect(nudge(0.07, 1, 0.01, 0, 1)).toBe(0.08)
    expect(nudge(1, 1, 0.01, 0, 1)).toBe(1)
    expect(nudge(0, -1, undefined, 0)).toBe(0)
    expect(nudge(3, 1)).toBe(4)
  })
})
