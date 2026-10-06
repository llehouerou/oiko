import { afterEach, expect, test, vi } from 'vitest'
import {
  allows,
  api,
  claimPairing,
  linkPerson,
  loadMe,
  meNow,
  pairLink,
  screen,
  signInLink,
  signInPage,
  signInWithLink,
  wasKiosk,
  type Level,
  type Me,
} from './access'

const alice: Me = { identity: { person: 'p1' }, name: 'Alice', level: 'admin', fresh: false, claimed: true, publicUrl: 'https://oiko.example' }
const nobody: Me = { identity: null, fresh: false, claimed: true, publicUrl: 'https://oiko.example' }

// Oiko answers each path with its status and body.
function oiko(answers: Record<string, [number, unknown]>) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (path: string) => {
      const [status, body] = answers[path] ?? [404, 'not found']
      return new Response(status === 204 ? null : JSON.stringify(body), { status })
    }),
  )
}

afterEach(() => vi.unstubAllGlobals())

test('the dashboard returns to the view it was on after signing in again', async () => {
  const hash = '#automations/a1'
  oiko({ '/api/me': [200, alice] })
  await loadMe()
  expect(screen(meNow(), hash)).toBe('automations')

  // The Session ended: the next request is refused.
  oiko({ '/api/runs/r1': [401, 'sign in first'], '/api/me': [200, nobody] })
  await api('/api/runs/r1')
  await vi.waitFor(() => expect(screen(meNow(), hash)).toBe('sign-in'))

  oiko({ '/api/me': [200, alice] })
  await loadMe()
  expect(screen(meNow(), hash)).toBe('automations')
})

test('nothing shows until Oiko tells who is signed in', async () => {
  expect(screen(null, '')).toBeNull()
  oiko({ '/api/me': [502, 'bad gateway'] })
  expect(await loadMe()).toBe(false)
})

test('each view has its hash; any other is the dashboard', () => {
  const views = ['', '#', '#history', '#automations', '#automations/a1', '#persons', '#programs', '#account', '#nonsense'].map((h) => screen(alice, h))
  expect(views).toEqual(['home', 'home', 'history', 'automations', 'automations', 'persons', 'programs', 'account', 'home'])
})

test('each Access level sees its views, and the dashboard for any other', () => {
  const hashes = ['#', '#history', '#automations/a1', '#persons', '#kiosks', '#programs', '#audit', '#account']
  const views = (me: Me) => hashes.map((h) => screen(me, h))
  const as = (level: Level): Me => ({ ...alice, level })
  expect(views(as('guest'))).toEqual(['home', 'home', 'home', 'home', 'home', 'home', 'home', 'account'])
  expect(views(as('member'))).toEqual(['home', 'history', 'automations', 'home', 'home', 'home', 'home', 'account'])
  expect(views(as('admin'))).toEqual(['home', 'history', 'automations', 'persons', 'kiosks', 'programs', 'audit', 'account'])
  // A Program, or a Kiosk, has no account, and manages no access, whatever its level.
  expect(views({ ...alice, identity: { program: 'x1' } })).toEqual(['home', 'history', 'automations', 'home', 'home', 'home', 'home', 'home'])
  expect(views({ ...alice, level: 'member', identity: { kiosk: 'k1' } })).toEqual(['home', 'history', 'automations', 'home', 'home', 'home', 'home', 'home'])
})

test('each Access level allows what those below it do', () => {
  const levels: Level[] = ['guest', 'member', 'admin']
  const table = levels.map((level) => levels.map((needed) => allows({ ...alice, level }, needed)))
  expect(table).toEqual([
    [true, false, false],
    [true, true, false],
    [true, true, true],
  ])
  expect(allows(nobody, 'guest')).toBe(false)
  expect(allows(null, 'guest')).toBe(false)
})

test('sign-in is offered on the Public URL and http://localhost only', () => {
  const none = { ...nobody, publicUrl: null }
  expect(
    [
      [nobody, 'https://oiko.example'],
      [nobody, 'http://localhost:8080'],
      [none, 'http://localhost:5173'],
      [nobody, 'http://oiko.example'],
      [nobody, 'http://192.168.1.2:8080'],
      [nobody, 'https://localhost'],
      [none, 'http://192.168.1.2:8080'],
      [{ ...none, claimed: false }, 'http://192.168.1.2:8080'],
    ].map(([me, origin]) => signInPage(me as Me, origin as string)),
  ).toEqual(['here', 'here', 'here', 'elsewhere', 'elsewhere', 'elsewhere', 'no-public-url', 'unclaimed'])
})

test('a Sign-in link points at the Public URL, its secret in the fragment', () => {
  expect(signInLink(alice, 'http://localhost:8080', 's3cret')).toBe('https://oiko.example/sign-in#s3cret')
  expect(signInLink({ ...alice, publicUrl: null }, 'http://localhost:8080', 's3cret')).toBe('http://localhost:8080/sign-in#s3cret')
})

test('a pairing approval points at the Public URL, its secret in the fragment', () => {
  expect(pairLink(alice, 'http://localhost:8080', 's3cret')).toBe('https://oiko.example/pair#s3cret')
  expect(pairLink({ ...alice, publicUrl: null }, 'http://localhost:8080', 's3cret')).toBe('http://localhost:8080/pair#s3cret')
})

test('a screen waits for its pairing, then signs in as the Kiosk and remembers it', async () => {
  const storage = new Map<string, string>()
  vi.stubGlobal('localStorage', { getItem: (k: string) => storage.get(k) ?? null, setItem: (k: string, v: string) => storage.set(k, v) })
  const kiosk: Me = { ...nobody, identity: { kiosk: 'k1' }, name: 'Hall tablet', level: 'guest' }
  oiko({ '/api/kiosk-pairing/claim': [202, null] })
  expect(await claimPairing()).toBe('waiting')
  expect(wasKiosk()).toBe(false)
  oiko({ '/api/kiosk-pairing/claim': [204, null], '/api/me': [200, kiosk] })
  expect(await claimPairing()).toBe('paired')
  expect(meNow()).toEqual(kiosk)
  expect(wasKiosk()).toBe(true)
  oiko({ '/api/kiosk-pairing/claim': [403, 'expired'] })
  expect(await claimPairing()).toBe('ended')
})

test('a Sign-in link tells whom it signs in, or why it no longer does', async () => {
  oiko({ '/api/sign-in/link/person': [200, { name: 'Bob' }], '/api/sign-in/link': [204, null] })
  expect(await linkPerson('s')).toEqual({ name: 'Bob' })
  oiko({ '/api/sign-in/link/person': [403, 'expired'], '/api/sign-in/link': [403, 'expired'] })
  expect(await linkPerson('s')).toEqual({ error: '"expired"' })
  expect(await signInWithLink('s')).toBe('"expired"')
})
