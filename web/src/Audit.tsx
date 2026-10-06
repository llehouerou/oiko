// The Audit log (ADR 0033): the Admin page of all of it (#audit), filtered by identity, and the list a
// Person's own page shows of what concerns them. Newest first, a page at a time.

import { useEffect, useState } from 'react'
import { AppBar } from './AppBar'
import { api } from './access'
import { describe, type Entry } from './audit'
import { date } from './manage'
import { useNames } from './origin'

// The entries where party, if given, acts or is acted upon; Oiko keeps a Person who is no Admin to theirs.
export function AuditLog({ party }: { party?: { kind: string; id: string } }) {
  const [entries, setEntries] = useState<Entry[] | null>(null)
  const [more, setMore] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const query = party ? `kind=${party.kind}&id=${party.id}` : ''
  const load = async (before?: number) => {
    const res = await api(`/api/audit?${query}${before ? `&before=${before}` : ''}`)
    if (!res.ok) return setError((await res.text()).trim())
    const page: Entry[] = await res.json()
    setEntries((es) => (before ? [...(es ?? []), ...page] : page))
    setMore(page.length === 100)
  }
  useEffect(() => void load(), [query])
  if (error) return <p className="text-red-400">{error}</p>
  return (
    <div className="space-y-2">
      {entries?.length === 0 && <p className="text-neutral-500">Nothing yet.</p>}
      <ul className="divide-y divide-neutral-800 rounded-xl bg-neutral-900">
        {entries?.map((e) => (
          <li key={e.id} className="flex flex-wrap items-baseline gap-x-3 px-4 py-2">
            <span className="w-40 shrink-0 text-neutral-500 tabular-nums">{date(e.time)}</span>
            <span className="min-w-0 flex-1">{describe(e)}</span>
            {e.browser && <span className="text-neutral-500">{e.browser}</span>}
          </li>
        ))}
      </ul>
      {more && (
        <button onClick={() => load(entries?.at(-1)?.id)} className="rounded bg-neutral-800 px-3 py-1 hover:bg-neutral-700">
          Older
        </button>
      )}
    </div>
  )
}

export function AuditPage() {
  const names = useNames()
  const [party, setParty] = useState('')
  const [kind, id] = party.split(':')
  return (
    <>
      <AppBar />
      <main className="mx-auto max-w-4xl space-y-6 p-4 text-sm">
        <header className="space-y-1">
          <h1 className="text-xl font-semibold">Audit log</h1>
          <p className="text-neutral-400">
            Every sign-in, refusal and change to access, kept a year. Each names who acted and whom it concerns as they were named then.
          </p>
        </header>
        <select value={party} onChange={(e) => setParty(e.target.value)} aria-label="Whose entries" className="rounded bg-neutral-800 px-2 py-1">
          <option value="">Everyone</option>
          {Object.entries(names ?? {}).flatMap(([k, byId]) =>
            Object.entries(byId).map(([i, name]) => (
              <option key={`${k}:${i}`} value={`${k}:${i}`}>
                {name}
              </option>
            )),
          )}
        </select>
        <AuditLog party={kind && id ? { kind, id } : undefined} />
      </main>
    </>
  )
}
