// The Admin page of Persons (#persons): who is known to Oiko, at which Access level, and their
// pending Sign-in link, if any (ADR 0030). An Admin creates a Person, then a Sign-in link to invite
// them, or to let them back in once they lost every Passkey. Every change asks for step-up first;
// Oiko keeps its last Admin Person.

import { useEffect, useState, type FormEvent } from 'react'
import { AppBar } from './AppBar'
import { confirm } from './confirm'
import { edit } from './store'
import { api, levels, signInLink, useMe, type Level } from './access'
import { stepUp } from './passkeys'
import { date, LevelSelect, ShareLink } from './manage'

type Person = {
  id: string
  name: string
  level: Level
  link: { creator: { id: string; name?: string }; created: string; expires: string } | null // pending
}

export function Persons() {
  const me = useMe()
  const self = me?.identity && 'person' in me.identity ? me.identity.person : null
  const [persons, setPersons] = useState<Person[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [shown, setShown] = useState<{ person: string; link: string; expires: string } | null>(null) // a link just created
  const load = async () => {
    const res = await api('/api/persons')
    if (res.ok) setPersons(await res.json())
    else setError((await res.text()).trim())
  }
  useEffect(() => void load(), [])
  // After a change: shows why it was refused, if it was, and the Persons as they now are.
  const change = async (err: string | null) => (setError(err), load())
  // A change, once step-up allows it.
  const fresh = async (method: 'PUT' | 'DELETE' | 'POST', path: string, body?: unknown) => (await stepUp()) && change(await edit(method, path, body))
  const create = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const form = e.currentTarget
    const data = new FormData(form)
    if (!(await stepUp())) return
    const err = await edit('POST', 'persons', { name: data.get('name'), level: data.get('level') })
    if (!err) form.reset()
    change(err)
  }
  const rename = (p: Person, name: string) => name.trim() !== p.name && fresh('PUT', `persons/${p.id}`, { name, level: p.level })
  const invite = async (p: Person) => {
    if (p.link && !(await confirm(`A new Sign-in link for ${p.name} stops the pending one.`, 'Create'))) return
    if (!(await stepUp())) return
    const res = await api(`/api/persons/${p.id}/sign-in-link`, { method: 'POST' })
    if (res.ok) {
      const { secret, expires } = await res.json()
      setShown({ person: p.id, link: signInLink(me!, location.origin, secret), expires })
    }
    change(res.ok ? null : (await res.text()).trim())
  }
  const button = 'rounded bg-neutral-800 px-3 py-1 hover:bg-neutral-700'
  return (
    <>
      <AppBar />
      <main className="mx-auto max-w-4xl space-y-6 p-4 text-sm">
        <header>
          <h1 className="text-xl font-semibold">Persons</h1>
          <p className="text-neutral-400">
            The household and its guests. Create a Person, then a Sign-in link to invite them: it signs them in once, within 24 hours, from a QR code or any
            messenger.
          </p>
        </header>
        {error && <p className="text-red-400">{error}</p>}
        <form onSubmit={create} className="flex flex-wrap gap-2">
          <input name="name" required placeholder="Name" aria-label="Name" className="min-w-40 flex-1 rounded bg-neutral-800 px-2 py-1" />
          <LevelSelect name="level" defaultValue="member" />
          <button type="submit" className="rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300">
            Create
          </button>
        </form>
        <ul className="space-y-3">
          {persons?.map((p) => (
            <li key={p.id} className="space-y-3 rounded-xl bg-neutral-900 p-4">
              <div className="flex flex-wrap items-center gap-3">
                <input
                  key={p.name}
                  defaultValue={p.name}
                  aria-label="Name"
                  onBlur={(e) => rename(p, e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
                  className="min-w-0 flex-1 truncate rounded bg-transparent px-1 text-base font-medium hover:bg-neutral-800 focus:bg-neutral-800"
                />
                {/* Nobody changes their own Access level. */}
                {p.id === self ? (
                  <span className="text-neutral-400">{levels[p.level]} · you</span>
                ) : (
                  <LevelSelect value={p.level} onChange={(level) => fresh('PUT', `persons/${p.id}`, { name: p.name, level })} />
                )}
              </div>
              {p.link && (
                <p className="text-neutral-400">
                  Sign-in link created by {p.link.creator.name ?? 'a removed Person'} on {date(p.link.created)}, valid until {date(p.link.expires)}
                </p>
              )}
              {shown?.person === p.id && <ShareLink link={shown.link} expires={shown.expires} onDone={() => setShown(null)} />}
              <div className="flex flex-wrap gap-2">
                <button onClick={() => invite(p)} className={button}>
                  Create a Sign-in link
                </button>
                {p.link && (
                  <button
                    onClick={async () =>
                      (await confirm(`Revoke ${p.name}'s Sign-in link? It stops working at once.`, 'Revoke')) && fresh('DELETE', `persons/${p.id}/sign-in-link`)
                    }
                    className={button}
                  >
                    Revoke the link
                  </button>
                )}
                <button
                  onClick={async () =>
                    (await confirm(`Remove ${p.name}? They are signed out everywhere; what they created keeps working.`, 'Remove')) &&
                    fresh('DELETE', `persons/${p.id}`)
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
