import { expect, test } from 'vitest'
import { byWhom, commandKind, type Names } from './origin'
import type { CommandState, Origin } from './types'

const names: Names = { person: { alice: 'Alice' }, kiosk: { hall: 'Kitchen tablet' }, program: { nr: 'Node-RED' } }
const automation = (id: string) => (id === 'movie' ? 'Movie night' : 'a deleted Automation')
const run: Origin = { automation: 'movie', step: 's', run: 'r' }

test('a Command reads who issued it, by the Name it has now', () => {
  const read = (o: Origin) => byWhom(o, names, automation)
  expect([{ person: 'alice' }, { kiosk: 'hall' }, { program: 'nr' }, run, 'unknown'].map((o) => read(o as Origin))).toEqual([
    'by Alice',
    'by Kitchen tablet',
    'by Node-RED',
    'by Movie night',
    'issuer unknown',
  ])
  expect([{ person: 'bob' }, { kiosk: 'gone' }, { program: 'gone' }].map((o) => read(o as Origin))).toEqual([
    'by a removed Person',
    'by a removed Kiosk',
    'by a removed Program',
  ])
})

test('without the Names, only the kind shows', () => {
  expect(byWhom({ person: 'alice' }, null, automation)).toBe('by a Person')
  expect(byWhom(run, null, automation)).toBe('by Movie night')
})

test('a Command marker is by hand, by a Program, by an Automation, or lost', () => {
  const c = (origin: Origin, status: CommandState['status'] = 'confirmed'): CommandState => ({ id: '', target: 'flag:a', status, origin })
  expect(
    [c({ person: 'alice' }), c({ kiosk: 'hall' }), c('unknown'), c({ program: 'nr' }), c(run), c(run, 'timed_out'), c({ program: 'nr' }, 'failed')].map(
      commandKind,
    ),
  ).toEqual(['hand', 'hand', 'hand', 'program', 'automation', 'lost', 'lost'])
})
