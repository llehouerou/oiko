// The Admin page of Programs (#programs): external software calling Oiko under its own identity,
// each with at most one Token, shown once when generated.

import { useEffect, useState, type FormEvent } from 'react'
import { AppBar } from './AppBar'
import { confirm } from './confirm'
import { edit } from './store'
import { levels, type Level } from './access'

type Program = {
  id: string
  name: string
  level: Level
  creator: { id: string; name?: string } // no name once they are removed
  created: string
  token: { generated: string; lastUse?: string } | null
}

const date = (t: string) => new Date(t).toLocaleString([], { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' })

export function Programs() {
  const [programs, setPrograms] = useState<Program[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [shown, setShown] = useState<{ program: string; token: string } | null>(null) // a Token just generated
  const load = async () => {
    const res = await fetch('/api/programs')
    if (res.ok) setPrograms(await res.json())
    else setError((await res.text()).trim())
  }
  useEffect(() => void load(), [])
  // Does a change, then shows the Programs as they now are, or why it was refused.
  const change = async (err: string | null) => (setError(err), load())
  const create = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const form = e.currentTarget
    const data = new FormData(form)
    const err = await edit('POST', 'programs', { name: data.get('name'), level: data.get('level') })
    if (!err) form.reset()
    change(err)
  }
  const generate = async (p: Program) => {
    if (p.token && !(await confirm(`A new Token for ${p.name} stops the current one at once.`, 'Generate'))) return
    const res = await fetch(`/api/programs/${p.id}/token`, { method: 'POST' })
    if (res.ok) setShown({ program: p.id, token: (await res.json()).token })
    change(res.ok ? null : (await res.text()).trim())
  }
  const button = 'rounded bg-neutral-800 px-3 py-1 hover:bg-neutral-700'
  return (
    <>
      <AppBar />
      <main className="mx-auto max-w-4xl space-y-6 p-4 text-sm">
        <header>
          <h1 className="text-xl font-semibold">Programs</h1>
          <p className="text-neutral-400">
            Software such as Node-RED calls Oiko with its own Token, sent as <code>Authorization: Bearer</code>. A Token is shown once; a lost one is generated
            again.
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
        {programs?.length === 0 && <p className="text-neutral-500">No Programs yet.</p>}
        <ul className="space-y-3">
          {programs?.map((p) => (
            <li key={p.id} className="space-y-3 rounded-xl bg-neutral-900 p-4">
              <div className="flex flex-wrap items-center gap-3">
                <p className="min-w-0 flex-1 truncate text-base font-medium">{p.name}</p>
                <LevelSelect value={p.level} onChange={async (level) => change(await edit('PUT', `programs/${p.id}`, { name: p.name, level }))} />
              </div>
              <p className="text-neutral-400">
                Created by {p.creator.name ?? 'a removed Person'} on {date(p.created)}
                {' · '}
                {p.token ? `Token generated ${date(p.token.generated)}, ${p.token.lastUse ? `last used ${date(p.token.lastUse)}` : 'never used'}` : 'no Token'}
              </p>
              {shown?.program === p.id && <ShownToken token={shown.token} onDone={() => setShown(null)} />}
              <div className="flex flex-wrap gap-2">
                <button onClick={() => generate(p)} className={button}>
                  {p.token ? 'Generate a new Token' : 'Generate a Token'}
                </button>
                {p.token && (
                  <button
                    onClick={async () =>
                      (await confirm(`Revoke ${p.name}'s Token? It stops working at once.`, 'Revoke')) && change(await edit('DELETE', `programs/${p.id}/token`))
                    }
                    className={button}
                  >
                    Revoke the Token
                  </button>
                )}
                <button
                  onClick={async () =>
                    (await confirm(`Remove ${p.name}? Its Token stops working at once.`, 'Remove')) && change(await edit('DELETE', `programs/${p.id}`))
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

function LevelSelect(props: { name?: string; defaultValue?: Level; value?: Level; onChange?: (l: Level) => void }) {
  const { onChange, ...rest } = props
  return (
    <select {...rest} aria-label="Access level" onChange={onChange && ((e) => onChange(e.target.value as Level))} className="rounded bg-neutral-800 px-2 py-1">
      {Object.entries(levels).map(([l, label]) => (
        <option key={l} value={l}>
          {label}
        </option>
      ))}
    </select>
  )
}

// A Token just generated, shown this once, with a Copy button.
function ShownToken({ token, onDone }: { token: string; onDone: () => void }) {
  const [copied, setCopied] = useState(false)
  return (
    <div className="space-y-2 rounded-lg border border-amber-400/40 bg-amber-400/10 p-3">
      <p className="text-amber-200">Copy this Token now: it will not be shown again.</p>
      <div className="flex flex-wrap items-center gap-2">
        <code className="min-w-0 flex-1 rounded bg-neutral-950 px-2 py-1 break-all select-all">{token}</code>
        <button
          onClick={() => navigator.clipboard?.writeText(token).then(() => setCopied(true))}
          className="rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300"
        >
          {copied ? 'Copied' : 'Copy'}
        </button>
        <button onClick={onDone} className="rounded bg-neutral-800 px-3 py-1 hover:bg-neutral-700">
          Done
        </button>
      </div>
    </div>
  )
}
