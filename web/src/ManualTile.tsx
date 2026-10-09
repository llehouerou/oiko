// An Automation's Tile, a bar per Manual trigger.

import { useEffect, useRef, useState } from 'react'
import { mdiCheck, mdiClose, mdiPlay } from '@mdi/js'
import { runManual } from './store'
import type { AutomationStatus } from './types'
import { Svg } from './icons'
import { NameLine, statusSize } from './TileParts'

// An Automation's Tile with Manual triggers: a bar for each, named after it. A lone trigger's bar
// is the whole Tile and carries the Automation's name. Only an enabled Automation runs; otherwise
// the tile is dimmed and says why. Its name may be hidden.
export function ManualTile({
  automation,
  hideName = false,
  onResult,
}: {
  automation: AutomationStatus
  hideName?: boolean
  onResult: (r: { text: string; error?: boolean }) => void
}) {
  const off = automation.status !== 'enabled' ? automation.status : null
  const triggers = automation.manualTriggers ?? []
  const lone = triggers.length === 1
  const run = (s: { step: string; name: string }) => async () => {
    const r = await runManual(automation.id, s.step)
    onResult({ ...r, text: `${automation.name} · ${s.name || 'Run'}: ${r.text}` })
    return r
  }
  return (
    <section className={`space-y-1.5 rounded-xl bg-neutral-900 p-1.5 ${off ? 'opacity-50' : ''}`}>
      {lone ? (
        <RunBar
          name={hideName ? null : automation.name}
          status={[hideName ? triggers[0]!.name || 'Run' : 'automation', off].filter(Boolean).join(' · ')}
          disabled={!!off}
          onRun={run(triggers[0]!)}
        />
      ) : (
        <>
          {(!hideName || off) && (
            <div className="px-2 pt-1">
              <NameLine name={automation.name} hidden={hideName} />
              <p className="text-xs text-neutral-400">{[!hideName && 'automation', off].filter(Boolean).join(' · ')}</p>
            </div>
          )}
          {triggers.map((s) => (
            <RunBar key={s.step} name={s.name || 'Run'} disabled={!!off} onRun={run(s)} />
          ))}
        </>
      )}
    </section>
  )
}

// A Manual trigger's bar: a tap runs it; its icon spins while it runs, then says how it went.
function RunBar({
  name,
  status,
  disabled,
  onRun,
}: {
  name: string | null // null: hidden, the status alone and as large
  status?: string
  disabled: boolean
  onRun: () => Promise<{ error?: boolean }>
}) {
  const [phase, setPhase] = useState<'idle' | 'running' | 'ok' | 'error'>('idle')
  const done = useRef<number>(undefined)
  useEffect(() => () => clearTimeout(done.current), [])
  const click = async () => {
    clearTimeout(done.current)
    setPhase('running')
    const r = await onRun()
    setPhase(r.error ? 'error' : 'ok')
    done.current = window.setTimeout(() => setPhase('idle'), 1500)
  }
  const ring = {
    idle: 'bg-sky-500/15 text-sky-300',
    running: 'bg-sky-500/15 text-sky-300',
    ok: 'bg-emerald-500/20 text-emerald-300',
    error: 'bg-red-500/20 text-red-300',
  }[phase]
  return (
    <button
      disabled={disabled || phase === 'running'}
      onClick={click}
      className="flex h-14 w-full items-center gap-3 rounded-lg bg-neutral-800 px-3 text-left transition select-none hover:bg-neutral-700 focus-visible:ring-2 focus-visible:ring-sky-400/60 focus-visible:outline-none active:scale-[.98] active:bg-neutral-600 disabled:cursor-not-allowed disabled:hover:bg-neutral-800 disabled:active:scale-100"
    >
      <span className={`grid size-8 shrink-0 place-items-center rounded-full transition-colors ${ring}`}>
        {phase === 'running' ? (
          <span className="size-4 animate-spin rounded-full border-2 border-sky-300/30 border-t-sky-300" />
        ) : (
          <Svg path={{ idle: mdiPlay, ok: mdiCheck, error: mdiClose }[phase]} className="size-5" />
        )}
      </span>
      <div className="min-w-0 flex-1">
        {name !== null && <NameLine name={name} hidden={false} />}
        {status && <p className={`truncate ${statusSize(name === null)}`}>{status}</p>}
      </div>
    </button>
  )
}
