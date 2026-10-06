import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { mdiCctv } from '@mdi/js'
import { api } from './access'
import { Svg } from './icons'
import { describeRecording, mediaURL, relistEvery } from './recordings'
import type { Recording, Target } from './types'

// useRecordings lists camera's Recordings over a range of len ms ending at end, now while end is
// null: again when the range moves, and every relistEvery while it follows now. from is where a
// range of every time starts. undefined while none were listed, [] once there are none.
export function useRecordings(camera: Target | undefined, len: number, end: number | null, from: number) {
  const [list, setList] = useState<Recording[]>()
  const [tick, setTick] = useState(0)
  useEffect(() => {
    if (!camera || end !== null) return
    const id = setInterval(() => setTick((n) => n + 1), relistEvery)
    return () => clearInterval(id)
  }, [camera, end])
  useEffect(() => {
    if (!camera) return
    const abort = new AbortController()
    const t = setTimeout(() => {
      const to = end ?? Date.now()
      const start = Number.isFinite(len) ? to - len : from
      api(`/api/recordings?${new URLSearchParams({ target: camera, from: String(Math.round(start)), to: String(Math.round(to)) })}`, { signal: abort.signal })
        .then((r) => (r.ok ? r.json() : []))
        .then(setList, () => {})
    }, 300) // a zoom's gestures settle first
    return () => {
      clearTimeout(t)
      abort.abort()
    }
  }, [camera, len, end, from, tick])
  return camera ? list : undefined
}

// shown bounds the thumbnails drawn at once: a longer range is zoomed into.
const shown = 60

// RecordingList draws a camera's Recordings, newest first, as thumbnails that each play their
// video when tapped.
export function RecordingList({ camera, list }: { camera: Target; list: Recording[] | undefined }) {
  const [playing, setPlaying] = useState<Recording>()
  return (
    <section className="space-y-2">
      <h3 className="text-sm text-neutral-400">
        Recordings{list && ` · ${list.length}`}
        {list === undefined && ' · loading'}
      </h3>
      {list?.length === 0 && <p className="text-xs text-neutral-500">None over this range.</p>}
      <div className="grid grid-cols-[repeat(auto-fill,minmax(10rem,1fr))] gap-2">
        {list?.slice(0, shown).map((r) => (
          <button
            key={r.id}
            onClick={() => setPlaying(r)}
            aria-label={`Play ${describeRecording(r)}, ${new Date(r.start).toLocaleString()}`}
            className="group relative aspect-video overflow-hidden rounded-lg bg-neutral-800 text-left"
          >
            <Thumbnail src={mediaURL(camera, r, 'thumbnail')} />
            <span className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/80 px-2 pt-4 pb-1 text-[11px] leading-tight text-neutral-100">
              <span className="block tabular-nums">
                {new Date(r.start).toLocaleString([], { weekday: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}
              </span>
              <span className="block text-neutral-300">{describeRecording(r).replace(/^Recording · /, '')}</span>
            </span>
          </button>
        ))}
      </div>
      {list && list.length > shown && <p className="text-xs text-neutral-500">…and {list.length - shown} more: zoom in</p>}
      {playing && createPortal(<RecordingPlayer camera={camera} recording={playing} onClose={() => setPlaying(undefined)} />, document.body)}
    </section>
  )
}

// A thumbnail, loaded once on screen; a camera drawn instead if it cannot be.
function Thumbnail({ src }: { src: string }) {
  const [broken, setBroken] = useState(false)
  return broken ? (
    <Svg path={mdiCctv} className="absolute inset-0 m-auto size-8 text-neutral-600" />
  ) : (
    <img src={src} alt="" loading="lazy" onError={() => setBroken(true)} className="size-full object-cover" />
  )
}

// RecordingPlayer plays a Recording's video full screen, from its start.
function RecordingPlayer({ camera, recording, onClose }: { camera: Target; recording: Recording; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  useEffect(() => dialog.current?.showModal(), [])
  return (
    <dialog
      ref={dialog}
      onClose={onClose}
      aria-label={describeRecording(recording)}
      className="m-0 h-dvh max-h-none w-dvw max-w-none bg-black p-0 text-neutral-100 backdrop:bg-black"
    >
      <video src={mediaURL(camera, recording, 'video')} controls autoPlay playsInline className="size-full object-contain" />
      <div className="absolute inset-x-0 top-0 flex items-center gap-3 bg-gradient-to-b from-black/70 p-3">
        <p className="min-w-0 flex-1 truncate text-sm">
          {new Date(recording.start).toLocaleString()} · {describeRecording(recording)}
        </p>
        <button onClick={() => dialog.current?.close()} aria-label="Close" className="rounded-full px-3 py-1 text-xl hover:bg-white/10">
          ✕
        </button>
      </div>
    </dialog>
  )
}
