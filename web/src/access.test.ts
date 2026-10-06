import { afterEach, expect, test, vi } from 'vitest'
import { api, loadMe, meNow, screen, signInPage, type Me } from './access'

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
  const views = ['', '#', '#history', '#automations', '#automations/a1', '#programs', '#account', '#nonsense'].map((h) => screen(alice, h))
  expect(views).toEqual(['home', 'home', 'history', 'automations', 'automations', 'programs', 'account', 'home'])
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
