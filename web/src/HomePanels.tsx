// The panels an Admin opens from the dashboard: an Area's, a Flag's, an Aggregate's and a Device's.

import { useState, type FormEvent, type ReactNode } from 'react'
import { edit, useAreas, useAvailability, useCatalogue, useCommand } from './store'
import type { Aggregate, Area, Device, Flag, Fn, Target } from './types'
import { aggregateTarget, deviceTarget, flagTarget, parseTarget, targetKind } from './targets'
import { confirm } from './confirm'
import { titleIfTruncated } from './truncated'
import { AreaIconPicker, IconPicker } from './icons'
import { roles, type Roles } from './roles'
import { aggregateSummary } from './tiles'
import { Panel } from './Panel'
import { CommandNote, Control, Reading } from './Tiles'

// Creates an Area, or renames and deletes one and gathers Devices in it at once; its Icon is saved
// with its Name. Its place among the others is arranged on the dashboard.
export function AreaPanel({
  area,
  areas,
  devices,
  tiles = [],
  onClose,
}: {
  area?: Area
  areas: Area[]
  devices: Device[]
  tiles?: { key: string; label: string }[]
  onClose: () => void
}) {
  const [error, setError] = useState<string | null>(null)
  const [icon, setIcon] = useState(area?.icon ?? '')
  // Its Devices first, then those without an Area; the order is the one at opening, so ticking a box does not move it.
  const [order] = useState(() => {
    const rank = (d: Device) => (d.area === area?.id ? 0 : d.area ? 2 : 1)
    return devices
      .filter((d) => d.functions?.length)
      .sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name))
      .map((d) => d.id)
  })
  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const body = { name: new FormData(e.currentTarget).get('name'), icon }
    const err = area ? await edit('PUT', `areas/${area.id}`, body) : await edit('POST', 'areas', body)
    if (err) setError(err)
    else onClose()
  }
  // Hides or shows item of one of the Area's display lists, at once.
  const display = async (list: 'hidden' | 'hiddenAggregates', item: string, hide: boolean) => {
    if (!area) return
    const now = { hidden: area.hidden ?? [], hiddenAggregates: area.hiddenAggregates ?? [] }
    now[list] = hide ? [...now[list], item] : now[list].filter((x) => x !== item)
    setError(await edit('PUT', `areas/${area.id}/display`, now))
  }
  const legend = 'mb-2 text-xs font-semibold tracking-wide text-neutral-500 uppercase'
  return (
    <Panel
      title={
        <h2 className="truncate text-lg font-medium" onMouseEnter={titleIfTruncated}>
          {area?.name ?? 'New area'}
        </h2>
      }
      onClose={onClose}
    >
      <form onSubmit={submit} className="space-y-4 text-sm">
        <input name="name" defaultValue={area?.name} placeholder="Name" aria-label="Name" className="w-full rounded bg-neutral-800 px-2 py-1" />
        <AreaIconPicker icon={icon} onPick={setIcon} />
        {error && <p className="text-red-400">{error}</p>}
        <div className="flex flex-wrap gap-2">
          <button type="submit" className="rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300">
            {area ? 'Save' : 'Create'}
          </button>
          {area && (
            <button
              type="button"
              onClick={async () =>
                (await confirm(`Delete ${area.name}? What it holds is left without an area.`, 'Delete')) && setError(await edit('DELETE', `areas/${area.id}`))
              }
              className="ml-auto rounded bg-neutral-800 px-3 py-1 text-red-400 hover:bg-neutral-700"
            >
              Delete
            </button>
          )}
        </div>
      </form>
      {area && (
        <fieldset className="space-y-1 text-sm">
          <legend className={legend}>Header</legend>
          {[
            ['light', 'Lights'],
            ['occupancy', 'Presence'],
            ['contact', 'Doors'],
            ['temperature', 'Temperature'],
            ['humidity', 'Humidity'],
            ['co2', 'CO₂'],
          ].map(([kind, label]) => (
            <label key={kind} className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={!area.hiddenAggregates?.includes(kind!)}
                onChange={(e) => display('hiddenAggregates', kind!, !e.target.checked)}
                className="accent-amber-400"
              />
              {label}
            </label>
          ))}
        </fieldset>
      )}
      {area && tiles.length > 0 && (
        <fieldset className="space-y-1 text-sm">
          <legend className={legend}>Tiles shown</legend>
          {tiles.map((t) => (
            <label key={t.key} className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={!area.hidden?.includes(t.key)}
                onChange={(e) => display('hidden', t.key, !e.target.checked)}
                className="accent-amber-400"
              />
              <span className="truncate">{t.label}</span>
            </label>
          ))}
        </fieldset>
      )}
      {area && (
        <fieldset className="max-h-80 space-y-1 overflow-y-auto text-sm">
          <legend className="mb-2 text-xs font-semibold tracking-wide text-neutral-500 uppercase">Devices</legend>
          {order.flatMap((id) => {
            const d = devices.find((d) => d.id === id)
            if (!d) return []
            const elsewhere = d.area !== area.id && areas.find((a) => a.id === d.area)?.name
            return [
              <label key={d.id} className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={d.area === area.id}
                  onChange={async (e) => setError(await edit('PUT', 'area', { target: deviceTarget(d.id), area: e.target.checked ? area.id : '' }))}
                  className="accent-amber-400"
                />
                <span className="truncate">{d.name}</span>
                {elsewhere && <span className="shrink-0 text-neutral-500">· {elsewhere}</span>}
              </label>,
            ]
          })}
        </fieldset>
      )}
    </Panel>
  )
}

// Sets the Area of target at once. With inherited, the Area of a Function's Device,
// "" stands for following it rather than for none.
function AreaSelect({
  target,
  area,
  inherited,
  onError,
  className = '',
}: {
  target: Target
  area?: string
  inherited?: string
  onError: (e: string | null) => void
  className?: string
}) {
  const areas = useAreas()
  const name = (id?: string) => areas.find((a) => a.id === id)?.name ?? 'none'
  return (
    <select
      value={area ?? ''}
      aria-label="Area"
      onChange={async (e) => onError(await edit('PUT', 'area', { target, area: e.target.value }))}
      className={`rounded bg-neutral-800 px-2 py-1 ${className}`}
    >
      <option value="">{inherited === undefined ? 'No area' : `As the device (${name(inherited)})`}</option>
      {areas.map((a) => (
        <option key={a.id} value={a.id}>
          {a.name}
        </option>
      ))}
    </select>
  )
}

// The Area of a Device, a Flag or an Aggregate; children add those of a Device's Functions.
function AreaField({ target, area, onError, children }: { target: Target; area?: string; onError: (e: string | null) => void; children?: ReactNode }) {
  return (
    <section className="space-y-2 text-sm">
      <h3 className="text-xs font-semibold tracking-wide text-neutral-500 uppercase">Area</h3>
      <AreaSelect target={target} area={area} onError={onError} className="w-full" />
      {children}
    </section>
  )
}

// Creates a Flag, or renames and deletes an existing one.
export function FlagPanel({ flag, onClose }: { flag?: Flag; onClose: () => void }) {
  const [error, setError] = useState<string | null>(null)
  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const body = { name: new FormData(e.currentTarget).get('name') }
    const err = flag ? await edit('PUT', `flags/${flag.id}`, body) : await edit('POST', 'flags', body)
    if (err) setError(err)
    else onClose()
  }
  return (
    <Panel
      title={
        <h2 className="truncate text-lg font-medium" onMouseEnter={titleIfTruncated}>
          {flag?.name ?? 'New flag'}
        </h2>
      }
      onClose={onClose}
    >
      <form onSubmit={submit} className="space-y-4 text-sm">
        <input name="name" defaultValue={flag?.name} placeholder="Name" aria-label="Name" className="w-full rounded bg-neutral-800 px-2 py-1" />
        {flag && <AreaField target={flagTarget(flag.id)} area={flag.area} onError={setError} />}
        {error && <p className="text-red-400">{error}</p>}
        <div className="flex justify-between gap-2">
          <button type="submit" className="rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300">
            {flag ? 'Save' : 'Create'}
          </button>
          {flag && (
            <button
              type="button"
              onClick={async () => (await confirm(`Delete ${flag.name}?`, 'Delete')) && setError(await edit('DELETE', `flags/${flag.id}`))}
              className="rounded bg-neutral-800 px-3 py-1 text-red-400 hover:bg-neutral-700"
            >
              Delete
            </button>
          )}
        </div>
      </form>
    </Panel>
  )
}

// Creates an Aggregate, or edits and deletes an existing one. Members are offered
// among the Functions, Flags and Aggregates of one kind at a time, Aggregates first.
export function AggregatePanel({
  aggregate,
  aggregates,
  onClose,
  onBack,
}: {
  aggregate?: Aggregate
  aggregates: Aggregate[]
  onClose: () => void
  onBack?: () => void
}) {
  const targets = useCatalogue()
  const fns = targets.list
    .filter((e) => targetKind(e.target) === 'function' || targetKind(e.target) === 'flag')
    .map((e) => ({ member: e.target, kind: e.kind, label: e.name }))
  const kinds = [...new Set(fns.map((f) => f.kind))].sort()
  const [kind, setKind] = useState(aggregate?.kind || kinds[0] || '')
  const [error, setError] = useState<string | null>(null)
  // Whether a contains the Aggregate id, directly or not; Oiko refuses such cycles.
  const contains = (a: Aggregate, id: string): boolean =>
    a.members.some((m) => m === aggregateTarget(id) || aggregates.some((n) => m === aggregateTarget(n.id) && contains(n, id)))
  const candidates: { member: Target; label: string }[] = [
    ...aggregates
      .filter((a) => a.kind === kind && a.id !== aggregate?.id && !(aggregate && contains(a, aggregate.id)))
      .map((a) => ({ member: aggregateTarget(a.id), label: `${a.name} · aggregate` })),
    ...fns.filter((f) => f.kind === kind),
  ]
  const isMember = (m: Target) => aggregate?.members.includes(m) ?? false
  // Current members first, then Aggregates; the order is the one at opening, so ticking a box does not move it.
  candidates.sort(
    (a, b) =>
      Number(isMember(b.member)) - Number(isMember(a.member)) ||
      Number(parseTarget(b.member)?.kind === 'aggregate') - Number(parseTarget(a.member)?.kind === 'aggregate') ||
      a.label.localeCompare(b.label),
  )
  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const form = new FormData(e.currentTarget)
    const members = form.getAll('member').map(String)
    const body = { name: form.get('name'), binary: form.get('binary'), numeric: form.get('numeric'), members }
    const err = aggregate ? await edit('PUT', `aggregates/${aggregate.id}`, body) : await edit('POST', 'aggregates', body)
    if (err) setError(err)
    else onClose()
  }
  const field = 'rounded bg-neutral-800 px-2 py-1'
  const title = aggregate ? (
    <>
      <h2 className="truncate text-lg font-medium" onMouseEnter={titleIfTruncated}>
        {aggregate.name}
      </h2>
      <p className="text-xs text-neutral-500">{aggregateSummary(aggregate)}</p>
    </>
  ) : (
    <h2 className="text-lg font-medium">New aggregate</h2>
  )
  // An Area Aggregate follows its Area: nothing to edit, only who is in it.
  if (aggregate?.derived) {
    const label = targets.name
    return (
      <Panel title={title} onClose={onClose} onBack={onBack}>
        <p className="text-sm text-neutral-400">Every {aggregate.kind} of its area, as the area changes.</p>
        <ul className="space-y-1 text-sm">
          {aggregate.members.map((m) => (
            <li key={m}>{label(m)}</li>
          ))}
        </ul>
      </Panel>
    )
  }
  return (
    <Panel title={title} onClose={onClose} onBack={onBack}>
      {aggregate && <AreaField target={aggregateTarget(aggregate.id)} area={aggregate.area} onError={setError} />}
      {aggregate?.kind === 'light' && <IconPicker target={aggregateTarget(aggregate.id)} icon={aggregate.icon} group onError={setError} />}
      <form onSubmit={submit} className="space-y-4 text-sm">
        <input name="name" defaultValue={aggregate?.name} placeholder="Name" aria-label="Name" className={`w-full ${field}`} />
        <div className="flex gap-2">
          <select value={kind} onChange={(e) => setKind(e.target.value)} aria-label="Kind" className={`flex-1 ${field}`}>
            {kinds.map((k) => (
              <option key={k}>{k}</option>
            ))}
          </select>
          <select name="binary" defaultValue={aggregate?.binary ?? 'any'} aria-label="Binary rule" className={field}>
            <option value="any">on when any member is</option>
            <option value="all">on when all members are</option>
          </select>
          <select name="numeric" defaultValue={aggregate?.numeric ?? 'mean'} aria-label="Numeric rule" className={field}>
            <option value="mean">mean</option>
            <option value="min">min</option>
            <option value="max">max</option>
            <option value="sum">sum</option>
          </select>
        </div>
        <fieldset className="max-h-64 space-y-1 overflow-y-auto">
          <legend className="mb-2 text-xs font-semibold tracking-wide text-neutral-500 uppercase">Members</legend>
          {candidates.map((c) => {
            const value = c.member
            return (
              <label key={value} className="flex items-center gap-2">
                <input type="checkbox" name="member" value={value} defaultChecked={isMember(c.member)} className="accent-amber-400" />
                {c.label}
              </label>
            )
          })}
        </fieldset>
        {error && <p className="text-red-400">{error}</p>}
        <div className="flex justify-between gap-2">
          <button type="submit" className="rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300">
            {aggregate ? 'Save' : 'Create'}
          </button>
          {aggregate && (
            <button
              type="button"
              onClick={async () => (await confirm(`Delete ${aggregate.name}?`, 'Delete')) && setError(await edit('DELETE', `aggregates/${aggregate.id}`))}
              className="rounded bg-neutral-800 px-3 py-1 text-red-400 hover:bg-neutral-700"
            >
              Delete
            </button>
          )}
        </div>
      </form>
    </Panel>
  )
}

// The Roles of a Device's own Capabilities and of each of its Functions'.
function deviceRoles(device: Device): { target: Target; fn?: Fn; r: Roles }[] {
  return [
    { target: deviceTarget(device.id), r: roles('', device.capabilities ?? []) },
    ...(device.functions ?? []).map((fn) => ({ target: deviceTarget(device.id, fn.key), fn, r: roles(fn.kind, fn.capabilities) })),
  ]
}

// Settings and diagnostics of a Device, whether they belong to it or to one of its Functions.
export function DevicePanel({ device, devices, onClose, onBack }: { device: Device; devices: Device[]; onClose: () => void; onBack?: () => void }) {
  const [error, setError] = useState<string | null>(null)
  const target = deviceTarget(device.id)
  const availability = useAvailability(target)
  const command = useCommand(target)
  const all = deviceRoles(device)
  const settings = all.flatMap(({ target, fn, r }) => r.settings.map((cap) => ({ target, cap, fn })))
  const diagnostics = all.flatMap(({ target, fn, r }) => [...r.health, ...r.diagnostics].map((cap) => ({ target, cap, fn })))
  // A Function of its own Capability's name needs no prefix.
  const label = (p: (typeof settings)[number]) => (p.fn && p.fn.key !== p.cap.key ? `${p.fn.key} · ${p.cap.label}` : p.cap.label)
  const title = (
    <>
      <NameField device={device} onError={setError} />
      <p className="text-xs text-neutral-500">
        {[device.vendor, device.model, device.nativeAddress, availability].filter(Boolean).join(' · ')}
        <CommandNote command={command} />
      </p>
    </>
  )

  return (
    <Panel title={title} onClose={onClose} onBack={onBack}>
      {error && <p className="text-sm text-red-400">{error}</p>}
      {device.detached && <DetachedSection device={device} devices={devices} onError={setError} />}
      <AreaField target={target} area={device.area} onError={setError}>
        {(device.functions?.length ?? 0) > 1 &&
          device.functions!.map((fn) => (
            <label key={fn.key} className="flex items-center justify-between gap-3">
              <span className="truncate text-neutral-400">{fn.key}</span>
              <AreaSelect target={deviceTarget(device.id, fn.key)} area={fn.area} inherited={device.area ?? ''} onError={setError} />
            </label>
          ))}
      </AreaField>
      {device.functions?.some((f) => f.kind === 'light') && <IconPicker target={target} icon={device.icon} group={false} onError={setError} />}
      {settings.length > 0 && (
        <section className="space-y-3">
          <h3 className="text-xs font-semibold tracking-wide text-neutral-500 uppercase">Settings</h3>
          {settings.map((p) => (
            <Control key={`${p.target}|${p.cap.key}`} {...p} cap={{ ...p.cap, label: label(p) }} />
          ))}
        </section>
      )}
      {diagnostics.length > 0 && (
        <section className="space-y-2">
          <h3 className="text-xs font-semibold tracking-wide text-neutral-500 uppercase">Diagnostics</h3>
          {diagnostics.map((p) => (
            <Reading key={`${p.target}|${p.cap.key}`} {...p} cap={{ ...p.cap, label: label(p) }} />
          ))}
        </section>
      )}
    </Panel>
  )
}

// Renames on Enter or when the field loses focus; Escape closes the panel as usual.
function NameField({ device, onError }: { device: Device; onError: (e: string | null) => void }) {
  const save = async (name: string) => {
    if (name.trim() === device.name) return
    onError(await edit('PATCH', `devices/${device.id}`, { name }))
  }
  return (
    <input
      defaultValue={device.name}
      aria-label="Name"
      onBlur={(e) => save(e.target.value)}
      onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
      className="-ml-1 w-full rounded bg-transparent px-1 text-lg font-medium hover:bg-neutral-800 focus:bg-neutral-800 focus:outline-none"
    />
  )
}

// A Detached Device keeps its identity until new hardware takes it over (Replace) or it is deleted.
function DetachedSection({ device, devices, onError }: { device: Device; devices: Device[]; onError: (e: string | null) => void }) {
  // Same model first: the usual replacement for a dead device.
  const candidates = devices
    .filter((d) => !d.detached)
    .sort((a, b) => Number(b.model === device.model) - Number(a.model === device.model) || a.name.localeCompare(b.name))
  return (
    <section className="space-y-3 rounded-lg bg-amber-950/40 p-3 text-sm">
      <p className="text-amber-200">This device has left zigbee2mqtt. Pair its replacement, then pick it here to keep this identity.</p>
      <div className="flex gap-2">
        <select
          defaultValue=""
          aria-label="Replace with"
          onChange={async (e) => {
            const picked = e.target
            const name = picked.selectedOptions[0]?.text
            if (!(await confirm(`${name} takes over ${device.name}? This cannot be undone.`, 'Replace'))) {
              picked.value = ''
              return
            }
            onError(await edit('POST', `devices/${device.id}/replace`, { with: picked.value }))
          }}
          className="min-w-0 flex-1 rounded bg-neutral-800 px-2 py-1"
        >
          <option value="" disabled>
            Replace with…
          </option>
          {candidates.map((d) => (
            <option key={d.id} value={d.id}>
              {d.name}
              {d.model && ` (${d.model})`}
            </option>
          ))}
        </select>
        <button
          onClick={async () => (await confirm(`Delete ${device.name} for good?`, 'Delete')) && onError(await edit('DELETE', `devices/${device.id}`))}
          className="rounded bg-neutral-800 px-3 py-1 text-red-400 hover:bg-neutral-700"
        >
          Delete
        </button>
      </div>
    </section>
  )
}
