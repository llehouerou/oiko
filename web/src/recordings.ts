// A camera's Recordings (ADR 0038): kept by its own system, listed and relayed by Oiko, shown in
// the History of its Device.
import { owner, type Catalogue } from './targets'
import { watchedFor } from './liveview'
import type { Recording, Target } from './types'

// The camera Function of the Device t is or belongs to, whose Recordings its History shows.
export const cameraOf = (c: Catalogue, t: Target) => c.list.find((e) => e.kind === 'camera' && owner(e.target) === owner(t))?.target

// What a Recording's marker and thumbnail tell.
export const describeRecording = (r: Recording) => ['Recording', r.trigger, watchedFor(r.duration)].filter(Boolean).join(' · ')

// Where a Recording's video or thumbnail is relayed.
export const mediaURL = (camera: Target, r: Recording, part: 'video' | 'thumbnail') =>
  `/api/recordings/${part}?${new URLSearchParams({ target: camera, id: r.id })}`

// How often Recordings are listed again while the range follows now.
export const relistEvery = 2 * 60 * 1000
