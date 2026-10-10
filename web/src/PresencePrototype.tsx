// PROTOTYPE, throwaway: wayfinder ticket "How does presence look and get edited in the web client?".
// Three variants of presence on the Home page, switchable via ?variant=A|B|C, above the real
// dashboard. Every Person, source and Value here is made up and lives in memory: nothing reaches
// Oiko. The simulation follows ADR 0049 (any source home, a departure delay each), ADR 0051 (the
// Home presence and its box) and ADR 0052 (an override holding until the rule agrees).

import { useEffect, useReducer, useState, type ReactNode } from 'react'
import {
  mdiAccount,
  mdiAccountGroup,
  mdiAccountOutline,
  mdiAccountQuestionOutline,
  mdiChevronDown,
  mdiChevronLeft,
  mdiChevronRight,
  mdiCogOutline,
  mdiGauge,
  mdiHandBackRightOutline,
  mdiHome,
  mdiHomeAccount,
  mdiHomeOutline,
  mdiWifi,
  mdiWifiOff,
} from '@mdi/js'
import { Svg } from './icons'
import { Panel } from './Panel'

const MIN = 60_000

// ---- the simulated home --------------------------------------------------------------------

type Kind = 'network' | 'flag' | 'contact' | 'occupancy'
interface Src {
  id: string
  name: string
  kind: Kind
  on: boolean
  since: number
}
interface Binding {
  src: string
  delay: number // minutes
}
interface Person {
  id: string
  name: string
  bindings: Binding[]
  forced?: boolean // the override, while it holds
  value?: boolean
  since: number
}
interface HomeCfg {
  doors: string[]
  signs: string[]
  grace: number // minutes
}
interface Agg {
  id: string
  name: string
  members: string[]
  rule: 'any' | 'all'
}
interface World {
  now: number
  routerOnline: boolean
  admin: boolean
  srcs: Src[]
  persons: Person[]
  home: HomeCfg
  box: boolean
  emptyAt?: number
  homeValue: boolean
  homeSince: number
  aggregates: Agg[]
}

function initial(): World {
  const now = Date.now()
  const src = (id: string, name: string, kind: Kind, on: boolean, ago: number): Src => ({ id, name, kind, on, since: now - ago * MIN })
  return step({
    now,
    routerOnline: true,
    admin: true,
    srcs: [
      src('phone-alice', "Alice's phone", 'network', true, 130),
      src('phone-bob', "Bob's phone", 'network', false, 40),
      src('flag-alice', 'Alice at home', 'flag', true, 125),
      src('front-door', 'Front door', 'contact', false, 130),
      src('garden-door', 'Garden door', 'contact', false, 300),
      src('hall', 'Hall motion', 'occupancy', false, 12),
      src('living', 'Living room motion', 'occupancy', false, 30),
      src('guests', 'Guests over', 'flag', false, 3000),
    ],
    persons: [
      { id: 'alice', name: 'Alice', bindings: [{ src: 'phone-alice', delay: 15 }, { src: 'flag-alice', delay: 0 }], value: true, since: now - 130 * MIN },
      { id: 'bob', name: 'Bob', bindings: [{ src: 'phone-bob', delay: 15 }], value: false, since: now - 25 * MIN },
      { id: 'chloe', name: 'Chloe', bindings: [], forced: true, value: true, since: now - 300 * MIN },
      { id: 'dan', name: 'Dan', bindings: [], since: now },
    ],
    home: { doors: ['front-door', 'garden-door'], signs: ['hall', 'living', 'guests'], grace: 5 },
    box: false,
    homeValue: true,
    homeSince: now - 300 * MIN,
    aggregates: [{ id: 'children', name: 'Children', members: ['chloe', 'dan'], rule: 'any' }],
  })
}

// Whether each of p's sources counts as home now: undefined when ignored (offline, no Value).
function counts(w: World, p: Person) {
  return p.bindings.map((b) => {
    const s = w.srcs.find((s) => s.id === b.src)
    if (!s || (s.kind === 'network' && !w.routerOnline)) return { b, s, counts: undefined }
    const until = s.on ? undefined : s.since + b.delay * MIN
    return { b, s, counts: s.on || w.now < until!, until }
  })
}

function rule(w: World, p: Person) {
  const known = counts(w, p).filter((c) => c.counts !== undefined)
  return known.length ? known.some((c) => c.counts) : undefined
}

// Recomputes every Presence and the Home presence from the sources and the clock.
function step(w: World): World {
  const persons = w.persons.map((p) => {
    const r = rule(w, p)
    const forced = p.forced !== undefined && p.forced !== r ? p.forced : undefined
    const value = forced ?? r ?? p.value
    return { ...p, forced, value, since: value === p.value ? p.since : w.now }
  })
  const empty = w.emptyAt !== undefined && w.now >= w.emptyAt
  const box = empty ? false : w.box
  const homeValue = persons.some((p) => p.value) || box
  return { ...w, persons, box, emptyAt: empty ? undefined : w.emptyAt, homeValue, homeSince: homeValue === w.homeValue ? w.homeSince : w.now }
}

type Action =
  | { type: 'tick'; ms: number }
  | { type: 'source'; id: string }
  | { type: 'router' }
  | { type: 'admin' }
  | { type: 'set'; person: string; value: boolean }
  | { type: 'bindings'; person: string; bindings: Binding[] }
  | { type: 'home'; cfg: HomeCfg }
  | { type: 'reset' }

function reduce(w: World, a: Action): World {
  switch (a.type) {
    case 'tick':
      return step({ ...w, now: w.now + a.ms })
    case 'source': {
      const s = w.srcs.find((s) => s.id === a.id)!
      const on = !s.on
      let { box, emptyAt } = w
      if (w.home.doors.includes(s.id) && !on) emptyAt = w.now + w.home.grace * MIN // a door closed
      if (w.home.signs.includes(s.id) && on && w.home.doors.length) (box = true), (emptyAt = undefined)
      return step({ ...w, box, emptyAt, srcs: w.srcs.map((x) => (x === s ? { ...x, on, since: w.now } : x)) })
    }
    case 'router':
      return step({ ...w, routerOnline: !w.routerOnline })
    case 'admin':
      return { ...w, admin: !w.admin }
    case 'set': // a Command: the override, cleared at once if the rule already agrees
      return step({ ...w, persons: w.persons.map((p) => (p.id === a.person ? { ...p, forced: a.value, value: a.value, since: p.value === a.value ? p.since : w.now } : p)) })
    case 'bindings':
      return step({ ...w, persons: w.persons.map((p) => (p.id === a.person ? { ...p, bindings: a.bindings } : p)) })
    case 'home':
      return step({ ...w, home: a.cfg })
    case 'reset':
      return initial()
  }
}

type Sim = { w: World; act: (a: Action) => void }

// ---- shared bits -------------------------------------------------------------------------------

const ago = (ms: number) => {
  const m = Math.floor(ms / MIN)
  if (m < 1) return 'now'
  if (m < 60) return `${m} min`
  if (m < 48 * 60) return `${Math.floor(m / 60)} h`
  return `${Math.floor(m / 1440)} d`
}
const clock = (t: number) => new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
const said = (v?: boolean) => (v === undefined ? 'Unknown' : v ? 'Home' : 'Away')
const personIcon = (v?: boolean) => (v === undefined ? mdiAccountQuestionOutline : v ? mdiAccount : mdiAccountOutline)
const toggle = (sim: Sim, p: Person) => sim.act({ type: 'set', person: p.id, value: !p.value })
const aggValue = (w: World, a: Agg) => {
  const vs = a.members.map((m) => w.persons.find((p) => p.id === m)?.value).filter((v) => v !== undefined)
  if (!vs.length) return undefined
  return a.rule === 'any' ? vs.some(Boolean) : vs.every(Boolean)
}
const aggHome = (w: World, a: Agg) => a.members.filter((m) => w.persons.find((p) => p.id === m)?.value).length

// Who keeps the home occupied, as the client can tell from the presences list alone.
function homeWhy(w: World) {
  const home = w.persons.filter((p) => p.value).map((p) => p.name)
  if (home.length) return new Intl.ListFormat('en').format(home)
  return w.homeValue ? 'someone, no Person' : 'nobody'
}

const kindLabel: Record<Kind, string> = { network: 'phone', flag: 'flag', contact: 'contact', occupancy: 'occupancy' }

// A source's live line: its state, and while it still counts after going away, until when.
function SourceLine({ w, c }: { w: World; c: ReturnType<typeof counts>[number] }) {
  if (!c.s) return <span className="text-red-400">gone</span>
  const s = c.s
  const state = s.kind === 'network' ? (s.on ? 'connected' : 'not connected') : s.kind === 'contact' ? (s.on ? 'open' : 'closed') : s.on ? 'on' : 'off'
  if (c.counts === undefined) return <span className="text-neutral-500">offline · ignored</span>
  return (
    <span className={c.counts ? 'text-emerald-300' : 'text-neutral-400'}>
      {state} · {ago(w.now - s.since)}
      {!s.on && c.counts && ` · counts as home until ${clock(c.until!)}`}
    </span>
  )
}

function Manual({ className = 'size-4' }: { className?: string }) {
  return (
    <span title="Set by hand" className="inline-flex">
      <Svg path={mdiHandBackRightOutline} className={className} />
    </span>
  )
}

const legend = 'mb-2 text-xs font-semibold tracking-wide text-neutral-500 uppercase'
const input = 'rounded bg-neutral-800 px-2 py-1'
const primary = 'rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300'
const secondary = 'rounded bg-neutral-800 px-3 py-1 hover:bg-neutral-700'

// A Person's Presence sources, each with its departure delay; saved whole.
function SourcesEditor({ w, bindings, onChange }: { w: World; bindings: Binding[]; onChange: (b: Binding[]) => void }) {
  const free = w.srcs.filter((s) => !bindings.some((b) => b.src === s.id))
  return (
    <div className="space-y-2 text-sm">
      {bindings.length === 0 && <p className="text-neutral-500">No source: set only by hand.</p>}
      {bindings.map((b) => {
        const s = w.srcs.find((s) => s.id === b.src)
        return (
          <div key={b.src} className="flex items-center gap-2">
            <span className="min-w-0 flex-1 truncate">
              {s?.name} <span className="text-neutral-500">· {s && kindLabel[s.kind]}</span>
            </span>
            <label className="flex items-center gap-1 text-neutral-400">
              away after
              <input
                type="number"
                min={0}
                value={b.delay}
                onChange={(e) => onChange(bindings.map((x) => (x === b ? { ...x, delay: Number(e.target.value) } : x)))}
                className={`w-14 ${input} text-neutral-100`}
              />
              min
            </label>
            <button onClick={() => onChange(bindings.filter((x) => x !== b))} aria-label={`Unbind ${s?.name}`} className="text-neutral-500 hover:text-red-400">
              ✕
            </button>
          </div>
        )
      })}
      <select
        value=""
        onChange={(e) => {
          const s = w.srcs.find((s) => s.id === e.target.value)!
          onChange([...bindings, { src: s.id, delay: s.kind === 'network' ? 15 : 0 }])
        }}
        className={`w-full ${input}`}
      >
        <option value="" disabled>
          Add a source…
        </option>
        {free.map((s) => (
          <option key={s.id} value={s.id}>
            {s.name} · {kindLabel[s.kind]}
          </option>
        ))}
      </select>
      <p className="text-xs text-neutral-500">Home while any source is on. A phone's Wi-Fi drops while it sleeps: give it about 15 min.</p>
    </div>
  )
}

// The Home presence's Entrance doors, Signs of life and exit grace.
function HomeEditor({ w, cfg, onChange }: { w: World; cfg: HomeCfg; onChange: (c: HomeCfg) => void }) {
  const pick = (list: 'doors' | 'signs', id: string, on: boolean) => onChange({ ...cfg, [list]: on ? [...cfg[list], id] : cfg[list].filter((x) => x !== id) })
  const box = (list: 'doors' | 'signs', kinds: Kind[]) =>
    w.srcs
      .filter((s) => kinds.includes(s.kind))
      .map((s) => (
        <label key={s.id} className="flex items-center gap-2">
          <input type="checkbox" checked={cfg[list].includes(s.id)} onChange={(e) => pick(list, s.id, e.target.checked)} className="accent-amber-400" />
          {s.name} <span className="text-neutral-500">· {kindLabel[s.kind]}</span>
        </label>
      ))
  return (
    <div className="space-y-4 text-sm">
      <p className="text-neutral-400">Home while any Person is, or while someone was seen inside since an Entrance door last closed.</p>
      <fieldset className="space-y-1">
        <legend className={legend}>Entrance doors</legend>
        {box('doors', ['contact'])}
      </fieldset>
      <fieldset className="space-y-1">
        <legend className={legend}>Signs of life</legend>
        {box('signs', ['occupancy', 'flag'])}
        {cfg.signs.length > 0 && !cfg.doors.length && <p className="text-red-400">Signs of life need an Entrance door.</p>}
      </fieldset>
      <label className="flex items-center gap-2 text-neutral-400">
        Empty when a door closes with no sign of life within
        <input type="number" min={0} value={cfg.grace} onChange={(e) => onChange({ ...cfg, grace: Number(e.target.value) })} className={`w-14 ${input} text-neutral-100`} />
        min
      </label>
    </div>
  )
}

// A Person's Presence panel, opened from its ⋯ (variant A).
function PresencePanel({ sim, person, onClose }: { sim: Sim; person: Person; onClose: () => void }) {
  const [draft, setDraft] = useState(person.bindings)
  return (
    <Panel
      title={
        <>
          <h2 className="text-lg font-medium">{person.name}'s presence</h2>
          <p className="text-xs text-neutral-500">presence · {said(person.value)}</p>
        </>
      }
      onClose={onClose}
    >
      <section>
        <h3 className={legend}>Presence sources</h3>
        <SourcesEditor w={sim.w} bindings={draft} onChange={setDraft} />
      </section>
      <button onClick={() => (sim.act({ type: 'bindings', person: person.id, bindings: draft }), onClose())} className={primary}>
        Save
      </button>
    </Panel>
  )
}

function HomePanel({ sim, onClose }: { sim: Sim; onClose: () => void }) {
  const [draft, setDraft] = useState(sim.w.home)
  return (
    <Panel title={<h2 className="text-lg font-medium">Home presence</h2>} onClose={onClose}>
      <HomeEditor w={sim.w} cfg={draft} onChange={setDraft} />
      <button onClick={() => (sim.act({ type: 'home', cfg: draft }), onClose())} className={primary}>
        Save
      </button>
    </Panel>
  )
}

// Every Presence's settings in one panel (variant B).
function AllPresencesPanel({ sim, onClose }: { sim: Sim; onClose: () => void }) {
  const [open, setOpen] = useState<string>('home')
  const [home, setHome] = useState(sim.w.home)
  const [drafts, setDrafts] = useState(() => Object.fromEntries(sim.w.persons.map((p) => [p.id, p.bindings])))
  const fold = (id: string, title: ReactNode, body: ReactNode) => (
    <section key={id} className="rounded-lg bg-neutral-800/40">
      <button onClick={() => setOpen(open === id ? '' : id)} className="flex w-full items-center gap-2 px-3 py-2 text-left">
        <Svg path={mdiChevronDown} className={`size-5 transition-transform ${open === id ? '' : '-rotate-90'}`} />
        {title}
      </button>
      {open === id && <div className="px-3 pb-3">{body}</div>}
    </section>
  )
  return (
    <Panel title={<h2 className="text-lg font-medium">Presence</h2>} onClose={onClose}>
      <div className="space-y-2">
        {fold('home', <span className="font-medium">Home</span>, <HomeEditor w={sim.w} cfg={home} onChange={setHome} />)}
        {sim.w.persons.map((p) =>
          fold(
            p.id,
            <span className="flex-1">
              <span className="font-medium">{p.name}</span>
              <span className="text-sm text-neutral-500"> · {drafts[p.id]!.length ? `${drafts[p.id]!.length} source${drafts[p.id]!.length > 1 ? 's' : ''}` : 'by hand only'}</span>
            </span>,
            <SourcesEditor w={sim.w} bindings={drafts[p.id]!} onChange={(b) => setDrafts({ ...drafts, [p.id]: b })} />,
          ),
        )}
      </div>
      <button
        onClick={() => {
          sim.act({ type: 'home', cfg: home })
          for (const p of sim.w.persons) sim.act({ type: 'bindings', person: p.id, bindings: drafts[p.id]! })
          onClose()
        }}
        className={primary}
      >
        Save
      </button>
    </Panel>
  )
}

// A phone's network Function, as today's roles draw it (a lone binary reading) and as a state.
function PhoneTiles({ w }: { w: World }) {
  const s = w.srcs.find((s) => s.id === 'phone-alice')!
  const on = s.on && w.routerOnline
  return (
    <div className="space-y-2">
      <h3 className={legend}>A phone's network Function, on its Tile</h3>
      <div className="grid grid-cols-[repeat(auto-fill,minmax(16rem,1fr))] gap-3">
        <figure className="space-y-1">
          <div className={`rounded-xl bg-neutral-900 p-1.5 ${w.routerOnline ? '' : 'opacity-50'}`}>
            <div className="flex h-14 items-center gap-3 rounded-lg bg-neutral-800 px-3">
              <Svg path={mdiGauge} className="size-6 text-neutral-500" />
              <p className="flex-1 truncate font-medium">{s.name}</p>
              <span className="text-2xl font-light">{s.on ? 'on' : 'off'}</span>
            </div>
          </div>
          <figcaption className="text-xs text-neutral-500">1 · as today: `connected` a lone reading</figcaption>
        </figure>
        <figure className="space-y-1">
          <div className={`rounded-xl bg-neutral-900 p-1.5 ${w.routerOnline ? '' : 'opacity-50'}`}>
            <div className={`flex h-14 items-center gap-3 rounded-lg px-3 ${on ? 'bg-emerald-500/20' : 'bg-neutral-800'}`}>
              <Svg path={s.on ? mdiWifi : mdiWifiOff} className={`size-6 ${on ? 'text-emerald-300' : 'text-neutral-500'}`} />
              <div className="min-w-0 flex-1">
                <p className="truncate font-medium">{s.name}</p>
                <p className={`truncate text-sm ${on ? 'text-emerald-300' : 'text-neutral-400'}`}>
                  {w.routerOnline ? `${s.on ? 'Connected' : 'Not connected'} · ${ago(w.now - s.since)}` : 'offline'}
                </p>
              </div>
            </div>
          </div>
          <figcaption className="text-xs text-neutral-500">2 · `connected` a state key: held for, History in periods</figcaption>
        </figure>
      </div>
    </div>
  )
}

// ---- variant A: pills beside the Flags' ------------------------------------------------------

function VariantA({ sim }: { sim: Sim }) {
  const { w } = sim
  const [open, setOpen] = useState<string | null>(null)
  const person = w.persons.find((p) => p.id === open)
  const pill = (on: boolean | undefined, body: ReactNode, onTap?: () => void, onMore?: () => void, title?: string) => (
    <div title={title} className={`flex items-center rounded-full text-sm ${on ? 'bg-emerald-400/20 text-emerald-200' : 'bg-neutral-800 text-neutral-300'}`}>
      <button onClick={onTap} disabled={!onTap} className={`flex items-center gap-1.5 py-1.5 pl-3 enabled:hover:text-white ${onMore ? 'pr-1' : 'pr-3'}`}>
        {body}
      </button>
      {onMore && (
        <button onClick={onMore} className="self-stretch pr-3 pl-1 text-neutral-500 hover:text-white">
          ⋯
        </button>
      )}
    </div>
  )
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center gap-2">
        {pill(
          w.homeValue,
          <>
            <Svg path={w.homeValue ? mdiHome : mdiHomeOutline} className="size-4" />
            <span className="font-medium">{w.homeValue ? 'Someone home' : 'Nobody home'}</span>
          </>,
          undefined,
          w.admin ? () => setOpen('home') : undefined,
          `${homeWhy(w)} · since ${clock(w.homeSince)}`,
        )}
        <span className="mx-1 h-5 w-px bg-neutral-700" />
        {w.persons.map((p) => (
          <span key={p.id}>
            {pill(
              p.value,
              <>
                <Svg path={personIcon(p.value)} className="size-4" />
                {p.name}
                {p.forced !== undefined && <Manual className="size-3.5 text-amber-300" />}
              </>,
              () => toggle(sim, p),
              w.admin ? () => setOpen(p.id) : undefined,
              `${said(p.value)} · since ${clock(p.since)}${p.forced !== undefined ? ' · set by hand' : ''}`,
            )}
          </span>
        ))}
        {w.aggregates.map((a) => (
          <span key={a.id}>
            {pill(
              aggValue(w, a),
              <>
                <Svg path={mdiAccountGroup} className="size-4" />
                {a.name} <span className="opacity-70">{aggHome(w, a)}/{a.members.length}</span>
              </>,
              () => a.members.forEach((m) => sim.act({ type: 'set', person: m, value: !aggValue(w, a) })),
            )}
          </span>
        ))}
        <span className="mx-1 h-5 w-px bg-neutral-700" />
        <span className="rounded-full bg-neutral-800 px-3 py-1.5 text-sm text-neutral-300">⚑ a Flag pill, as today</span>
      </div>
      <p className="text-xs text-neutral-500">
        A tap toggles a Person (an override, ✋ while it holds); the Home pill has no override. Hover for since and why. ⋯ opens its panel, to an Admin.
      </p>
      <PhoneTiles w={w} />
      {open === 'home' && <HomePanel sim={sim} onClose={() => setOpen(null)} />}
      {person && <PresencePanel key={person.id} sim={sim} person={person} onClose={() => setOpen(null)} />}
    </div>
  )
}

// ---- variant B: a Household Section of bar Tiles ---------------------------------------------

function VariantB({ sim }: { sim: Sim }) {
  const { w } = sim
  const [settings, setSettings] = useState(false)
  const bar = (key: string, on: boolean | undefined, icon: string, name: string, status: ReactNode, onTap: () => void) => (
    <section key={key} className="rounded-xl bg-neutral-900 p-1.5">
      <div
        role="button"
        tabIndex={0}
        onClick={onTap}
        className={`group/card relative flex h-14 cursor-pointer items-center gap-3 rounded-lg px-3 select-none ${on ? 'bg-amber-400/20' : 'bg-neutral-800 hover:bg-neutral-700/60'}`}
      >
        <Svg path={icon} className={`size-6 shrink-0 ${on ? 'text-amber-300' : 'text-neutral-500'}`} />
        <div className="min-w-0 flex-1">
          <p className="truncate font-medium">{name}</p>
          <p className="flex items-center gap-1 truncate text-sm text-neutral-400">{status}</p>
        </div>
      </div>
    </section>
  )
  return (
    <div className="space-y-6">
      <section className="overflow-hidden rounded-2xl border border-neutral-800 bg-neutral-900/30">
        <header className="flex items-center gap-2 bg-linear-to-r from-neutral-800/60 to-transparent px-4 py-3">
          <Svg path={mdiHomeAccount} className="size-6" />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-1">
              <h2 className="text-xl font-semibold tracking-tight">Household</h2>
              {w.admin && (
                <button onClick={() => setSettings(true)} aria-label="Presence settings" className="grid size-7 place-items-center rounded-full text-neutral-500 hover:bg-neutral-800 hover:text-white">
                  <Svg path={mdiCogOutline} className="size-5" />
                </button>
              )}
            </div>
            <p className="text-sm">
              <span className={w.homeValue ? 'text-emerald-300' : 'text-neutral-400'}>
                {w.homeValue ? 'Someone home' : 'Nobody home'} · {ago(w.now - w.homeSince)}
              </span>
              <span className="text-neutral-500"> · {homeWhy(w)}</span>
            </p>
          </div>
        </header>
        <div className="grid grid-cols-[repeat(auto-fill,minmax(16rem,1fr))] gap-3 p-3">
          {w.persons.map((p) =>
            bar(
              p.id,
              p.value,
              personIcon(p.value),
              p.name,
              <>
                {said(p.value)}
                {p.value !== undefined && ` · ${ago(w.now - p.since)}`}
                {p.forced !== undefined && (
                  <>
                    {' · '}
                    <Manual className="size-3.5 text-amber-300" /> by hand
                  </>
                )}
              </>,
              () => toggle(sim, p),
            ),
          )}
          {w.aggregates.map((a) =>
            bar(
              a.id,
              aggValue(w, a),
              mdiAccountGroup,
              a.name,
              `${aggHome(w, a)} of ${a.members.length} home`,
              () => a.members.forEach((m) => sim.act({ type: 'set', person: m, value: !aggValue(w, a) })),
            ),
          )}
        </div>
      </section>
      <p className="text-xs text-neutral-500">
        A built-in Section above the Areas', as a Tile each: a Presence is a bar like a Flag, a tap toggles it. The header carries the Home presence; its ⚙ opens every
        Presence's settings in one panel, to an Admin. On a custom Dashboard each Presence is a Tile to place.
      </p>
      <PhoneTiles w={w} />
      {settings && <AllPresencesPanel sim={sim} onClose={() => setSettings(false)} />}
    </div>
  )
}

// ---- variant C: faces in a banner, a sheet per Person ----------------------------------------

function Face({ p, size = 'size-14' }: { p: Person; size?: string }) {
  const ring = p.value === undefined ? 'border-2 border-dashed border-neutral-600 text-neutral-500' : p.value ? 'bg-emerald-500/25 ring-2 ring-emerald-400 text-emerald-100' : 'bg-neutral-800 text-neutral-500'
  return (
    <span className={`relative grid ${size} shrink-0 place-items-center rounded-full text-xl font-semibold ${ring}`}>
      {p.name[0]}
      {p.forced !== undefined && (
        <span className="absolute -right-1 -bottom-1 grid size-6 place-items-center rounded-full bg-amber-400 text-neutral-900">
          <Manual />
        </span>
      )}
    </span>
  )
}

// A made-up 24 h of a Presence: home, away, and the stretch set by hand hatched.
function DayStrip({ p }: { p: Person }) {
  const spans: [from: number, to: number, home: boolean, manual?: boolean][] =
    p.bindings.length === 0
      ? [[0, 100, true, true]]
      : [
          [0, 32, true],
          [32, 70, false],
          [70, 82, false, true],
          [82, 100, !!p.value],
        ]
  return (
    <div className="space-y-1">
      <div className="relative h-4 overflow-hidden rounded bg-neutral-800">
        {spans.map(([from, to, home, manual]) => (
          <span
            key={from}
            className={`absolute inset-y-0 ${home ? 'bg-emerald-500/60' : 'bg-neutral-700'}`}
            style={{
              left: `${from}%`,
              width: `${to - from}%`,
              backgroundImage: manual ? 'repeating-linear-gradient(45deg, transparent 0 4px, rgba(251,191,36,.6) 4px 6px)' : undefined,
            }}
          />
        ))}
      </div>
      <p className="flex justify-between text-xs text-neutral-500">
        <span>24 h · home green, by hand hatched</span>
        <span>now</span>
      </p>
    </div>
  )
}

function PersonSheet({ sim, person, onClose }: { sim: Sim; person: Person; onClose: () => void }) {
  const { w } = sim
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(person.bindings)
  const seg = (v: boolean) => (
    <button
      onClick={() => sim.act({ type: 'set', person: person.id, value: v })}
      className={`flex-1 rounded-lg py-2 font-medium ${person.value === v ? (v ? 'bg-emerald-500/30 text-emerald-100' : 'bg-neutral-700 text-white') : 'text-neutral-400 hover:bg-neutral-800'}`}
    >
      {v ? 'Home' : 'Away'}
    </button>
  )
  return (
    <Panel
      title={
        <div className="flex items-center gap-3">
          <Face p={person} size="size-12" />
          <div>
            <h2 className="text-lg font-medium">{person.name}</h2>
            <p className="text-sm text-neutral-400">
              {said(person.value)}
              {person.value !== undefined && ` since ${clock(person.since)} · ${ago(w.now - person.since)}`}
            </p>
          </div>
        </div>
      }
      onClose={onClose}
    >
      <div className="flex gap-1 rounded-xl bg-neutral-900 p-1 ring-1 ring-neutral-800">
        {seg(true)}
        {seg(false)}
      </div>
      {person.forced !== undefined && (
        <p className="flex gap-2 rounded-lg bg-amber-950/40 p-3 text-sm text-amber-200">
          <Manual className="size-5 shrink-0" />
          {person.bindings.length
            ? `Set by hand. ${person.name} follows the sources again once they say ${said(person.forced).toLowerCase()} too.`
            : `${person.name} has no source: only a hand sets this.`}
        </p>
      )}
      <DayStrip p={person} />
      <section className="space-y-2 text-sm">
        <div className="flex items-center">
          <h3 className={`${legend} mb-0 flex-1`}>Sources</h3>
          {w.admin && !editing && (
            <button onClick={() => (setDraft(person.bindings), setEditing(true))} className="text-neutral-400 hover:text-white">
              Edit
            </button>
          )}
        </div>
        {editing ? (
          <>
            <SourcesEditor w={w} bindings={draft} onChange={setDraft} />
            <div className="flex gap-2">
              <button onClick={() => (sim.act({ type: 'bindings', person: person.id, bindings: draft }), setEditing(false))} className={primary}>
                Save
              </button>
              <button onClick={() => setEditing(false)} className={secondary}>
                Cancel
              </button>
            </div>
          </>
        ) : (
          <>
            {person.bindings.length === 0 && <p className="text-neutral-500">None: set only by hand.</p>}
            {counts(w, person).map((c) => (
              <div key={c.b.src} className="flex flex-wrap justify-between gap-x-3">
                <span>{c.s?.name}</span>
                <SourceLine w={w} c={c} />
              </div>
            ))}
          </>
        )}
      </section>
    </Panel>
  )
}

function HomeSheet({ sim, onClose }: { sim: Sim; onClose: () => void }) {
  const { w } = sim
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(w.home)
  const name = (id: string) => w.srcs.find((s) => s.id === id)
  const persons = w.persons.filter((p) => p.value)
  return (
    <Panel
      title={
        <>
          <h2 className="text-lg font-medium">Home</h2>
          <p className="text-sm text-neutral-400">
            {w.homeValue ? 'Someone home' : 'Nobody home'} since {clock(w.homeSince)}
          </p>
        </>
      }
      onClose={onClose}
    >
      {editing ? (
        <>
          <HomeEditor w={w} cfg={draft} onChange={setDraft} />
          <div className="flex gap-2">
            <button onClick={() => (sim.act({ type: 'home', cfg: draft }), setEditing(false))} className={primary}>
              Save
            </button>
            <button onClick={() => setEditing(false)} className={secondary}>
              Cancel
            </button>
          </div>
        </>
      ) : (
        <div className="space-y-3 text-sm">
          <p>
            <span className="text-neutral-400">Persons home: </span>
            {persons.length ? persons.map((p) => p.name).join(', ') : 'none'}
          </p>
          <p>
            <span className="text-neutral-400">Someone else inside: </span>
            {w.box ? (w.emptyAt ? `yes, unless no sign of life by ${clock(w.emptyAt)}` : 'yes, seen since the last door closed') : 'no'}
          </p>
          <div>
            <h3 className={legend}>Entrance doors</h3>
            {w.home.doors.map((id) => (
              <p key={id} className="flex justify-between">
                {name(id)?.name}
                <span className="text-neutral-400">{name(id)?.on ? 'open' : 'closed'}</span>
              </p>
            ))}
          </div>
          <div>
            <h3 className={legend}>Signs of life</h3>
            {w.home.signs.map((id) => (
              <p key={id} className="flex justify-between">
                {name(id)?.name}
                <span className="text-neutral-400">
                  {name(id)?.on ? 'on' : 'off'} · {ago(w.now - (name(id)?.since ?? w.now))}
                </span>
              </p>
            ))}
          </div>
          {w.admin && (
            <button onClick={() => (setDraft(w.home), setEditing(true))} className={secondary}>
              Edit
            </button>
          )}
        </div>
      )}
    </Panel>
  )
}

function VariantC({ sim }: { sim: Sim }) {
  const { w } = sim
  const [open, setOpen] = useState<string | null>(null)
  const person = w.persons.find((p) => p.id === open)
  return (
    <div className="space-y-6">
      <section className="flex flex-wrap items-center gap-x-8 gap-y-4 rounded-2xl border border-neutral-800 bg-linear-to-r from-neutral-800/60 to-transparent px-5 py-4">
        <button onClick={() => setOpen('home')} className="flex items-center gap-3 text-left hover:text-amber-200">
          <span className={`grid size-14 place-items-center rounded-2xl ${w.homeValue ? 'bg-emerald-500/25 text-emerald-300' : 'bg-neutral-800 text-neutral-500'}`}>
            <Svg path={w.homeValue ? mdiHome : mdiHomeOutline} className="size-8" />
          </span>
          <span>
            <span className="block text-xl font-semibold">{w.homeValue ? 'Someone home' : 'Nobody home'}</span>
            <span className="block text-sm text-neutral-400">
              {ago(w.now - w.homeSince)}
              {w.homeValue && !w.persons.some((p) => p.value) && ' · not one of you'}
            </span>
          </span>
        </button>
        <div className="flex flex-wrap gap-4">
          {w.persons.map((p) => (
            <button key={p.id} onClick={() => setOpen(p.id)} className="flex w-16 flex-col items-center gap-1 hover:text-amber-200">
              <Face p={p} />
              <span className="w-full truncate text-center text-sm">{p.name}</span>
              <span className="text-xs text-neutral-500">{p.value === undefined ? '—' : ago(w.now - p.since)}</span>
            </button>
          ))}
          {w.aggregates.map((a) => (
            <div key={a.id} className="flex w-16 flex-col items-center gap-1">
              <span className={`grid size-14 place-items-center rounded-full ${aggValue(w, a) ? 'bg-emerald-500/15 text-emerald-200' : 'bg-neutral-800 text-neutral-500'}`}>
                <Svg path={mdiAccountGroup} className="size-7" />
              </span>
              <span className="text-sm">{a.name}</span>
              <span className="text-xs text-neutral-500">
                {aggHome(w, a)}/{a.members.length}
              </span>
            </div>
          ))}
        </div>
      </section>
      <p className="text-xs text-neutral-500">
        A banner over every Dashboard: a tap on a face opens its sheet (Home / Away, its sources live, its day); an Admin edits the sources right there. The house
        opens the Home presence's sheet.
      </p>
      <PhoneTiles w={w} />
      {open === 'home' && <HomeSheet sim={sim} onClose={() => setOpen(null)} />}
      {person && <PersonSheet key={person.id} sim={sim} person={person} onClose={() => setOpen(null)} />}
    </div>
  )
}

// ---- the simulation's controls and the variant switcher --------------------------------------

function SimBar({ sim }: { sim: Sim }) {
  const { w, act } = sim
  const [folded, setFolded] = useState(false)
  const chip = (on: boolean, label: string, onClick: () => void) => (
    <button key={label} onClick={onClick} className={`rounded-full px-2 py-0.5 ${on ? 'bg-emerald-500/30 text-emerald-100' : 'bg-neutral-800 text-neutral-400'}`}>
      {label}
    </button>
  )
  return (
    <aside className="fixed bottom-4 left-4 z-30 max-w-sm space-y-2 rounded-xl border border-dashed border-fuchsia-500/60 bg-neutral-950/95 p-3 text-xs shadow-2xl">
      <button onClick={() => setFolded(!folded)} className="flex w-full justify-between font-semibold text-fuchsia-300">
        PROTOTYPE · simulation <span>{clock(w.now)}</span>
      </button>
      {!folded && (
        <>
          <div className="flex flex-wrap gap-1">
            {w.srcs.map((s) => chip(s.on, s.name, () => act({ type: 'source', id: s.id })))}
            {chip(w.routerOnline, 'router online', () => act({ type: 'router' }))}
          </div>
          <div className="flex flex-wrap gap-1">
            {chip(false, '+1 min', () => act({ type: 'tick', ms: MIN }))}
            {chip(false, '+5 min', () => act({ type: 'tick', ms: 5 * MIN }))}
            {chip(false, '+15 min', () => act({ type: 'tick', ms: 15 * MIN }))}
            {chip(w.admin, 'as an Admin', () => act({ type: 'admin' }))}
            {chip(false, 'reset', () => act({ type: 'reset' }))}
          </div>
          <p className="text-neutral-500">box {w.box ? `holds someone${w.emptyAt ? `, empty at ${clock(w.emptyAt)}` : ''}` : 'empty'}</p>
        </>
      )}
    </aside>
  )
}

const variants = { A: 'Pills beside the Flags', B: 'A Household Section', C: 'Faces and sheets' } as const
type Key = keyof typeof variants
const keys = Object.keys(variants) as Key[]

function Switcher({ current, onPick }: { current: Key; onPick: (k: Key) => void }) {
  const go = (d: number) => onPick(keys[(keys.indexOf(current) + d + keys.length) % keys.length]!)
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement).closest('input, textarea, select, [contenteditable]')) return
      if (e.key === 'ArrowLeft') go(-1)
      if (e.key === 'ArrowRight') go(1)
    }
    addEventListener('keydown', key)
    return () => removeEventListener('keydown', key)
  })
  return (
    <div className="fixed bottom-4 left-1/2 z-30 flex -translate-x-1/2 items-center gap-2 rounded-full bg-fuchsia-600 px-2 py-1 text-sm font-medium text-white shadow-2xl">
      <button onClick={() => go(-1)} aria-label="Previous variant">
        <Svg path={mdiChevronLeft} className="size-6" />
      </button>
      <span>
        {current} · {variants[current]}
      </span>
      <button onClick={() => go(1)} aria-label="Next variant">
        <Svg path={mdiChevronRight} className="size-6" />
      </button>
    </div>
  )
}

export function PresencePrototype() {
  const [w, act] = useReducer(reduce, undefined, initial)
  const [variant, setVariant] = useState<Key>(() => {
    const v = new URLSearchParams(location.search).get('variant')
    return keys.includes(v as Key) ? (v as Key) : 'A'
  })
  useEffect(() => {
    const t = setInterval(() => act({ type: 'tick', ms: 1000 }), 1000)
    return () => clearInterval(t)
  }, [])
  const pick = (k: Key) => {
    history.replaceState(null, '', `?variant=${k}${location.hash}`)
    setVariant(k)
  }
  const sim = { w, act }
  return (
    <div className="rounded-2xl border border-dashed border-fuchsia-500/40 p-4">
      {variant === 'A' && <VariantA sim={sim} />}
      {variant === 'B' && <VariantB sim={sim} />}
      {variant === 'C' && <VariantC sim={sim} />}
      <SimBar sim={sim} />
      <Switcher current={variant} onPick={pick} />
    </div>
  )
}
