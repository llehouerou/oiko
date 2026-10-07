import { useState } from 'react'
import { DndContext, PointerSensor, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { edit } from './store'
import type { DashboardSection } from './dashboard'
import { move, placements, stored, type Arranged } from './layout'
import { landing } from './Arrange'
import { Section } from './Section'

// A Dashboard, drawn from the Sections handed to it. A wide screen sets them in two columns,
// alternately: the first on the left, the second on the right, and so on, whatever their heights.
// Narrower, the columns melt away and the Sections come one under another, in their order. While
// arranging, an Admin drags an Area's Section among the others, and a tile to a cell of its Area.
export function Dashboard({
  sections,
  arranging,
  onOpen,
  onResult,
}: {
  sections: DashboardSection[]
  arranging: boolean
  onOpen?: (id: string, back?: () => void) => void // an Admin's: an Area's, a Device's, an Aggregate's or a Flag's panel
  onResult: (r: { text: string; error?: boolean }) => void
}) {
  const areas = sections.flatMap((s) => (s.area ? [s.area.id] : []))
  // The sections this browser folds, by Area id ('' for Others).
  const [collapsed, setCollapsed] = useState<string[]>(() => JSON.parse(localStorage.getItem('oiko.collapsed') ?? '[]'))
  const collapse = (id: string) => {
    const next = collapsed.includes(id) ? collapsed.filter((c) => c !== id) : [...collapsed, id]
    localStorage.setItem('oiko.collapsed', JSON.stringify(next))
    setCollapsed(next)
  }
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))
  const report = (err: string | null) => err && onResult({ text: err, error: true })
  const saveLayout = async (area: string, columns: number, layout: Arranged[]) =>
    report(await edit('PUT', `areas/${area}/layout`, { columns, layout: stored(layout).map(({ key, ...p }) => ({ tile: key, ...p })) }))
  // A dragged Area takes the place of the one it lands on; a dragged tile, the cell it lands on.
  const dropped = async ({ active, over }: DragEndEvent) => {
    const from = active.data.current
    const to = over?.data.current
    if (!from || !to) return
    if (from.type === 'area') {
      if (to.area === from.area) return
      const at = areas.indexOf(to.area)
      const ids = areas.filter((id) => id !== from.area)
      ids.splice(at, 0, from.area)
      return report(await edit('PUT', 'areas', { order: ids }))
    }
    const s = sections.find((s) => s.area?.id === from.area)
    if (s?.columns) saveLayout(from.area, s.columns, move(placements(s.tiles), from.tile, to.col, to.row, s.columns))
  }
  return (
    <DndContext sensors={sensors} collisionDetection={landing} onDragEnd={dropped}>
      <div className="flex flex-col gap-6 xl:flex-row xl:items-start">
        {[0, 1].map((column) => (
          <div key={column} className="contents xl:flex xl:min-w-0 xl:flex-1 xl:flex-col xl:gap-6">
            {sections.map((s, i) => {
              const id = s.area?.id ?? ''
              return (
                i % 2 === column && (
                  <div key={id} style={{ order: i }}>
                    <Section
                      section={s}
                      title={s.area?.name ?? (areas.length ? 'Others' : undefined)}
                      collapsed={collapsed.includes(id)}
                      onCollapse={() => collapse(id)}
                      onOpen={onOpen}
                      onResult={onResult}
                      arranging={arranging && s.area ? { area: id, onLayout: (columns, layout) => saveLayout(id, columns, layout) } : undefined}
                    />
                  </div>
                )
              )
            })}
          </div>
        ))}
      </div>
    </DndContext>
  )
}
