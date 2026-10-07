// Arranging an Area's Layout on the dashboard: the cells a tile may land in, the veil that drags
// and sizes a tile, and the steppers that count columns and rows.

import type { CSSProperties, ReactNode } from 'react'
import { pointerWithin, useDraggable, useDroppable, type CollisionDetection } from '@dnd-kit/core'
import { mdiArrowExpandHorizontal, mdiArrowExpandVertical } from '@mdi/js'
import { Svg } from './icons'
import { maxRows, rows, type Place } from './layout'

// A tile of the dashboard, drawn: key is its Target, which an Area may hide, or its Automation's id.
export interface TileNode {
  key: string
  label: string
  place?: Place // in an Area's Layout
  node: ReactNode
}

// Where a drag lands: an Area on another Area, a tile in a cell of its own Area's grid.
export const landing: CollisionDetection = (args) => {
  const from = args.active.data.current
  const fits = (to?: Record<string, unknown>) => to?.type === from?.type && (from?.type === 'area' || to?.area === from?.area)
  return pointerWithin({ ...args, droppableContainers: args.droppableContainers.filter((d) => fits(d.data.current)) })
}

// A place's cells in the grid of a Layout. A tile several rows tall fills them, whatever its content.
export const cells = (p: Place): CSSProperties => ({
  gridColumn: `${p.col + 1} / span ${p.width}`,
  gridRow: `${p.row + 1} / span ${rows(p)}`,
  ...(rows(p) > 1 && { alignSelf: 'stretch', display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)' }),
})

// A cell of an Area's grid while arranging, where one of its tiles may land.
export function Cell({ area, col, row }: { area: string; col: number; row: number }) {
  const { setNodeRef, isOver } = useDroppable({ id: `cell:${area}:${col}:${row}`, data: { type: 'tile', area, col, row } })
  return (
    <div
      ref={setNodeRef}
      style={{ gridColumn: col + 1, gridRow: row + 1 }}
      className={`min-h-14 self-stretch rounded-xl border border-dashed ${isOver ? 'border-amber-400 bg-amber-400/10' : 'border-neutral-700/60'}`}
    />
  )
}

// A tile while arranging: a veil over it keeps its controls from a tap and drags it to another
// cell; its width steps from one column to them all, its height from one row to maxRows.
export function ArrangedTile({
  area,
  tile,
  hidden,
  columns,
  onSize,
}: {
  area: string
  tile: TileNode
  hidden: boolean
  columns: number
  onSize: (size: { width?: number; height?: number }) => void
}) {
  const { setNodeRef, listeners, attributes, transform, isDragging } = useDraggable({
    id: `tile:${area}:${tile.key}`,
    data: { type: 'tile', area, tile: tile.key },
  })
  const place = tile.place!
  return (
    <div
      ref={setNodeRef}
      style={{ ...cells(place), transform: transform ? `translate3d(${transform.x}px, ${transform.y}px, 0)` : undefined }}
      className={`relative ${isDragging ? 'z-20 shadow-2xl' : ''}`}
    >
      <div className={`grid grid-cols-[minmax(0,1fr)] ${hidden ? 'opacity-30' : ''}`}>{tile.node}</div>
      <div
        {...listeners}
        {...attributes}
        aria-label={`Move ${tile.label}`}
        className="absolute inset-0 flex cursor-grab touch-none items-end justify-end gap-1 rounded-xl p-1.5 ring-1 ring-amber-400/50 hover:bg-amber-400/5"
      >
        <Stepper
          icon={mdiArrowExpandHorizontal}
          value={place.width}
          max={columns}
          label="columns wide"
          onChange={(width) => onSize({ width })}
          className="bg-neutral-950/90 text-xs"
        />
        <Stepper
          icon={mdiArrowExpandVertical}
          value={rows(place)}
          max={maxRows}
          label="rows tall"
          onChange={(height) => onSize({ height })}
          className="bg-neutral-950/90 text-xs"
        />
      </div>
    </div>
  )
}

// − value +, from 1 to max, after an icon telling what it counts; a press on it never starts a drag.
export function Stepper({
  icon,
  value,
  max,
  label,
  onChange,
  className = '',
}: {
  icon?: string
  value: number
  max: number
  label: string
  onChange: (n: number) => void
  className?: string
}) {
  const step = 'grid size-7 place-items-center rounded-full hover:bg-neutral-700 disabled:opacity-30 disabled:hover:bg-transparent'
  return (
    <span onPointerDown={(e) => e.stopPropagation()} className={`flex items-center rounded-full bg-neutral-800 text-neutral-200 ${className}`}>
      <button disabled={value <= 1} onClick={() => onChange(value - 1)} aria-label={`Fewer ${label}`} className={step}>
        −
      </button>
      <span title={label} className="flex min-w-4 items-center justify-center gap-0.5 tabular-nums">
        {icon && <Svg path={icon} className="size-3.5 text-neutral-400" />}
        {value}
      </span>
      <button disabled={value >= max} onClick={() => onChange(value + 1)} aria-label={`More ${label}`} className={step}>
        +
      </button>
    </span>
  )
}
