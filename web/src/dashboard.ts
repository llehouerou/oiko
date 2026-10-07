import { roles } from './roles'
import { aggregateTarget, catalogue, deviceTarget, flagTarget, fnOf, parseTarget } from './targets'
import { aggregateSummary, memberLeaves, tileCaps, tileShape, titled, type Shape } from './tiles'
import { arrange, defaultColumns, type Place } from './layout'
import type { Aggregate, Area, AutomationStatus, Capability, CustomDashboard, CustomSection, Device, Flag, Fn, PlacedTile, Target } from './types'

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

// A Section of a Dashboard: an Area's, one of a custom Dashboard's own, or Others (neither). Key
// names it on its Dashboard. An Area's header shows its status, from its Area Aggregates (climate,
// doors, presence), and its light bar: its lone light's Tile, or its Aggregate of lights'. An
// Area's tiles come in its Layout's reading order, an own Section's in its own. On a custom
// Dashboard, a Section has its place on the Dashboard's Layout.
export interface DashboardSection {
  key: string
  area?: Area
  own?: { name?: string; icon?: string }
  columns?: number // of an Area's Layout, or an own Section's
  climate: Aggregate[]
  doors?: Aggregate
  presence?: Aggregate
  bar?: TargetTile
  tiles: DashboardTile[]
  place?: Place
}

export const climateKinds = ['temperature', 'humidity', 'co2']

// The rows a tile takes in a Layout unless an Admin sets its height: a tile of readings, one
// per line of its cells (two to a line, three once two columns wide); any other, one.
function naturalRows(t: DashboardTile, width: number) {
  if (t.kind !== 'target' || t.shape.kind !== 'readings') return 1
  const perLine = t.shape.readings.length >= 3 && width >= 2 ? 3 : 2
  return Math.ceil(t.shape.readings.length / perLine)
}

// The built-in Dashboard: Flags without an Area as pills under its header, then a Section per
// Area in their order, then Others, if it holds anything. The Automations with a Manual trigger
// open Others. custom draws a custom Dashboard from the same home.
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
      const fns = (device.functions ?? []).filter((fn) => (fn.area ?? device.area ?? '') === id && deviceTarget(device.id, fn.key) !== lone && hasTile(fn))
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
      key: `area:${area.id}`,
      area,
      columns,
      climate: climateKinds.flatMap((k) => areaAggregate(area, k) ?? []),
      doors: areaAggregate(area, 'contact'),
      presence: areaAggregate(area, 'occupancy'),
      bar: bulb ?? (lights && aggregateTile(lights)),
      tiles: arrange(
        tiles(area.id, lone),
        columns,
        area.layout?.map(({ tile, ...p }) => ({ key: tile, ...p })),
        naturalRows,
      ),
    }
  }
  const others: DashboardSection = { key: 'others', climate: [], tiles: tiles('') }
  const sections = areas.map(section)
  // The Tile of t placed in an own Section, if it shows anything: a Device's shows every one of
  // its Functions that has a Tile, whatever their Areas.
  const placed = (t: PlacedTile): DashboardTile[] => {
    if (t.automation !== undefined) return manual.filter((a) => a.id === t.automation).map((a) => ({ key: a.id, label: a.name, kind: 'manual', automation: a }))
    const p = parseTarget(t.target)
    const tile = (label: string, tt: TargetTile): DashboardTile[] => [{ key: t.target, label, kind: 'target', ...tt }]
    if (p?.kind === 'device' && !p.function) {
      return devices
        .filter((d) => d.id === p.id)
        .flatMap((d) => {
          const fns = (d.functions ?? []).filter(hasTile)
          return fns.length ? tile(d.name, deviceTile(d, fns)) : []
        })
    }
    if (p?.kind === 'aggregate') return aggregates.filter((a) => a.id === p.id).flatMap((a) => tile(a.name, aggregateTile(a)))
    if (p?.kind === 'flag') return flags.filter((f) => f.id === p.id).flatMap((f) => tile(f.name, flagTile(f)))
    return []
  }
  // A custom Dashboard's Section: an Area's as the built-in Dashboard shows it, or an own one, not
  // shown while none of its Tiles shows anything, but while editing.
  const customSection = (s: CustomSection, editing: boolean): DashboardSection[] => {
    if (s.area !== undefined) return sections.filter((a) => a.area!.id === s.area)
    const own = (s.tiles ?? []).flatMap((t) => placed(t).map((d) => [d, t] as const))
    if (!own.length && !editing) return []
    const tiles = arrange(
      own.map(([d]) => d),
      s.columns,
      own.map(([d, t]) => at(d.key, t)),
      naturalRows,
    )
    return [{ key: `own:${s.id}`, own: { name: s.name, icon: s.icon }, columns: s.columns, climate: [], tiles }]
  }
  return {
    pills: flags.filter((f) => !f.area),
    sections: [...sections, ...(others.tiles.length ? [others] : [])],
    // The Sections of custom Dashboard d, each in its place on its Layout, in reading order. What
    // no longer exists shows nothing, its cells left empty.
    custom: (d: CustomDashboard, editing = false) => {
      const shown = d.sections.flatMap((s) => customSection(s, editing).map((c) => [c, s] as const))
      return arrange(
        shown.map(([c]) => c),
        d.columns,
        shown.map(([c, s]) => at(c.key, s)),
      )
    },
  }
}

// The place of occupant key at spot, on a Layout.
const at = (key: string, { col, row, width, height }: Place) => ({ key, col, row, width, height })

// Whether a Function shows on a Device's Tile: a camera has a Tile though it has no Capability.
const hasTile = (fn: Fn) => tileCaps(fn).length > 0 || fn.kind === 'camera'

// The custom Dashboard hash opens, #dashboard/<id>, if it is one of dashboards; undefined for the
// built-in one, which any other address opens.
export function shown(hash: string, dashboards: CustomDashboard[]) {
  const id = hash.match(/^#dashboard\/(.+)$/)?.[1]
  return dashboards.find((d) => d.id === id)
}
