// The Tiles of a Dashboard, drawn as dashboard.ts resolves them.

import type { ReactNode } from 'react'
import { useAvailability } from './store'
import type { Fn } from './types'
import { titleIfTruncated } from './truncated'
import { titled } from './tiles'
import type { TargetTile } from './dashboard'
import { BarTile } from './BarTile'
import { CameraTile } from './CameraTile'
import { SensorTile } from './SensorTile'
import { FunctionControls, HealthBadge } from './Capabilities'
import { More, useLongPress } from './TileParts'
// A Target's Tile, as dashboard.ts resolves it, drawn as its shape says: a bar, a sensor's, or its
// Functions' controls. A lone Function shares the Tile's header; a Function's kind or key shows as
// a heading only where it adds something. Dimmed while its Device is detached or it is offline.
// Its name may be hidden: an Admin's ⋯, or a long press on its header, then opens its panel.
export function Tile({
  tile,
  compact,
  hideName = false,
  onOpen,
}: {
  tile: TargetTile
  compact?: boolean
  hideName?: boolean
  onOpen?: (back?: () => void) => void
}) {
  const offline = useAvailability(tile.subject) === 'offline'
  const longPress = useLongPress(hideName && onOpen ? () => onOpen() : undefined) // a Tile of controls' header, its name hidden
  const { shape, name, fns } = tile
  const dimmed = tile.detached || offline
  const note = [tile.detached && 'detached', offline && 'offline'].filter(Boolean).join(' · ')
  const badges = tile.health.map((h) => <HealthBadge key={`${h.target}|${h.cap.key}`} {...h} />)
  if (shape.kind === 'bar')
    return (
      <BarTile
        name={name}
        icon={tile.icon}
        members={tile.members}
        note={note}
        badges={badges}
        target={shape.target}
        fn={shape.fn}
        toggle={shape.toggle}
        dimmed={dimmed}
        compact={compact}
        hideName={hideName}
        onSettings={onOpen}
      />
    )
  const props = { name, note, badges, dimmed, hideName, onOpen: onOpen && (() => onOpen()) }
  if (shape.kind === 'camera') return <CameraTile {...props} target={shape.target} state={shape.state} />
  if (shape.kind !== 'controls') return <SensorTile {...props} shape={shape} />
  const header = (command?: ReactNode) => (
    <div className="min-w-0 select-none" {...longPress}>
      {hideName ? null : onOpen ? (
        <button onClick={() => onOpen()} onMouseEnter={titleIfTruncated} className="block max-w-full truncate text-left font-medium hover:underline">
          {name}
        </button>
      ) : (
        <p onMouseEnter={titleIfTruncated} className="truncate font-medium">
          {name}
        </p>
      )}
      <p className="flex items-center gap-2 text-xs text-neutral-500">
        <span>
          {[tile.summary, note].filter(Boolean).join(' · ')}
          {command}
        </span>
        {badges}
      </p>
    </div>
  )
  const heading = (fn: Fn) =>
    titled(fn)
      ? (command: ReactNode) => (
          <span className="text-sm text-neutral-400">
            {fn.capabilities.length === 1 ? fn.capabilities[0]?.label : fn.key}
            <span className="text-xs text-neutral-500">{command}</span>
          </span>
        )
      : undefined
  const [only] = fns
  return (
    <section className={`group/card relative space-y-3 rounded-xl bg-neutral-900 p-4 ${dimmed ? 'opacity-50' : ''}`}>
      {hideName && <More onOpen={onOpen && (() => onOpen())} />}
      {fns.length === 1 && only ? (
        <FunctionControls {...only} heading={header} />
      ) : (
        <>
          {header()}
          {fns.map((f) => (
            <FunctionControls key={f.target} {...f} heading={heading(f.fn)} />
          ))}
        </>
      )}
    </section>
  )
}
