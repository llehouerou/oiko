// The signed-in Person's own page (#account): their Passkeys, each named by its provider, added and
// removed after step-up.

import { useEffect, useState } from 'react'
import { AppBar } from './AppBar'
import { confirm } from './confirm'
import { edit } from './store'
import { api, levels, useMe } from './access'
import { addPasskey, stepUp } from './passkeys'
import { date } from './Programs'

type Passkey = { id: string; provider?: string; created: string; lastUse?: string }

export function Account() {
  const me = useMe()
  const [passkeys, setPasskeys] = useState<Passkey[] | null>(null)
  const [error, setError] = useState<string | null>(null)
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
  return (
    <>
      <AppBar />
      <main className="mx-auto max-w-4xl space-y-6 p-4 text-sm">
        <header>
          <h1 className="text-xl font-semibold">{me?.name}</h1>
          <p className="text-neutral-400">{me?.level && levels[me.level]}</p>
        </header>
        <section className="space-y-3">
          <h2 className="text-base font-medium">Passkeys</h2>
          <p className="text-neutral-400">
            A Passkey signs you in without a password, unlocked by your fingerprint, face or screen lock. Your phone's also signs you in on a computer.
          </p>
          {error && <p className="text-red-400">{error}</p>}
          {passkeys?.length === 0 && <p className="text-neutral-500">No Passkeys yet: once this session ends, you will need a new sign-in link.</p>}
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
      </main>
    </>
  )
}
