import { useEffect, useRef, useState } from 'react'
import { mdiVolumeHigh, mdiVolumeOff } from '@mdi/js'
import { api } from './access'
import { Svg } from './icons'
import { keep, maxLag, mediaSourceOf, timeUp, type Viewing } from './liveview'
import type { Target } from './types'

// A camera's Live view, full screen (ADR 0036, 0037): its fragmented MP4, read from the answer to a
// POST into a Media Source, muted until unmuted. It ends when closed or when its page is hidden; a
// camera on battery asks whether one is still watching once its time is up.
export function LiveViewer({ target, name, onClose }: { target: Target; name: string; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const video = useRef<HTMLVideoElement>(null)
  const [viewing, setViewing] = useState<Viewing>({ kind: 'loading' })
  const [muted, setMuted] = useState(true)
  const [round, setRound] = useState(0) // each Keep watching opens it again
  useEffect(() => dialog.current?.showModal(), [])
  useEffect(() => void (video.current!.muted = muted), [muted]) // the property, which React does not keep
  useEffect(() => {
    const hidden = () => document.hidden && onClose()
    document.addEventListener('visibilitychange', hidden)
    return () => document.removeEventListener('visibilitychange', hidden)
  }, [onClose])

  useEffect(() => {
    const v = video.current!
    const MS = mediaSourceOf(window as never)
    if (!MS) {
      setViewing({ kind: 'unsupported' })
      return
    }
    setViewing({ kind: 'loading' })
    const abort = new AbortController()
    const ms = new MS()
    v.disableRemotePlayback = true // ManagedMediaSource plays only so
    const src = URL.createObjectURL(ms)
    v.src = src
    const playing = () => setViewing({ kind: 'playing' })
    v.addEventListener('playing', playing)
    let until: string | null = null
    const watch = async () => {
      const res = await api('/api/live-view', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ target }),
        signal: abort.signal,
      })
      if (!res.ok || !res.body) throw new Error(res.status === 503 ? (await res.text()).trim() || 'Too many Live views' : 'The camera sent no Live view')
      until = res.headers.get('Live-View-Until')
      const type = res.headers.get('Content-Type') ?? ''
      if (!MS.isTypeSupported(type)) throw new Error(`This browser does not play ${type}`)
      if (ms.readyState !== 'open') await new Promise((r) => ms.addEventListener('sourceopen', r, { once: true }))
      const sb = ms.addSourceBuffer(type)
      const updated = () => new Promise((r) => sb.addEventListener('updateend', r, { once: true }))
      const reader = res.body.getReader()
      for (;;) {
        const { done, value } = await reader.read()
        if (done) return
        sb.appendBuffer(value)
        await updated()
        if (!sb.buffered.length) continue
        const end = sb.buffered.end(sb.buffered.length - 1)
        if (v.paused && v.readyState >= 2) void v.play().catch(() => {})
        if (end - v.currentTime > maxLag) v.currentTime = end - 0.5 // live, not behind it
        if (v.currentTime - sb.buffered.start(0) > 2 * keep) {
          sb.remove(0, v.currentTime - keep)
          await updated()
        }
      }
    }
    watch().then(
      () => !abort.signal.aborted && setViewing(timeUp(until, Date.now()) ? { kind: 'timeUp' } : { kind: 'failed', why: 'The camera stopped sending' }),
      (e: Error) => !abort.signal.aborted && setViewing({ kind: 'failed', why: e.message }),
    )
    return () => {
      abort.abort()
      v.removeEventListener('playing', playing)
      v.removeAttribute('src')
      v.load()
      URL.revokeObjectURL(src)
    }
  }, [target, round])

  return (
    <dialog
      ref={dialog}
      onClose={onClose}
      aria-label={`${name}, Live view`}
      className="m-0 h-dvh max-h-none w-dvw max-w-none bg-black p-0 text-neutral-100 backdrop:bg-black"
    >
      <video ref={video} muted={muted} playsInline className="size-full object-contain" />
      <div className="absolute inset-x-0 top-0 flex items-center gap-3 bg-gradient-to-b from-black/70 p-3">
        <p className="min-w-0 flex-1 truncate font-medium">{name}</p>
        {viewing.kind === 'playing' && (
          <button onClick={() => setMuted(!muted)} aria-label={muted ? 'Unmute' : 'Mute'} className="rounded-full p-2 hover:bg-white/10">
            <Svg path={muted ? mdiVolumeOff : mdiVolumeHigh} className="size-6" />
          </button>
        )}
        <button onClick={onClose} aria-label="Close" className="rounded-full px-3 py-1 text-xl hover:bg-white/10">
          ✕
        </button>
      </div>
      {viewing.kind !== 'playing' && (
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-4 p-6 text-center">
          {viewing.kind === 'loading' && (
            <>
              <span className="size-8 animate-spin rounded-full border-2 border-neutral-500 border-t-white" />
              <p className="text-sm text-neutral-400">Waking the camera…</p>
            </>
          )}
          {viewing.kind === 'timeUp' && (
            <>
              <p>Still watching?</p>
              <button onClick={() => setRound(round + 1)} className="rounded bg-amber-500 px-4 py-2 font-medium text-black hover:bg-amber-400">
                Keep watching
              </button>
            </>
          )}
          {viewing.kind === 'failed' && (
            <>
              <p className="text-sm text-neutral-300">{viewing.why}</p>
              <button onClick={() => setRound(round + 1)} className="rounded bg-neutral-800 px-4 py-2 hover:bg-neutral-700">
                Try again
              </button>
            </>
          )}
          {viewing.kind === 'unsupported' && <p className="text-sm text-neutral-300">A Live view needs a newer browser: on an iPhone, iOS 17.1 or later.</p>}
        </div>
      )}
    </dialog>
  )
}
