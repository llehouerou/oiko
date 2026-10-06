// Who is signed in to this browser, as /api/me tells, and signing in and out.

import { useSyncExternalStore } from 'react'

export type Level = 'guest' | 'member' | 'admin'

export type Me = {
  identity: { person: string } | null // null: not signed in
  name?: string
  level?: Level
  fresh: boolean // signed in lately enough for step-up
  claimed: boolean // Oiko has an Admin
  publicUrl: string | null
}

let me: Me | null = null // null until known
const listeners = new Set<() => void>()

export async function loadMe() {
  const res = await fetch('/api/me')
  if (!res.ok) return
  me = await res.json()
  listeners.forEach((l) => l())
}

export const useMe = () =>
  useSyncExternalStore(
    (l) => (listeners.add(l), () => listeners.delete(l)),
    () => me,
  )

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
  await fetch('/api/sign-out', { method: 'POST' })
  await loadMe()
}
