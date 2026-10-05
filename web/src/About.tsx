// What Oiko is built from: its version, the Bridges of the configuration and whether each is
// online, and every type of Bridge compiled in with the module and version it comes from, each
// module with what the module proxy lists of its Releases. And the banner telling which are newer,
// and how to apply them.

import { useEffect, useRef, useState } from 'react'
import { mdiUpdate } from '@mdi/js'
import { Svg } from './icons'
import { useBridges, useReleases } from './store'
import type { Build, ReleaseStatus } from './types'
import { oiko, upgrade } from './upgrade'

function useBuild() {
  const [build, setBuild] = useState<Build | null>(null)
  useEffect(() => {
    fetch('/api/build')
      .then((r) => (r.ok ? r.json() : null))
      .then(setBuild, () => {})
  }, [])
  return build
}

// A module's Releases against the version built in.
function newest(s?: ReleaseStatus) {
  if (!s?.newest) return 'newest unknown'
  const text = s.newer ? `${s.newest} available` : s.current ? 'up to date' : `newest ${s.newest}`
  return s.breaking ? `${text}, ${s.breaking} (may break)` : text
}

export function About({ onClose }: { onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  useEffect(() => dialog.current?.showModal(), [])
  const build = useBuild()
  const online = useBridges()
  const releases = useReleases()
  const release = (module?: string) => releases.find((s) => s.module === module)
  const heading = 'mb-2 text-xs font-medium tracking-wide text-neutral-500 uppercase'
  return (
    <dialog
      ref={dialog}
      onClose={onClose}
      onClick={(e) => e.target === dialog.current && dialog.current.close()}
      className="m-auto w-full max-w-lg rounded-xl bg-neutral-900 p-0 text-neutral-100 backdrop:bg-black/60"
    >
      <div className="space-y-5 p-5 text-sm">
        <header className="flex items-start justify-between gap-3">
          <div>
            <h2 className="text-lg font-semibold">
              Oiko <span className="font-normal text-neutral-400">{build && (build.version ?? 'development build')}</span>
            </h2>
            <p className="text-xs text-neutral-500">{newest(release(oiko))}</p>
          </div>
          <button onClick={() => dialog.current?.close()} aria-label="Close" className="text-neutral-400 hover:text-white">
            ✕
          </button>
        </header>
        {build && (
          <>
            <section>
              <h3 className={heading}>Bridges</h3>
              {Object.keys(build.bridges).length === 0 && <p className="text-neutral-500">None configured</p>}
              <ul className="space-y-1">
                {Object.entries(build.bridges)
                  .sort(([a], [b]) => a.localeCompare(b))
                  .map(([name, type]) => (
                    <li key={name} className="flex items-center gap-3">
                      <span className={`size-2 shrink-0 rounded-full ${online[name] ? 'bg-emerald-400' : 'bg-amber-400'}`} />
                      <span className="flex-1 truncate">{name}</span>
                      <span className="text-neutral-400">{type}</span>
                      <span className="w-14 text-right text-neutral-500">{online[name] ? 'online' : 'offline'}</span>
                    </li>
                  ))}
              </ul>
            </section>
            <section>
              <h3 className={heading}>Types of Bridge</h3>
              <ul className="space-y-1">
                {build.types.map((t) => (
                  <li key={t.type} className="flex flex-wrap items-baseline gap-x-3">
                    <span>{t.type}</span>
                    <span className="text-neutral-500">{t.builtIn ? 'built-in' : 'added'}</span>
                    {!t.builtIn && (
                      <span className="ml-auto truncate text-neutral-400" title={t.package}>
                        {t.module ?? t.package} {t.version ?? 'unknown'}
                        <span className="text-neutral-500"> · {newest(release(t.module))}</span>
                      </span>
                    )}
                  </li>
                ))}
              </ul>
            </section>
          </>
        )}
      </div>
    </dialog>
  )
}

// Above the dashboard, each module with a Release newer than the one built in, until dismissed. This
// browser remembers what it dismissed, by module, and shows the banner again for anything newer.
export function ReleaseBanner() {
  const releases = useReleases()
  const build = useBuild()
  const pre = useRef<HTMLPreElement>(null)
  const [copied, setCopied] = useState(false)
  const [dismissed, setDismissed] = useState<Record<string, string>>(() => JSON.parse(localStorage.getItem('oiko.releases') ?? '{}'))
  const offer = (s: ReleaseStatus) => [s.newer && s.newest, s.breaking].filter(Boolean).join(' ')
  const shown = releases.filter((s) => s.current && offer(s) && dismissed[s.module] !== offer(s))
  if (shown.length === 0) return null
  const dismiss = () => {
    const next = { ...dismissed, ...Object.fromEntries(shown.map((s) => [s.module, offer(s)])) }
    localStorage.setItem('oiko.releases', JSON.stringify(next))
    setDismissed(next)
  }
  const how = build && upgrade(build, releases)
  // The Clipboard API exists only in a secure context, which Oiko on a LAN over HTTP is not.
  const copy = () => {
    if (navigator.clipboard) navigator.clipboard.writeText(how!.code)
    else {
      getSelection()?.selectAllChildren(pre.current!)
      document.execCommand('copy')
    }
    setCopied(true)
  }
  return (
    <div role="status" className="flex items-start gap-3 rounded-xl border border-amber-400/30 bg-amber-400/10 px-4 py-3 text-sm">
      <Svg path={mdiUpdate} className="size-5 shrink-0 text-amber-300" />
      <div className="min-w-0 flex-1">
        <p className="font-medium text-amber-200">Newer releases</p>
        <ul className="mt-1 space-y-0.5">
          {shown.map((s) => (
            <li key={s.module} className="flex flex-wrap gap-x-2">
              <span className="truncate">{s.module === oiko ? 'Oiko' : s.module}</span>
              <span className="text-neutral-400">
                {s.current} → {[s.newer && s.newest, s.breaking && `${s.breaking} (may break)`].filter(Boolean).join(', ')}
              </span>
            </li>
          ))}
        </ul>
        {how && (
          <>
            <p className="mt-2 text-neutral-300">{how.text}</p>
            <div className="relative mt-1">
              <pre ref={pre} className="overflow-x-auto rounded-lg bg-black/40 p-2 pr-14 text-xs text-neutral-200">
                {how.code}
              </pre>
              <button onClick={copy} className="absolute top-1 right-1 rounded px-2 py-0.5 text-xs text-neutral-400 hover:text-white">
                {copied ? 'Copied' : 'Copy'}
              </button>
            </div>
          </>
        )}
      </div>
      <button onClick={dismiss} aria-label="Dismiss" className="text-neutral-400 hover:text-white">
        ✕
      </button>
    </div>
  )
}
