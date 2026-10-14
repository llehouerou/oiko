// The Presence row (ADR 0055): the Home presence's chip, then a chip per Person, then one per
// Aggregate of Presences, at the top of a Dashboard. A Person's and the Home presence's open their
// sheet: a Person's, where Home / Away sets their Presence by hand (ADR 0052) and an Admin binds its
// Presence sources; the Home presence's, where an Admin binds its Entrance doors and Signs of life
// (ADR 0051, 0054). A Presence's Tile opens the same sheet.

import { useEffect, useState, type ReactNode } from 'react'
import { mdiAccountGroupOutline, mdiClose, mdiHandBackRightOutline, mdiHomeOutline } from '@mdi/js'
import {
  edit,
  sendCommand,
  useAvailability,
  useBridges,
  useCatalogue,
  useCommand,
  useDerived,
  useDevices,
  useNow,
  useOnCount,
  useAggregates,
  usePresences,
  useValue,
} from './store'
import { useAllows } from './access'
import { DAY, HistoryView, key, useHistory } from './history'
import {
  boxUntil,
  chip,
  groupChip,
  groups,
  HOME,
  homeChip,
  persons,
  readCapability,
  someoneElse,
  sourceState,
  sourceText,
  stretches,
  type PresenceState,
} from './presence'
import { deviceTarget, owner, presenceTarget, targetKind, type Entry } from './targets'
import { memberLeaves } from './tiles'
import { CommandNote } from './Capabilities'
import { Panel } from './Panel'
import { Svg } from './icons'
import type { Aggregate, Presence, PresenceSource, Ref, Target } from './types'

const said: Record<PresenceState, string> = { home: 'Home', away: 'Away', unknown: 'Unknown' }

const homeOf = (p: Presence): Ref => ({ target: presenceTarget(p.id), capability: 'home' })

// usePresence is what p's chip and sheet show, followed as its Values change.
function usePresence(p: Presence) {
  const target = presenceTarget(p.id)
  const home = useValue({ target, capability: 'home' })
  const manual = useValue({ target, capability: 'manual' })
  return { target, home, ...chip(p, home, manual, useNow()) }
}

// useHome is what the Home presence's chip and sheet show, followed as its Value and the Persons'
// change.
function useHome(p: Presence, people: Presence[]) {
  const home = useValue(homeOf(p))
  const personsHome = useOnCount(people.map(homeOf))
  return { home, personsHome, ...homeChip(home, personsHome, useNow()) }
}

// A face with the initial: filled while home, dimmed while away, dashed with no Value, with a hand
// while set by hand.
function Face({ initial, state, manual, size }: { initial: ReactNode; state: PresenceState; manual: boolean; size: 'small' | 'large' }) {
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

// The Home presence's chip: a house, someone home or not, and for how long.
function HomeChip({ presence, people, onOpen }: { presence: Presence; people: Presence[]; onOpen: () => void }) {
  const { state, text, held } = useHome(presence, people)
  return (
    <button onClick={onOpen} aria-label={text} className="flex items-center gap-2 rounded-full bg-neutral-900 py-1.5 pr-3 pl-1.5 text-sm hover:bg-neutral-800">
      <Face initial={<Svg path={mdiHomeOutline} className="size-3.5" />} state={state} manual={false} size="small" />
      <span className={state === 'home' ? '' : 'text-neutral-400'}>{text}</span>
      {held && <span className="text-xs text-neutral-500">{held}</span>}
    </button>
  )
}

// An Aggregate of Presences' chip: its Name and how many of its members are home. It only shows.
function GroupChip({ aggregate, aggregates }: { aggregate: Aggregate; aggregates: Aggregate[] }) {
  const homes = memberLeaves(aggregate, aggregates).map((target) => ({ target, capability: 'home' }))
  const text = useDerived(homes, (vs) => groupChip(aggregate, vs))
  const someone = useOnCount(homes) > 0
  return (
    <span className="flex items-center gap-2 rounded-full bg-neutral-900 py-1.5 pr-3 pl-1.5 text-sm">
      <Face initial={<Svg path={mdiAccountGroupOutline} className="size-3.5" />} state={someone ? 'home' : 'away'} manual={false} size="small" />
      <span className={someone ? '' : 'text-neutral-400'}>{text}</span>
    </span>
  )
}

// The row: the Home presence, then the Persons by Name, then the Aggregates of Presences by Name;
// nothing until Oiko has told them.
export function PresenceRow() {
  const presences = usePresences()
  const aggregates = useAggregates()
  const [open, setOpen] = useState<string | null>(null)
  const home = presences.find((p) => p.id === HOME)
  const people = persons(presences)
  const opened = presences.find((p) => p.id === open)
  if (!presences.length) return null
  return (
    <>
      <div className="flex flex-wrap gap-2">
        {home && <HomeChip presence={home} people={people} onOpen={() => setOpen(HOME)} />}
        {people.map((p) => (
          <Chip key={p.id} presence={p} onOpen={() => setOpen(p.id)} />
        ))}
        {groups(aggregates).map((a) => (
          <GroupChip key={a.id} aggregate={a} aggregates={aggregates} />
        ))}
      </div>
      {opened && <PresenceSheet presence={opened} onClose={() => setOpen(null)} />}
    </>
  )
}

// PresenceSheet is a Presence's sheet: the Home presence's or a Person's.
export function PresenceSheet({ presence, onClose }: { presence: Presence; onClose: () => void }) {
  const people = persons(usePresences())
  return presence.id === HOME ? (
    <HomeSheet presence={presence} people={people} onClose={onClose} />
  ) : (
    <PersonSheet key={presence.id} presence={presence} onClose={onClose} />
  )
}

const clock = (t: string | number) => new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })

// The Home presence's sheet: its Value and since when, the Persons home, someone else inside and
// until when, and its Entrance doors and Signs of life live.
function HomeSheet({ presence, people, onClose }: { presence: Presence; people: Presence[]; onClose: () => void }) {
  const { home, personsHome, state, text, held } = useHome(presence, people)
  const names = useDerived(people.map(homeOf), (vs) =>
    people
      .filter((_, i) => vs[i]?.data === true)
      .map((p) => p.name)
      .join(', '),
  )
  const targets = useCatalogue()
  const refOf = (t: Target): Ref => {
    const e = targets.get(t)
    return { target: t, capability: (e && readCapability(e)?.key) ?? '' }
  }
  const now = useNow()
  const doors = presence.entranceDoors ?? []
  const signs = presence.signsOfLife ?? []
  const until = useDerived([...doors, ...signs].map(refOf), (vs) => boxUntil(vs.slice(0, doors.length), vs.slice(doors.length), presence.exitGrace ?? 0, now))
  return (
    <Panel
      title={
        <div className="flex items-center gap-3">
          <Face initial={<Svg path={mdiHomeOutline} className="size-6" />} state={state} manual={false} size="large" />
          <div className="min-w-0">
            <h2 className="truncate text-lg font-medium">{presence.name}</h2>
            <p className="text-sm text-neutral-400">
              {text}
              {home?.since && ` since ${clock(home.since)} · ${held}`}
            </p>
          </div>
        </div>
      }
      onClose={onClose}
    >
      <div className="space-y-1 text-sm">
        <p>{names ? `Home: ${names}` : 'None of you is home.'}</p>
        {someoneElse(home, personsHome) && (
          <p className="text-emerald-200">Someone else is inside{until ? `, until ${clock(until)} unless seen again` : ''}.</p>
        )}
      </div>
      <HomeBindings presence={presence} />
    </Panel>
  )
}

type Bindings = { entranceDoors: Target[]; signsOfLife: Target[]; exitGrace: number }

// The Home presence's Entrance doors and Signs of life, each with its live state, and its exit
// grace. An Admin edits them in place and saves them whole.
function HomeBindings({ presence }: { presence: Presence }) {
  const admin = useAllows('admin')
  const targets = useCatalogue()
  const { draft, setDraft, error, close, save } = useDraft<Bindings>(`presences/${presence.id}`, (d) => d)
  const saved: Bindings = { entranceDoors: presence.entranceDoors ?? [], signsOfLife: presence.signsOfLife ?? [], exitGrace: presence.exitGrace ?? 0 }
  const shown = draft ?? saved
  const list = (which: 'entranceDoors' | 'signsOfLife', title: string, one: string, none: string, editButton: boolean) => (
    <section className="space-y-2">
      <div className="flex items-center justify-between">
        <h3 className="text-xs font-semibold tracking-wide text-neutral-500 uppercase">{title}</h3>
        {editButton && admin && !draft && (
          <button onClick={() => setDraft(saved)} className="text-sm text-amber-400 hover:text-amber-300">
            Edit
          </button>
        )}
      </div>
      {!shown[which].length && <p className="text-sm text-neutral-500">{none}</p>}
      <ul className="space-y-2 text-sm">
        {shown[which].map((t) => (
          <BindingRow
            key={t}
            source={{ target: t }}
            entry={targets.get(t)}
            name={targets.name(t)}
            onChange={draft ? () => setDraft({ ...draft, [which]: draft[which].filter((d) => d !== t) }) : undefined}
            delay={false}
          />
        ))}
      </ul>
      {draft && <AddBinding label={`Add ${one}`} bound={draft[which]} onAdd={(t) => setDraft({ ...draft, [which]: [...draft[which], t] })} />}
    </section>
  )
  return (
    <>
      {list('entranceDoors', 'Entrance doors', 'an Entrance door', 'None: only the Persons count.', true)}
      {list('signsOfLife', 'Signs of life', 'a Sign of life', 'None.', false)}
      <p className="flex items-center gap-2 text-sm text-neutral-400">
        <span>Exit grace:</span>
        {draft ? (
          <input
            type="number"
            min={0}
            value={draft.exitGrace / 60}
            onChange={(e) => setDraft({ ...draft, exitGrace: Math.max(0, Math.round(Number(e.target.value) * 60)) })}
            aria-label="Exit grace, in minutes"
            className="w-14 rounded bg-neutral-800 px-1.5 py-0.5 text-right text-sm text-neutral-100"
          />
        ) : (
          <span>{shown.exitGrace / 60}</span>
        )}
        <span>min after an Entrance door closes</span>
      </p>
      {draft && <SaveBar error={error} onSave={save} onCancel={close} />}
    </>
  )
}

// What may be bound, in a select adding it: a Function, an Aggregate or a Flag with a binary state
// or main control, none of bound, and no Aggregate of Presences.
function AddBinding({ label, bound, onAdd }: { label: string; bound: Target[]; onAdd: (t: Target) => void }) {
  const targets = useCatalogue()
  const offered = (group: string) =>
    targets.list.filter((e) => targetKind(e.target) === group && e.kind !== 'presence' && readCapability(e) && !bound.includes(e.target))
  return (
    <select value="" onChange={(e) => onAdd(e.target.value)} aria-label={label} className="w-full rounded bg-neutral-800 px-2 py-1 text-sm">
      <option value="">{label}…</option>
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
  )
}

// useDraft edits what path keeps in place: a draft while editing, saved whole as body makes it of
// the draft; an error keeps it open.
function useDraft<T>(path: string, body: (draft: T) => unknown) {
  const [draft, setDraft] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const close = () => {
    setDraft(null)
    setError(null)
  }
  const save = async () => {
    const err = draft === null ? null : await edit('PUT', path, body(draft))
    if (err) setError(err)
    else close()
  }
  return { draft, setDraft, error, close, save }
}

function SaveBar({ error, onSave, onCancel }: { error: string | null; onSave: () => void; onCancel: () => void }) {
  return (
    <div className="space-y-2 text-sm">
      {error && <p className="text-red-400">{error}</p>}
      <div className="flex gap-2">
        <button onClick={onSave} className="rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300">
          Save
        </button>
        <button onClick={onCancel} className="rounded bg-neutral-800 px-3 py-1 hover:bg-neutral-700">
          Cancel
        </button>
      </div>
    </div>
  )
}

// A Person's sheet: the Value and since when, Home / Away, a note while set by hand, its Presence
// sources live, and to a Member, who reads the past, the last 24 h with the stretches set by hand
// hatched.
function PersonSheet({ presence, onClose }: { presence: Presence; onClose: () => void }) {
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
          Set by hand: {presence.name} stays {said[state].toLowerCase()} until {presence.sources?.length ? 'the Presence sources agree or it is ' : ''}set
          again.
        </p>
      )}
      <Sources presence={presence} />
      {member && <Day target={target} />}
    </Panel>
  )
}

// A Presence's Presence sources, each with its live state. An Admin edits them in place, the live state still
// in sight, and saves them whole.
function Sources({ presence }: { presence: Presence }) {
  const admin = useAllows('admin')
  const targets = useCatalogue()
  const { draft, setDraft, error, close, save } = useDraft<PresenceSource[]>(`presences/${presence.id}`, (sources) => ({ sources }))
  const list = draft ?? presence.sources ?? []
  if (!list.length && !admin) return null
  return (
    <section className="space-y-2">
      <div className="flex items-center justify-between">
        <h3 className="text-xs font-semibold tracking-wide text-neutral-500 uppercase">Presence sources</h3>
        {admin && !draft && (
          <button onClick={() => setDraft(list)} className="text-sm text-amber-400 hover:text-amber-300">
            Edit
          </button>
        )}
      </div>
      {!list.length && <p className="text-sm text-neutral-500">None: set by hand only.</p>}
      <ul className="space-y-2 text-sm">
        {list.map((s) => (
          <BindingRow
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
        <>
          <AddBinding
            label="Add a Presence source"
            bound={draft.map((s) => s.target)}
            onAdd={(target) => setDraft([...draft, { target, departureDelay: 0 }])}
          />
          <SaveBar error={error} onSave={save} onCancel={close} />
        </>
      )}
    </section>
  )
}

// One bound Target and what it says now; while editing, a way to unbind it (onChange with none)
// and, if delay, its departure delay in minutes.
function BindingRow({
  source,
  entry,
  name,
  onChange,
  delay = true,
}: {
  source: PresenceSource
  entry?: Entry
  name: string
  onChange?: (changed?: PresenceSource) => void
  delay?: boolean
}) {
  const cap = entry && readCapability(entry)
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
          {delay && (
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
          )}
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
