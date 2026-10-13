// The Presence row (ADR 0055): a chip per Person at the top of a Dashboard, each opening the
// Person's sheet, where Home / Away sets their Presence by hand (ADR 0052) and an Admin binds its
// Presence sources (ADR 0054).

import { useEffect, useState } from 'react'
import { mdiClose, mdiHandBackRightOutline } from '@mdi/js'
import { edit, sendCommand, useAvailability, useBridges, useCatalogue, useCommand, useDevices, useNow, usePresences, useValue } from './store'
import { useAllows } from './access'
import { DAY, HistoryView, key, useHistory } from './history'
import { chip, ordered, readCapability, sourceState, sourceText, stretches, type PresenceState } from './presence'
import { deviceTarget, owner, presenceTarget, targetKind, type Entry } from './targets'
import { CommandNote } from './Capabilities'
import { Panel } from './Panel'
import { Svg } from './icons'
import type { Presence, PresenceSource } from './types'

const said: Record<PresenceState, string> = { home: 'Home', away: 'Away', unknown: 'Unknown' }

// usePresence is what p's chip and sheet show, followed as its Values change.
function usePresence(p: Presence) {
  const target = presenceTarget(p.id)
  const home = useValue({ target, capability: 'home' })
  const manual = useValue({ target, capability: 'manual' })
  return { target, home, ...chip(p, home, manual, useNow()) }
}

// A face with the initial: filled while home, dimmed while away, dashed with no Value, with a hand
// while set by hand.
function Face({ initial, state, manual, size }: { initial: string; state: PresenceState; manual: boolean; size: 'small' | 'large' }) {
  const small = size === 'small'
  const look = {
    home: 'bg-emerald-500/30 text-emerald-100 ring-1 ring-emerald-400',
    away: 'bg-neutral-800 text-neutral-500',
    unknown: 'border border-dashed border-neutral-500 text-neutral-500',
  }[state]
  return (
    <span className={`relative grid shrink-0 place-items-center rounded-full font-semibold ${small ? 'size-5 text-[0.625rem]' : 'size-12 text-xl'} ${look}`}>
      {initial}
      {manual && (
        <span
          title="Set by hand"
          className={`absolute grid place-items-center rounded-full bg-amber-400 text-neutral-900 ${small ? '-right-1 -bottom-1 size-3' : '-right-1 -bottom-1 size-5'}`}
        >
          <Svg path={mdiHandBackRightOutline} className={small ? 'size-2.5' : 'size-4'} />
        </span>
      )}
    </span>
  )
}

function Chip({ presence, onOpen }: { presence: Presence; onOpen: () => void }) {
  const { initial, state, manual, held } = usePresence(presence)
  return (
    <button
      onClick={onOpen}
      aria-label={`${presence.name}: ${said[state].toLowerCase()}${manual ? ', set by hand' : ''}`}
      className="flex items-center gap-2 rounded-full bg-neutral-900 py-1.5 pr-3 pl-1.5 text-sm hover:bg-neutral-800"
    >
      <Face initial={initial} state={state} manual={manual} size="small" />
      <span className={state === 'home' ? '' : 'text-neutral-400'}>{presence.name}</span>
      {held && <span className="text-xs text-neutral-500">{held}</span>}
    </button>
  )
}

// The row, by Name; nothing when Oiko has no Person yet.
export function PresenceRow() {
  const presences = usePresences()
  const [open, setOpen] = useState<string | null>(null)
  const opened = presences.find((p) => p.id === open)
  if (!presences.length) return null
  return (
    <>
      <div className="flex flex-wrap gap-2">
        {ordered(presences).map((p) => (
          <Chip key={p.id} presence={p} onOpen={() => setOpen(p.id)} />
        ))}
      </div>
      {opened && <PresenceSheet key={opened.id} presence={opened} onClose={() => setOpen(null)} />}
    </>
  )
}

const clock = (t: string | number) => new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })

// A Person's sheet: the Value and since when, Home / Away, a note while set by hand, its Presence
// sources live, and to a Member, who reads the past, the last 24 h with the stretches set by hand
// hatched.
function PresenceSheet({ presence, onClose }: { presence: Presence; onClose: () => void }) {
  const { target, home, initial, state, manual, held } = usePresence(presence)
  const command = useCommand(target)
  const member = useAllows('member')
  const choice = (v: boolean) => {
    const picked = (state === 'home') === v && state !== 'unknown'
    return (
      <button
        onClick={() => sendCommand(target, { home: v })}
        aria-pressed={picked}
        className={`flex-1 rounded-lg py-2 font-medium ${picked ? (v ? 'bg-emerald-500/30 text-emerald-100' : 'bg-neutral-700 text-white') : 'text-neutral-400 hover:bg-neutral-800'}`}
      >
        {v ? 'Home' : 'Away'}
      </button>
    )
  }
  return (
    <Panel
      title={
        <div className="flex items-center gap-3">
          <Face initial={initial} state={state} manual={manual} size="large" />
          <div className="min-w-0">
            <h2 className="truncate text-lg font-medium">{presence.name}</h2>
            <p className="text-sm text-neutral-400">
              {said[state]}
              {home?.since && ` since ${clock(home.since)} · ${held}`}
              <CommandNote command={command} />
            </p>
          </div>
        </div>
      }
      onClose={onClose}
    >
      <div className="flex gap-1 rounded-xl bg-neutral-950 p-1 ring-1 ring-neutral-800">
        {choice(true)}
        {choice(false)}
      </div>
      {manual && (
        <p className="flex gap-2 rounded-lg bg-amber-950/40 p-3 text-sm text-amber-200">
          <Svg path={mdiHandBackRightOutline} className="size-5 shrink-0" />
          Set by hand: {presence.name} stays {said[state].toLowerCase()} until {presence.sources.length ? 'the Presence sources agree or it is ' : ''}set again.
        </p>
      )}
      <Sources presence={presence} />
      {member && <Day target={target} />}
    </Panel>
  )
}

// A Presence's sources, each with its live state. An Admin edits them in place, the live state still
// in sight, and saves them whole.
function Sources({ presence }: { presence: Presence }) {
  const admin = useAllows('admin')
  const targets = useCatalogue()
  const [draft, setDraft] = useState<PresenceSource[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const list = draft ?? presence.sources
  if (!list.length && !admin) return null
  const close = () => {
    setDraft(null)
    setError(null)
  }
  const save = async () => {
    const err = await edit('PUT', `presences/${presence.id}`, { sources: draft })
    if (err) setError(err)
    else close()
  }
  // What may be bound: a Function, an Aggregate or a Flag with a binary state or main control.
  const offered = (group: string) =>
    targets.list.filter((e) => targetKind(e.target) === group && readCapability(e.kind, e.capabilities) && !list.some((s) => s.target === e.target))
  return (
    <section className="space-y-2">
      <div className="flex items-center justify-between">
        <h3 className="text-xs font-semibold tracking-wide text-neutral-500 uppercase">Presence sources</h3>
        {admin && !draft && (
          <button onClick={() => setDraft(presence.sources)} className="text-sm text-amber-400 hover:text-amber-300">
            Edit
          </button>
        )}
      </div>
      {!list.length && <p className="text-sm text-neutral-500">None: set by hand only.</p>}
      <ul className="space-y-2 text-sm">
        {list.map((s) => (
          <SourceRow
            key={s.target}
            source={s}
            entry={targets.get(s.target)}
            name={targets.name(s.target)}
            onChange={
              draft
                ? (changed) => setDraft(changed ? draft.map((d) => (d.target === s.target ? changed : d)) : draft.filter((d) => d.target !== s.target))
                : undefined
            }
          />
        ))}
      </ul>
      {draft && (
        <div className="space-y-2 text-sm">
          <select
            value=""
            onChange={(e) => setDraft([...draft, { target: e.target.value, departureDelay: 0 }])}
            aria-label="Add a Presence source"
            className="w-full rounded bg-neutral-800 px-2 py-1"
          >
            <option value="">Add a Presence source…</option>
            {(['function', 'aggregate', 'flag'] as const).map((group) => (
              <optgroup key={group} label={{ function: 'Functions', aggregate: 'Aggregates', flag: 'Flags' }[group]}>
                {offered(group).map((e) => (
                  <option key={e.target} value={e.target}>
                    {e.name}
                  </option>
                ))}
              </optgroup>
            ))}
          </select>
          {error && <p className="text-red-400">{error}</p>}
          <div className="flex gap-2">
            <button onClick={save} className="rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300">
              Save
            </button>
            <button onClick={close} className="rounded bg-neutral-800 px-3 py-1 hover:bg-neutral-700">
              Cancel
            </button>
          </div>
        </div>
      )}
    </section>
  )
}

// One Presence source and what it says now; while editing, its departure delay in minutes and a way
// to unbind it (onChange with none).
function SourceRow({ source, entry, name, onChange }: { source: PresenceSource; entry?: Entry; name: string; onChange?: (changed?: PresenceSource) => void }) {
  const cap = entry && readCapability(entry.kind, entry.capabilities)
  const value = useValue({ target: source.target, capability: cap?.key ?? '' })
  const availability = useAvailability(source.target)
  const device = useDevices().find((d) => deviceTarget(d.id) === owner(source.target))
  const bridges = useBridges()
  const offline = availability === 'offline' || !!device?.detached || (device !== undefined && bridges[device.bridge] === false)
  const state = sourceState(source, cap, value, offline, useNow())
  const dot = 'ignored' in state ? 'border border-dashed border-neutral-500' : state.counts ? 'bg-emerald-400' : 'bg-neutral-600'
  return (
    <li className="flex items-center gap-2">
      <span className={`size-2.5 shrink-0 rounded-full ${dot}`} />
      <div className="min-w-0 flex-1">
        <p className="truncate">{name}</p>
        <p className="text-xs text-neutral-500">{sourceText(state, clock)}</p>
      </div>
      {onChange && (
        <>
          <label className="flex items-center gap-1 text-xs text-neutral-400">
            <input
              type="number"
              min={0}
              value={(source.departureDelay ?? 0) / 60}
              onChange={(e) => onChange({ ...source, departureDelay: Math.max(0, Math.round(Number(e.target.value) * 60)) })}
              aria-label={`Departure delay of ${name}, in minutes`}
              className="w-14 rounded bg-neutral-800 px-1.5 py-0.5 text-right text-sm text-neutral-100"
            />
            min
          </label>
          <button onClick={() => onChange()} aria-label={`Unbind ${name}`} className="rounded p-1 text-neutral-400 hover:bg-neutral-800 hover:text-white">
            <Svg path={mdiClose} className="size-4" />
          </button>
        </>
      )}
    </li>
  )
}

// The last 24 h of a Presence: home green, away grey, nothing known blank, set by hand hatched.
function Day({ target }: { target: string }) {
  const [view] = useState(() => new HistoryView())
  const loaded = useHistory(view)
  const now = useNow()
  const home = { target, capability: 'home' }
  const manual = { target, capability: 'manual' }
  useEffect(() => {
    const now = Date.now()
    void view.load([home, manual], now - DAY, now, 1000, true)
    return () => view.close()
  }, [view, target]) // eslint-disable-line react-hooks/exhaustive-deps -- refs by value
  const end = Math.max(now, ...[...loaded.series.values()].map((ps) => ps.at(-1)?.t ?? 0))
  const from = end - DAY
  const band = (on: boolean) => (on ? 'bg-emerald-500/60' : 'bg-neutral-600')
  const hatched = 'repeating-linear-gradient(45deg, transparent 0 4px, rgba(251, 191, 36, 0.7) 4px 6px)'
  return (
    <div className="space-y-1">
      <div className="relative h-4 overflow-hidden rounded bg-neutral-800">
        {stretches(loaded.series.get(key(home)) ?? [], from, end).map((s) => (
          <span key={s.left} className={`absolute inset-y-0 ${band(s.on)}`} style={{ left: `${s.left}%`, width: `${s.width}%` }} />
        ))}
        {stretches(loaded.series.get(key(manual)) ?? [], from, end)
          .filter((s) => s.on)
          .map((s) => (
            <span key={s.left} className="absolute inset-y-0" style={{ left: `${s.left}%`, width: `${s.width}%`, backgroundImage: hatched }} />
          ))}
      </div>
      <p className="flex justify-between text-xs text-neutral-500">
        <span>24 h ago</span>
        <span>now</span>
      </p>
    </div>
  )
}
