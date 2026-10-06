// The page a Sign-in link opens (/sign-in#<secret>): it tells whom the link signs in ("You've been
// invited to Oiko as Bob"), and Continue signs them in, then offers a Passkey (ADR 0030). A link
// that no longer works says so, naming no one.

import { useEffect, useState } from 'react'
import { mdiHomeAutomation } from '@mdi/js'
import { Svg } from './icons'
import { linkPerson, signInWithLink } from './access'
import { PasskeyOffer } from './passkeys'

export function SignInLink({ onDone }: { onDone: () => void }) {
  const [secret] = useState(() => location.hash.slice(1))
  const [person, setPerson] = useState<{ name: string } | { error: string } | null>(null)
  const [offer, setOffer] = useState(false)
  useEffect(() => void linkPerson(secret).then(setPerson), [secret])
  const proceed = async () => {
    const error = await signInWithLink(secret)
    if (error) return setPerson({ error })
    history.replaceState(null, '', '/sign-in') // spent: out of the address and the browser's history
    setOffer(true)
  }
  const primary = 'rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300'
  return (
    <main className="grid min-h-dvh place-items-center p-4 text-neutral-100">
      <div className="w-full max-w-sm space-y-5 rounded-xl bg-neutral-900 p-6 text-sm">
        <header className="flex items-center gap-3">
          <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-linear-to-br from-amber-300 to-orange-500 text-neutral-950">
            <Svg path={mdiHomeAutomation} className="size-[62%]" />
          </span>
          <h1 className="text-lg font-semibold">Oiko</h1>
        </header>
        {offer ? (
          <PasskeyOffer onDone={onDone} />
        ) : person && 'error' in person ? (
          <>
            <p>{person.error}</p>
            <button onClick={onDone} className={primary}>
              Open Oiko
            </button>
          </>
        ) : person ? (
          <>
            <p>
              You've been invited to Oiko as <strong>{person.name}</strong>.
            </p>
            <button onClick={proceed} className={primary}>
              Continue
            </button>
          </>
        ) : null}
      </div>
    </main>
  )
}
