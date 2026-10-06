import { describe, expect, test } from 'vitest'
import flow5a from '../../../internal/automation/testdata/flow5a-veranda.json'
import type { Capability, CommandRecord, CommandState, RunEnd, Trace } from '../types'
import { catalogue } from '../targets'
import { evidence, mergeCommands, mergeRuns, onPath, pathOf, runSummary } from './runtime'

const cap = (key: string, label: string, extra: Partial<Capability> = {}): Capability => ({
  key,
  label,
  type: 'binary',
  access: { observable: true, settable: true, queryable: false },
  category: 'primary',
  ...extra,
})
const fn = (id: string, name: string, kind: string, capabilities: Capability[]) => ({
  id,
  name,
  nativeAddress: id,
  functions: [{ key: kind, kind, capabilities }],
  capabilities: null,
})
const options = catalogue(
  [
    fn('strip', 'Office strip', 'light', [
      cap('effect', 'Effect', { type: 'enum' }),
      cap('state', 'State'),
      cap('effect_speed', 'Effect speed', { type: 'numeric' }),
      cap('brightness', 'Brightness', { type: 'numeric', unit: '%' }),
    ]),
    fn('remote', 'Remote', 'button', []),
  ],
  [{ id: 'motion', name: 'Motion veranda', members: [], binary: 'any', numeric: 'mean', kind: 'occupancy', capabilities: [cap('occupancy', 'Occupancy')] }],
  [],
)
const motionOn = { step: 'motion', kind: 'valueTrigger', target: 'aggregate:motion', capability: 'occupancy', value: true, time: '2026-09-28T19:59:58Z' }

const run = (id: string, time: string, extra: Partial<RunEnd> = {}): RunEnd => ({
  automation: 'a',
  run: id,
  time,
  outcome: 'nothing',
  trigger: motionOn,
  commands: 0,
  ...extra,
})

// A Run of 5a stopped at the lux condition: the lights were off and it was bright.
const stoppedAtLux: Trace = {
  run: 'r',
  automation: 'a',
  time: '2026-09-28T20:00:00Z',
  outcome: 'nothing',
  trigger: motionOn,
  steps: [
    { step: 'motion', fired: ['out'] },
    { step: 'lit', fired: ['false'], read: false, at: '2026-09-28T19:00:00Z' },
    { step: 'dark', fired: ['false'], read: 350, at: '2026-09-28T19:57:00Z' },
  ],
}
const hms = /\d\d:\d\d:\d\d/

describe('a picked Run', () => {
  const path = pathOf(stoppedAtLux)
  const edges = flow5a.edges.map((e) => ({ source: e.from.step, sourceHandle: e.from.handle, target: e.to.step }))

  test('reaches the Steps up to the lux condition, and no other', () => {
    const reached = flow5a.steps.map((s) => s.id).filter((id) => path.reached.has(id))
    expect(reached).toEqual(['motion', 'lit', 'dark'])
  })

  test('follows the edges out of the handles it fired', () => {
    expect(edges.filter((e) => onPath(path, e)).map((e) => `${e.source}.${e.sourceHandle}>${e.target}`)).toEqual(['motion.out>lit', 'lit.false>dark'])
  })

  test('shows the lux Value read with its age', () => {
    expect(evidence(stoppedAtLux.steps[2]!, stoppedAtLux, options)).toEqual(['read 350, 3 min old'])
  })

  test('shows on its trigger what happened, and when', () => {
    const [line] = evidence(stoppedAtLux.steps[0]!, stoppedAtLux, options)
    expect(line).toMatch(hms)
    expect(line).toMatch(/ Motion veranda: Occupancy on$/)
  })

  test('shows on a Manual trigger who started it', () => {
    const manual: Trace = { ...stoppedAtLux, trigger: { ...stoppedAtLux.trigger, target: '', capability: '', by: { person: 'alice' } } }
    expect(evidence(manual.steps[0]!, manual, options, [], 'by Alice')[0]).toMatch(/ fired, by Alice$/)
  })

  test('a Step that fired nothing, as JSON has it, is reached', () => {
    const t: Trace = { ...stoppedAtLux, steps: [{ step: 'long', fired: null, action: 'cancel' }] }
    expect(pathOf(t)).toEqual({ reached: new Set(['long']), fired: new Set() })
  })
})

describe('evidence', () => {
  const tr: Trace = { ...stoppedAtLux, steps: [] }
  test('an unknown Value', () => {
    expect(evidence({ step: 's', fired: [], unknown: true }, tr, options)).toEqual(['read unknown'])
  })
  test('a Value read without its time, as older Traces have it', () => {
    expect(evidence({ step: 's', fired: ['false'], read: false }, tr, options)).toEqual(['read off'])
  })
  test("a kind's own lines, after what was read", () => {
    expect(evidence({ step: 's', fired: ['false'], read: false }, tr, options, ['own'])).toEqual(['read off', 'own'])
  })
  test('a Timer deadline', () => {
    const [line] = evidence({ step: 's', fired: [], action: 'start', until: '2026-09-28T20:15:00Z' }, tr, options)
    expect(line).toMatch(/^start — until /)
  })
  test('a trigger without a target says when it fired', () => {
    const t: Trace = { ...tr, trigger: { step: 'dusk', kind: 'sunTrigger', target: '', capability: '', value: null, time: tr.time, catchUp: true } }
    expect(evidence({ step: 'dusk', fired: ['out'] }, t, options)).toEqual([expect.stringMatching(/^\d\d:\d\d:\d\d fired, caught up$/)])
  })
  test('Commands say what they asked for and how it went, a Code error and print', () => {
    const r = {
      step: 's',
      fired: [],
      commands: [
        { target: 'device:strip/light', id: 'c1', values: { effect: 'colorloop', effect_speed: 0.5 }, status: 'confirmed' as const },
        { target: 'device:strip/light', id: 'c2', values: { state: true, brightness: 40 } },
        { target: 'device:strip/light', refused: 'bridge offline' },
      ],
      error: 'boom',
      print: 'hello',
    }
    const lines = evidence(r, tr, options)
    expect(lines.map((l) => l.replace(hms, 'T'))).toEqual([
      'T Office strip: Effect colorloop, Effect speed 0.5 — confirmed',
      'T Office strip: on, Brightness 40 %',
      'T Office strip: refused, bridge offline',
      'error: boom',
      'print: hello',
    ])
  })
})

describe('a Run in the drawer', () => {
  const stepName = (id: string) => `step ${id}`
  test('says what started it and what it did', () => {
    expect(runSummary(run('1', stoppedAtLux.time, { outcome: 'acted', commands: 2 }), options, stepName)).toEqual({
      trigger: 'Motion veranda: Occupancy on',
      result: '2 commands',
    })
    expect(runSummary(run('1', stoppedAtLux.time), options, stepName).result).toBe('nothing to do')
  })
  test('an event names the event, a trigger without a target its Step', () => {
    const pressed = {
      step: 'p',
      kind: 'eventTrigger',
      target: 'device:remote/button',
      capability: 'action',
      value: 'single',
      time: stoppedAtLux.time,
    }
    expect(runSummary(run('1', stoppedAtLux.time, { trigger: pressed }), options, stepName).trigger).toBe('Remote: single')
    const timed = { ...pressed, kind: 'timeTrigger', target: '', capability: '', value: null }
    expect(runSummary(run('1', stoppedAtLux.time, { trigger: timed }), options, stepName).trigger).toBe('step p')
  })
})

test('live Runs join the fetched ones, the latest first, once each', () => {
  const fetched = [run('2', '2026-09-28T19:00:02Z'), run('1', '2026-09-28T19:00:01Z')]
  const live = [run('3', '2026-09-28T19:00:03Z'), run('2', '2026-09-28T19:00:02Z')]
  expect(mergeRuns(live, fetched).map((r) => r.run)).toEqual(['3', '2', '1'])
})

describe('Command history', () => {
  const kept: CommandRecord[] = [{ id: 'c1', target: 'device:d/f', status: 'pending', origin: 'unknown', time: '2026-09-28T19:00:00Z' }]
  test('the live status of a kept Command wins', () => {
    const live: CommandState = { id: 'c1', target: 'device:d/f', status: 'confirmed', origin: { person: 'alice' } }
    expect(mergeCommands(kept, live).map((c) => c.status)).toEqual(['confirmed'])
  })
  test('a live Command not kept yet comes first', () => {
    const live: CommandState = { id: 'c2', target: 'device:d/f', status: 'pending', origin: { program: 'nr' } }
    expect(mergeCommands(kept, live).map((c) => c.id)).toEqual(['c2', 'c1'])
  })
  test('a refusal from this browser is no Command', () => {
    expect(mergeCommands(kept, { id: '', target: 'device:d/f', status: 'failed' })).toEqual(kept)
  })
})
