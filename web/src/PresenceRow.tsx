// The Presence row (ADR 0055): a chip per Person at the top of a Dashboard, each opening the
// Person's sheet, where Home / Away sets their Presence by hand (ADR 0052).

import { useEffect, useState } from 'react'
import { mdiHandBackRightOutline } from '@mdi/js'
import { sendCommand, useCommand, useNow, usePresences, useValue } from './store'
import { useAllows } from './access'
import { DAY, HistoryView, key, useHistory } from './history'
import { chip, ordered, stretches, type PresenceState } from './presence'
import { presenceTarget } from './targets'
import { CommandNote } from './Capabilities'
import { Panel } from './Panel'
import { Svg } from './icons'
import type { Presence } from './types'

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

const clock = (t: string) => new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })

// A Person's sheet: the Value and since when, Home / Away, a note while set by hand, and to a
// Member, who reads the past, the last 24 h with the stretches set by hand hatched.
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
          Set by hand: {presence.name} stays {said[state].toLowerCase()} until set again.
        </p>
      )}
      {member && <Day target={target} />}
    </Panel>
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
