// Who issued a Command or started a Run, as the dashboard tells it (ADR 0031):
// by identity, its Name looked up from /api/names when shown.

import { useEffect, useSyncExternalStore } from 'react'
import type { CommandState, Origin } from './types'

type Kind = 'person' | 'kiosk' | 'program'

// The Names of the Persons, Kiosks and Programs, by kind then id.
export type Names = Partial<Record<Kind, Record<string, string>>>

const kinds: Record<Kind, string> = { person: 'Person', kiosk: 'Kiosk', program: 'Program' }

// The Person, Kiosk or Program o names, if it names one.
function identity(o: Origin | undefined): [Kind, string] | undefined {
  if (!o || o === 'unknown') return undefined
  if ('person' in o) return ['person', o.person]
  if ('kiosk' in o) return ['kiosk', o.kiosk]
  if ('program' in o) return ['program', o.program]
}

// The Run o is, if it is one.
export const runOf = (o: Origin | undefined) => (o && o !== 'unknown' && 'automation' in o ? o : undefined)

// Who o is, as it reads: "by Alice", "by Movie night", "issuer unknown", or
// "by a removed Kiosk" once gone from names. Without names (not loaded, or
// not this observer's to read) only the kind shows: "by a Person".
export function byWhom(o: Origin | undefined, names: Names | null, automation: (id: string) => string): string {
  const run = runOf(o)
  if (run) return `by ${automation(run.automation)}`
  const id = identity(o)
  if (!id) return 'issuer unknown'
  const [kind, key] = id
  if (!names) return `by a ${kinds[kind]}`
  return `by ${names[kind]?.[key] ?? `a removed ${kinds[kind]}`}`
}

// How a Command's marker shows in the History: lost when it failed or timed
// out, else by what issued it: by hand (a Person, a Kiosk, or unknown), a
// Program, or an Automation.
export type CommandKind = 'hand' | 'program' | 'automation' | 'lost'
export function commandKind(c: CommandState): CommandKind {
  if (c.status === 'failed' || c.status === 'timed_out') return 'lost'
  if (runOf(c.origin)) return 'automation'
  return identity(c.origin)?.[0] === 'program' ? 'program' : 'hand'
}

let names: Names | null = null
let loading = false
const listeners = new Set<() => void>()

// useNames is the Names, fetched again each time a component using them
// mounts, so renames show; null until known, or when this observer may not
// read them.
export function useNames() {
  useEffect(() => {
    if (loading) return
    loading = true
    fetch('/api/names')
      .then((r) => (r.ok ? r.json() : null))
      .then(
        (n: Names | null) => {
          names = n
          listeners.forEach((l) => l())
        },
        () => {},
      )
      .finally(() => (loading = false))
  }, [])
  return useSyncExternalStore(
    (l) => (listeners.add(l), () => listeners.delete(l)),
    () => names,
  )
}
