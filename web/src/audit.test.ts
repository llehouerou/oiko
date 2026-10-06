import { expect, test } from 'vitest'
import { describe as tell, type Entry } from './audit'

const alice = { kind: 'person', id: 'p1', name: 'Alice' } as const
const unknown = { kind: 'unknown' } as const
const entry = (e: Partial<Entry>): Entry => ({ id: 1, time: '2026-03-01T12:00:00Z', event: '', actor: unknown, subject: unknown, ...e })

test('an entry reads as a sentence naming who acted and whom it concerns as they were named then', () => {
  expect(
    tell(entry({ event: 'person-created', actor: alice, subject: { kind: 'person', id: 'p2', name: 'Bob', current: 'Robert' }, detail: { level: 'guest' } })),
  ).toBe('Alice created Bob (now Robert), Guest')
  expect(tell(entry({ event: 'signed-in', actor: alice, subject: alice, detail: { method: 'link', by: { kind: 'person', id: 'p9' } } }))).toBe(
    'Alice signed in with a Sign-in link from a removed Person',
  )
  expect(tell(entry({ event: 'session-ended', actor: { kind: 'oiko' }, subject: alice, detail: { reason: 'expired' } }))).toBe('A Session of Alice expired')
  expect(
    tell(entry({ event: 'level-changed', actor: alice, subject: { kind: 'program', id: 'x', name: 'Node-RED' }, detail: { from: 'member', to: 'admin' } })),
  ).toBe('Alice changed Node-RED from Member to Admin')
})

test('a Kiosk pairing reads as who approved it, for which screen', () => {
  const hall = { kind: 'kiosk', id: 'k1', name: 'Hall tablet' } as const
  expect(tell(entry({ event: 'kiosk-created', actor: alice, subject: hall, detail: { level: 'guest' } }))).toBe('Alice created the Kiosk Hall tablet, Guest')
  expect(tell(entry({ event: 'kiosk-paired', actor: alice, subject: hall }))).toBe('Alice paired a screen as Hall tablet')
  expect(tell(entry({ event: 'signed-in', actor: hall, subject: hall, detail: { method: 'pairing', by: alice } }))).toBe(
    'Hall tablet signed in as paired by Alice',
  )
  expect(tell(entry({ event: 'session-ended', actor: alice, subject: hall, detail: { reason: 'paired again' } }))).toBe(
    'A Session of Hall tablet ended as it was paired again',
  )
  expect(tell(entry({ event: 'pairing-refused', actor: alice }))).toBe('Alice used a Kiosk pairing code that had expired')
  expect(tell(entry({ event: 'pairing-refused' }))).toBe('A Kiosk pairing request was refused')
})

test('anonymous refusals past the budget read as a count for the hour', () => {
  expect(tell(entry({ event: 'token-refused' }))).toBe('A Token was refused')
  expect(tell(entry({ event: 'token-refused', detail: { count: 37 } }))).toBe('A Token was refused, 37 more times that hour')
  expect(tell(entry({ event: 'passkey-refused', subject: alice }))).toBe('A Passkey for Alice was refused')
})
