// The Home tab of the bar, for a Person: the current Dashboard's Name, opening a menu of the
// Dashboards they show, in their order (ADR 0044), then entries to create one and to edit the list.

import { useRef, useState, type FormEvent } from 'react'
import { DndContext, PointerSensor, useDraggable, useDroppable, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { mdiChevronDown, mdiDrag, mdiFormatListBulleted, mdiPlus, mdiViewDashboardOutline } from '@mdi/js'
import type { CustomDashboard, ListEntry } from './types'
import { api, useAllows } from './access'
import { edit } from './store'
import { Switch } from './controls'
import { Svg } from './icons'
import { Panel } from './Panel'

export function DashboardMenu({
  current,
  dashboards,
  list,
  onCreated,
}: {
  current?: CustomDashboard
  dashboards: CustomDashboard[]
  list: ListEntry[]
  onCreated: (id: string) => void // a Dashboard created from the menu, to open in its editor
}) {
  const button = useRef<HTMLButtonElement>(null)
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState(false)
  const name = (id: string) => (id === 'builtin' ? 'Home' : (dashboards.find((d) => d.id === id)?.name ?? id))
  const item = (id: string) => (
    <a
      key={id}
      href={`#dashboard/${id}`}
      aria-current={(current?.id ?? 'builtin') === id ? 'page' : undefined}
      onClick={() => document.getElementById('dashboards')?.hidePopover()}
      className="block truncate rounded-lg px-2.5 py-2 hover:bg-neutral-800 aria-[current]:text-amber-300"
    >
      {name(id)}
    </a>
  )
  return (
    <>
      <button
        ref={button}
        popoverTarget="dashboards"
        title="Dashboards"
        aria-current="page"
        className="flex min-w-0 items-center gap-2 rounded-full bg-neutral-700 px-3 py-1.5 text-sm text-white shadow sm:px-4"
      >
        {/* a phone keeps the Name alone */}
        <Svg path={mdiViewDashboardOutline} className="hidden size-5 shrink-0 sm:block" />
        <span className="max-w-24 truncate sm:max-w-32 md:max-w-48">{current?.name ?? 'Home'}</span>
        <Svg path={mdiChevronDown} className="hidden size-4 shrink-0 text-neutral-400 sm:block" />
      </button>
      <div
        id="dashboards"
        popover="auto"
        // A popover sits in the top layer, out of the bar: it opens under the button, flush left.
        onBeforeToggle={(e) => {
          const r = button.current?.getBoundingClientRect()
          if (e.newState !== 'open' || !r) return
          e.currentTarget.style.top = `${r.bottom + 8}px`
          e.currentTarget.style.left = `${r.left}px`
        }}
        className="inset-auto m-0 w-64 rounded-xl border border-neutral-800 bg-neutral-900 p-1.5 text-sm text-neutral-100 shadow-2xl"
      >
        {list.filter((e) => !e.hidden).map((e) => item(e.id))}
        <button
          popoverTarget="dashboards"
          popoverTargetAction="hide"
          onClick={() => setCreating(true)}
          className="mt-1 flex w-full items-center gap-3 rounded-lg border-t border-neutral-800 px-2.5 py-2 text-left hover:bg-neutral-800"
        >
          <Svg path={mdiPlus} className="size-5 text-neutral-400" />
          New dashboard
        </button>
        <button
          popoverTarget="dashboards"
          popoverTargetAction="hide"
          onClick={() => setEditing(true)}
          className="flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left hover:bg-neutral-800"
        >
          <Svg path={mdiFormatListBulleted} className="size-5 text-neutral-400" />
          Edit list
        </button>
      </div>
      {creating && <CreateDashboard title="New dashboard" onCreated={onCreated} onClose={() => setCreating(false)} />}
      {editing && <EditList list={list} name={name} onClose={() => setEditing(false)} />}
    </>
  )
}

// The Person's list: every Dashboard they see, in their order, a handle dragging one onto another's
// place, a switch showing or hiding it in the menu, the hidden ones dimmed; the last one shown stays
// shown. Each change is saved at once, and comes back through the stream.
function EditList({ list, name, onClose }: { list: ListEntry[]; name: (id: string) => string; onClose: () => void }) {
  const [error, setError] = useState<string | null>(null)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))
  const save = async (next: ListEntry[]) => setError(await edit('PUT', 'me/dashboards', next))
  const shown = list.filter((e) => !e.hidden).length
  const dropped = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return
    const next = list.filter((e) => e.id !== active.id)
    next.splice(
      list.findIndex((e) => e.id === over.id),
      0,
      list.find((e) => e.id === active.id)!,
    )
    save(next)
  }
  const show = (id: string, on: boolean) => save(list.map((e): ListEntry => (e.id !== id ? e : on ? { id } : { id, hidden: true })))
  return (
    <Panel title={<h2 className="text-lg font-medium">Dashboards</h2>} onClose={onClose}>
      <DndContext sensors={sensors} onDragEnd={dropped}>
        <ul className="space-y-1 text-sm">
          {list.map((e) => (
            <ListRow key={e.id} entry={e} name={name(e.id)} last={!e.hidden && shown === 1} onShow={(on) => show(e.id, on)} />
          ))}
        </ul>
      </DndContext>
      {error && <p className="text-sm text-red-400">{error}</p>}
    </Panel>
  )
}

function ListRow({ entry, name, last, onShow }: { entry: ListEntry; name: string; last: boolean; onShow: (on: boolean) => void }) {
  const drag = useDraggable({ id: entry.id })
  const drop = useDroppable({ id: entry.id })
  return (
    <li
      ref={(el) => (drag.setNodeRef(el), drop.setNodeRef(el))}
      style={drag.transform ? { transform: `translate3d(${drag.transform.x}px, ${drag.transform.y}px, 0)` } : undefined}
      className={`flex items-center gap-2 rounded-lg p-1 ${drag.isDragging ? 'relative z-10 bg-neutral-800 shadow-xl' : drop.isOver ? 'bg-neutral-800' : ''}`}
    >
      <button
        {...drag.listeners}
        {...drag.attributes}
        aria-label={`Move ${name}`}
        title="Drag to move"
        className="grid size-8 shrink-0 cursor-grab touch-none place-items-center rounded text-neutral-400 hover:bg-neutral-800"
      >
        <Svg path={mdiDrag} className="size-5" />
      </button>
      <span className={`min-w-0 flex-1 truncate ${entry.hidden ? 'text-neutral-500' : ''}`}>{name}</span>
      <Switch
        on={!entry.hidden}
        onChange={onShow}
        small
        disabled={last}
        label={last ? 'The last one shown stays shown' : entry.hidden ? `Show ${name}` : `Hide ${name}`}
      />
    </li>
  )
}
// Asks for a Name, and an Admin whether it is shared or their own, creates a Dashboard of it, empty
// or holding copy, opens it at its address and tells onCreated its id.
export function CreateDashboard({
  title,
  copy,
  onCreated,
  onClose,
}: {
  title: string
  copy?: Pick<CustomDashboard, 'columns' | 'sections'>
  onCreated: (id: string) => void
  onClose: () => void
}) {
  const admin = useAllows('admin')
  const [error, setError] = useState<string | null>(null)
  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const form = new FormData(e.currentTarget)
    const res = await api('/api/dashboards', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ shared: form.get('shared') === 'shared', name: form.get('name'), sections: [], ...copy }),
    })
    if (!res.ok) return setError((await res.text()).trim())
    const { id }: { id: string } = await res.json()
    location.hash = `#dashboard/${id}`
    onCreated(id)
    onClose()
  }
  return (
    <Panel title={<h2 className="text-lg font-medium">{title}</h2>} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4 text-sm">
        <input name="name" placeholder="Name" aria-label="Name" autoFocus className="w-full rounded bg-neutral-800 px-2 py-1" />
        {admin && (
          <select name="shared" aria-label="Who sees it" defaultValue="personal" className="w-full rounded bg-neutral-800 px-2 py-1">
            <option value="personal">Personal: you alone see it</option>
            <option value="shared">Shared: every Person sees it, Admins edit it</option>
          </select>
        )}
        {error && <p className="text-red-400">{error}</p>}
        <button type="submit" className="rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300">
          {copy ? 'Duplicate' : 'Create'}
        </button>
      </form>
    </Panel>
  )
}
