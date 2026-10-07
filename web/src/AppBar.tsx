// Oiko's chrome: a bar on top of every page (Oiko, opening what it is built from, its pages, the
// connection, who is signed in, the page's settings),
// and on the dashboard a floating + that creates an Area, an Aggregate or a Flag.

import { useRef, useState, type ReactNode } from 'react'
import {
  mdiAccountCircleOutline,
  mdiAccountMultipleOutline,
  mdiApi,
  mdiChartLine,
  mdiClipboardTextClockOutline,
  mdiClose,
  mdiCogOutline,
  mdiFlagPlusOutline,
  mdiGroup,
  mdiHomePlusOutline,
  mdiKeyVariant,
  mdiLogout,
  mdiPlus,
  mdiRobotOutline,
  mdiTabletDashboard,
  mdiTimelineClockOutline,
  mdiViewDashboardEditOutline,
  mdiViewDashboardOutline,
} from '@mdi/js'
import { Switch } from './controls'
import { Svg } from './icons'
import { useConnection } from './store'
import { About } from './About'
import { Logo } from './Logo'
import { allows, levels, signOut, useMe } from './access'

const pages = [
  { href: '#', label: 'Home', icon: mdiViewDashboardOutline, level: 'guest' },
  { href: '#history', label: 'Timeline', icon: mdiTimelineClockOutline, level: 'member' },
  { href: '#automations', label: 'Automations', icon: mdiRobotOutline, level: 'member' },
] as const

export type Page = (typeof pages)[number]['href']

// The pages its Access level shows as tabs in the middle, the current one lit, if it is one; a narrow
// screen keeps their icons only. home takes the Home tab's place: the Dashboard menu. onLeave may
// hold the page, e.g. on unsaved changes; settings are the page's, under ⚙. Only an Admin opens
// what Oiko is built from.
export function AppBar({ page, home, onLeave, settings }: { page?: Page; home?: ReactNode; onLeave?: () => Promise<boolean>; settings?: ReactNode }) {
  const [about, setAbout] = useState(false) // what Oiko is built from, opened from its logo
  const me = useMe()
  const admin = allows(me, 'admin')
  return (
    <header className="sticky top-0 z-10 shrink-0 border-b border-neutral-800/80 bg-neutral-950/80 backdrop-blur">
      <div className="mx-auto flex h-16 max-w-[120rem] items-center gap-4 px-4 lg:px-8">
        <div className="flex flex-1 items-center">
          <button onClick={() => setAbout(true)} disabled={!admin} title={admin ? 'About Oiko' : undefined} className="flex items-center gap-3 rounded-xl">
            <Logo />
          </button>
          {about && <About onClose={() => setAbout(false)} />}
        </div>
        <nav className="flex rounded-full bg-neutral-900 p-1">
          {pages
            .filter((p) => allows(me, p.level))
            .map((p) =>
              p.href === '#' && home ? (
                <div key={p.href}>{home}</div>
              ) : (
                <a
                  key={p.href}
                  href={p.href}
                  title={p.label}
                  aria-current={p.href === page ? 'page' : undefined}
                  onClick={async (e) => {
                    if (!onLeave || p.href === page) return
                    e.preventDefault()
                    if (await onLeave()) location.hash = p.href
                  }}
                  className={`flex items-center gap-2 rounded-full px-3 py-1.5 text-sm sm:px-4 ${p.href === page ? 'bg-neutral-700 text-white shadow' : 'text-neutral-400 hover:text-white'}`}
                >
                  <Svg path={p.icon} className="size-5" />
                  <span className="hidden md:inline">{p.label}</span>
                </a>
              ),
            )}
        </nav>
        <div className="flex flex-1 items-center justify-end gap-2">
          <ConnectionBadge />
          <Account />
          {settings && <Settings>{settings}</Settings>}
        </div>
      </div>
    </header>
  )
}

// Whether the page follows Oiko live: a lone green dot while it does, the default; otherwise what is
// wrong, which a narrow screen keeps to its dot.
function ConnectionBadge() {
  const state = useConnection()
  if (state === 'online') return <span title="Live" aria-label="Live" className="mx-1 size-2 rounded-full bg-emerald-400" />
  const [label, color] = state === 'disconnected' ? ['Disconnected', 'bg-red-500'] : [`${state} offline`, 'bg-amber-400']
  return (
    <span title={label} className="flex items-center gap-2 rounded-full bg-neutral-900 px-3 py-1.5 text-xs text-neutral-300">
      <span className={`size-2 rounded-full ${color}`} />
      <span className="hidden sm:inline">{label}</span>
    </span>
  )
}

// Who is signed in, with a menu to their own page, an Admin's pages (Persons, Kiosks, Programs, the Audit
// log), and signing out, which a Kiosk never does (ADR 0029).
function Account() {
  const me = useMe()
  if (!me?.identity) return null
  return (
    <>
      <button
        popoverTarget="account"
        title={me.name}
        className="flex items-center gap-2 rounded-full px-2 py-1.5 text-sm text-neutral-300 hover:bg-neutral-800 hover:text-white"
      >
        <Svg path={mdiAccountCircleOutline} className="size-5" />
        <span className="hidden max-w-32 truncate md:inline">{me.name}</span>
      </button>
      <div
        id="account"
        popover="auto"
        className="inset-auto top-16 right-4 m-0 min-w-60 rounded-xl border border-neutral-800 bg-neutral-900 p-1.5 text-sm text-neutral-100 shadow-2xl"
      >
        <p className="truncate px-2.5 pt-2 font-medium">{me.name}</p>
        <p className="px-2.5 pb-2 text-xs text-neutral-400">{me.level && levels[me.level]}</p>
        {'person' in me.identity && (
          <a
            href="#account"
            onClick={() => document.getElementById('account')?.hidePopover()}
            className="flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left hover:bg-neutral-800"
          >
            <Svg path={mdiKeyVariant} className="size-5 text-neutral-400" />
            Your account
          </a>
        )}
        {me.level === 'admin' && 'person' in me.identity && (
          <a
            href="#persons"
            onClick={() => document.getElementById('account')?.hidePopover()}
            className="flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left hover:bg-neutral-800"
          >
            <Svg path={mdiAccountMultipleOutline} className="size-5 text-neutral-400" />
            Persons
          </a>
        )}
        {me.level === 'admin' && 'person' in me.identity && (
          <a
            href="#kiosks"
            onClick={() => document.getElementById('account')?.hidePopover()}
            className="flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left hover:bg-neutral-800"
          >
            <Svg path={mdiTabletDashboard} className="size-5 text-neutral-400" />
            Kiosks
          </a>
        )}
        {me.level === 'admin' && 'person' in me.identity && (
          <a
            href="#programs"
            onClick={() => document.getElementById('account')?.hidePopover()}
            className="flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left hover:bg-neutral-800"
          >
            <Svg path={mdiApi} className="size-5 text-neutral-400" />
            Programs
          </a>
        )}
        {me.level === 'admin' && 'person' in me.identity && (
          <a
            href="#audit"
            onClick={() => document.getElementById('account')?.hidePopover()}
            className="flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left hover:bg-neutral-800"
          >
            <Svg path={mdiClipboardTextClockOutline} className="size-5 text-neutral-400" />
            Audit log
          </a>
        )}
        {!('kiosk' in me.identity) && (
          <button
            popoverTarget="account"
            popoverTargetAction="hide"
            onClick={() => void signOut()}
            className="flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left hover:bg-neutral-800"
          >
            <Svg path={mdiLogout} className="size-5 text-neutral-400" />
            Sign out
          </button>
        )}
      </div>
    </>
  )
}

// A page's settings, in a menu under ⚙.
function Settings({ children }: { children: ReactNode }) {
  const button = useRef<HTMLButtonElement>(null)
  return (
    <>
      <button
        ref={button}
        popoverTarget="settings"
        aria-label="Settings"
        title="Settings"
        className="grid size-9 place-items-center rounded-full text-neutral-400 hover:bg-neutral-800 hover:text-white"
      >
        <Svg path={mdiCogOutline} className="size-5" />
      </button>
      <div
        id="settings"
        popover="auto"
        // A popover sits in the top layer, out of the bar: it opens under the button, flush right.
        onBeforeToggle={(e) => {
          const r = button.current?.getBoundingClientRect()
          if (e.newState !== 'open' || !r) return
          e.currentTarget.style.top = `${r.bottom + 8}px`
          e.currentTarget.style.right = `${document.documentElement.clientWidth - r.right}px`
        }}
        className="inset-auto m-0 min-w-60 rounded-xl border border-neutral-800 bg-neutral-900 p-1.5 text-neutral-100 shadow-2xl"
      >
        {children}
      </div>
    </>
  )
}

// The dashboard's setting: whether its tiles show their 24 h charts, as this browser remembers.
export function ChartsSetting({ charts, onCharts }: { charts: boolean; onCharts: () => void }) {
  return (
    <label className="flex items-center gap-3 rounded-lg px-2.5 py-2 text-sm">
      <Svg path={mdiChartLine} className="size-5 text-neutral-400" />
      <span className="flex-1">Charts on tiles</span>
      <Switch on={charts} onChange={onCharts} label="Charts on tiles" small />
    </label>
  )
}

// The dashboard's entry into arranging its Areas and their tiles; it closes the menu.
export function ArrangeSetting({ onArrange }: { onArrange: () => void }) {
  return (
    <button
      popoverTarget="settings"
      popoverTargetAction="hide"
      onClick={onArrange}
      className="flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left text-sm hover:bg-neutral-800"
    >
      <Svg path={mdiViewDashboardEditOutline} className="size-5 text-neutral-400" />
      Arrange dashboard
    </button>
  )
}

const creations = [
  { id: 'new-area', label: 'New area', icon: mdiHomePlusOutline },
  { id: 'new-aggregate', label: 'New aggregate', icon: mdiGroup },
  { id: 'new-flag', label: 'New flag', icon: mdiFlagPlusOutline },
] as const

export type Creation = (typeof creations)[number]['id']

// The floating +: what can be created unfolds above it, over the dimmed page, and ✕ folds it back.
export function CreateButton({ onCreate }: { onCreate: (what: Creation) => void }) {
  const menu = useRef<HTMLDivElement>(null)
  const round = 'grid place-items-center shadow-xl'
  const fab = `${round} size-14 rounded-2xl bg-amber-400 text-neutral-950 shadow-amber-500/20 hover:bg-amber-300`
  return (
    <>
      <button popoverTarget="create" aria-label="Create" className={`fixed right-6 bottom-6 z-10 ${fab}`}>
        <Svg path={mdiPlus} className="size-7" />
      </button>
      <div
        id="create"
        ref={menu}
        popover="auto"
        className="inset-auto right-6 bottom-6 m-0 flex-col items-end gap-3 overflow-visible bg-transparent p-0 text-neutral-100 backdrop:bg-black/40 open:flex"
      >
        {creations.map((c) => (
          <button key={c.id} onClick={() => (menu.current?.hidePopover(), onCreate(c.id))} className="group flex items-center gap-3">
            <span className="rounded-lg bg-neutral-900 px-3 py-1.5 text-sm shadow-lg">{c.label}</span>
            <span className={`${round} size-11 rounded-full bg-neutral-800 text-amber-300 group-hover:bg-neutral-700`}>
              <Svg path={c.icon} className="size-5" />
            </span>
          </button>
        ))}
        <button popoverTarget="create" popoverTargetAction="hide" aria-label="Close" className={fab}>
          <Svg path={mdiClose} className="size-7" />
        </button>
      </div>
    </>
  )
}
