// Who is signed in to this browser, as /api/me tells, signing in and out, and what the page shows
// for it.

import { useSyncExternalStore } from 'react'

export type Level = 'guest' | 'member' | 'admin'

export const levels: Record<Level, string> = { guest: 'Guest', member: 'Member', admin: 'Admin' }

export type Me = {
  identity: { person: string } | { program: string } | null // null: not signed in
  name?: string
  level?: Level
  fresh: boolean // signed in lately enough for step-up
  claimed: boolean // Oiko has an Admin
  publicUrl: string | null
}

let me: Me | null = null // null until known
const listeners = new Set<() => void>()

// Asks who is signed in; false if Oiko did not say.
export async function loadMe() {
  try {
    const res = await fetch('/api/me')
    if (!res.ok) return false
    me = await res.json()
  } catch {
    return false
  }
  listeners.forEach((l) => l())
  return true
}

// Asks who is signed in until Oiko says, once at a time.
let asking = false
export async function knowMe() {
  if (asking) return
  asking = true
  while (!(await loadMe())) await new Promise((r) => setTimeout(r, 3000))
  asking = false
}

// Who is signed in, as last known.
export const meNow = () => me

export const useMe = () => useSyncExternalStore((l) => (listeners.add(l), () => listeners.delete(l)), meNow)

const ranks: Record<Level, number> = { guest: 0, member: 1, admin: 2 }

// Whether whoever me is does what level does (ADR 0023): a Guest observes and commands the home as it
// is now, a Member also reads its past and how its Automations are built, an Admin also edits it and
// sees what Oiko is built from. Oiko refuses the rest; the page only hides it.
export const allows = (me: Me | null, level: Level) => !!me?.level && ranks[me.level] >= ranks[level]

export const useAllows = (level: Level) => allows(useMe(), level)

// api is fetch for Oiko's API. A refusal for want of credentials means this browser's Session
// ended: who is signed in is asked again, which shows sign-in.
export async function api(path: string, init?: RequestInit) {
  const res = await fetch(path, init)
  if (res.status === 401) void knowMe()
  return res
}

// Claims a fresh Oiko with its Setup link's secret, as its first Admin, named name. Says why not, if refused.
export async function claim(secret: string, name: string) {
  const res = await fetch('/api/setup', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ secret, name }),
  })
  if (!res.ok) return (await res.text()).trim()
  await loadMe()
  return null
}

export async function signOut() {
  await api('/api/sign-out', { method: 'POST' })
  await loadMe()
}

export type View = 'home' | 'history' | 'automations' | 'programs' | 'account'

// What the page shows: nothing until it knows who is signed in, sign-in while nobody is, or else the
// view hash names, if their Access level shows it, the dashboard otherwise. Sign-in leaves the hash
// alone, so signing in again returns to the same view.
export function screen(me: Me | null, hash: string): View | 'sign-in' | null {
  if (!me) return null
  if (!me.identity) return 'sign-in'
  const person = 'person' in me.identity
  if (hash.startsWith('#programs') && person && allows(me, 'admin')) return 'programs'
  if (hash.startsWith('#account') && person) return 'account'
  if (hash.startsWith('#automations') && allows(me, 'member')) return 'automations'
  if (hash.startsWith('#history') && allows(me, 'member')) return 'history'
  return 'home'
}

// Which sign-in page a browser at origin gets (ADR 0027): while Oiko has no Admin, a pointer to its
// Setup link; on the Public URL or http://localhost, sign-in itself; elsewhere a pointer to the
// Public URL, or the need for one.
export function signInPage(me: Me, origin: string): 'unclaimed' | 'here' | 'elsewhere' | 'no-public-url' {
  if (!me.claimed) return 'unclaimed'
  const u = new URL(origin)
  if (origin === me.publicUrl || (u.protocol === 'http:' && u.hostname === 'localhost')) return 'here'
  return me.publicUrl ? 'elsewhere' : 'no-public-url'
}
