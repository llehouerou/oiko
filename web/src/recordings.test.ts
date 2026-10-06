import { expect, test } from 'vitest'
import { cameraOf, describeRecording, mediaURL } from './recordings'
import type { Catalogue, Entry } from './targets'

const entry = (target: string, kind: string): Entry => ({ target, name: target, kind, capabilities: [], ownerCapabilities: [] })
const catalogue = (list: Entry[]): Catalogue => ({ list, get: (t) => list.find((e) => e.target === t), name: (t) => t })

test('a sheet lists the Recordings of the camera of its Device, whichever Function it opened on', () => {
  const c = catalogue([
    entry('device:garden/occupancy', 'occupancy'),
    entry('device:garden/camera', 'camera'),
    entry('device:porch/camera', 'camera'),
    entry('device:office/light', 'light'),
    entry('device:garden', ''),
  ])
  expect(cameraOf(c, 'device:garden/occupancy')).toBe('device:garden/camera')
  expect(cameraOf(c, 'device:garden/camera')).toBe('device:garden/camera')
  expect(cameraOf(c, 'device:garden')).toBe('device:garden/camera')
  expect(cameraOf(c, 'device:office/light')).toBeUndefined()
  expect(cameraOf(c, 'flag:away')).toBeUndefined()
})

test('a Recording tells what triggered it and how long it lasted', () => {
  expect(describeRecording({ id: 'a', start: '2026-10-01T08:30:00Z', duration: 12_400, trigger: 'motion' })).toBe('Recording · motion · 12 s')
  expect(describeRecording({ id: 'b', start: '2026-10-01T08:30:00Z', duration: 150_000 })).toBe('Recording · 3 min')
})

test("a Recording's media is relayed by its camera and id", () => {
  expect(mediaURL('device:garden/camera', { id: 'a b/1', start: '', duration: 0 }, 'thumbnail')).toBe(
    '/api/recordings/thumbnail?target=device%3Agarden%2Fcamera&id=a+b%2F1',
  )
})
