// The signed-in Person's own page (#account): their Name; their Passkeys, each named by its provider,
// added and removed after step-up, after which they are offered to sign out every other device; their
// Sessions, which they end without step-up (ADR 0025); a Sign-in link to sign in another device of
// theirs; and what the Audit log holds of them.

import { useEffect, useState, type FormEvent } from 'react'
import { AppBar } from './AppBar'
import { confirm } from './confirm'
import { edit } from './store'
import { api, levels, loadMe, signInLink, signOut, useMe } from './access'
import { addPasskey, stepUp } from './passkeys'
import { PasskeyList, SessionList, ShareLink, type Passkey, type Session } from './manage'
import { AuditLog } from './Audit'

export function Account() {
  const me = useMe()
  const [passkeys, setPasskeys] = useState<Passkey[] | null>(null)
  const [sessions, setSessions] = useState<Session[]>([])
  const [error, setError] = useState<string | null>(null)
  const [shown, setShown] = useState<{ link: string; expires: string } | null>(null) // a link just created
  const load = async () => {
    const [keys, signedIn] = await Promise.all([api('/api/me/passkeys'), api('/api/me/sessions')])
    if (!keys.ok) return setError((await keys.text()).trim())
    setPasskeys(await keys.json())
    if (signedIn.ok) setSessions(await signedIn.json())
  }
  useEffect(() => void load(), [])
  const change = async (err: string | null) => (setError(err), load())
  // After a Passkey is added or removed: whoever holds another of this Person's Sessions may be who
  // they are locking out.
  const offer = async (err: string | null) => {
    if (!err && sessions.some((s) => !s.current) && (await confirm('Sign out all your other devices too?', 'Sign out'))) {
      err = await edit('DELETE', 'me/sessions')
    }
    change(err)
  }
  const add = async () => (await stepUp()) && offer(await addPasskey())
  const remove = async (k: Passkey) =>
    (await confirm(`Remove this ${k.provider ?? 'Passkey'}? It will no longer sign you in.`, 'Remove')) &&
    (await stepUp()) &&
    offer(await edit('DELETE', `me/passkeys/${k.id}`))
  const end = async (s: Session) => (s.current ? signOut() : change(await edit('DELETE', `me/sessions/${s.id}`)))
  const rename = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const err = await edit('PUT', 'me', { name: new FormData(e.currentTarget).get('name') })
    setError(err)
    if (!err) await loadMe()
  }
  const anotherDevice = async () => {
    if (!me?.identity || !('person' in me.identity) || !(await stepUp())) return
    const res = await api(`/api/persons/${me.identity.person}/sign-in-link`, { method: 'POST' })
    if (!res.ok) return setError((await res.text()).trim())
    const { secret, expires } = await res.json()
    setShown({ link: signInLink(me, location.origin, secret), expires })
  }
  return (
    <>
      <AppBar />
      <main className="mx-auto max-w-4xl space-y-6 p-4 text-sm">
        <header className="space-y-2">
          <form key={me?.name} onSubmit={rename} className="flex flex-wrap items-center gap-2">
            <input
              name="name"
              required
              defaultValue={me?.name}
              aria-label="Your name"
              className="min-w-40 flex-1 rounded bg-neutral-800 px-2 py-1 text-xl font-semibold"
            />
            <button type="submit" className="rounded bg-neutral-800 px-3 py-1 hover:bg-neutral-700">
              Rename
            </button>
          </form>
          <p className="text-neutral-400">{me?.level && levels[me.level]}</p>
        </header>
        {error && <p className="text-red-400">{error}</p>}
        <section className="space-y-3">
          <h2 className="text-base font-medium">Passkeys</h2>
          <p className="text-neutral-400">
            A Passkey signs you in without a password, unlocked by your fingerprint, face or screen lock. Your phone's also signs you in on a computer.
          </p>
          {passkeys?.length === 0 && <p className="text-neutral-500">No Passkeys yet: once this Session ends, you will need a new Sign-in link.</p>}
          <PasskeyList passkeys={passkeys ?? []} onRemove={remove} />
          <button onClick={add} className="rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300">
            Add a Passkey
          </button>
        </section>
        <section className="space-y-3">
          <h2 className="text-base font-medium">Sessions</h2>
          <p className="text-neutral-400">Each browser signed in as you, until you sign it out, or after 30 days without use.</p>
          <SessionList sessions={sessions} onEnd={end} />
          {sessions.filter((s) => !s.current).length > 1 && (
            <button onClick={async () => change(await edit('DELETE', 'me/sessions'))} className="rounded bg-neutral-800 px-3 py-1 hover:bg-neutral-700">
              Sign out all other devices
            </button>
          )}
        </section>
        <section className="space-y-3">
          <h2 className="text-base font-medium">Another device</h2>
          <p className="text-neutral-400">A Sign-in link signs you in on another device of yours, once, within 15 minutes.</p>
          {shown ? (
            <ShareLink link={shown.link} expires={shown.expires} onDone={() => setShown(null)} />
          ) : (
            <button onClick={anotherDevice} className="rounded bg-neutral-800 px-3 py-1 hover:bg-neutral-700">
              Create a Sign-in link
            </button>
          )}
        </section>
        {me?.identity && 'person' in me.identity && (
          <section className="space-y-3">
            <h2 className="text-base font-medium">Your activity</h2>
            <p className="text-neutral-400">Your sign-ins, and every change to your access, kept a year.</p>
            <AuditLog party={{ kind: 'person', id: me.identity.person }} />
          </section>
        )}
      </main>
    </>
  )
}
