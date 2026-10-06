import { expect, test } from 'vitest'
import { mediaSourceOf, timeUp, watchedFor } from './liveview'

test('a Live view plays through MediaSource, or ManagedMediaSource on an iPhone, or not at all', () => {
  const MS = class {} as unknown as typeof MediaSource
  const Managed = class {} as unknown as typeof MediaSource
  expect(mediaSourceOf({ MediaSource: MS, ManagedMediaSource: Managed })).toBe(MS)
  expect(mediaSourceOf({ ManagedMediaSource: Managed })).toBe(Managed)
  expect(mediaSourceOf({})).toBeUndefined()
})

test('a Live view that ends at its time asks whether one still watches; one that ends before it failed', () => {
  const until = '2026-10-01T08:35:00Z'
  expect(timeUp(until, Date.parse(until))).toBe(true)
  expect(timeUp(until, Date.parse(until) - 2000)).toBe(true) // the camera's last frames arrive a little early
  expect(timeUp(until, Date.parse(until) - 60_000)).toBe(false)
  expect(timeUp(null, Date.now())).toBe(false) // a camera not on battery has no time
})

test('a Live view marker tells how long it lasted', () => {
  expect(watchedFor(12_400)).toBe('12 s')
  expect(watchedFor(150_000)).toBe('3 min')
})
