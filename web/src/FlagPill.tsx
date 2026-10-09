// A Flag without an Area, as a pill under the page header.

import { mdiFlag, mdiFlagOutline } from '@mdi/js'
import { sendCommand, useCommand, useValue } from './store'
import type { Flag } from './types'
import { flagTarget } from './targets'
import { Svg } from './icons'
import { CommandNote } from './Capabilities'

// A Flag without an Area, as a pill under the page header: a tap toggles it, ⋯ opens its panel, to an Admin.
export function FlagPill({ flag, onOpen }: { flag: Flag; onOpen?: () => void }) {
  const target = flagTarget(flag.id)
  const on = useValue({ target, capability: 'on' })?.data === true
  const command = useCommand(target)
  return (
    <div className={`flex items-center rounded-full text-sm ${on ? 'bg-amber-400/25 text-amber-200' : 'bg-neutral-800 text-neutral-300'}`}>
      <button
        role="switch"
        aria-checked={on}
        onClick={() => sendCommand(target, { on: 'toggle' })}
        className="flex items-center gap-1.5 py-1.5 pr-1 pl-3 hover:text-white"
      >
        <Svg path={on ? mdiFlag : mdiFlagOutline} className="size-4" />
        {flag.name}
        <CommandNote command={command} />
      </button>
      {onOpen && (
        <button onClick={onOpen} aria-label={`${flag.name} settings`} className="self-stretch pr-3 pl-1 text-neutral-500 hover:text-white">
          ⋯
        </button>
      )}
    </div>
  )
}
