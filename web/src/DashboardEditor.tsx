import { useState, type ReactNode } from 'react'
import { DndContext, PointerSensor, pointerWithin, useDraggable, useDroppable, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import {
  mdiArrowExpandHorizontal,
  mdiArrowExpandVertical,
  mdiCheck,
  mdiDeleteOutline,
  mdiDrag,
  mdiPlus,
  mdiShapeOutline,
  mdiViewColumnOutline,
  mdiViewGridOutline,
} from '@mdi/js'
import { edit } from './store'
import type { Area, CustomDashboard } from './types'
import type { DashboardSection } from './dashboard'
import { addSection, moveSection, removeSection, resizeSection, setColumns, setSectionColumns, updateSection, type SectionContent } from './editDashboard'
import { defaultColumns, maxColumns, maxRows, rows } from './layout'
import { confirm } from './confirm'
import { AreaIcon, AreaIconPicker, Svg } from './icons'
import { Panel } from './Panel'
import { cells, Stepper } from './Arrange'
import { Section } from './Section'

// A custom Dashboard turned into its editor, in place: everything live and at full size. A bar on
// top holds its Name, its columns, Delete and Done; every free cell of its grid has a + adding a
// Section there; each Section has a bar that drags it to another cell, steps its size and removes
// it, and on an own Section edits its Name, Icon and columns. An Area's Section shows the Area
// dimmed: it follows the Area, arranged on Home. Each change saves the whole Dashboard at once.
export function DashboardEditor({
  dashboard: d,
  sections,
  areas,
  onDone,
  onResult,
}: {
  dashboard: CustomDashboard
  sections: DashboardSection[] // d's, as custom draws them while editing
  areas: Area[]
  onDone: () => void
  onResult: (r: { text: string; error?: boolean }) => void
}) {
  const [adding, setAdding] = useState<{ col: number; row: number } | null>(null)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))
  const save = async (next: CustomDashboard) => {
    const err = await edit('PUT', `dashboards/${d.id}`, next)
    if (err) onResult({ text: err, error: true })
  }
  const remove = async () => {
    if (!(await confirm(`Delete the dashboard ${d.name}?`, 'Delete'))) return
    const err = await edit('DELETE', `dashboards/${d.id}`)
    if (err) return onResult({ text: err, error: true })
    onDone()
    location.hash = '#'
  }
  const dropped = ({ active, over }: DragEndEvent) => {
    const to = over?.data.current
    if (to) save(moveSection(d, String(active.data.current?.key), to.col, to.row))
  }
  // every cell, one row more than the Sections take, for one to be added or land in
  const height = Math.max(0, ...sections.map((s) => s.place!.row + rows(s.place!))) + 1
  const taken = new Set(sections.flatMap((s) => covered(s.place!)))
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3 rounded-2xl border border-amber-400/40 bg-neutral-900 p-3">
        <NameInput name={d.name} label="Dashboard name" onName={(name) => name && save({ ...d, name })} className="min-w-40 flex-1 text-lg font-semibold" />
        <Stepper icon={mdiViewColumnOutline} value={d.columns} max={maxColumns} label="columns" onChange={(n) => save(setColumns(d, n))} className="text-sm" />
        <button onClick={remove} className="flex items-center gap-1 rounded-full px-3 py-1.5 text-sm text-red-300 hover:bg-red-950">
          <Svg path={mdiDeleteOutline} className="size-5" />
          Delete
        </button>
        <button
          onClick={onDone}
          className="flex items-center gap-1 rounded-full bg-amber-400 px-4 py-1.5 text-sm font-medium text-neutral-950 hover:bg-amber-300"
        >
          <Svg path={mdiCheck} className="size-5" />
          Done
        </button>
      </div>
      <DndContext sensors={sensors} collisionDetection={pointerWithin} onDragEnd={dropped}>
        <div className="grid auto-rows-[minmax(5rem,auto)] items-start gap-6" style={{ gridTemplateColumns: `repeat(${d.columns}, minmax(0, 1fr))` }}>
          {[...Array(height * d.columns).keys()].map((i) => {
            const col = i % d.columns
            const row = Math.floor(i / d.columns)
            return <FreeCell key={i} col={col} row={row} free={!taken.has(`${col},${row}`)} onAdd={() => setAdding({ col, row })} />
          })}
          {sections.map((s) => (
            <EditedSection
              key={s.key}
              section={s}
              columns={d.columns}
              onSize={(size) => save(resizeSection(d, s.key, size))}
              onRemove={() => save(removeSection(d, s.key))}
              onChange={(change) => save(updateSection(d, s.key, change))}
              onColumns={(n) => save(setSectionColumns(d, s.key, n))}
            >
              <Section section={s} title={s.area?.name ?? s.own?.name} collapsed={false} onCollapse={() => {}} onResult={onResult} />
            </EditedSection>
          ))}
        </div>
      </DndContext>
      {adding && (
        <AddSection
          areas={areas}
          on={d.sections.flatMap((s) => (s.area !== undefined ? [s.area] : []))}
          onAdd={(s) => (setAdding(null), save(addSection(d, s, adding.col, adding.row)))}
          onClose={() => setAdding(null)}
        />
      )}
    </div>
  )
}

// The cells a place covers, each as "col,row".
const covered = (p: { col: number; row: number; width: number; height?: number }) =>
  [...Array(rows(p)).keys()].flatMap((r) => [...Array(p.width).keys()].map((c) => `${p.col + c},${p.row + r}`))

// A cell of the Dashboard's grid, where a dragged Section lands; a free one has a + adding one.
function FreeCell({ col, row, free, onAdd }: { col: number; row: number; free: boolean; onAdd: () => void }) {
  const { setNodeRef, isOver } = useDroppable({ id: `cell:${col}:${row}`, data: { col, row } })
  return (
    <div
      ref={setNodeRef}
      style={{ gridColumn: col + 1, gridRow: row + 1 }}
      className={`grid min-h-20 place-items-center self-stretch rounded-2xl border border-dashed ${isOver ? 'border-amber-400 bg-amber-400/10' : 'border-neutral-700/60'}`}
    >
      {free && (
        <button
          onClick={onAdd}
          aria-label={`Add a section at column ${col + 1}, row ${row + 1}`}
          className="grid size-10 place-items-center rounded-full text-neutral-500 hover:bg-neutral-800 hover:text-white"
        >
          <Svg path={mdiPlus} className="size-6" />
        </button>
      )}
    </div>
  )
}

// A Section in the editor, in its cells, under its bar: the handle that drags it, its own Name,
// Icon and columns if it is an own Section, its width and height, and Remove.
function EditedSection({
  section: s,
  columns,
  onSize,
  onRemove,
  onChange,
  onColumns,
  children,
}: {
  section: DashboardSection
  columns: number
  onSize: (size: { width?: number; height?: number }) => void
  onRemove: () => void
  onChange: (change: { name?: string; icon?: string }) => void
  onColumns: (n: number) => void
  children: ReactNode
}) {
  const [picking, setPicking] = useState(false)
  const { setNodeRef, listeners, attributes, transform, isDragging } = useDraggable({ id: `edit:${s.key}`, data: { key: s.key } })
  const place = s.place!
  const label = s.area?.name ?? (s.own?.name || 'Section')
  return (
    <div
      ref={setNodeRef}
      style={{ ...cells(place), transform: transform ? `translate3d(${transform.x}px, ${transform.y}px, 0)` : undefined }}
      className={`relative space-y-2 rounded-2xl bg-neutral-950 ${isDragging ? 'z-20 shadow-2xl' : ''}`} // hiding the cells under it
    >
      <div className="flex flex-wrap items-center gap-2 rounded-xl bg-neutral-800 p-1.5 text-sm">
        <button
          {...listeners}
          {...attributes}
          aria-label={`Move ${label}`}
          title="Drag to move"
          className="grid size-8 cursor-grab touch-none place-items-center rounded-lg text-amber-300 hover:bg-neutral-700"
        >
          <Svg path={mdiDrag} className="size-6" />
        </button>
        {s.own && (
          <>
            <button
              onClick={() => setPicking(true)}
              aria-label={`${label} icon`}
              title="Icon"
              className="grid size-8 place-items-center rounded-lg bg-neutral-900 hover:bg-neutral-700"
            >
              {s.own.icon ? <AreaIcon icon={s.own.icon} className="size-5" /> : <Svg path={mdiShapeOutline} className="size-5 text-neutral-500" />}
            </button>
            <NameInput
              name={s.own.name ?? ''}
              label={`${label} name`}
              placeholder="No name"
              onName={(name) => onChange({ name })}
              className="min-w-32 flex-1"
            />
            <Stepper icon={mdiViewGridOutline} value={s.columns ?? defaultColumns} max={maxColumns} label="columns of tiles" onChange={onColumns} />
          </>
        )}
        <span className="ml-auto flex flex-wrap gap-1">
          <Stepper icon={mdiArrowExpandHorizontal} value={place.width} max={columns} label="columns wide" onChange={(width) => onSize({ width })} />
          <Stepper icon={mdiArrowExpandVertical} value={rows(place)} max={maxRows} label="rows tall" onChange={(height) => onSize({ height })} />
          <button
            onClick={onRemove}
            aria-label={`Remove ${label}`}
            title="Remove"
            className="grid size-8 place-items-center rounded-lg text-neutral-400 hover:bg-red-950 hover:text-red-300"
          >
            <Svg path={mdiDeleteOutline} className="size-5" />
          </button>
        </span>
      </div>
      {s.area ? (
        <>
          <p className="px-1 text-xs text-neutral-500">Follows the Area: its tiles are arranged on Home.</p>
          <div className="pointer-events-none opacity-50">{children}</div>
        </>
      ) : (
        <>
          {!s.tiles.length && <p className="px-1 text-xs text-neutral-500">Nobody sees this section until it has tiles.</p>}
          {children}
        </>
      )}
      {picking && s.own && (
        <Panel title={<h2 className="text-lg font-medium">{label}</h2>} onClose={() => setPicking(false)}>
          <AreaIconPicker icon={s.own.icon ?? ''} onPick={(icon) => (setPicking(false), onChange({ icon }))} />
        </Panel>
      )}
    </div>
  )
}

// A Name edited in place, saved once it changes, on Enter or when left.
function NameInput({
  name,
  label,
  placeholder,
  onName,
  className,
}: {
  name: string
  label: string
  placeholder?: string
  onName: (name: string) => void
  className: string
}) {
  return (
    <input
      key={name} // a Name saved elsewhere replaces what is typed
      defaultValue={name}
      aria-label={label}
      placeholder={placeholder}
      onBlur={(e) => e.target.value.trim() !== name && onName(e.target.value.trim())}
      onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
      className={`rounded-lg bg-neutral-950/60 px-2 py-1 ${className}`}
    />
  )
}

// Asks what to add in a cell: an own Section, or an Area's, those already on the Dashboard greyed out.
function AddSection({ areas, on, onAdd, onClose }: { areas: Area[]; on: string[]; onAdd: (s: SectionContent) => void; onClose: () => void }) {
  const item = 'flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left hover:bg-neutral-800 disabled:text-neutral-600 disabled:hover:bg-transparent'
  return (
    <Panel title={<h2 className="text-lg font-medium">Add a section</h2>} onClose={onClose}>
      <div className="space-y-1 text-sm">
        {/* Oiko gives it its id on the save */}
        <button onClick={() => onAdd({ id: '', columns: defaultColumns })} className={item}>
          <Svg path={mdiViewGridOutline} className="size-5 text-neutral-400" />
          Own section
        </button>
        <h3 className="px-2.5 pt-3 text-xs font-semibold tracking-wide text-neutral-500 uppercase">Areas</h3>
        {areas.map((a) => (
          <button key={a.id} disabled={on.includes(a.id)} onClick={() => onAdd({ area: a.id })} className={item}>
            <span className="grid size-5 place-items-center">
              <AreaIcon icon={a.icon} className="size-5" />
            </span>
            {a.name}
          </button>
        ))}
      </div>
    </Panel>
  )
}
