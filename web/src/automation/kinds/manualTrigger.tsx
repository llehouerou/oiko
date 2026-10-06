import { createContext, useContext } from 'react'
import type { StepKind } from '.'
import { Hint, type FormProps } from './fields'

// Starts a Run when a Person, a Kiosk or a Program asks for it: a button on the dashboard,
// named after the Step, or Run here.
export const manualTrigger: StepKind = {
  label: 'Manual',
  group: 'Trigger',
  inputs: [],
  outputs: ['out'],
  params: () => ({}),
  Form: ManualForm,
  summary: () => ['a button on the dashboard'],
}

// How the editor runs a Manual trigger Step: blocked says why it can't, if it can't.
export const ManualRun = createContext<{ blocked: string | null; run: (step: string) => void }>({ blocked: 'not saved', run: () => {} })

function ManualForm({ id }: FormProps) {
  const { blocked, run } = useContext(ManualRun)
  return (
    <>
      <Hint>A button on the dashboard, named after this Step, runs it.</Hint>
      <div className="flex items-center gap-2">
        <button disabled={!!blocked} onClick={() => run(id)} className="rounded bg-neutral-700 px-2 py-0.5 text-white hover:bg-neutral-600 disabled:opacity-40">
          Run
        </button>
        {blocked && <span className="text-neutral-500">{blocked}</span>}
      </div>
    </>
  )
}
