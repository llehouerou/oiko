import { describe, expect, test } from 'vitest'
import type { StepState } from '../../types'
import { catalogue as targetCatalogue } from '../../targets'
import { catalogue, type Kind, type StepKind } from '.'

const badge = (s: StepState, params = {}, now = new Date('2026-09-28T19:40:00Z')) => (catalogue[s.kind as Kind] as StepKind).badge?.(s, params, now) ?? null

describe('Step state badge', () => {
  const due = '2026-09-28T19:47:00Z'
  test('a running Timer shows its deadline', () => {
    expect(badge({ step: 's', kind: 'timer', deadline: due })).toMatch(/^⏱ running — due \d\d:\d\d$/)
  })
  test('a stopped Timer, or one past its deadline, shows nothing', () => {
    expect(badge({ step: 's', kind: 'timer' })).toBeNull()
    expect(badge({ step: 's', kind: 'timer', deadline: '2026-09-28T19:39:00Z' })).toBeNull()
  })
  test('a closed Cooldown shows when it opens, an open one nothing', () => {
    const st: StepState = { step: 's', kind: 'cooldown', passed: [{ at: '2026-09-28T19:39:00Z' }] }
    expect(badge(st, { duration: 600 })).toMatch(/^closed — opens /)
    expect(badge(st, { duration: 30 })).toBeNull()
  })
  test('a Cooldown per target counts the targets it is closed for', () => {
    const st: StepState = {
      step: 's',
      kind: 'cooldown',
      passed: [
        { target: 'device:a/f', at: '2026-09-28T19:39:00Z' },
        { target: 'device:b/f', at: '2026-09-28T19:00:00Z' },
      ],
    }
    expect(badge(st, { duration: 600, perTarget: true })).toBe('closed for 1 target')
  })
  test('an enabled presence simulation shows its next switch', () => {
    expect(badge({ step: 's', kind: 'presenceSimulation', enabled: true, deadline: due, schedule: [{ at: due, on: true }] })).toMatch(/^enabled — on at /)
  })
  test('a Step that remembers nothing shows nothing', () => {
    expect(badge({ step: 's', kind: 'command' })).toBeNull()
  })
})

test('a time window says its verdict', () => {
  expect(catalogue.timeWindow.evidence?.({ step: 's', fired: ['false'] })).toEqual(['outside the window'])
  expect(catalogue.timeWindow.evidence?.({ step: 's', fired: ['true'] })).toEqual(['inside the window'])
})

test('a notify Step shows what it sent', () => {
  expect(catalogue.notify.evidence?.({ step: 's', fired: ['then'], notification: { title: 'Camera down', message: 'Gate' } })).toEqual([
    'sent: Camera down — Gate',
  ])
})

describe('Step summary', () => {
  const settable = { observable: true, settable: true, queryable: false }
  const options = targetCatalogue(
    [],
    [
      {
        id: 'office',
        name: 'Office',
        members: [],
        binary: 'any',
        numeric: 'mean',
        kind: 'light',
        capabilities: [
          { key: 'state', label: 'State', type: 'binary', access: settable, category: 'primary' },
          { key: 'brightness', label: 'Brightness', type: 'numeric', min: 0, max: 254, access: settable, category: 'primary' },
          { key: 'color_temp', label: 'Color temp', type: 'numeric', unit: 'mired', min: 153, max: 500, access: settable, category: 'primary' },
          { key: 'effect', label: 'Effect', type: 'enum', options: ['blink'], access: settable, category: 'primary' },
        ],
      },
    ],
    [],
  )
  test('a Command names its targets, then its values as a light tile shows them', () => {
    const p = {
      targets: ['aggregate:office', 'device:gone/light'],
      values: { brightness: 254, state: true, color_temp: 196, effect: 'blink' },
      transition: 0.5,
    }
    expect(catalogue.command.summary(p, options)).toEqual(['Office, deleted target', 'on, 100 %, 5100 K · cool, Effect blink, in 0.5 s'])
  })
  test('a toggle reads as such', () => {
    expect(catalogue.command.summary({ targets: ['aggregate:office'], values: { state: 'toggle' } }, options)).toEqual(['Office', 'toggle'])
  })
  test('an Availability trigger reads as a sentence', () => {
    expect(catalogue.availabilityTrigger.summary({ target: 'aggregate:office', op: 'ne', availability: 'online', heldFor: 900 }, options)).toEqual([
      'Office',
      'not online for 15 min',
    ])
  })
  test('an unset Step says nothing', () => {
    expect(catalogue.eventTrigger.summary({ events: [] }, options)).toEqual([])
  })
})
