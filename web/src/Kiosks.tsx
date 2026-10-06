// The Admin page of Kiosks (#kiosks): shared screens signed in as themselves (ADR 0029), paired from
// their own screen. Signing one out or removing it only takes access away, so it needs no step-up;
// renaming it or changing its Access level does.

import { useEffect, useState } from 'react'
import { AppBar } from './AppBar'
import { confirm } from './confirm'
import { edit } from './store'
import { api, type Level } from './access'
import { stepUp } from './passkeys'
import { date, LevelSelect } from './manage'

export type Kiosk = {
  id: string
  name: string
  level: Level
  pairedBy: { id: string; name?: string } // no name once they are removed
  paired: string
  lastUse?: string
  signedIn: boolean
}

export function Kiosks() {
  const [kiosks, setKiosks] = useState<Kiosk[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const load = async () => {
    const res = await api('/api/kiosks')
    if (res.ok) setKiosks(await res.json())
    else setError((await res.text()).trim())
  }
  useEffect(() => void load(), [])
  const change = async (err: string | null) => (setError(err), load())
  const define = async (k: Kiosk, edits: { name?: string; level?: Level }) =>
    (await stepUp()) && change(await edit('PUT', `kiosks/${k.id}`, { name: k.name, level: k.level, ...edits }))
  const button = 'rounded bg-neutral-800 px-3 py-1 hover:bg-neutral-700'
  return (
    <>
      <AppBar />
      <main className="mx-auto max-w-4xl space-y-6 p-4 text-sm">
        <header>
          <h1 className="text-xl font-semibold">Kiosks</h1>
          <p className="text-neutral-400">
            Shared screens, such as a wall tablet, signed in as themselves. To add one, open Oiko on it and choose “Use this screen as a Kiosk”, then scan its
            QR code from here. A Kiosk stays signed in while it is used, and signs out after 30 days unused.
          </p>
        </header>
        {error && <p className="text-red-400">{error}</p>}
        {kiosks?.length === 0 && <p className="text-neutral-500">No Kiosks yet.</p>}
        <ul className="space-y-3">
          {kiosks?.map((k) => (
            <li key={k.id} className="space-y-3 rounded-xl bg-neutral-900 p-4">
              <div className="flex flex-wrap items-center gap-3">
                <input
                  key={k.name}
                  defaultValue={k.name}
                  aria-label="Name"
                  onBlur={(e) => e.target.value.trim() !== k.name && define(k, { name: e.target.value })}
                  onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
                  className="min-w-0 flex-1 truncate rounded bg-transparent px-1 text-base font-medium hover:bg-neutral-800 focus:bg-neutral-800"
                />
                <LevelSelect value={k.level} only={['guest', 'member']} onChange={(level) => define(k, { level })} />
              </div>
              <p className="text-neutral-400">
                Paired by {k.pairedBy.name ?? 'a removed Person'} on {date(k.paired)}
                {' · '}
                {k.signedIn ? `last used ${k.lastUse ? date(k.lastUse) : 'never'}` : 'signed out: pair it again from its screen'}
              </p>
              <div className="flex flex-wrap gap-2">
                {k.signedIn && (
                  <button
                    onClick={async () =>
                      (await confirm(`Sign ${k.name} out? Its screen shows the pairing code again.`, 'Sign out')) &&
                      change(await edit('DELETE', `kiosks/${k.id}/session`))
                    }
                    className={button}
                  >
                    Sign out
                  </button>
                )}
                <button
                  onClick={async () =>
                    (await confirm(`Remove ${k.name}? Its screen is signed out at once.`, 'Remove')) && change(await edit('DELETE', `kiosks/${k.id}`))
                  }
                  className={`${button} ml-auto text-red-400`}
                >
                  Remove
                </button>
              </div>
            </li>
          ))}
        </ul>
      </main>
    </>
  )
}
