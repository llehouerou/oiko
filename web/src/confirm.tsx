import { useEffect, useRef, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'

// Shows what render draws, outside the page, until it calls done with what the promise resolves to.
export function modal<T>(render: (done: (v: T) => void) => ReactNode): Promise<T> {
  const host = document.body.appendChild(document.createElement('div'))
  const root = createRoot(host)
  return new Promise((resolve) => {
    root.render(
      render((v) => {
        root.unmount()
        host.remove()
        resolve(v)
      }),
    )
  })
}

// The app's window.confirm: a modal that resolves true on the action button, and false
// on Cancel, Escape or a click on the backdrop. Cancel takes the focus, so Enter is harmless.
export function confirm(message: string, action: string): Promise<boolean> {
  return modal((done) => <Confirm message={message} action={action} onDone={done} />)
}

function Confirm({ message, action, onDone }: { message: string; action: string; onDone: (ok: boolean) => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  useEffect(() => dialog.current?.showModal(), [])
  return (
    <dialog
      ref={dialog}
      onClose={(e) => onDone(e.currentTarget.returnValue === 'ok')}
      onClick={(e) => e.target === dialog.current && dialog.current.close()}
      className="m-auto w-full max-w-sm rounded-xl bg-neutral-900 p-0 text-neutral-100 backdrop:bg-black/60"
    >
      <form method="dialog" className="space-y-5 p-5 text-sm">
        <p>{message}</p>
        <div className="flex justify-end gap-2">
          <button value="" className="rounded bg-neutral-800 px-3 py-1 hover:bg-neutral-700">
            Cancel
          </button>
          <button value="ok" className="rounded bg-red-500 px-3 py-1 font-medium text-white hover:bg-red-400">
            {action}
          </button>
        </div>
      </form>
    </dialog>
  )
}
