// The web client's own Icons, each under the name Oiko keeps for it: a light's, for its Device or
// Aggregate, and an Area's. Material Design Icons; a light's filled while it is on and outlined
// while it is off where the set has both.

import {
  mdiBalcony,
  mdiBathtub,
  mdiBedKing,
  mdiBookshelf,
  mdiBulkheadLight,
  mdiCancel,
  mdiDesk,
  mdiDoor,
  mdiDumbbell,
  mdiFlower,
  mdiFridge,
  mdiGamepadVariant,
  mdiGarage,
  mdiGrill,
  mdiHanger,
  mdiHome,
  mdiHomeRoof,
  mdiPiano,
  mdiPool,
  mdiShower,
  mdiSilverwareForkKnife,
  mdiSofa,
  mdiStairs,
  mdiStove,
  mdiTableChair,
  mdiTeddyBear,
  mdiTelevision,
  mdiToilet,
  mdiTools,
  mdiTree,
  mdiWardrobe,
  mdiWashingMachine,
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

// A picture to pick: its Icon name, what it shows, and its Material Design path.
interface Choice {
  name: string
  label: string
  path: string
}

// An Area's Icon pictures what it is for; it has none by default.
const areaIcons: Choice[] = [
  { name: 'home', label: 'House', path: mdiHome },
  { name: 'sofa', label: 'Sofa', path: mdiSofa },
  { name: 'television', label: 'Television', path: mdiTelevision },
  { name: 'table-chair', label: 'Dining table', path: mdiTableChair },
  { name: 'silverware-fork-knife', label: 'Cutlery', path: mdiSilverwareForkKnife },
  { name: 'stove', label: 'Stove', path: mdiStove },
  { name: 'fridge', label: 'Fridge', path: mdiFridge },
  { name: 'bed-king', label: 'Bed', path: mdiBedKing },
  { name: 'teddy-bear', label: 'Teddy bear', path: mdiTeddyBear },
  { name: 'wardrobe', label: 'Wardrobe', path: mdiWardrobe },
  { name: 'hanger', label: 'Hanger', path: mdiHanger },
  { name: 'shower', label: 'Shower', path: mdiShower },
  { name: 'bathtub', label: 'Bathtub', path: mdiBathtub },
  { name: 'toilet', label: 'Toilet', path: mdiToilet },
  { name: 'washing-machine', label: 'Washing machine', path: mdiWashingMachine },
  { name: 'desk', label: 'Desk', path: mdiDesk },
  { name: 'bookshelf', label: 'Bookshelf', path: mdiBookshelf },
  { name: 'gamepad-variant', label: 'Gamepad', path: mdiGamepadVariant },
  { name: 'dumbbell', label: 'Dumbbell', path: mdiDumbbell },
  { name: 'piano', label: 'Piano', path: mdiPiano },
  { name: 'stairs', label: 'Stairs', path: mdiStairs },
  { name: 'door', label: 'Door', path: mdiDoor },
  { name: 'home-roof', label: 'Roof', path: mdiHomeRoof },
  { name: 'garage', label: 'Garage', path: mdiGarage },
  { name: 'tools', label: 'Tools', path: mdiTools },
  { name: 'balcony', label: 'Balcony', path: mdiBalcony },
  { name: 'flower', label: 'Flower', path: mdiFlower },
  { name: 'tree', label: 'Tree', path: mdiTree },
  { name: 'pool', label: 'Pool', path: mdiPool },
  { name: 'grill', label: 'Grill', path: mdiGrill },
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

// An Area's Icon, or nothing when it has none, or none this web client knows.
export function AreaIcon({ icon, className }: { icon?: string; className: string }) {
  const path = areaIcons.find((i) => i.name === icon)?.path
  return path ? <Svg path={path} className={className} /> : null
}

// A grid of Icons to pick one from, current highlighted.
function IconGrid({ icons, current, onPick }: { icons: Choice[]; current: string; onPick: (name: string) => void }) {
  return (
    <section className="space-y-3">
      <h3 className="text-xs font-semibold tracking-wide text-neutral-500 uppercase">Icon</h3>
      <div className="grid grid-cols-[repeat(auto-fill,minmax(2.75rem,1fr))] gap-1">
        {icons.map((i) => (
          <button
            type="button"
            key={i.name}
            title={i.label}
            aria-label={i.label}
            aria-pressed={i.name === current}
            onClick={() => onPick(i.name)}
            className={`grid aspect-square place-items-center rounded-lg ${i.name === current ? 'bg-amber-400 text-neutral-900' : 'bg-neutral-800 text-neutral-300 hover:bg-neutral-700'}`}
          >
            <Svg path={i.path} className="size-6" />
          </button>
        ))}
      </div>
    </section>
  )
}

// Sets the Icon of a light's Device or Aggregate at once. Picking the default
// clears it, so the light follows the default.
export function IconPicker({ target, icon, group, onError }: { target: Target; icon?: string; group: boolean; onError: (e: string | null) => void }) {
  return (
    <IconGrid
      icons={lightIcons.map((i) => ({ ...i, path: i.on }))}
      current={find(icon, group).name}
      onPick={async (name) => onError(await edit('PUT', 'icon', { target, icon: name === fallback(group) ? '' : name }))}
    />
  )
}

// Picks an Area's Icon, or an own Section's; the first choice, '', is none.
export function AreaIconPicker({ icon, onPick }: { icon: string; onPick: (icon: string) => void }) {
  return <IconGrid icons={[{ name: '', label: 'No icon', path: mdiCancel }, ...areaIcons]} current={icon} onPick={onPick} />
}
