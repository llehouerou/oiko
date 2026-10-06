// The page a browser that is not signed in gets, in place of the view it asked for: signing in
// returns to it (ADR 0027).

import { useState } from 'react'
import { mdiHomeAutomation } from '@mdi/js'
import { Svg } from './icons'
import { signInPage, type Me } from './access'
import { signInWithPasskey } from './passkeys'

export function SignIn({ me }: { me: Me }) {
  const page = signInPage(me, location.origin)
  const [error, setError] = useState<string | null>(null)
  return (
    <main className="grid min-h-dvh place-items-center p-4 text-neutral-100">
      <div className="w-full max-w-sm space-y-5 rounded-xl bg-neutral-900 p-6 text-sm">
        <header className="flex items-center gap-3">
          <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-linear-to-br from-amber-300 to-orange-500 text-neutral-950">
            <Svg path={mdiHomeAutomation} className="size-[62%]" />
          </span>
          <h1 className="text-lg font-semibold">Sign in to Oiko</h1>
        </header>
        {page === 'unclaimed' && <p>This Oiko has no Admin yet. Open the Setup link Oiko's log shows at each start to claim it.</p>}
        {page === 'no-public-url' && (
          <p>
            Signing in here needs a Public URL: set <code>publicUrl</code> in Oiko's <code>config.json</code>. Until then, sign in at http://localhost, on
            Oiko's host.
          </p>
        )}
        {page === 'elsewhere' && (
          <p>
            Oiko signs you in at{' '}
            <a href={me.publicUrl + location.pathname + location.hash} className="text-amber-300 hover:underline">
              {me.publicUrl}
            </a>
            .
          </p>
        )}
        {page === 'here' && (
          <>
            <p>You are not signed in on this browser.</p>
            {error && <p className="text-red-400">{error}</p>}
            <button
              onClick={async () => setError(await signInWithPasskey())}
              className="w-full rounded bg-amber-400 px-3 py-2 font-medium text-neutral-900 hover:bg-amber-300"
            >
              Sign in with a Passkey
            </button>
            <p className="text-neutral-400">No Passkey on this device? Your browser can use your phone's. Otherwise, ask an Admin for a sign-in link.</p>
          </>
        )}
      </div>
    </main>
  )
}
