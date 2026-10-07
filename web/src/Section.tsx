import { useState } from 'react'
import { useDraggable, useDroppable } from '@dnd-kit/core'
import { mdiChevronDown, mdiCogOutline, mdiDrag, mdiGauge } from '@mdi/js'
import type { Aggregate } from './types'
import { aggregateTarget, parseTarget } from './targets'
import { AreaIcon, Svg } from './icons'
import type { DashboardSection, TargetTile } from './dashboard'
import { maxColumns, placements, reflow, resize, rows, type Arranged } from './layout'
import { ArrangedTile, Cell, cells, Stepper, type TileNode } from './Arrange'
import { ManualTile, ReadingText, readingIcons, StateText, Tile } from './Tiles'

// The narrowest a column of a Layout gets, in px (14rem), and the gap between two.
const minColumn = 224
const columnGap = 12

// What arranging the dashboard hands an Area's section: how to save its Layout.
export interface Arranging {
  onLayout: (columns: number, layout: Arranged[]) => void
}

// A Section of a Dashboard, drawn from what dashboard.ts derived of it: an Area's, or Others, that
// of the tiles without one. An Area's header is a banner: its Icon if it has one, then its name
// large, its status (climate, presence, doors) on a line under it, its light bar on the right,
// which a folded section keeps. The bar has no chart: its ⋯ sheet has its History. An Area's tiles
// sit where its Layout places them, gaps included, while its columns fit; narrower, they come one
// after another, as Others' do. The tiles it hides wait behind a link; a tap on its name folds it
// all away. While arranging, an Area's section is open, its header drags it among the others, and
// its grid shows every cell, hidden tiles dimmed in theirs.
export function Section({
  section,
  title,
  collapsed,
  onCollapse,
  onOpen,
  onResult,
  arranging,
}: {
  section: DashboardSection
  title?: string
  collapsed: boolean
  onCollapse: () => void
  onOpen?: (id: string, back?: () => void) => void // an Admin's: an Area's, a Device's, an Aggregate's or a Flag's panel
  onResult: (r: { text: string; error?: boolean }) => void // a Manual trigger's
  arranging?: Arranging
}) {
  const { columns } = section
  const hidden = section.area?.hidden ?? []
  // A Tile's ⋯ opens its Device's, Aggregate's or Flag's panel.
  const opener = (t: TargetTile) => onOpen && ((back?: () => void) => onOpen(parseTarget(t.subject)!.id, back))
  const onSettings = onOpen && section.area && (() => onOpen(section.area!.id))
  const tiles = section.tiles.map((t): TileNode => ({
    ...t,
    node: t.kind === 'manual' ? <ManualTile key={t.key} automation={t.automation} onResult={onResult} /> : <Tile key={t.key} tile={t} onOpen={opener(t)} />,
  }))
  const status = (
    <>
      <Climate aggregates={section.climate} />
      {section.presence && <AreaState aggregate={section.presence} />}
      {section.doors && <AreaState aggregate={section.doors} />}
    </>
  )
  const bar = section.bar && <Tile tile={section.bar} compact onOpen={opener(section.bar)} />
  const [showHidden, setShowHidden] = useState(false)
  const [width, setWidth] = useState(0)
  const measure = (el: HTMLDivElement | null) => {
    if (!el) return
    const o = new ResizeObserver(([e]) => setWidth(e!.contentRect.width))
    o.observe(el)
    return () => o.disconnect()
  }
  const area = section.area?.id ?? ''
  const drag = useDraggable({ id: `area:${area}`, data: { type: 'area', area }, disabled: !arranging })
  const drop = useDroppable({ id: `area:${area}`, data: { type: 'area', area }, disabled: !arranging })
  const shown = tiles.filter((t) => !hidden.includes(t.key))
  const folded = tiles.length - shown.length
  const visible = showHidden ? tiles : shown
  const open = !collapsed || !title || !!arranging
  const grid = columns !== undefined && width >= columns * minColumn + (columns - 1) * columnGap
  const layout = placements(tiles)
  return (
    <section
      ref={(el) => (drag.setNodeRef(el), drop.setNodeRef(el))}
      style={drag.transform ? { transform: `translate3d(${drag.transform.x}px, ${drag.transform.y}px, 0)` } : undefined}
      className={`overflow-hidden rounded-2xl border ${drag.isDragging ? 'relative z-20 border-amber-400 bg-neutral-900 shadow-2xl' : drop.isOver ? 'border-amber-400' : 'border-neutral-800 bg-neutral-900/30'}`}
    >
      {title && (
        <header className="flex flex-wrap items-stretch gap-x-4 gap-y-3 bg-linear-to-r from-neutral-800/60 to-transparent px-4 py-3">
          {/* the bar as tall as the header, beside the name; from sm, too narrow for both, it goes under it */}
          <div className="-ml-2 flex min-w-0 flex-1 gap-2 sm:min-w-48">
            {/* on its left, as tall as the header: the fold, or while arranging the handle that drags it */}
            {arranging ? (
              <button
                {...drag.listeners}
                {...drag.attributes}
                aria-label={`Move ${title}`}
                title="Drag to move"
                className="grid w-9 shrink-0 cursor-grab touch-none place-items-center rounded-xl text-amber-300 hover:bg-neutral-800"
              >
                <Svg path={mdiDrag} className="size-7" />
              </button>
            ) : (
              // the name folds it too, and speaks for both
              <button
                onClick={onCollapse}
                tabIndex={-1}
                aria-hidden
                className="group grid w-9 shrink-0 place-items-center text-neutral-300 hover:text-amber-200"
              >
                <span className="grid size-9 place-items-center rounded-full bg-neutral-800 group-hover:bg-neutral-700">
                  <Svg path={mdiChevronDown} className={`size-6 transition-transform ${open ? '' : '-rotate-90'}`} />
                </span>
              </button>
            )}
            <div className="min-w-0 flex-1 self-center">
              <div className="flex items-center gap-1">
                <button
                  onClick={onCollapse}
                  disabled={!!arranging}
                  aria-expanded={open}
                  className="flex min-w-0 items-center gap-2 enabled:hover:text-amber-200"
                >
                  <AreaIcon icon={section.area?.icon} className="size-6 shrink-0" />
                  <h2 className="truncate text-xl font-semibold tracking-tight">{title}</h2>
                </button>
                {onSettings && !arranging && (
                  <button
                    onClick={onSettings}
                    aria-label={`${title} settings`}
                    title="Settings"
                    className="grid size-7 shrink-0 place-items-center rounded-full text-neutral-500 hover:bg-neutral-800 hover:text-white"
                  >
                    <Svg path={mdiCogOutline} className="size-5" />
                  </button>
                )}
                {arranging && columns && (
                  <Stepper
                    value={columns}
                    max={maxColumns}
                    label="columns"
                    onChange={(n) => arranging.onLayout(n, reflow(layout, n))}
                    className="ml-auto text-sm"
                  />
                )}
              </div>
              <div className="mt-0.5 flex flex-wrap gap-x-4 gap-y-1 text-sm empty:hidden">{status}</div>
            </div>
          </div>
          {bar}
        </header>
      )}
      {open && (
        <div ref={measure} className="space-y-3 p-3 empty:hidden">
          {arranging && columns ? (
            <div className="grid auto-rows-[minmax(3.5rem,auto)] items-start gap-3" style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}>
              {/* every cell, one row more than the tiles take, for a tile to land in */}
              {[...Array((Math.max(0, ...layout.map((p) => p.row + rows(p))) + 1) * columns).keys()].map((i) => (
                <Cell key={i} area={area} col={i % columns} row={Math.floor(i / columns)} />
              ))}
              {tiles.map((t) => (
                <ArrangedTile
                  key={t.key}
                  area={area}
                  tile={t}
                  hidden={hidden.includes(t.key)}
                  columns={columns}
                  onSize={(size) => arranging.onLayout(columns, resize(layout, t.key, size, columns))}
                />
              ))}
            </div>
          ) : visible.length > 0 && grid ? (
            <div className="grid auto-rows-[minmax(3.5rem,auto)] items-start gap-3" style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}>
              {visible.map((t) => (
                <div key={t.key} style={t.place && cells(t.place)}>
                  {t.node}
                </div>
              ))}
            </div>
          ) : visible.length > 0 ? (
            <div className="grid grid-flow-dense grid-cols-[repeat(auto-fill,minmax(16rem,1fr))] items-start gap-3">{visible.map((t) => t.node)}</div>
          ) : (
            !bar && !folded && <p className="text-sm text-neutral-600">Nothing here yet: add devices from its settings.</p>
          )}
          {folded > 0 && !arranging && (
            <button onClick={() => setShowHidden(!showHidden)} className="text-xs text-neutral-500 hover:text-white">
              {showHidden ? `Hide ${folded} again` : `${folded} hidden`}
            </button>
          )}
        </div>
      )}
    </section>
  )
}

// An Area's presence or doors in its header: the state of its Aggregate of occupancy or of contacts.
function AreaState({ aggregate }: { aggregate: Aggregate }) {
  const cap = aggregate.capabilities?.find((c) => c.type === 'binary')
  return cap ? <StateText target={aggregateTarget(aggregate.id)} cap={cap} /> : null
}

// An Area's climate in its header: its mean temperature and humidity, its worst CO₂, each after its
// icon. A tap on one opens its History.
function Climate({ aggregates }: { aggregates: Aggregate[] }) {
  return aggregates.flatMap((a) => {
    const cap = a.capabilities?.find((c) => c.key === a.kind)
    return cap
      ? [
          <span key={a.id} className="flex items-center gap-1 whitespace-nowrap">
            <Svg path={readingIcons[cap.key] ?? mdiGauge} className="size-4 text-neutral-500" />
            <ReadingText target={aggregateTarget(a.id)} cap={cap} />
          </span>,
        ]
      : []
  })
}
