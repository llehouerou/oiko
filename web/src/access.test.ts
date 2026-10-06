import { afterEach, expect, test, vi } from 'vitest'
import { allows, api, linkPerson, loadMe, meNow, screen, signInLink, signInPage, signInWithLink, type Level, type Me } from './access'

const alice: Me = { identity: { person: 'p1' }, name: 'Alice', level: 'admin', fresh: false, claimed: true, publicUrl: 'https://oiko.example' }
const nobody: Me = { identity: null, fresh: false, claimed: true, publicUrl: 'https://oiko.example' }

// Oiko answers each path with its status and body.
function oiko(answers: Record<string, [number, unknown]>) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (path: string) => {
      const [status, body] = answers[path] ?? [404, 'not found']
      return new Response(JSON.stringify(body), { status })
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
  const hashes = ['#', '#history', '#automations/a1', '#persons', '#programs', '#account']
  const views = (me: Me) => hashes.map((h) => screen(me, h))
  const as = (level: Level): Me => ({ ...alice, level })
  expect(views(as('guest'))).toEqual(['home', 'home', 'home', 'home', 'home', 'account'])
  expect(views(as('member'))).toEqual(['home', 'history', 'automations', 'home', 'home', 'account'])
  expect(views(as('admin'))).toEqual(['home', 'history', 'automations', 'persons', 'programs', 'account'])
  // A Program has no account, and manages no access, whatever its level.
  expect(views({ ...alice, identity: { program: 'x1' } })).toEqual(['home', 'history', 'automations', 'home', 'home', 'home'])
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

test('a spent Sign-in link says so in plain words, naming no one', async () => {
  const spent = 'refused: this Sign-in link has expired or was already used; ask whoever sent it for a new one'
  oiko({ '/api/sign-in/link/person': [200, { name: 'Bob' }], '/api/sign-in/link': [403, spent] })
  expect(await linkPerson('s')).toEqual({ name: 'Bob' })
  expect(await signInWithLink('s')).toBe('This Sign-in link has expired or was already used. Ask whoever sent it for a new one.')
  oiko({ '/api/sign-in/link/person': [403, 'sign in at https://oiko.example'] })
  expect(await linkPerson('s')).toEqual({ error: '"sign in at https://oiko.example"' })
})
