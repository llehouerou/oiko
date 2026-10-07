import { useEffect, useRef, type ReactNode } from 'react'
import { ChartsShown } from './MiniChart'

// A modal panel; it closes on ✕, Escape or a click on the backdrop. With onBack,
// ←, Escape and a phone's back gesture go back instead.
export function Panel({ title, onClose, onBack, children }: { title: ReactNode; onClose: () => void; onBack?: () => void; children: ReactNode }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const backing = useRef(false)
  useEffect(() => dialog.current?.showModal(), [])
  return (
    <dialog
      ref={dialog}
      onCancel={(e) => e.target === e.currentTarget && (backing.current = true)}
      onClose={(e) => e.target === e.currentTarget && (backing.current && onBack ? onBack() : onClose())} // not a dialog opened from it
      onClick={(e) => e.target === dialog.current && dialog.current.close()}
      className="m-auto w-full max-w-lg rounded-xl bg-neutral-900 p-0 text-neutral-100 backdrop:bg-black/60"
    >
      <div className="space-y-5 p-5">
        <header className="flex items-start justify-between gap-3">
          {onBack && (
            <button onClick={() => ((backing.current = true), dialog.current?.close())} aria-label="Back" className="text-neutral-400 hover:text-white">
              ←
            </button>
          )}
          <div className="min-w-0 flex-1">{title}</div>
          <button onClick={() => dialog.current?.close()} aria-label="Close" className="text-neutral-400 hover:text-white">
            ✕
          </button>
        </header>
        <ChartsShown value>{children}</ChartsShown>
      </div>
    </dialog>
  )
}
