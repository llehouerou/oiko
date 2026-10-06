// The Automation at runtime, beside its graph: the Runs drawer, a picked
// Run's evidence on each Step, each Step's current state, and the Command
// history of a Command Step's targets.

import { createContext, useContext, useEffect, useState } from 'react'
import { deleteRuns, useCatalogue, useCommand, useLiveRuns, useNow } from '../store'
import { confirm } from '../confirm'
import type { CommandRecord, RunEnd, StepState, Target, Trace } from '../types'
import type { StepKind } from './kinds'
import type { Params } from './model'
import { clock, evidence, mergeCommands, mergeRuns, runSummary, type Path } from './runtime'
import { titleIfTruncated } from '../truncated'
import { byWhom, runOf, useNames } from '../origin'

export interface Inspection {
  trace?: Trace // of the picked Run
  path?: Path // of the picked Run
  states: StepState[] // what the Steps remember now
  openRun: (automation: string, run: string) => void // picks a Run, switching Automation if need be
  automationName: (id: string) => string
}

export const Inspect = createContext<Inspection>({ states: [], openRun: () => {}, automationName: (id) => id })

const outcomeColor = { acted: 'text-emerald-400', nothing: 'text-neutral-400', error: 'text-red-400' }

export function RunsDrawer({
  automation,
  picked,
  onPick,
  stepName,
}: {
  automation: string
  picked: string | null
  onPick: (run: string | null) => void
  stepName: (id: string) => string
}) {
  const live = useLiveRuns(automation)
  const targets = useCatalogue()
  const names = useNames()
  const now = useNow() // Commands lose entries too: the counter follows the clock, not only Runs
  const [fetched, setFetched] = useState<RunEnd[]>([])
  const [lost, setLost] = useState(0)
  useEffect(() => {
    fetch(`/api/automations/${automation}/runs`)
      .then((r) => (r.ok ? r.json() : []))
      .then(setFetched, () => {})
  }, [automation])
  useEffect(() => {
    fetch('/api/lost-entries')
      .then((r) => (r.ok ? r.json() : { lost: 0 }))
      .then(
        (b: { lost: number }) => setLost(b.lost),
        () => {},
      )
  }, [live, now])
  const runs = mergeRuns(live, fetched)
  const [selected, setSelected] = useState<Set<string> | null>(null) // null: not selecting
  const [error, setError] = useState<string | null>(null)
  const shown = runs.filter((r) => selected?.has(r.run))
  const all = runs.length > 0 && shown.length === runs.length
  const toggle = (run: string) => setSelected((s) => s && new Set(s.has(run) ? [...s].filter((id) => id !== run) : [...s, run]))
  // Deletes the Runs ids, or all of them without ids.
  const remove = async (ids?: string[]) => {
    setError(await deleteRuns(automation, ids))
    setFetched((f) => (ids ? f.filter((r) => !ids.includes(r.run)) : []))
    setSelected(null)
    if (picked && (!ids || ids.includes(picked))) onPick(null)
  }
  const action = 'text-neutral-400 hover:text-white'
  const [hidden, setHidden] = useState(() => localStorage.getItem('oiko.runsHidden') === '1')
  const hide = (h: boolean) => {
    localStorage.setItem('oiko.runsHidden', h ? '1' : '0')
    setHidden(h)
  }
  // Hidden, the Runs leave a strip where they sit: beside the canvas on a wide screen, below it otherwise.
  if (hidden) {
    return (
      <button
        onClick={() => hide(false)}
        title="Show Runs"
        className="flex shrink-0 items-center gap-2 border-t border-neutral-800 px-3 py-1 text-xs text-neutral-500 hover:text-white 2xl:flex-col 2xl:border-t-0 2xl:border-l 2xl:px-1.5 2xl:py-3"
      >
        <PanelIcon show />
        <span className="font-semibold tracking-wide uppercase 2xl:[writing-mode:vertical-rl]">Runs</span>
      </button>
    )
  }
  return (
    <div className="flex h-48 shrink-0 flex-col border-t border-neutral-800 2xl:h-auto 2xl:w-96 2xl:border-t-0 2xl:border-l">
      <div className="flex flex-wrap items-center gap-x-3 px-3 py-1 text-xs">
        {selected && (
          <input
            type="checkbox"
            aria-label="Select all Runs"
            checked={all}
            onChange={() => setSelected(new Set(all ? [] : runs.map((r) => r.run)))}
            className="accent-amber-400"
          />
        )}
        <button onClick={() => hide(true)} title="Hide Runs" className={action}>
          <PanelIcon />
        </button>
        <span className="font-semibold tracking-wide text-neutral-500 uppercase">Runs</span>
        {picked && (
          <button onClick={() => onPick(null)} className={action}>
            exit replay
          </button>
        )}
        {selected ? (
          <>
            {shown.length > 0 && (
              <button
                onClick={async () => (await confirm(`Delete ${shown.length} Run${shown.length > 1 ? 's' : ''}?`, 'Delete')) && remove(shown.map((r) => r.run))}
                className="text-red-400 hover:text-red-300"
              >
                delete {shown.length} selected
              </button>
            )}
            <button onClick={() => setSelected(null)} className={action}>
              cancel
            </button>
          </>
        ) : (
          runs.length > 0 && (
            <>
              <button onClick={() => setSelected(new Set())} className={action}>
                select
              </button>
              <button onClick={async () => (await confirm('Delete every Run of this Automation?', 'Delete all')) && remove()} className={action}>
                clear history
              </button>
            </>
          )
        )}
        {error && <span className="text-red-400">{error}</span>}
        {lost > 0 && <span className="ml-auto text-orange-400">⚠ {lost} Trace or Command entries lost</span>}
      </div>
      <ul className="min-h-0 flex-1 overflow-y-auto px-1 text-xs">
        {runs.length === 0 && <li className="px-2 text-neutral-500">No Run yet.</li>}
        {runs.map((r) => {
          const { trigger, result } = runSummary(r, targets, stepName)
          return (
            <li key={r.run} className={`group flex items-center gap-1 rounded pl-2 ${r.run === picked ? 'bg-neutral-800' : 'hover:bg-neutral-900'}`}>
              {selected && (
                <input type="checkbox" aria-label="Select Run" checked={selected.has(r.run)} onChange={() => toggle(r.run)} className="accent-amber-400" />
              )}
              <button
                onClick={() => (selected ? toggle(r.run) : onPick(r.run === picked ? null : r.run))}
                className="flex min-w-0 flex-1 items-center gap-3 px-1 py-0.5 text-left"
              >
                {/* Today's times line up; an older Run's date widens its own row only. */}
                <span className="min-w-14 shrink-0 whitespace-nowrap text-neutral-400 tabular-nums">{clock(r.time, new Date(now), true)}</span>
                <span className="truncate" onMouseEnter={titleIfTruncated}>
                  {trigger}
                  {r.trigger.by && `, ${byWhom(r.trigger.by, names, (id) => id)}`}
                </span>
                {r.trigger.catchUp && <span className="shrink-0 rounded bg-sky-900 px-1 text-sky-200">catch-up</span>}
                <span className={`shrink-0 ${outcomeColor[r.outcome]}`}>→ {result}</span>
              </button>
              {!selected && (
                <button
                  onClick={() => remove([r.run])}
                  aria-label="Delete Run"
                  title="Delete Run"
                  className="invisible px-2 text-neutral-500 group-hover:visible hover:text-red-400"
                >
                  ✕
                </button>
              )}
            </li>
          )
        })}
      </ul>
    </div>
  )
}

// Lucide's panel-bottom / panel-right icons, after where the Runs sit: the
// panel docked on that side, with a chevron to hide it or, with show, bring it back.
function PanelIcon({ show }: { show?: boolean }) {
  const svg = { viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 2, strokeLinecap: 'round', strokeLinejoin: 'round' } as const
  return (
    <>
      <svg {...svg} className="size-4 2xl:hidden" aria-hidden>
        <rect x="3" y="3" width="18" height="18" rx="2" />
        <path d="M3 15h18" />
        <path d={show ? 'm9 10 3-3 3 3' : 'm15 8-3 3-3-3'} />
      </svg>
      <svg {...svg} className="hidden size-4 2xl:block" aria-hidden>
        <rect x="3" y="3" width="18" height="18" rx="2" />
        <path d="M15 3v18" />
        <path d={show ? 'm10 15-3-3 3-3' : 'm8 9 3 3-3 3'} />
      </svg>
    </>
  )
}

// What Step id remembers now, read-only, for its header.
export function StateBadge({ id, params, badge }: { id: string; params: Params; badge: StepKind['badge'] }) {
  const s = useContext(Inspect).states.find((x) => x.step === id)
  const now = useNow()
  const text = s && badge?.(s, params, new Date(now))
  if (!text) return null
  return <span className="shrink-0 rounded bg-amber-950 px-1.5 text-[10px] whitespace-nowrap text-amber-200">{text}</span>
}

// The evidence of the picked Run on Step id, if it reached it.
export function Evidence({ id, own }: { id: string; own: StepKind['evidence'] }) {
  const { trace, automationName } = useContext(Inspect)
  const targets = useCatalogue()
  const names = useNames()
  const by = trace?.trigger.by && byWhom(trace.trigger.by, names, automationName)
  const lines = trace?.steps.filter((r) => r.step === id).flatMap((r) => evidence(r, trace, targets, own?.(r), by)) ?? []
  if (!lines.length) return null
  return (
    <ul className="space-y-0.5 border-t border-amber-900 bg-amber-950/40 px-3 py-1.5 font-mono text-[10px] break-all whitespace-pre-wrap text-amber-100">
      {lines.map((l, i) => (
        <li key={i}>{l}</li>
      ))}
    </ul>
  )
}

const statusColor = {
  pending: 'text-sky-300',
  confirmed: 'text-emerald-400',
  failed: 'text-red-400',
  timed_out: 'text-red-400',
  superseded: 'text-neutral-500',
}

// The latest Commands on a Command Step's target, with their status and
// origin; a Run origin opens that Run.
export function CommandHistory({ target }: { target: Target }) {
  const { openRun, automationName } = useContext(Inspect)
  const names = useNames()
  const live = useCommand(target)
  const [kept, setKept] = useState<CommandRecord[]>([])
  useEffect(() => {
    if (!target) return
    let stale = false
    fetch(`/api/commands?target=${encodeURIComponent(target)}`)
      .then((r) => (r.ok ? r.json() : []))
      .then(
        (cs: CommandRecord[]) => stale || setKept(cs),
        () => {},
      )
    return () => {
      stale = true
    }
  }, [target, live?.id, live?.status])
  const commands = mergeCommands(kept, live).slice(0, 5)
  if (!target || !commands.length) return null
  return (
    <details className="w-full text-[10px]">
      <summary className="cursor-pointer text-neutral-500">recent commands</summary>
      <ul>
        {commands.map(({ id, time, status, origin }) => {
          const run = runOf(origin)
          return (
            <li key={id} className="flex gap-2">
              <span className="w-24 shrink-0 text-neutral-500">{time ? clock(time) : 'now'}</span>
              <span className={`w-16 shrink-0 ${statusColor[status]}`}>{status}</span>
              {run ? (
                <button
                  onClick={() => openRun(run.automation, run.run)}
                  onMouseEnter={titleIfTruncated}
                  className="truncate text-amber-400 hover:text-amber-300"
                >
                  {automationName(run.automation)} ›
                </button>
              ) : (
                <span onMouseEnter={titleIfTruncated} className="truncate text-neutral-400">
                  {byWhom(origin, names, automationName)}
                </span>
              )}
            </li>
          )
        })}
      </ul>
    </details>
  )
}
