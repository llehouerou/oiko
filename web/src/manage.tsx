// What the pages managing access share: dates, the Access level picker, and handing over a secret
// shown once: a Copy button, and a Sign-in link with its QR code and the device's share sheet (ADR
// 0030). Oiko never sends a link itself.

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

export function LevelSelect(props: { name?: string; defaultValue?: Level; value?: Level; onChange?: (l: Level) => void }) {
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

// A Sign-in link just created, shown this once: its QR code, drawn inline (the CSP refuses data:
// URLs), to scan from another device, or the link itself to copy or share through a messenger.
export function ShareLink({ link, expires, onDone }: { link: string; expires: string; onDone: () => void }) {
  const { size, d } = qrPath(link)
  return (
    <div className="space-y-3 rounded-lg border border-amber-400/40 bg-amber-400/10 p-3">
      <p className="text-amber-200">Scan it on the other device, or send it: it signs in once, until {date(expires)}. It will not be shown again.</p>
      <svg
        viewBox={`-4 -4 ${size + 8} ${size + 8}`}
        role="img"
        aria-label="QR code of the Sign-in link"
        shapeRendering="crispEdges"
        className="size-56 rounded bg-white"
      >
        <path d={d} fill="#000" />
      </svg>
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
