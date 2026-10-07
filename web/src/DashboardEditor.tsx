import { Fragment, useState, type ReactNode } from 'react'
import { DndContext, PointerSensor, useDraggable, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { mdiCheck, mdiContentCopy, mdiDeleteOutline, mdiDrag, mdiMagnify, mdiShapeOutline, mdiViewColumnOutline, mdiViewGridOutline } from '@mdi/js'
import { edit } from './store'
import type { Area, CustomDashboard } from './types'
import { sectionKey, tileKey, type DashboardSection, type TileChoice } from './dashboard'
import {
  addSection,
  addTile,
  layTiles,
  moveSection,
  nameSection,
  removeSection,
  removeTile,
  resizeSection,
  setDashboardColumns,
  type SectionContent,
} from './editDashboard'
import { defaultColumns, maxColumns, move, placements, reflow, type Arranged } from './layout'
import { confirm } from './confirm'
import { AreaIcon, AreaIconPicker, Svg } from './icons'
import { Panel } from './Panel'
import { ArrangedGrid, Cells, cells, landing, SizeSteppers, Stepper } from './Arrange'
import { Section, tileNodes } from './Section'

// A custom Dashboard turned into its editor, in place: everything live and at full size. A bar on
// top holds its Name, its columns, Duplicate, Delete and Done; every free cell of its grid has a +
// adding a Section there; each Section has a bar that drags it to another cell, steps its size and
// removes it, and on an own Section edits its Name, Icon and columns. An own Section's Tiles are
// arranged as an Area's are on Home, a + in each free cell picking one to add. An Area's Section
// shows the Area dimmed: it follows the Area, arranged on Home. Each change saves the whole
// Dashboard at once.
export function DashboardEditor({
  dashboard: d,
  sections,
  areas,
  choices,
  onDuplicate,
  onDone,
  onResult,
}: {
  dashboard: CustomDashboard
  sections: DashboardSection[] // d's, as custom draws them while editing
  areas: Area[]
  choices: (query: string, taken: string[]) => { area?: Area; tiles: TileChoice[] }[] // the home's Tiles
  onDuplicate: () => void
  onDone: () => void
  onResult: (r: { text: string; error?: boolean }) => void
}) {
  const [adding, setAdding] = useState<{ col: number; row: number } | null>(null)
  // Where a Tile is being added: an own Section's key, and a cell of its grid.
  const [addingTile, setAddingTile] = useState<{ section: string; col: number; row: number } | null>(null)
  const tileSection = addingTile && sections.find((s) => s.key === addingTile.section)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))
  // Whether next is saved; a refusal is told.
  const save = async (next: CustomDashboard) => {
    const err = await edit('PUT', `dashboards/${d.id}`, next)
    if (err) onResult({ text: err, error: true })
    return !err
  }
  const remove = async () => {
    if (!(await confirm(`Delete the dashboard ${d.name}?`, 'Delete'))) return
    const err = await edit('DELETE', `dashboards/${d.id}`)
    if (err) return onResult({ text: err, error: true })
    onDone()
    location.hash = '#'
  }
  // own Section s's Tiles in columns, those it draws where layout places them.
  const lay = (s: DashboardSection, columns: number, layout: Arranged[]) => save(layTiles(d, s.key, columns, layout))
  // A dragged Section takes the cell it lands on; a dragged Tile, the cell of its Section it lands on.
  const dropped = ({ active, over }: DragEndEvent) => {
    const from = active.data.current
    const to = over?.data.current
    if (!from || !to) return
    if (from.type === 'section') return save(moveSection(d, from.key, to.col, to.row))
    const s = sections.find((s) => s.key === from.grid)
    if (s?.columns) lay(s, s.columns, move(placements(s.tiles), from.tile, to.col, to.row, s.columns))
  }
  // The keys of the Tiles own Section key holds, those showing nothing included.
  const held = (key: string) => d.sections.flatMap((s) => (s.area === undefined && sectionKey(s) === key ? (s.tiles ?? []).map(tileKey) : []))
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3 rounded-2xl border border-amber-400/40 bg-neutral-900 p-3">
        <NameInput
          name={d.name}
          label="Dashboard name"
          onName={async (name) => !!name && save({ ...d, name })}
          className="min-w-40 flex-1 text-lg font-semibold"
        />
        <Stepper
          icon={mdiViewColumnOutline}
          value={d.columns}
          max={maxColumns}
          label="columns"
          onChange={(n) => save(setDashboardColumns(d, n))}
          className="text-sm"
        />
        <button onClick={onDuplicate} className="flex items-center gap-1 rounded-full px-3 py-1.5 text-sm text-neutral-300 hover:bg-neutral-800">
          <Svg path={mdiContentCopy} className="size-5" />
          Duplicate
        </button>
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
      <DndContext sensors={sensors} collisionDetection={landing} onDragEnd={dropped}>
        <div className="grid auto-rows-[minmax(5rem,auto)] items-start gap-6" style={{ gridTemplateColumns: `repeat(${d.columns}, minmax(0, 1fr))` }}>
          <Cells
            grid="dashboard"
            type="section"
            columns={d.columns}
            layout={sections.map((s) => s.place!)}
            what="a section"
            onAdd={(col, row) => setAdding({ col, row })}
          />
          {sections.map((s) => (
            <EditedSection
              key={s.key}
              section={s}
              columns={d.columns}
              onSize={(size) => save(resizeSection(d, s.key, size))}
              onRemove={() => save(removeSection(d, s.key))}
              onName={(change) => save(nameSection(d, s.key, change))}
              onColumns={(n) => lay(s, n, reflow(placements(s.tiles), n))}
            >
              {s.own ? (
                <div className="px-1">
                  <ArrangedGrid
                    grid={s.key}
                    columns={s.columns!}
                    tiles={tileNodes(s.tiles, onResult)}
                    onLayout={(n, layout) => lay(s, n, layout)}
                    onAdd={(col, row) => setAddingTile({ section: s.key, col, row })}
                    onRemove={(tile) => save(removeTile(d, s.key, tile))}
                  />
                </div>
              ) : (
                <Section section={s} title={s.area?.name} collapsed={false} onCollapse={() => {}} onResult={onResult} />
              )}
            </EditedSection>
          ))}
        </div>
      </DndContext>
      {adding && (
        <AddSection
          areas={areas}
          present={d.sections.flatMap((s) => (s.area !== undefined ? [s.area] : []))}
          onAdd={(s) => (setAdding(null), save(addSection(d, s, adding.col, adding.row)))}
          onClose={() => setAdding(null)}
        />
      )}
      {addingTile && tileSection && (
        <AddTile
          choices={(query) => choices(query, held(tileSection.key))}
          onAdd={(c) => {
            setAddingTile(null)
            save(addTile(d, tileSection.key, placements(tileSection.tiles), c.tile, addingTile.col, addingTile.row))
          }}
          onClose={() => setAddingTile(null)}
        />
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
  onName,
  onColumns,
  children,
}: {
  section: DashboardSection
  columns: number
  onSize: (size: { width?: number; height?: number }) => void
  onRemove: () => void
  onName: (change: { name?: string; icon?: string }) => Promise<boolean>
  onColumns: (n: number) => void
  children: ReactNode
}) {
  const [picking, setPicking] = useState(false)
  const { setNodeRef, listeners, attributes, transform, isDragging } = useDraggable({
    id: `edit:${s.key}`,
    data: { type: 'section', grid: 'dashboard', key: s.key },
  })
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
            <NameInput name={s.own.name ?? ''} label={`${label} name`} placeholder="No name" onName={(name) => onName({ name })} className="min-w-32 flex-1" />
            <Stepper icon={mdiViewGridOutline} value={s.columns!} max={maxColumns} label="columns of tiles" onChange={onColumns} />
          </>
        )}
        <span className="ml-auto flex flex-wrap gap-1">
          <SizeSteppers place={place} columns={columns} onSize={onSize} />
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
          <AreaIconPicker icon={s.own.icon ?? ''} onPick={(icon) => (setPicking(false), onName({ icon }))} />
        </Panel>
      )}
    </div>
  )
}

// A Name edited in place, saved once it changes, on Enter or when left; one not saved goes back.
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
  onName: (name: string) => Promise<boolean>
  className: string
}) {
  return (
    <input
      key={name} // a Name saved elsewhere replaces what is typed
      defaultValue={name}
      aria-label={label}
      placeholder={placeholder}
      onBlur={async (e) => {
        const input = e.target
        if (input.value.trim() !== name && !(await onName(input.value.trim()))) input.value = name
      }}
      onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
      className={`rounded-lg bg-neutral-950/60 px-2 py-1 ${className}`}
    />
  )
}

// Asks what to add in a cell: an own Section, or an Area's, those already on the Dashboard greyed out.
function AddSection({ areas, present, onAdd, onClose }: { areas: Area[]; present: string[]; onAdd: (s: SectionContent) => void; onClose: () => void }) {
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
          <button key={a.id} disabled={present.includes(a.id)} onClick={() => onAdd({ area: a.id })} className={item}>
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

// Asks which Tile to add in a cell of an own Section: a search over the home's, by Area, those the
// Section holds greyed out.
function AddTile({
  choices,
  onAdd,
  onClose,
}: {
  choices: (query: string) => { area?: Area; tiles: TileChoice[] }[]
  onAdd: (c: TileChoice) => void
  onClose: () => void
}) {
  const [query, setQuery] = useState('')
  const groups = choices(query)
  return (
    <Panel title={<h2 className="text-lg font-medium">Add a tile</h2>} onClose={onClose}>
      <label className="flex items-center gap-2 rounded-lg bg-neutral-800 px-2.5 py-2 text-sm">
        <Svg path={mdiMagnify} className="size-5 text-neutral-400" />
        <input
          autoFocus
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search"
          aria-label="Search tiles"
          className="min-w-0 flex-1 bg-transparent outline-none"
        />
      </label>
      <div className="mt-2 space-y-1 text-sm">
        {groups.map((g) => (
          <section key={g.area?.id ?? ''}>
            <h3 className="flex items-center gap-2 px-2.5 pt-3 pb-1 text-xs font-semibold tracking-wide text-neutral-500 uppercase">
              <AreaIcon icon={g.area?.icon} className="size-4" />
              {g.area?.name ?? 'No area'}
            </h3>
            {g.tiles.map((c) => (
              <Fragment key={c.key}>
                <Choice choice={c} onAdd={onAdd} />
                {c.functions && (
                  <div className="pl-4">
                    {c.functions.map((f) => (
                      <Choice key={f.key} choice={f} onAdd={onAdd} />
                    ))}
                  </div>
                )}
              </Fragment>
            ))}
          </section>
        ))}
        {!groups.length && <p className="px-2.5 py-2 text-neutral-500">No tile matches.</p>}
      </div>
    </Panel>
  )
}

// A Tile to pick, greyed out if the Section holds it.
function Choice({ choice: c, onAdd }: { choice: TileChoice; onAdd: (c: TileChoice) => void }) {
  return (
    <button
      disabled={c.taken}
      onClick={() => onAdd(c)}
      className="flex w-full items-baseline gap-2 rounded-lg px-2.5 py-2 text-left hover:bg-neutral-800 disabled:text-neutral-600 disabled:hover:bg-transparent"
    >
      <span className="min-w-0 flex-1 truncate">{c.name}</span>
      <span className="text-xs text-neutral-500">{c.taken ? 'In this section' : c.kind}</span>
    </button>
  )
}
