// Client-side mirror of Home, fed by the SSE stream. Each piece of state has
// its own key, so an Update re-renders only the components reading that key.

import { useCallback, useState, useSyncExternalStore } from 'react'
import type {
  Aggregate,
  Area,
  AutomationStatus,
  Availability,
  CommandState,
  Device,
  Flag,
  Ref,
  ReleaseStatus,
  Releases,
  RunEnd,
  Snapshot,
  Target,
  Update,
  Value,
} from './types'
import { catalogue, owner, type Catalogue } from './targets'
import { api, knowMe } from './access'

type Listener = () => void

let devices: Device[] = []
let aggregates: Aggregate[] = []
let flags: Flag[] = []
let areas: Area[] = []
let automations: AutomationStatus[] = []
let bridges: Record<string, boolean> = {}
let releases: ReleaseStatus[] = []
let connected = false
let now = Date.now()
const values = new Map<string, Value>()
const events = new Map<string, Value>()
const availability = new Map<string, Availability>()
const commands = new Map<string, CommandState>()
const runs = new Map<string, RunEnd[]>() // announced since the page loaded, by Automation, the latest first
const noRuns: RunEnd[] = []
let targets: Catalogue | null = null // built again once devices, aggregates or flags change
const catalogueNow = () => (targets ??= catalogue(devices, aggregates, flags))

const listeners = new Map<string, Set<Listener>>()
const subscribers = new Map<string, (l: Listener) => () => void>()

function subscribeTo(key: string) {
  let subscribe = subscribers.get(key)
  if (!subscribe) {
    subscribe = (l) => {
      let set = listeners.get(key)
      if (!set) listeners.set(key, (set = new Set()))
      set.add(l)
      return () => set.delete(l)
    }
    subscribers.set(key, subscribe)
  }
  return subscribe
}

function notify(key: string) {
  listeners.get(key)?.forEach((l) => l())
}

const refKey = (r: Ref) => `${r.target}|${r.capability}`
const commandKey = (t: Target) => `command|${t}`

function apply(msg: Snapshot | Update) {
  switch (msg.kind) {
    case 'snapshot':
      devices = msg.devices
      aggregates = msg.aggregates
      flags = msg.flags
      areas = msg.areas
      automations = msg.automations
      bridges = msg.bridges
      releases = msg.releases
      values.clear()
      for (const { ref, value } of msg.values) values.set(refKey(ref), value)
      events.clear()
      for (const { ref, value } of msg.events) events.set(refKey(ref), value)
      availability.clear()
      for (const [t, a] of Object.entries(msg.availability)) availability.set(t, a)
      commands.clear()
      targets = null
      listeners.forEach((set) => set.forEach((l) => l()))
      return
    case 'devices':
      devices = msg.devices ?? []
      targets = null
      notify('catalogue')
      return notify('devices')
    case 'aggregates':
      aggregates = msg.aggregates ?? []
      targets = null
      notify('catalogue')
      return notify('aggregates')
    case 'flags':
      flags = msg.flags ?? []
      targets = null
      notify('catalogue')
      return notify('flags')
    case 'areas':
      areas = msg.areas ?? []
      return notify('areas')
    case 'automations':
      automations = msg.automations ?? []
      return notify('automations')
    case 'value':
    case 'refresh': {
      const k = refKey(msg.ref!)
      if (msg.value) values.set(k, msg.value)
      else values.delete(k) // an Aggregate whose members no longer have a Value
      return notify(k)
    }
    case 'event': {
      const k = refKey(msg.ref!)
      events.set(k, msg.value!)
      return notify(`event|${k}`)
    }
    case 'availability': {
      availability.set(msg.target!, msg.availability!)
      return notify(`availability|${msg.target}`)
    }
    case 'bridge':
      bridges = { ...bridges, [msg.bridge!]: msg.bridgeOnline! }
      return notify('connection')
    case 'command': {
      const c = msg.command!
      const k = commandKey(c.target)
      // A newer Command on the same target is announced first; only a Command superseded
      // from elsewhere (an Aggregate's, by a member's) is still the current one.
      if (c.status === 'superseded' && commands.get(k)?.id !== c.id) return
      commands.set(k, c)
      return notify(k)
    }
    case 'run': {
      const r = msg.run!
      runs.set(r.automation, [r, ...(runs.get(r.automation) ?? [])].slice(0, 100))
      return notify(`run|${r.automation}`)
    }
  }
}

const followers: ((msg: Snapshot | Update) => void)[] = []

// follow calls f with each message of the stream, once the store has applied it.
export function follow(f: (msg: Snapshot | Update) => void) {
  followers.push(f)
}

// What the store holds now, read outside React.
export const current = {
  catalogue: catalogueNow,
  value: (r: Ref) => values.get(refKey(r)),
  event: (r: Ref) => events.get(refKey(r)),
  availability: (t: Target) => availability.get(t),
}

// connect follows /api/updates while this browser is signed in; the function it answers stops.
// The browser retries a dropped stream by itself, but gives up on an error answer: a proxy's while
// Oiko restarts, or a refusal once the Session ended. Each error asks who is signed in, so an ended
// Session shows sign-in, which stops the stream; otherwise a new one opens 3 s after giving up.
export function connect() {
  let source: EventSource
  let retry: ReturnType<typeof setTimeout> | undefined
  const open = () => {
    source = new EventSource('/api/updates')
    source.onopen = () => {
      connected = true
      notify('connection')
    }
    source.onerror = () => {
      connected = false
      notify('connection')
      void knowMe()
      if (source.readyState === EventSource.CLOSED) retry = setTimeout(open, 3000)
    }
    source.onmessage = (e) => {
      const msg: Snapshot | Update | Releases = JSON.parse(e.data)
      if (msg.kind === 'releases') {
        releases = msg.releases
        return notify('releases')
      }
      apply(msg)
      followers.forEach((f) => f(msg))
    }
  }
  open()
  const tick = setInterval(() => {
    now = Date.now()
    notify('now')
  }, 10_000)
  return () => {
    source.close()
    clearTimeout(retry)
    clearInterval(tick)
    connected = false
    notify('connection')
  }
}

// A paced send goes out at most every PACE ms per Target, always ending on its latest values.
const PACE = 150
const paces = new Map<Target, { last: number; trailing?: ReturnType<typeof setTimeout> }>()

// sendCommand asks values of target. transition: fade duration in seconds, for devices that
// support it (lights). paced: a control being dragged, whose sends are paced per Target. Any
// send replaces the Target's pending paced one, as a newer Command supersedes it.
export function sendCommand(target: Target, values: Record<string, unknown>, { transition, paced = false }: { transition?: number; paced?: boolean } = {}) {
  const p = paces.get(target) ?? { last: -Infinity }
  paces.set(target, p)
  clearTimeout(p.trailing)
  const send = () => {
    p.last = Date.now()
    p.trailing = undefined
    void post(target, values, transition)
  }
  const wait = paced ? PACE - (Date.now() - p.last) : 0
  if (wait > 0) p.trailing = setTimeout(send, wait)
  else send()
}

async function post(target: Target, values: Record<string, unknown>, transition?: number) {
  const res = await api('/api/commands', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ target, values, transition }),
  })
  if (!res.ok) {
    const k = commandKey(target)
    commands.set(k, { target, id: '', status: 'failed', error: (await res.text()).trim() })
    notify(k)
  }
}

// Lights fade over slightly longer than PACE, so they glide from one value to the next.
const FADE = 0.2

// What a control of Capability key of target shows and sends. value is the one just set until
// the device reports it, or its Command ends otherwise, then the reported one; hold(true) keeps
// the one set while a drag goes on. set sends paced, fading a light (fade).
export function useControl<T>(target: Target, key: string, { fade = false } = {}) {
  const reported = useValue({ target, capability: key })?.data as T | undefined
  const command = useCommand(target)
  const [picked, setPicked] = useState<{ v: T } | null>(null)
  const [held, hold] = useState(false)
  const ended = command?.status === 'timed_out' || command?.status === 'failed' || command?.status === 'superseded'
  if (picked && !held && (JSON.stringify(reported) === JSON.stringify(picked.v) || ended)) setPicked(null)
  const set = (v: T) => {
    setPicked({ v })
    sendCommand(target, { [key]: v }, { paced: true, transition: fade ? FADE : undefined })
  }
  return { value: picked ? picked.v : reported, set, picking: picked !== null, hold }
}

// Changes Oiko's configuration: path is under /api, e.g. devices/{id}/replace, aggregates or flags/{id}.
// The change itself arrives through the stream; this only reports a refusal.
export async function edit(method: 'PATCH' | 'PUT' | 'DELETE' | 'POST', path: string, body?: unknown) {
  const res = await api(`/api/${path}`, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  return res.ok ? null : (await res.text()).trim()
}

// Runs Automation automation from its Manual trigger step, at once. Says what
// the Run did, or why it could not run; run is the Run's id, to see its Trace.
export async function runManual(automation: string, step: string): Promise<{ text: string; error?: boolean; run?: string }> {
  const res = await api(`/api/automations/${automation}/steps/${step}/run`, { method: 'POST' })
  if (!res.ok) return { text: (await res.text()).trim(), error: true }
  const end: RunEnd = await res.json()
  const did = { acted: `${end.commands} command${end.commands > 1 ? 's' : ''}`, nothing: 'nothing to do', error: 'error, see its Trace' }
  return { text: did[end.outcome], error: end.outcome === 'error', run: end.run }
}

export const useDevices = () => useSyncExternalStore(subscribeTo('devices'), () => devices)

export const useAggregates = () => useSyncExternalStore(subscribeTo('aggregates'), () => aggregates)

export const useFlags = () => useSyncExternalStore(subscribeTo('flags'), () => flags)

export const useAreas = () => useSyncExternalStore(subscribeTo('areas'), () => areas)

// Every Target of the home, resolved (targets.ts).
export const useCatalogue = () => useSyncExternalStore(subscribeTo('catalogue'), catalogueNow)

export const useAutomationStatuses = () => useSyncExternalStore(subscribeTo('automations'), () => automations)

export function useValue(ref: Ref) {
  const k = refKey(ref)
  return useSyncExternalStore(subscribeTo(k), () => values.get(k))
}

// How many of refs are on, followed as each changes.
export function useOnCount(refs: Ref[]) {
  const keys = refs.map(refKey).join('\n')
  const subscribe = useCallback(
    (l: Listener) => {
      const offs = keys ? keys.split('\n').map((k) => subscribeTo(k)(l)) : []
      return () => offs.forEach((off) => off())
    },
    [keys],
  )
  return useSyncExternalStore(subscribe, () => (keys ? keys.split('\n').filter((k) => values.get(k)?.data === true).length : 0))
}

export function useLastEvent(ref: Ref) {
  const k = refKey(ref)
  return useSyncExternalStore(subscribeTo(`event|${k}`), () => events.get(k))
}

// Of a Device, an Aggregate or a Flag; a Function's is its Device's.
export function useAvailability(target: Target) {
  const t = owner(target)
  return useSyncExternalStore(subscribeTo(`availability|${t}`), () => availability.get(t) ?? 'unknown')
}

export function useCommand(target: Target) {
  const k = commandKey(target)
  return useSyncExternalStore(subscribeTo(k), () => commands.get(k))
}

// The Runs of an Automation announced since the page loaded, the latest first.
export const useLiveRuns = (automation: string) => useSyncExternalStore(subscribeTo(`run|${automation}`), () => runs.get(automation) ?? noRuns)

// Deletes Runs of an Automation from the history, or all of them without ids,
// and forgets those announced live. It reports a refusal, if any.
export async function deleteRuns(automation: string, ids?: string[]) {
  const errors = await Promise.all(ids ? ids.map((id) => edit('DELETE', `runs/${id}`)) : [edit('DELETE', `automations/${automation}/runs`)])
  const live = runs.get(automation)
  if (live) runs.set(automation, ids ? live.filter((r) => !ids.includes(r.run)) : [])
  notify(`run|${automation}`)
  return errors.find((e) => e !== null) ?? null
}

// Whether each Bridge is online, by name.
export const useBridges = () => useSyncExternalStore(subscribeTo('connection'), () => bridges)

// What the module proxy lists of each module built into Oiko, Oiko's first.
export const useReleases = () => useSyncExternalStore(subscribeTo('releases'), () => releases)

// 'disconnected', 'online', or the names of the Bridges offline.
export const useConnection = () =>
  useSyncExternalStore(subscribeTo('connection'), () => {
    if (!connected) return 'disconnected'
    const offline = Object.keys(bridges).filter((name) => !bridges[name])
    return offline.length ? offline.sort().join(', ') : 'online'
  })

export const useNow = () => useSyncExternalStore(subscribeTo('now'), () => now)
