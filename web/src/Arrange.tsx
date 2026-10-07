// Arranging a Layout of tiles, an Area's on the built-in Dashboard or an own Section's in a
// Dashboard's editor: the cells a tile may land in, the veil that drags and sizes a tile, and the
// steppers that count columns and rows.

import type { CSSProperties, ReactNode } from 'react'
import { pointerWithin, useDraggable, useDroppable, type CollisionDetection } from '@dnd-kit/core'
import { mdiArrowExpandHorizontal, mdiArrowExpandVertical, mdiClose, mdiPlus } from '@mdi/js'
import { Svg } from './icons'
import { cellsOf, maxRows, placements, resize, rows, type Arranged, type Place } from './layout'

// A tile of a Dashboard, drawn: key names it on its Layout: its Target, which an Area may hide, or
// its Automation.
export interface TileNode {
  key: string
  label: string
  place?: Place // in its Layout
  node: ReactNode
}

// Where a drag lands: an Area on another Area, a Section in a cell of its Dashboard, a tile in a
// cell of its own grid.
export const landing: CollisionDetection = (args) => {
  const from = args.active.data.current
  const fits = (to?: Record<string, unknown>) => to?.type === from?.type && (from?.type === 'area' || to?.grid === from?.grid)
  return pointerWithin({ ...args, droppableContainers: args.droppableContainers.filter((d) => fits(d.data.current)) })
}

// The tiles of grid while arranging, in a grid of columns: every cell, for a tile to land in, and
// each tile under its veil, those hidden dimmed. Each change hands the whole Layout to onLayout; a
// drop is the page's. In an own Section, a free cell has a + adding a tile there (onAdd), and a
// tile a button removing it (onRemove).
export function ArrangedGrid({
  grid,
  columns,
  tiles,
  hidden = [],
  onLayout,
  onAdd,
  onRemove,
}: {
  grid: string
  columns: number
  tiles: TileNode[]
  hidden?: string[]
  onLayout: (columns: number, layout: Arranged[]) => void
  onAdd?: (col: number, row: number) => void
  onRemove?: (key: string) => void
}) {
  const layout = placements(tiles)
  return (
    <div className="grid auto-rows-[minmax(3.5rem,auto)] items-start gap-3" style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}>
      <Cells grid={grid} type="tile" columns={columns} layout={layout} what="a tile" onAdd={onAdd} />
      {tiles.map((t) => (
        <ArrangedTile
          key={t.key}
          grid={grid}
          tile={t}
          hidden={hidden.includes(t.key)}
          columns={columns}
          onSize={(size) => onLayout(columns, resize(layout, t.key, size, columns))}
          onRemove={onRemove && (() => onRemove(t.key))}
        />
      ))}
    </div>
  )
}

// A place's cells in the grid of a Layout. A tile several rows tall fills them, whatever its content.
export const cells = (p: Place): CSSProperties => ({
  gridColumn: `${p.col + 1} / span ${p.width}`,
  gridRow: `${p.row + 1} / span ${rows(p)}`,
  ...(rows(p) > 1 && { alignSelf: 'stretch', display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)' }),
})

// Every cell of grid, a Layout of columns, one row more than layout takes, where what is dragged of
// type may land. With onAdd, a free one has a + adding what there.
export function Cells({
  grid,
  type,
  columns,
  layout,
  what,
  onAdd,
}: {
  grid: string
  type: string
  columns: number
  layout: Place[]
  what: string
  onAdd?: (col: number, row: number) => void
}) {
  const taken = new Set(layout.flatMap((p) => cellsOf(p, columns)))
  const height = Math.max(0, ...layout.map((p) => p.row + rows(p))) + 1
  return [...Array(height * columns).keys()].map((i) => {
    const col = i % columns
    const row = Math.floor(i / columns)
    return (
      <Cell key={i} grid={grid} type={type} col={col} row={row}>
        {onAdd && !taken.has(i) && (
          <button
            onClick={() => onAdd(col, row)}
            aria-label={`Add ${what} at column ${col + 1}, row ${row + 1}`}
            className="grid size-9 place-items-center rounded-full text-neutral-500 hover:bg-neutral-800 hover:text-white"
          >
            <Svg path={mdiPlus} className="size-5" />
          </button>
        )}
      </Cell>
    )
  })
}

// A cell of grid while arranging, where what is dragged of type may land.
function Cell({ grid, type, col, row, children }: { grid: string; type: string; col: number; row: number; children?: ReactNode }) {
  const { setNodeRef, isOver } = useDroppable({ id: `cell:${grid}:${col}:${row}`, data: { type, grid, col, row } })
  return (
    <div
      ref={setNodeRef}
      style={{ gridColumn: col + 1, gridRow: row + 1 }}
      className={`grid min-h-14 place-items-center self-stretch rounded-xl border border-dashed ${isOver ? 'border-amber-400 bg-amber-400/10' : 'border-neutral-700/60'}`}
    >
      {children}
    </div>
  )
}

// A tile while arranging: a veil over it keeps its controls from a tap and drags it to another
// cell of its grid; its width steps from one column to them all, its height from one row to
// maxRows; with onRemove, a button in its corner removes it.
function ArrangedTile({
  grid,
  tile,
  hidden,
  columns,
  onSize,
  onRemove,
}: {
  grid: string
  tile: TileNode
  hidden: boolean
  columns: number
  onSize: (size: { width?: number; height?: number }) => void
  onRemove?: () => void
}) {
  const { setNodeRef, listeners, attributes, transform, isDragging } = useDraggable({
    id: `tile:${grid}:${tile.key}`,
    data: { type: 'tile', grid, tile: tile.key },
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
        <SizeSteppers place={place} columns={columns} onSize={onSize} className="bg-neutral-950/90 text-xs" />
      </div>
      {onRemove && (
        <button
          onClick={onRemove}
          aria-label={`Remove ${tile.label}`}
          title="Remove"
          className="absolute top-1.5 right-1.5 grid size-7 place-items-center rounded-full bg-neutral-950/90 text-neutral-300 hover:bg-red-950 hover:text-red-300"
        >
          <Svg path={mdiClose} className="size-4" />
        </button>
      )}
    </div>
  )
}

// The width of what sits at place, from one column to them all, and its height, from one row to maxRows.
export function SizeSteppers({
  place,
  columns,
  onSize,
  className,
}: {
  place: Place
  columns: number
  onSize: (size: { width?: number; height?: number }) => void
  className?: string
}) {
  return (
    <>
      <Stepper
        icon={mdiArrowExpandHorizontal}
        value={place.width}
        max={columns}
        label="columns wide"
        onChange={(width) => onSize({ width })}
        className={className}
      />
      <Stepper
        icon={mdiArrowExpandVertical}
        value={rows(place)}
        max={maxRows}
        label="rows tall"
        onChange={(height) => onSize({ height })}
        className={className}
      />
    </>
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
