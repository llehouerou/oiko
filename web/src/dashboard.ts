import { roles } from './roles'
import { aggregateTarget, catalogue, deviceTarget, flagTarget, fnOf } from './targets'
import { aggregateSummary, memberLeaves, tileCaps, tileShape, titled, type Shape } from './tiles'
import { arrange, defaultColumns, type Place } from './layout'
import type { Aggregate, Area, AutomationStatus, Capability, Device, Flag, Fn, Target } from './types'

// A Target's Tile, resolved once: everything about it that does not change while it is shown.
// Its Availability, Values and Commands are read live.
export interface TargetTile {
  subject: Target // the Device itself, the Aggregate or the Flag: its Availability dims the Tile
  name: string
  icon?: string
  detached: boolean
  health: { target: Target; cap: Capability }[] // the Device's and its Functions', shown only when wrong
  members?: Target[] // an Aggregate of lights' lights
  summary: string // the header line of a Tile of controls
  fns: { target: Target; fn: Fn }[] // the Functions it shows
  shape: Shape
}

// A Tile of the dashboard: key is its Target, which an Area may hide, or its Automation's id. In
// an Area, it has a place in its Layout.
export type DashboardTile = { key: string; label: string; place?: Place } & (
  { kind: 'manual'; automation: AutomationStatus } | ({ kind: 'target' } & TargetTile)
)

// An Area's part of the dashboard, or Others (no area). Its header shows its status, from its
// Area Aggregates (climate, doors, presence), and its light bar: its lone light's Tile, or its
// Aggregate of lights'. An Area's tiles come in its Layout's reading order.
export interface DashboardSection {
  area?: Area
  columns?: number // of an Area's Layout
  climate: Aggregate[]
  doors?: Aggregate
  presence?: Aggregate
  bar?: TargetTile
  tiles: DashboardTile[]
}

export const climateKinds = ['temperature', 'humidity', 'co2']

// The rows a tile takes in a Layout unless an Admin sets its height: a tile of readings, one
// per line of its cells (two to a line, three once two columns wide); any other, one.
function naturalRows(t: DashboardTile, width: number) {
  if (t.kind !== 'target' || t.shape.kind !== 'readings') return 1
  const perLine = t.shape.readings.length >= 3 && width >= 2 ? 3 : 2
  return Math.ceil(t.shape.readings.length / perLine)
}

// The dashboard: Flags without an Area as pills under its header, then a Section per Area in
// their order, then Others, if it holds anything. The Automations with a Manual trigger open
// Others.
export function dashboard(devices: Device[], aggregates: Aggregate[], flags: Flag[], areas: Area[], automations: AutomationStatus[]) {
  const manual = automations.filter((a) => a.manualTriggers?.length)
  const targets = catalogue(devices, aggregates, flags)
  // Every Device, Aggregate and Flag here is in the catalogue: both come from the same lists.
  const entry = (t: Target) => targets.get(t)!
  const resolved = (t: Omit<TargetTile, 'shape'>): TargetTile => ({ ...t, shape: tileShape(t.fns) })
  // A Device's Tile shows fns, its Functions in one Area; its health, all of them.
  const deviceTile = (device: Device, fns: Fn[]) =>
    resolved({
      subject: deviceTarget(device.id),
      name: device.name,
      icon: device.icon,
      detached: !!device.detached,
      health: [deviceTarget(device.id), ...(device.functions ?? []).map((fn) => deviceTarget(device.id, fn.key))].flatMap((target) => {
        const e = entry(target)
        return roles(e.kind, e.capabilities).health.map((cap) => ({ target, cap }))
      }),
      summary: fns.length === 1 && titled(fns[0]!) ? fns[0]!.kind : '',
      fns: fns.map((fn) => ({ target: deviceTarget(device.id, fn.key), fn })),
    })
  // An Aggregate's Tile has the controls of a single Function; Oiko relays its Commands to every member.
  const aggregateTile = (a: Aggregate) => {
    const e = entry(aggregateTarget(a.id))
    return resolved({
      subject: e.target,
      name: a.name,
      icon: a.icon,
      detached: false,
      health: [],
      members: e.kind === 'light' ? memberLeaves(a, aggregates) : undefined,
      summary: aggregateSummary(a),
      fns: [{ target: e.target, fn: fnOf(e) }],
    })
  }
  const flagTile = (f: Flag) => {
    const e = entry(flagTarget(f.id))
    return resolved({ subject: e.target, name: f.name, detached: false, health: [], summary: '', fns: [{ target: e.target, fn: fnOf(e) }] })
  }
  // The Area Aggregate of kind that Area id's header shows, if the Area holds any Function of that kind.
  const areaAggregate = (area: Area, kind: string) =>
    area.hiddenAggregates?.includes(kind) ? undefined : aggregates.find((a) => a.derived && a.area === area.id && a.kind === kind)
  // The tiles of Area id, '' for Others: a Device's tile there gathers its Functions there. The
  // Area's own Aggregates and lone light are in its header instead.
  const tiles = (id: string, lone?: Target): DashboardTile[] => [
    ...(id ? [] : manual.map((a) => ({ key: a.id, label: a.name, kind: 'manual' as const, automation: a }))),
    ...flags
      .filter((f) => id && f.area === id) // those without one are pills
      .map((f) => ({ key: flagTarget(f.id), label: f.name, kind: 'target' as const, ...flagTile(f) })),
    ...aggregates
      .filter((a) => !a.derived && (a.area ?? '') === id)
      .map((a) => ({ key: aggregateTarget(a.id), label: a.name, kind: 'target' as const, ...aggregateTile(a) })),
    ...devices.flatMap((device) => {
      const fns = (device.functions ?? []).filter(
        (fn) => (fn.area ?? device.area ?? '') === id && deviceTarget(device.id, fn.key) !== lone && tileCaps(fn).length > 0,
      )
      return fns.length ? [{ key: deviceTarget(device.id), label: device.name, kind: 'target' as const, ...deviceTile(device, fns) }] : []
    }),
  ]
  const section = (area: Area): DashboardSection => {
    const lights = areaAggregate(area, 'light')
    const lone = lights?.members.length === 1 ? lights.members[0] : undefined
    const bulb = lone
      ? devices.flatMap((device) => (device.functions ?? []).filter((fn) => deviceTarget(device.id, fn.key) === lone).map((fn) => deviceTile(device, [fn])))[0]
      : undefined
    const columns = area.columns ?? defaultColumns
    return {
      area,
      columns,
      climate: climateKinds.flatMap((k) => areaAggregate(area, k) ?? []),
      doors: areaAggregate(area, 'contact'),
      presence: areaAggregate(area, 'occupancy'),
      bar: bulb ?? (lights && aggregateTile(lights)),
      tiles: arrange(tiles(area.id, lone), columns, area.layout, naturalRows),
    }
  }
  const others: DashboardSection = { climate: [], tiles: tiles('') }
  return {
    pills: flags.filter((f) => !f.area),
    sections: [...areas.map(section), ...(others.tiles.length ? [others] : [])],
  }
}
