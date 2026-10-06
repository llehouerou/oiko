// What the pages managing access share: dates, the Access level picker, a Person's Sessions and
// Passkeys, and handing over a secret shown once: a Copy button, and a Sign-in link with its QR code
// and the device's share sheet (ADR 0030). Oiko never sends a link itself.

import { useState } from 'react'
import qrcode from 'qrcode-generator'
import { levels, type Level } from './access'

const primary = 'rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300'
const secondary = 'rounded bg-neutral-800 px-3 py-1 hover:bg-neutral-700'

export const date = (t: string) => new Date(t).toLocaleString([], { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' })

// A Guest's end date is picked as the last day of their access, in this browser's time zone: it ends
// as the next one begins. endsAfter is that instant for a date input's value, null for none; lastDay
// is the date input's value for an end date.
export function endsAfter(day: string): string | null {
  if (!day) return null
  const [y, m, d] = day.split('-').map(Number)
  return new Date(y!, m! - 1, d! + 1).toISOString()
}
export function lastDay(ends: string): string {
  const t = new Date(new Date(ends).getTime() - 1)
  return `${t.getFullYear()}-${String(t.getMonth() + 1).padStart(2, '0')}-${String(t.getDate()).padStart(2, '0')}`
}
export const day = (ends: string) => new Date(new Date(ends).getTime() - 1).toLocaleDateString([], { day: 'numeric', month: 'short', year: 'numeric' })

// A Session as /api/me/sessions and /api/persons/{id}/sessions list it (ADR 0025).
export type Session = {
  id: string
  browser: string
  method: string
  provider?: string
  by?: { id: string; name?: string }
  signedIn: string
  lastUse: string
  current: boolean
}

// How s signed in, in words: with a Passkey from its provider, a Sign-in link from whoever created it…
export function howSignedIn(s: Session): string {
  switch (s.method) {
    case 'setup':
      return 'with the Setup link'
    case 'passkey':
      return s.provider ? `with a Passkey from ${s.provider}` : 'with a Passkey'
    case 'link':
      return s.by ? `with a Sign-in link from ${s.by.name ?? 'a removed Person'}` : 'with a Sign-in link'
    case 'host':
      return "with a Sign-in link from Oiko's host"
  }
  return ''
}

export type Passkey = { id: string; provider?: string; created: string; lastUse?: string }

const row = 'flex flex-wrap items-center gap-3 rounded-xl bg-neutral-900 p-4'

// Sessions, each with a button ending it.
export function SessionList({ sessions, onEnd }: { sessions: Session[]; onEnd: (s: Session) => void }) {
  return (
    <ul className="space-y-3">
      {sessions.map((s) => (
        <li key={s.id} className={row}>
          <div className="min-w-0 flex-1">
            <p className="truncate text-base font-medium">
              {s.browser || 'Unknown browser'}
              {s.current && <span className="ml-2 text-sm font-normal text-amber-300">this device</span>}
            </p>
            <p className="text-neutral-400">
              Signed in {howSignedIn(s)} on {date(s.signedIn)} · last used {date(s.lastUse)}
            </p>
          </div>
          <button onClick={() => onEnd(s)} className={secondary}>
            Sign out
          </button>
        </li>
      ))}
    </ul>
  )
}

// Passkeys, each named by its provider, with a button removing it.
export function PasskeyList({ passkeys, onRemove }: { passkeys: Passkey[]; onRemove: (k: Passkey) => void }) {
  return (
    <ul className="space-y-3">
      {passkeys.map((k) => (
        <li key={k.id} className={row}>
          <div className="min-w-0 flex-1">
            <p className="truncate text-base font-medium">{k.provider ?? 'Passkey'}</p>
            <p className="text-neutral-400">
              Added {date(k.created)} · {k.lastUse ? `last used ${date(k.lastUse)}` : 'never used'}
            </p>
          </div>
          <button onClick={() => onRemove(k)} className={`${secondary} text-red-400`}>
            Remove
          </button>
        </li>
      ))}
    </ul>
  )
}

// Picks an Access level among only, every one by default: a Kiosk is never an Admin.
export function LevelSelect(props: { name?: string; defaultValue?: Level; value?: Level; onChange?: (l: Level) => void; only?: Level[] }) {
  const { onChange, only, ...rest } = props
  return (
    <select {...rest} aria-label="Access level" onChange={onChange && ((e) => onChange(e.target.value as Level))} className="rounded bg-neutral-800 px-2 py-1">
      {Object.entries(levels)
        .filter(([l]) => !only || only.includes(l as Level))
        .map(([l, label]) => (
          <option key={l} value={l}>
            {label}
          </option>
        ))}
    </select>
  )
}

// Copies text; refused, text stays on screen to select.
export function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <button
      onClick={() =>
        navigator.clipboard?.writeText(text).then(
          () => setCopied(true),
          () => {},
        )
      }
      className={primary}
    >
      {copied ? 'Copied' : 'Copy'}
    </button>
  )
}

// The dark modules of text's QR code as an SVG path, one unit each, and how many a side counts.
export function qrPath(text: string) {
  const qr = qrcode(0, 'M')
  qr.addData(text)
  qr.make()
  const size = qr.getModuleCount()
  let d = ''
  for (let row = 0; row < size; row++) for (let col = 0; col < size; col++) if (qr.isDark(row, col)) d += `M${col} ${row}h1v1h-1z`
  return { size, d }
}

// text's QR code, drawn inline: the CSP refuses data: URLs.
export function QrCode({ text, label }: { text: string; label: string }) {
  const { size, d } = qrPath(text)
  return (
    <svg viewBox={`-4 -4 ${size + 8} ${size + 8}`} role="img" aria-label={label} shapeRendering="crispEdges" className="size-56 rounded bg-white">
      <path d={d} fill="#000" />
    </svg>
  )
}

// A Sign-in link just created, shown this once: its QR code to scan from another device, or the link
// itself to copy or share through a messenger.
export function ShareLink({ link, expires, onDone }: { link: string; expires: string; onDone: () => void }) {
  return (
    <div className="space-y-3 rounded-lg border border-amber-400/40 bg-amber-400/10 p-3">
      <p className="text-amber-200">Scan it on the other device, or send it: it signs in once, until {date(expires)}. It will not be shown again.</p>
      <QrCode text={link} label="QR code of the Sign-in link" />
      <div className="flex flex-wrap items-center gap-2">
        <code className="min-w-0 flex-1 rounded bg-neutral-950 px-2 py-1 break-all select-all">{link}</code>
        <CopyButton text={link} />
        {'share' in navigator && (
          <button onClick={() => navigator.share({ title: 'Sign in to Oiko', url: link }).catch(() => {})} className={secondary}>
            Share
          </button>
        )}
        <button onClick={onDone} className={secondary}>
          Done
        </button>
      </div>
    </div>
  )
}
