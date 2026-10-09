// What every Tile is made of: its name line, its ⋯ and long press, and how old a Value is.

import { useRef, type MouseEvent, type PointerEvent, type ReactNode } from 'react'
import { mdiDotsHorizontal } from '@mdi/js'
import { titleIfTruncated } from './truncated'
import { Svg } from './icons'

export interface SensorProps {
  name: string
  note: string // detached, offline…
  badges?: ReactNode // its health, when wrong
  dimmed: boolean
  hideName: boolean
  onOpen?: () => void // an Admin's: its Device's or Aggregate's panel
}

// A bar Tile's name, unless hidden, with its health beside it.
export function NameLine({
  name,
  hidden,
  badges,
  className = 'truncate text-base',
}: {
  name: string
  hidden: boolean
  badges?: ReactNode
  className?: string
}) {
  return (
    <div className="flex items-center gap-2 text-xs">
      {!hidden && (
        <p className={`${className} font-medium`} onMouseEnter={titleIfTruncated}>
          {name}
        </p>
      )}
      {badges}
    </div>
  )
}

// A bar Tile's status line: in its name's place and as large once the name is hidden.
export const statusSize = (hideName: boolean) => (hideName ? 'text-base text-neutral-200' : 'text-xs text-neutral-400')

// A Tile's ⋯, opening its sheet or panel, if anything does: small, in the corner of the nearest
// group/card, over what is there, shown while the pointer hovers it. A touch screen has none: a
// long press opens the same (useLongPress).
export function More({ onOpen }: { onOpen?: () => void }) {
  if (!onOpen) return null
  return (
    <button
      onPointerDown={(e) => e.stopPropagation()}
      onClick={(e) => (e.stopPropagation(), onOpen())}
      onKeyDown={(e) => e.stopPropagation()}
      aria-label="More"
      className="absolute top-1 right-1 hidden size-6 place-items-center rounded-full text-neutral-400 opacity-0 transition group-hover/card:opacity-100 hover:bg-white/10 hover:text-neutral-100 focus-visible:opacity-100 pointer-fine:grid"
    >
      <Svg path={mdiDotsHorizontal} className="size-4" />
    </button>
  )
}

// The handlers of an element a long press opens with: onLong once held still for half a second;
// the click that ends it then goes nowhere. Only presses on the element itself count, not those
// React bubbles from a sheet portaled out of it.
export function useLongPress(onLong?: () => void) {
  const press = useRef<{ x: number; y: number; timer: number } | null>(null)
  const fired = useRef(false)
  if (!onLong) return {}
  const stop = () => {
    if (press.current) clearTimeout(press.current.timer)
    press.current = null
  }
  return {
    onPointerDown: (e: PointerEvent<HTMLElement>) => {
      if (!e.currentTarget.contains(e.target as Node)) return
      fired.current = false
      press.current = { x: e.clientX, y: e.clientY, timer: window.setTimeout(() => ((fired.current = true), stop(), onLong()), 500) }
    },
    onPointerMove: (e: PointerEvent<HTMLElement>) => {
      const p = press.current
      if (p && Math.hypot(e.clientX - p.x, e.clientY - p.y) > 10) stop()
    },
    onPointerUp: stop,
    onPointerCancel: stop,
    onContextMenu: (e: MouseEvent<HTMLElement>) => e.preventDefault(),
    onClickCapture: (e: MouseEvent<HTMLElement>) => {
      if (!fired.current) return
      e.stopPropagation()
      fired.current = false
    },
  }
}

const relative = new Intl.RelativeTimeFormat('en', { numeric: 'auto', style: 'narrow' })

export function age(at: string, now: number) {
  const seconds = Math.round((new Date(at).getTime() - now) / 1000)
  if (seconds > -10) return 'just now'
  if (seconds > -3600) return relative.format(Math.round(seconds / 60), 'minute')
  if (seconds > -86400) return relative.format(Math.round(seconds / 3600), 'hour')
  return relative.format(Math.round(seconds / 86400), 'day')
}
