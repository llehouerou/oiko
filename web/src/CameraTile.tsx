// A camera's Tile: its Picture, and its Live view a tap away.

import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { mdiCctv, mdiPlay } from '@mdi/js'
import { useNow } from './store'
import type { Target } from './types'
import { Svg } from './icons'
import { LiveViewer } from './LiveViewer'
import type { Part } from './tiles'
import { StateText } from './Capabilities'
import { age, More, NameLine, statusSize, useLongPress, type SensorProps } from './TileParts'

// How often a camera's Tile reads its Picture again while its page shows (ADR 0036).
const PICTURE_EVERY = 5 * 60 * 1000

// A camera's Picture: its URL, new each time it is read, so that its image loads again, and when
// it was taken; undefined until first read, null when there is none. A HEAD tells when it was
// taken, which an image cannot. Read when shown, when its page shows again, and every
// PICTURE_EVERY while it shows.
function usePicture(target: Target) {
  const [picture, setPicture] = useState<{ src: string; taken: string | null } | null>()
  useEffect(() => {
    let gone = false
    const read = async () => {
      if (document.hidden) return
      const src = `/api/picture?target=${encodeURIComponent(target)}&read=${Date.now()}`
      const res = await fetch(src, { method: 'HEAD' }).catch(() => null)
      if (!gone) setPicture(res?.ok ? { src, taken: res.headers.get('Last-Modified') } : null)
    }
    read()
    const every = setInterval(read, PICTURE_EVERY)
    document.addEventListener('visibilitychange', read)
    return () => {
      gone = true
      clearInterval(every)
      document.removeEventListener('visibilitychange', read)
    }
  }, [target])
  return picture
}

// A camera's Tile: its Picture and how old it is, its Device's state beside its name; a camera
// drawn instead until it has a Picture, or once it cannot load it. A tap on it opens its Live view,
// never by itself: watching may wake a camera on battery (ADR 0036). ⋯ or a long press opens its panel.
export function CameraTile({ target, state, name, note, badges, dimmed, hideName, onOpen }: SensorProps & { target: Target; state?: Part }) {
  const picture = usePicture(target)
  const [broken, setBroken] = useState<string>() // the src that failed to load
  const [live, setLive] = useState(false)
  const now = useNow()
  const longPress = useLongPress(onOpen)
  const shown = picture && picture.src !== broken ? picture : undefined
  const status = [shown ? shown.taken && age(shown.taken, now) : picture === undefined ? 'loading' : 'no picture', note].filter(Boolean).join(' · ')
  return (
    <section {...longPress} className={`group/card space-y-1.5 rounded-xl bg-neutral-900 p-1.5 select-none ${dimmed ? 'opacity-50' : ''}`}>
      <button
        onClick={() => setLive(true)}
        aria-label={`Watch ${name} live`}
        className="group relative block aspect-video w-full overflow-hidden rounded-lg bg-neutral-800"
      >
        {shown ? (
          <img src={shown.src} alt={`${name}, its latest picture`} onError={() => setBroken(shown.src)} className="size-full object-cover" />
        ) : (
          <Svg path={mdiCctv} className="absolute inset-0 m-auto size-10 text-neutral-600" />
        )}
        <span className="absolute right-2 bottom-2 rounded-full bg-black/60 p-1.5 text-white group-hover:bg-amber-500 group-hover:text-black">
          <Svg path={mdiPlay} className="size-5" />
        </span>
      </button>
      {live && createPortal(<LiveViewer target={target} name={name} onClose={() => setLive(false)} />, document.body)}
      <div className="relative flex items-center gap-3 px-3">
        <div className="min-w-0 flex-1">
          <NameLine name={name} hidden={hideName} badges={badges} />
          <div className={`flex items-center gap-3 ${statusSize(hideName)}`}>
            <span className="truncate">{status}</span>
            {state && <StateText {...state} />}
          </div>
        </div>
        <More onOpen={onOpen} />
      </div>
    </section>
  )
}
