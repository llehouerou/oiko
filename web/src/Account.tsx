// The signed-in Person's own page (#account): their Name; their Passkeys, each named by its provider,
// added and removed after step-up; a Sign-in link to sign in another device of theirs; and what the
// Audit log holds of them.

import { useEffect, useState, type FormEvent } from 'react'
import { AppBar } from './AppBar'
import { confirm } from './confirm'
import { edit } from './store'
import { api, levels, loadMe, signInLink, useMe } from './access'
import { addPasskey, stepUp } from './passkeys'
import { date, ShareLink } from './manage'
import { AuditLog } from './Audit'

type Passkey = { id: string; provider?: string; created: string; lastUse?: string }

export function Account() {
  const me = useMe()
  const [passkeys, setPasskeys] = useState<Passkey[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [shown, setShown] = useState<{ link: string; expires: string } | null>(null) // a link just created
  const load = async () => {
    const res = await api('/api/me/passkeys')
    if (res.ok) setPasskeys(await res.json())
    else setError((await res.text()).trim())
  }
  useEffect(() => void load(), [])
  const change = async (err: string | null) => (setError(err), load())
  const add = async () => (await stepUp()) && change(await addPasskey())
  const remove = async (k: Passkey) =>
    (await confirm(`Remove this ${k.provider ?? 'Passkey'}? It will no longer sign you in.`, 'Remove')) &&
    (await stepUp()) &&
    change(await edit('DELETE', `me/passkeys/${k.id}`))
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
          <ul className="space-y-3">
            {passkeys?.map((k) => (
              <li key={k.id} className="flex flex-wrap items-center gap-3 rounded-xl bg-neutral-900 p-4">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-base font-medium">{k.provider ?? 'Passkey'}</p>
                  <p className="text-neutral-400">
                    Added {date(k.created)} · {k.lastUse ? `last used ${date(k.lastUse)}` : 'never used'}
                  </p>
                </div>
                <button onClick={() => remove(k)} className="rounded bg-neutral-800 px-3 py-1 text-red-400 hover:bg-neutral-700">
                  Remove
                </button>
              </li>
            ))}
          </ul>
          <button onClick={add} className="rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300">
            Add a Passkey
          </button>
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
