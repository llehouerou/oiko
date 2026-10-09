// The bar Tile of a light, a plug, a siren or a Flag.

import { useRef, useState, type CSSProperties, type PointerEvent, type ReactNode } from 'react'
import { mdiAlarmLight, mdiAlarmLightOutline, mdiCogOutline, mdiFlag, mdiFlagOutline, mdiPowerPlug, mdiPowerPlugOutline } from '@mdi/js'
import { sendCommand, useCommand, useControl, useOnCount, useValue } from './store'
import type { Fn, Target } from './types'
import { titleIfTruncated } from './truncated'
import { display } from './controls'
import { LightIcon, Svg } from './icons'
import { MiniChart } from './MiniChart'
import { Panel } from './Panel'
import { CommandNote, FunctionControls } from './Capabilities'
import { More, NameLine, statusSize } from './TileParts'

const clamp = (f: number) => Math.min(1, Math.max(0, f))

// A bar's icon, off and on, outside lights: those have their own, picked per Device.
const barIcons: Record<string, [off: string, on: string]> = {
  switch: [mdiPowerPlugOutline, mdiPowerPlug],
  alarm: [mdiAlarmLightOutline, mdiAlarmLight],
  flag: [mdiFlagOutline, mdiFlag],
}

// A light's, a plug's or a siren's tile is a single control, a bar: a tap toggles it, a sideways
// drag sets a light's brightness from where it was, a vertical one scrolls the page. ⋯ or a long
// press opens its other controls and readings, and from there its Device's or Aggregate's panel,
// which goes back to them. In an Area's header on a phone, the bar is a square: its icon alone, its
// level filling it from the bottom.
export function BarTile({
  name,
  icon,
  members,
  note,
  badges,
  target,
  toggle: key,
  fn,
  dimmed,
  compact = false,
  hideName = false,
  onSettings,
}: {
  name: string
  icon?: string
  members?: Target[] // an Aggregate's lights
  note: string // detached, offline…
  badges?: ReactNode // its health, when wrong
  target: Target
  toggle: string // its main control, an on/off
  fn: Fn
  dimmed: boolean
  compact?: boolean // in an Area's header: the bar alone, its name only on its sheet; a square on a phone
  hideName?: boolean
  onSettings?: (back: () => void) => void // an Admin's: its Device's or Aggregate's panel
}) {
  const cap = fn.capabilities.find((c) => c.key === 'brightness' && c.access.settable && c.min != null && c.max != null)
  const on = useValue({ target, capability: key })?.data === true
  const powerCap = fn.capabilities.find((c) => c.key === 'power')
  const power = useValue({ target, capability: 'power' })?.data // a plug's, while on
  const command = useCommand(target)
  const { value: picked, set, picking, hold } = useControl<number>(target, 'brightness', { fade: true })
  const brightness = typeof picked === 'number' ? picked : undefined
  const [sheet, setSheet] = useState(false)
  const gesture = useRef<{ x: number; y: number; from: number; width: number; moving: boolean; press: number } | null>(null)
  const tapped = useRef(false) // the gesture that just ended neither dimmed nor scrolled
  const lit = on || picking
  const group = members !== undefined
  const litCount = useOnCount((members ?? []).map((m) => ({ target: m, capability: 'state' })))
  const min = Math.max(1, cap?.min ?? 1) // brightness 0 switches the light off: that is the tap's job
  const max = cap?.max ?? 1
  const level = !lit ? 0 : cap && brightness !== undefined ? clamp((brightness - min) / (max - min)) : 1
  const toggle = () => sendCommand(target, { [key]: 'toggle' })
  const dim = (f: number) => {
    if (!cap) return
    set(Math.round(min + clamp(f) * (max - min)))
  }

  const down = (e: PointerEvent<HTMLDivElement>) => {
    const press = window.setTimeout(() => ((gesture.current = null), hold(false), setSheet(true)), 500)
    // a drag across the phone's square still dims over a finger's travel
    gesture.current = { x: e.clientX, y: e.clientY, from: level, width: Math.max(e.currentTarget.clientWidth, 160), moving: false, press }
  }
  const end = () => {
    const g = gesture.current
    if (g) clearTimeout(g.press)
    gesture.current = null
    hold(false)
    return g
  }
  const move = (e: PointerEvent<HTMLDivElement>) => {
    const g = gesture.current
    if (!g) return
    const dx = e.clientX - g.x
    if (!g.moving) {
      if (Math.abs(e.clientY - g.y) > 10) return void end() // a scroll
      if (Math.abs(dx) < 8) return
      g.moving = true
      clearTimeout(g.press)
      e.currentTarget.setPointerCapture(e.pointerId)
      hold(true)
    }
    dim(g.from + dx / g.width)
  }
  const up = () => {
    const g = end()
    tapped.current = !!g && !g.moving // not after a long press either: it ended the gesture
  }
  // The toggle waits for the click, not the pointerup: a mobile browser sends no
  // click for a touch that scrolled or stopped a fling, so scrolling never switches a light.
  const click = () => tapped.current && ((tapped.current = false), toggle())

  const status = (
    <>
      {[
        lit && group && `${litCount}/${members.length}`,
        !lit ? 'off' : cap && brightness !== undefined ? display(brightness, cap) : 'on',
        lit && powerCap && power !== undefined && display(power, powerCap),
        note,
      ]
        .filter(Boolean)
        .join(' · ')}
      <CommandNote command={command} />
    </>
  )
  return (
    <>
      <section
        title={compact ? name : undefined}
        className={`${compact ? 'flex w-14 shrink-0 sm:w-56' : 'rounded-xl bg-neutral-900 p-1.5'} ${dimmed ? 'opacity-50' : ''}`}
      >
        <div
          role="switch"
          aria-checked={lit}
          aria-label={name}
          tabIndex={0}
          onPointerDown={down}
          onPointerMove={move}
          onPointerUp={up}
          onPointerCancel={() => (end(), (tapped.current = false))}
          onContextMenu={(e) => e.preventDefault()}
          onClick={click}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') toggle()
            else if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') dim(level + (e.key === 'ArrowRight' ? 0.1 : -0.1))
            else return
            e.preventDefault()
          }}
          className={`group/card relative ${compact ? 'min-h-12 flex-1' : 'h-14'} cursor-pointer touch-pan-y overflow-hidden rounded-lg bg-neutral-800 select-none`}
        >
          <div
            className={`absolute bg-amber-400/25 ${compact ? 'inset-x-0 bottom-0 h-(--level) sm:inset-x-auto sm:inset-y-0 sm:left-0 sm:h-auto sm:w-(--level)' : 'inset-y-0 left-0 w-(--level)'}`}
            style={{ '--level': `${lit ? Math.max(level, 0.04) * 100 : 0}%` } as CSSProperties}
          />
          <div className={`relative flex h-full items-center ${compact ? 'justify-center gap-2.5 sm:justify-start sm:px-3' : 'gap-3 px-3'}`}>
            {fn.kind === 'light' ? (
              <LightIcon icon={icon} group={group} lit={lit} className={`size-6 shrink-0 ${lit ? 'text-amber-300' : 'text-neutral-500'}`} />
            ) : (
              <Svg path={(barIcons[fn.kind] ?? barIcons.switch!)[lit ? 1 : 0]} className={`size-6 shrink-0 ${lit ? 'text-amber-300' : 'text-neutral-500'}`} />
            )}
            {compact ? (
              <p className="hidden min-w-0 flex-1 truncate text-sm text-neutral-300 sm:block">{status}</p>
            ) : (
              <div className="min-w-0 flex-1">
                <NameLine name={name} hidden={hideName} badges={badges} />
                <p className={`truncate ${statusSize(hideName)}`}>{status}</p>
              </div>
            )}
            <More onOpen={() => setSheet(true)} />
          </div>
        </div>
        {!compact && (
          <div className="mt-2 px-2.5 pb-1 empty:hidden">
            <MiniChart target={target} fn={fn} />
          </div>
        )}
      </section>
      {sheet && (
        <Panel
          title={
            <>
              <h2 className="truncate text-lg font-medium" onMouseEnter={titleIfTruncated}>
                {name}
              </h2>
              <p className="text-xs text-neutral-500">{status}</p>
            </>
          }
          onClose={() => setSheet(false)}
        >
          <FunctionControls target={target} fn={fn} />
          {onSettings && (
            <div className="mt-6 flex justify-end border-t border-neutral-800 pt-4">
              <button
                onClick={() => (setSheet(false), onSettings(() => setSheet(true)))}
                className="flex items-center gap-1.5 rounded-full bg-neutral-800 py-1.5 pr-3.5 pl-2.5 text-sm text-neutral-300 hover:bg-neutral-700 hover:text-white"
              >
                <Svg path={mdiCogOutline} className="size-4" />
                Settings
              </button>
            </div>
          )}
        </Panel>
      )}
    </>
  )
}
