// The Audit log as the dashboard reads it (ADR 0033): each entry told in a sentence, its identities as
// they were named then, with their current Name beside when it changed.

import { levels, type Level } from './access'
import { day } from './manage'

export type Party = { kind: 'person' | 'kiosk' | 'program' | 'host' | 'oiko' | 'unknown'; id?: string; name?: string; current?: string }

export type Entry = {
  id: number
  time: string
  event: string
  actor: Party
  subject: Party
  browser?: string
  detail?: Record<string, unknown>
}

const kinds: Record<string, string> = { person: 'Person', kiosk: 'Kiosk', program: 'Program' }

// Who p is: their Name then, and now if it changed; "a removed Person" once gone unnamed.
export function who(p: Party | undefined): string {
  if (!p || p.kind === 'unknown') return 'Someone unknown'
  if (p.kind === 'oiko') return 'Oiko'
  if (p.kind === 'host') return "Oiko's host"
  const name = p.name ?? `a removed ${kinds[p.kind]}`
  return p.current ? `${name} (now ${p.current})` : name
}

const level = (l: unknown) => levels[l as Level] ?? String(l)

const sentences: Record<string, (e: Entry, a: string, s: string) => string> = {
  'setup-refused': () => 'A Setup link was refused',
  'link-refused': () => 'A Sign-in link was refused',
  'passkey-refused': (e, _, s) => (e.subject.kind === 'unknown' ? 'An unknown Passkey was refused' : `A Passkey for ${s} was refused`),
  'token-refused': () => 'A Token was refused',
  'step-up-refused': (_, a) => `${a} did not confirm a Passkey`,
  'signed-in': (e, _, s) => {
    const by = e.detail?.by as Party | undefined
    const how: Record<string, string> = {
      setup: 'with the Setup link',
      passkey: 'with a Passkey',
      link: by ? `with a Sign-in link from ${who(by)}` : 'with a Sign-in link',
    }
    return `${s} signed in ${how[String(e.detail?.method)] ?? ''}`.trim()
  },
  'session-ended': (e, a, s) => {
    const reasons: Record<string, string> = {
      'signed out': `${s} signed out`,
      expired: `A Session of ${s} expired`,
      'access ended': `A Session of ${s} ended with their access`,
      removed: `A Session of ${s} ended with their removal`,
    }
    return reasons[String(e.detail?.reason)] ?? `${a} ended a Session of ${s}`
  },
  'person-created': (e, a, s) => `${a} created ${s}, ${level(e.detail?.level)}`,
  'person-renamed': (e, a, s) => (e.actor.id === e.subject.id ? `${e.detail?.from} renamed themself ${s}` : `${a} renamed ${e.detail?.from} to ${s}`),
  'person-removed': (_, a, s) => `${a} removed ${s}`,
  'level-changed': (e, a, s) => `${a} changed ${s} from ${level(e.detail?.from)} to ${level(e.detail?.to)}`,
  'end-date-changed': (e, a, s) => {
    const { from, to } = (e.detail ?? {}) as { from?: string; to?: string }
    if (!to) return `${a} removed the end date of ${s}`
    return `${a} ${from ? 'moved' : 'set'} the last day of ${s} to ${day(to)}`
  },
  'end-date-reached': (_, __, s) => `The access of ${s} ended`,
  'link-created': (e, a, s) => (e.actor.id === e.subject.id ? `${a} created a Sign-in link for themself` : `${a} created a Sign-in link for ${s}`),
  'link-revoked': (_, a, s) => `${a} revoked the Sign-in link of ${s}`,
  'link-expired': (_, __, s) => `The Sign-in link of ${s} expired unused`,
  'passkey-added': (e, a) => `${a} added a Passkey${e.detail?.provider ? ` (${e.detail.provider})` : ''}`,
  'passkey-removed': (e, a) => `${a} removed a Passkey${e.detail?.provider ? ` (${e.detail.provider})` : ''}`,
  'program-created': (e, a, s) => `${a} created the Program ${s}, ${level(e.detail?.level)}`,
  'program-renamed': (e, a, s) => `${a} renamed the Program ${e.detail?.from} to ${s}`,
  'program-removed': (_, a, s) => `${a} removed the Program ${s}`,
  'token-generated': (_, a, s) => `${a} generated a Token for ${s}`,
  'token-revoked': (_, a, s) => `${a} revoked the Token of ${s}`,
}

// What e tells, in a sentence; the hourly count of anonymous refusals past the budget says how many.
export function describe(e: Entry): string {
  const sentence = sentences[e.event]?.(e, who(e.actor), who(e.subject)) ?? e.event
  const count = e.detail?.count
  return typeof count === 'number' ? `${sentence}, ${count} more times that hour` : sentence
}
