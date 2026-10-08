import { useState } from 'react'
import { DndContext, PointerSensor, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { edit } from './store'
import type { DashboardSection } from './dashboard'
import type { SavedLayout } from './tileLayout'
import { cells, landing } from './Arrange'
import { Section, useWidth } from './Section'

// What a Dashboard hands its Sections: who may open a panel from them, where a Manual trigger's
// result goes.
interface Handlers {
  onOpen?: (id: string, back?: () => void) => void // an Admin's: an Area's, a Device's, an Aggregate's or a Flag's panel
  onResult: (r: { text: string; error?: boolean }) => void
}

// The Sections this browser folds on a Dashboard, kept under key, and how to fold or unfold one.
function useFolds(key: string) {
  const [folded, setFolded] = useState<string[]>(() => JSON.parse(localStorage.getItem(key) ?? '[]'))
  const fold = (id: string) => {
    const next = folded.includes(id) ? folded.filter((c) => c !== id) : [...folded, id]
    localStorage.setItem(key, JSON.stringify(next))
    setFolded(next)
  }
  return [folded, fold] as const
}

// The built-in Dashboard, drawn from the Sections handed to it. A wide screen sets them in two
// columns, alternately: the first on the left, the second on the right, and so on, whatever their
// heights. Narrower, the columns melt away and the Sections come one under another, in their
// order. While arranging, an Admin drags an Area's Section among the others, and a tile to a cell
// of its Area, which its grid saves.
export function Dashboard({ sections, arranging, onOpen, onResult }: { sections: DashboardSection[]; arranging: boolean } & Handlers) {
  const areaIds = sections.flatMap((s) => (s.area ? [s.area.id] : []))
  // The sections this browser folds, by Area id ('' for Others).
  const [collapsed, collapse] = useFolds('oiko.collapsed')
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))
  const report = (err: string | null) => err && onResult({ text: err, error: true })
  const saveLayout = async (area: string, { columns, layout }: SavedLayout) =>
    report(await edit('PUT', `areas/${area}/layout`, { columns, layout: layout.map(({ key, ...p }) => ({ tile: key, ...p })) }))
  // A dragged Area takes the place of the one it lands on.
  const dropped = async ({ active, over }: DragEndEvent) => {
    const from = active.data.current
    const to = over?.data.current
    if (from?.type !== 'area' || !to || to.area === from.area) return
    const at = areaIds.indexOf(to.area)
    const ids = areaIds.filter((id) => id !== from.area)
    ids.splice(at, 0, from.area)
    report(await edit('PUT', 'areas', { order: ids }))
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
                      title={s.area?.name ?? (areaIds.length ? 'Others' : undefined)}
                      collapsed={collapsed.includes(id)}
                      onCollapse={() => collapse(id)}
                      onOpen={onOpen}
                      onResult={onResult}
                      arranging={arranging && s.area ? { onLayout: (saved) => saveLayout(id, saved) } : undefined}
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

// The narrowest a column of a custom Dashboard's Layout gets, in px (20rem), and the gap between two.
const minSectionColumn = 320
const sectionGap = 24

// A custom Dashboard, drawn from its Sections: each in its place on its Layout of columns while
// they fit; narrower, a phone among others, they come one under another in reading order, gaps
// dropped. While it keeps its Layout, so do its own Sections, however narrow: both were arranged
// together. This browser keeps which Sections it folds on this Dashboard alone.
export function CustomDashboardView({ id, columns, sections, onOpen, onResult }: { id: string; columns: number; sections: DashboardSection[] } & Handlers) {
  const [collapsed, collapse] = useFolds(`oiko.collapsed.${id}`)
  const [width, measure] = useWidth()
  const grid = width >= columns * minSectionColumn + (columns - 1) * sectionGap
  if (!sections.length) return <p className="text-sm text-neutral-500">Nothing on this dashboard yet.</p>
  return (
    <DndContext>
      <div
        ref={measure}
        className={grid ? 'grid items-start gap-6' : 'flex flex-col gap-6'}
        style={grid ? { gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` } : undefined}
      >
        {sections.map((s) => (
          <div key={s.key} style={grid && s.place ? cells(s.place) : undefined}>
            <Section
              section={s}
              title={s.area?.name ?? s.own?.name}
              collapsed={collapsed.includes(s.key)}
              onCollapse={() => collapse(s.key)}
              onOpen={onOpen}
              onResult={onResult}
              keepLayout={grid && !!s.own}
            />
          </div>
        ))}
      </div>
    </DndContext>
  )
}
