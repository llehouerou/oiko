// The Home tab of the bar, for a Person: the current Dashboard's Name, opening a menu of the
// Dashboards they see (the built-in one, the shared ones, then their own) and an entry to create one.

import { useRef, useState, type FormEvent } from 'react'
import { mdiChevronDown, mdiPlus, mdiViewDashboardOutline } from '@mdi/js'
import type { CustomDashboard } from './types'
import { api, useAllows } from './access'
import { Svg } from './icons'
import { Panel } from './Panel'

export function DashboardMenu({ current, dashboards }: { current?: CustomDashboard; dashboards: CustomDashboard[] }) {
  const button = useRef<HTMLButtonElement>(null)
  const [creating, setCreating] = useState(false)
  const item = (id: string, name: string) => (
    <a
      key={id}
      href={`#dashboard/${id}`}
      aria-current={(current?.id ?? 'builtin') === id ? 'page' : undefined}
      onClick={() => document.getElementById('dashboards')?.hidePopover()}
      className="block truncate rounded-lg px-2.5 py-2 hover:bg-neutral-800 aria-[current]:text-amber-300"
    >
      {name}
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
        {item('builtin', 'Home')}
        {[...dashboards.filter((d) => d.shared), ...dashboards.filter((d) => !d.shared)].map((d) => item(d.id, d.name))}
        <button
          popoverTarget="dashboards"
          popoverTargetAction="hide"
          onClick={() => setCreating(true)}
          className="mt-1 flex w-full items-center gap-3 rounded-lg border-t border-neutral-800 px-2.5 py-2 text-left hover:bg-neutral-800"
        >
          <Svg path={mdiPlus} className="size-5 text-neutral-400" />
          New dashboard
        </button>
      </div>
      {creating && <NewDashboard onClose={() => setCreating(false)} />}
    </>
  )
}

// Asks for a Name, and an Admin whether it is shared or their own, creates an empty Dashboard of
// it, and opens it.
function NewDashboard({ onClose }: { onClose: () => void }) {
  const admin = useAllows('admin')
  const [error, setError] = useState<string | null>(null)
  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const form = new FormData(e.currentTarget)
    const res = await api('/api/dashboards', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ shared: form.get('shared') === 'shared', name: form.get('name'), sections: [] }),
    })
    if (!res.ok) return setError((await res.text()).trim())
    const { id }: { id: string } = await res.json()
    location.hash = `#dashboard/${id}`
    onClose()
  }
  return (
    <Panel title={<h2 className="text-lg font-medium">New dashboard</h2>} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4 text-sm">
        <input name="name" placeholder="Name" aria-label="Name" autoFocus className="w-full rounded bg-neutral-800 px-2 py-1" />
        {admin && (
          <select name="shared" aria-label="Who sees it" defaultValue="personal" className="w-full rounded bg-neutral-800 px-2 py-1">
            <option value="personal">Personal: you alone see it</option>
            <option value="shared">Shared: everyone sees it, Admins edit it</option>
          </select>
        )}
        {error && <p className="text-red-400">{error}</p>}
        <button type="submit" className="rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300">
          Create
        </button>
      </form>
    </Panel>
  )
}
