// Passkeys in the browser (ADR 0024, 0025): signing in with one, confirming one for step-up, adding
// one, and the offer of one after a sign-in by link.

import { useEffect, useRef, useState } from 'react'
import { api, loadMe, meNow } from './access'
import { modal } from './confirm'

// One WebAuthn ceremony through Oiko at path: its options from path/options, the browser's passkey
// sheet, then the answer to path. Says why it failed, if it did.
async function ceremony(path: string, create: boolean): Promise<string | null> {
  const res = await api(`${path}/options`, { method: 'POST' })
  if (!res.ok) return (await res.text()).trim()
  const { publicKey } = await res.json()
  let credential: Credential | null
  try {
    credential = create
      ? await navigator.credentials.create({ publicKey: PublicKeyCredential.parseCreationOptionsFromJSON(publicKey) })
      : await navigator.credentials.get({ publicKey: PublicKeyCredential.parseRequestOptionsFromJSON(publicKey) })
  } catch (e) {
    if (e instanceof DOMException && e.name === 'NotAllowedError') return 'No Passkey was used.'
    if (e instanceof DOMException && e.name === 'InvalidStateError') return 'This device already holds one of your Passkeys.'
    return e instanceof Error ? e.message : String(e)
  }
  const done = await api(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(credential) })
  if (!done.ok) return (await done.text()).trim()
  await loadMe()
  return null
}

// The passkey sheet picks the account: no username to type.
export const signInWithPasskey = () => ceremony('/api/sign-in/passkey', false)
export const addPasskey = () => ceremony('/api/me/passkeys', true)

// Asked before any action that could grant lasting access (ADR 0025): resolves true once this Session
// signed in, or confirmed a Passkey, within 10 minutes; otherwise asks to confirm one first. A Person
// without a Passkey is told to sign in by link again.
export async function stepUp(): Promise<boolean> {
  await loadMe()
  if (meNow()?.fresh) return true
  return modal((done) => <StepUp onDone={done} />)
}

const primary = 'rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300'
const secondary = 'rounded bg-neutral-800 px-3 py-1 hover:bg-neutral-700'

function StepUp({ onDone }: { onDone: (ok: boolean) => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const ok = useRef(false)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => dialog.current?.showModal(), [])
  const confirm = async () => {
    const err = await ceremony('/api/step-up', false)
    if (err) return setError(err)
    ok.current = true
    dialog.current?.close()
  }
  return (
    <dialog
      ref={dialog}
      onClose={() => onDone(ok.current)}
      onClick={(e) => e.target === dialog.current && dialog.current.close()}
      className="m-auto w-full max-w-sm rounded-xl bg-neutral-900 p-0 text-neutral-100 backdrop:bg-black/60"
    >
      <div className="space-y-5 p-5 text-sm">
        <h2 className="text-base font-medium">Confirm it is you</h2>
        <p>This needs a fresh proof that it is you: confirm one of your Passkeys.</p>
        {error && <p className="text-red-400">{error}</p>}
        <div className="flex justify-end gap-2">
          <button onClick={() => dialog.current?.close()} className={secondary}>
            Cancel
          </button>
          <button onClick={confirm} className={primary}>
            Use a Passkey
          </button>
        </div>
      </div>
    </dialog>
  )
}

// After a sign-in by link: a Passkey for this device, in plain words, which the Person may decline,
// then learning a new link will be needed once this Session ends.
export function PasskeyOffer({ onDone }: { onDone: () => void }) {
  const [error, setError] = useState<string | null>(null)
  const [declined, setDeclined] = useState(false)
  if (declined)
    return (
      <div className="space-y-4">
        <p>Without a Passkey, you will need a new sign-in link once this device's session ends.</p>
        <button onClick={onDone} className={primary}>
          Open Oiko
        </button>
      </div>
    )
  return (
    <div className="space-y-4">
      <p>Sign in next time with a Passkey: your fingerprint, face or screen lock unlocks it on this device. There is no password to remember.</p>
      {error && <p className="text-red-400">{error}</p>}
      <div className="flex flex-wrap gap-2">
        <button
          onClick={async () => {
            const err = await addPasskey()
            if (err) setError(err)
            else onDone()
          }}
          className={primary}
        >
          Add a Passkey
        </button>
        <button onClick={() => setDeclined(true)} className={secondary}>
          Not now
        </button>
      </div>
    </div>
  )
}
