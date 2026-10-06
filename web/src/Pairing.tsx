// Kiosk pairing (ADR 0029). On the screen to pair, KioskOffer shows a QR code and waits: only this
// browser, which holds the request's claim secret, receives the Kiosk's Session. On an Admin's phone,
// the QR code opens /pair#<approval secret>, where ApprovePairing enrols the screen as a new Kiosk or
// pairs an existing one again, after step-up.

import { useEffect, useState, type FormEvent } from 'react'
import { api, claimPairing, levels, pairLink, requestPairing, type Me } from './access'
import { stepUp } from './passkeys'
import { edit } from './store'
import { LevelSelect, QrCode } from './manage'
import type { Kiosk } from './Kiosks'

const primary = 'rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300'
const secondary = 'rounded bg-neutral-800 px-3 py-1 hover:bg-neutral-700'

// The screen's side: a pairing request's QR code, replaced by a fresh one as each expires, until an
// Admin approves one and this browser signs in as the Kiosk.
export function KioskOffer({ me, onCancel }: { me: Me; onCancel: () => void }) {
  const [secret, setSecret] = useState<string | null>(null)
  const [refused, setRefused] = useState(false)
  useEffect(() => {
    let stop = false
    void (async () => {
      while (!stop) {
        const s = await requestPairing()
        if (stop) return
        if (!s) return setRefused(true)
        setSecret(s)
        let state: Awaited<ReturnType<typeof claimPairing>> = 'waiting'
        while (state === 'waiting') {
          await new Promise((r) => setTimeout(r, 2000))
          if (stop) return
          state = await claimPairing()
        }
        if (state === 'paired') return
      }
    })()
    return () => {
      stop = true
    }
  }, [])
  return (
    <>
      <p>Scan this code with a phone signed in as an Admin to use this screen as a Kiosk: it then stays signed in as itself, for everyone standing at it.</p>
      {refused && <p className="text-red-400">Oiko refused to pair this screen.</p>}
      {secret && <QrCode text={pairLink(me, location.origin, secret)} label="QR code to pair this screen" />}
      <button onClick={onCancel} className="text-neutral-400 hover:text-white">
        Sign in as a Person instead
      </button>
    </>
  )
}

// The Admin's side (/pair#<secret>): the request's age and browser, then a new Kiosk or an existing one.
export function ApprovePairing({ onDone }: { onDone: () => void }) {
  const [secret] = useState(() => location.hash.slice(1))
  const [request, setRequest] = useState<{ created: string; browser: string } | { error: string } | null>(null)
  const [kiosks, setKiosks] = useState<Kiosk[]>([])
  const [kiosk, setKiosk] = useState('') // an existing Kiosk's id; '' for a new one
  const [approved, setApproved] = useState<string | null>(null) // the Kiosk's Name
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    void (async () => {
      const res = await api('/api/kiosk-pairing/request', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ secret }) })
      setRequest(res.ok ? await res.json() : { error: (await res.text()).trim() })
      const ks = await api('/api/kiosks')
      if (ks.ok) setKiosks(await ks.json())
    })()
  }, [secret])
  const approve = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const data = new FormData(e.currentTarget)
    if (!(await stepUp())) return
    const err = await edit('POST', 'kiosk-pairing/approve', { secret, kiosk, name: data.get('name') ?? '', level: data.get('level') ?? '' })
    if (err) return setError(err)
    history.replaceState(null, '', '/pair') // spent: out of the address and the browser's history
    setApproved(kiosk ? (kiosks.find((k) => k.id === kiosk)?.name ?? '') : String(data.get('name')))
  }
  const minutes = request && 'created' in request ? Math.round((Date.now() - new Date(request.created).getTime()) / 60000) : 0
  return (
    <main className="grid min-h-dvh place-items-center p-4 text-neutral-100">
      <div className="w-full max-w-sm space-y-5 rounded-xl bg-neutral-900 p-6 text-sm">
        <h1 className="text-lg font-semibold">Pair a Kiosk</h1>
        {approved !== null ? (
          <p>Approved: the screen signs in as {approved} within seconds.</p>
        ) : request && 'error' in request ? (
          <p>{request.error}</p>
        ) : request ? (
          <form onSubmit={approve} className="space-y-4">
            <p>
              This enrols a <strong>shared screen</strong>: anyone standing at it uses Oiko as the Kiosk, without signing in. Approve only a screen you are
              looking at.
            </p>
            <p className="text-neutral-400">
              Asked {minutes < 1 ? 'less than a minute' : `${minutes} minute${minutes === 1 ? '' : 's'}`} ago from {request.browser}.
            </p>
            {kiosks.length > 0 && (
              <select value={kiosk} onChange={(e) => setKiosk(e.target.value)} aria-label="Kiosk" className="w-full rounded bg-neutral-800 px-2 py-1">
                <option value="">A new Kiosk</option>
                {kiosks.map((k) => (
                  <option key={k.id} value={k.id}>
                    {k.name} ({levels[k.level]}), signing its current screen out
                  </option>
                ))}
              </select>
            )}
            {kiosk === '' && (
              <div className="flex flex-wrap gap-2">
                <input
                  name="name"
                  required
                  placeholder="Name, e.g. Hall tablet"
                  aria-label="Name"
                  className="min-w-40 flex-1 rounded bg-neutral-800 px-2 py-1"
                />
                <LevelSelect name="level" defaultValue="guest" only={['guest', 'member']} />
              </div>
            )}
            {error && <p className="text-red-400">{error}</p>}
            <button type="submit" className={primary}>
              Approve
            </button>
          </form>
        ) : null}
        <button onClick={onDone} className={secondary}>
          {approved !== null ? 'Done' : 'Cancel'}
        </button>
      </div>
    </main>
  )
}
