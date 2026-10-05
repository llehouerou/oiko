import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { sendCommand } from './store'

// What reached /api/commands, in order: each Target with the values it was asked. Each test
// has Targets of its own: the pacing of one lasts beyond it.
let sent: string[] = []

beforeEach(() => {
  vi.useFakeTimers()
  sent = []
  vi.stubGlobal('fetch', (_: string, init: RequestInit) => {
    const { target, values } = JSON.parse(init.body as string)
    sent.push(`${target} ${JSON.stringify(values)}`)
    return Promise.resolve({ ok: true })
  })
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

test('paced sends go out at most every 150 ms, ending on the last value', () => {
  for (const v of [10, 20, 30]) sendCommand('device:a/light', { brightness: v }, { paced: true })
  expect(sent).toEqual(['device:a/light {"brightness":10}'])
  vi.advanceTimersByTime(150)
  expect(sent).toEqual(['device:a/light {"brightness":10}', 'device:a/light {"brightness":30}'])
})

test("two Targets pace apart: one's last value survives a drag on the other", () => {
  sendCommand('device:b/light', { brightness: 10 }, { paced: true })
  sendCommand('device:b/light', { brightness: 20 }, { paced: true })
  sendCommand('device:c/light', { brightness: 50 }, { paced: true })
  sendCommand('device:c/light', { brightness: 60 }, { paced: true })
  vi.advanceTimersByTime(150)
  expect(sent.sort()).toEqual([
    'device:b/light {"brightness":10}',
    'device:b/light {"brightness":20}',
    'device:c/light {"brightness":50}',
    'device:c/light {"brightness":60}',
  ])
})

test("a send that is not paced, a toggle, drops the Target's pending paced one", () => {
  sendCommand('device:d/light', { brightness: 10 }, { paced: true })
  sendCommand('device:d/light', { brightness: 20 }, { paced: true })
  sendCommand('device:d/light', { state: 'toggle' })
  vi.advanceTimersByTime(150)
  expect(sent).toEqual(['device:d/light {"brightness":10}', 'device:d/light {"state":"toggle"}'])
})
