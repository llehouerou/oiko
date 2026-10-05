// The pictures a light's tile may show, each under the Icon name Oiko keeps
// for its Device or Aggregate: Material Design Icons, filled while the light
// is on and outlined while it is off where the set has both.

import {
  mdiBulkheadLight,
  mdiCeilingFanLight,
  mdiCeilingLight,
  mdiCeilingLightMultiple,
  mdiCeilingLightMultipleOutline,
  mdiCeilingLightOutline,
  mdiChandelier,
  mdiCoachLamp,
  mdiDeskLamp,
  mdiDeskLampOn,
  mdiFloorLamp,
  mdiFloorLampOutline,
  mdiFloorLampTorchiere,
  mdiFloorLampTorchiereOutline,
  mdiGlobeLight,
  mdiGlobeLightOutline,
  mdiLamp,
  mdiLampOutline,
  mdiLedStripVariant,
  mdiLightRecessed,
  mdiLightbulb,
  mdiLightbulbGroup,
  mdiLightbulbGroupOutline,
  mdiLightbulbOutline,
  mdiLightbulbSpot,
  mdiOutdoorLamp,
  mdiStringLights,
  mdiTrackLight,
  mdiWallSconce,
  mdiWallSconceOutline,
  mdiWallSconceRound,
  mdiWallSconceRoundOutline,
} from '@mdi/js'
import { edit } from './store'
import type { Target } from './types'

const lightIcons: { name: string; label: string; on: string; off?: string }[] = [
  { name: 'lightbulb', label: 'Bulb', on: mdiLightbulb, off: mdiLightbulbOutline },
  { name: 'lightbulb-group', label: 'Bulbs', on: mdiLightbulbGroup, off: mdiLightbulbGroupOutline },
  { name: 'lightbulb-spot', label: 'Spot bulb', on: mdiLightbulbSpot },
  { name: 'ceiling-light', label: 'Ceiling light', on: mdiCeilingLight, off: mdiCeilingLightOutline },
  { name: 'ceiling-light-multiple', label: 'Ceiling lights', on: mdiCeilingLightMultiple, off: mdiCeilingLightMultipleOutline },
  { name: 'light-recessed', label: 'Recessed light', on: mdiLightRecessed },
  { name: 'track-light', label: 'Track light', on: mdiTrackLight },
  { name: 'chandelier', label: 'Chandelier', on: mdiChandelier },
  { name: 'globe-light', label: 'Globe light', on: mdiGlobeLight, off: mdiGlobeLightOutline },
  { name: 'ceiling-fan-light', label: 'Ceiling fan light', on: mdiCeilingFanLight },
  { name: 'lamp', label: 'Table lamp', on: mdiLamp, off: mdiLampOutline },
  { name: 'desk-lamp', label: 'Desk lamp', on: mdiDeskLampOn, off: mdiDeskLamp },
  { name: 'floor-lamp', label: 'Floor lamp', on: mdiFloorLamp, off: mdiFloorLampOutline },
  { name: 'floor-lamp-torchiere', label: 'Torchiere', on: mdiFloorLampTorchiere, off: mdiFloorLampTorchiereOutline },
  { name: 'wall-sconce', label: 'Wall sconce', on: mdiWallSconce, off: mdiWallSconceOutline },
  { name: 'wall-sconce-round', label: 'Round wall sconce', on: mdiWallSconceRound, off: mdiWallSconceRoundOutline },
  { name: 'bulkhead-light', label: 'Bulkhead light', on: mdiBulkheadLight },
  { name: 'outdoor-lamp', label: 'Outdoor lamp', on: mdiOutdoorLamp },
  { name: 'coach-lamp', label: 'Coach lamp', on: mdiCoachLamp },
  { name: 'led-strip-variant', label: 'LED strip', on: mdiLedStripVariant },
  { name: 'string-lights', label: 'String lights', on: mdiStringLights },
]

// A light shows a bulb by default, an Aggregate of lights several.
const fallback = (group: boolean) => (group ? 'lightbulb-group' : 'lightbulb')

function find(icon: string | undefined, group: boolean) {
  return lightIcons.find((i) => i.name === icon) ?? lightIcons.find((i) => i.name === fallback(group))!
}

// One Material Design Icon, in the text colour.
export function Svg({ path, className }: { path: string; className: string }) {
  return (
    <svg viewBox="0 0 24 24" className={className} aria-hidden>
      <path d={path} fill="currentColor" />
    </svg>
  )
}

export function LightIcon({ icon, group, lit, className }: { icon?: string; group: boolean; lit: boolean; className: string }) {
  const i = find(icon, group)
  return <Svg path={lit ? i.on : (i.off ?? i.on)} className={className} />
}

// Sets the Icon of a light's Device or Aggregate at once. Picking the default
// clears it, so the light follows the default.
export function IconPicker({ target, icon, group, onError }: { target: Target; icon?: string; group: boolean; onError: (e: string | null) => void }) {
  const current = find(icon, group).name
  return (
    <section className="space-y-3">
      <h3 className="text-xs font-semibold tracking-wide text-neutral-500 uppercase">Icon</h3>
      <div className="grid grid-cols-[repeat(auto-fill,minmax(2.75rem,1fr))] gap-1">
        {lightIcons.map((i) => (
          <button
            key={i.name}
            title={i.label}
            aria-label={i.label}
            aria-pressed={i.name === current}
            onClick={async () => onError(await edit('PUT', 'icon', { target, icon: i.name === fallback(group) ? '' : i.name }))}
            className={`grid aspect-square place-items-center rounded-lg ${i.name === current ? 'bg-amber-400 text-neutral-900' : 'bg-neutral-800 text-neutral-300 hover:bg-neutral-700'}`}
          >
            <Svg path={i.on} className="size-6" />
          </button>
        ))}
      </div>
    </section>
  )
}
