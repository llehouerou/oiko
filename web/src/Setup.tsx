// The page a Setup link opens (/setup#<secret>): whoever opens it names themselves and becomes the
// first Admin of a fresh Oiko, signed in, then is offered a Passkey.

import { useState, type FormEvent } from 'react'
import { Mark } from './Logo'
import { claim, useMe } from './access'
import { PasskeyOffer } from './passkeys'

export function Setup({ onDone }: { onDone: () => void }) {
  const me = useMe()
  const [error, setError] = useState<string | null>(null)
  const [offer, setOffer] = useState(false)
  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const err = await claim(location.hash.slice(1), String(new FormData(e.currentTarget).get('name')))
    if (err) setError(err)
    else setOffer(true)
  }
  return (
    <main className="grid min-h-dvh place-items-center p-4 text-neutral-100">
      <div className="w-full max-w-sm space-y-5 rounded-xl bg-neutral-900 p-6 text-sm">
        <header className="flex items-center gap-3">
          <Mark />
          <h1 className="text-lg font-semibold">Set up Oiko</h1>
        </header>
        {offer ? (
          <PasskeyOffer onDone={onDone} />
        ) : me?.claimed && !me.identity ? (
          <>
            <p>This Oiko already has an Admin: its Setup link no longer works.</p>
            <button onClick={onDone} className="rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300">
              Open Oiko
            </button>
          </>
        ) : (
          <form onSubmit={submit} className="space-y-4">
            <p>You become this Oiko's first Admin: you will invite the rest of the household.</p>
            <label className="block space-y-1">
              <span className="text-neutral-400">Your name</span>
              <input name="name" required autoFocus autoComplete="name" className="w-full rounded bg-neutral-800 px-2 py-1" />
            </label>
            {error && <p className="text-red-400">{error}</p>}
            <button type="submit" className="rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300">
              Continue
            </button>
          </form>
        )}
      </div>
    </main>
  )
}
