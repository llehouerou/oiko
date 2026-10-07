import { roles } from './roles'
import { aggregateTarget, catalogue, deviceTarget, flagTarget, fnOf, parseTarget } from './targets'
import { aggregateSummary, memberLeaves, tileCaps, tileShape, titled, type Shape } from './tiles'
import { arrange, defaultColumns, type Place } from './layout'
import type {
  Aggregate,
  Area,
  AreaSection,
  AutomationStatus,
  Capability,
  CustomDashboard,
  CustomSection,
  Device,
  Flag,
  Fn,
  OwnSection,
  PlacedTile,
  Target,
  TileRef,
} from './types'

// The key that names a Section on its Dashboard's Layout: an Area's by its Area, an own one by its id.
export const sectionKey = (s: AreaSection | OwnSection) => (s.area !== undefined ? `area:${s.area}` : `own:${s.id}`)

// The key that names a Tile on its own Section's Layout: its Target, or its Automation's id.
export const tileKey = (t: TileRef) => t.target ?? `automation:${t.automation}`

// A Tile to pick for an own Section, under its Area: what it is, and whether the Section holds it.
// A Device with several Functions that have a Tile offers each alone, in functions.
export interface TileChoice {
  key: string
  tile: TileRef
  name: string
  kind: 'Device' | 'Function' | 'Aggregate' | 'Flag' | 'Automation'
  area?: string // its Area's id
  taken: boolean
  functions?: TileChoice[]
}

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
  // A Device's Tile under name: it shows fns, some of its Functions (those in one Area, or one
  // Function placed alone); its health, all of them.
  const deviceTile = (device: Device, fns: Fn[], name = device.name) =>
    resolved({
      subject: deviceTarget(device.id),
      name,
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
    ...(id ? [] : manual.map((a) => ({ key: tileKey({ automation: a.id }), label: a.name, kind: 'manual' as const, automation: a }))),
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
      key: sectionKey({ area: area.id }),
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
  // its Functions that has a Tile, whatever their Areas; a Function's, that Function alone.
  const placed = (t: PlacedTile): DashboardTile[] => {
    const key = tileKey(t)
    if (t.automation !== undefined) return manual.filter((a) => a.id === t.automation).map((a) => ({ key, label: a.name, kind: 'manual', automation: a }))
    const p = parseTarget(t.target)
    const tile = (label: string, tt: TargetTile): DashboardTile[] => [{ key, label, kind: 'target', ...tt }]
    if (p?.kind === 'device') {
      return devices
        .filter((d) => d.id === p.id)
        .flatMap((d) => {
          const fns = (d.functions ?? []).filter((fn) => hasTile(fn) && (!p.function || fn.key === p.function))
          if (!fns.length) return []
          const name = p.function ? functionName(d, p.function) : d.name
          return tile(name, deviceTile(d, fns, name))
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
    return [{ key: sectionKey(s), own: { name: s.name, icon: s.icon }, columns: s.columns, climate: [], tiles }]
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
    // Every Tile of the home whose name holds query, by Area in their order, then those without
    // one; taken are the keys of those the Section holds already. A Device has a Tile if any of its
    // Functions has one, and offers each alone under it if several do, searched by its name; an
    // Aggregate, an Area's included, a Flag and an Automation with a Manual trigger always do.
    choices: (query: string, taken: string[]) => {
      const q = query.trim().toLowerCase()
      const matches = (name: string) => name.toLowerCase().includes(q)
      const choice = (tile: TileRef, name: string, kind: TileChoice['kind'], area?: string): TileChoice => ({
        key: tileKey(tile),
        tile,
        name,
        kind,
        area,
        taken: taken.includes(tileKey(tile)),
      })
      const deviceChoice = (d: Device): TileChoice[] => {
        const fns = (d.functions ?? []).filter(hasTile)
        const c = choice({ target: deviceTarget(d.id) }, d.name, 'Device', d.area)
        if (fns.length < 2) return fns.length ? [c] : []
        return [{ ...c, functions: fns.map((fn) => choice({ target: deviceTarget(d.id, fn.key) }, functionName(d, fn.key), 'Function', d.area)) }]
      }
      // A Device is found by its name or one of its Functions'; it then offers those found.
      const found = (c: TileChoice): TileChoice[] => {
        const functions = c.functions?.filter((f) => matches(f.name))
        return matches(c.name) || functions?.length ? [{ ...c, functions }] : []
      }
      const all = [
        ...devices.flatMap(deviceChoice),
        ...aggregates.map((a) => choice({ target: aggregateTarget(a.id) }, a.name, 'Aggregate', a.area)),
        ...flags.map((f) => choice({ target: flagTarget(f.id) }, f.name, 'Flag', f.area)),
        ...manual.map((a) => choice({ automation: a.id }, a.name, 'Automation')),
      ]
        .flatMap(found)
        .sort((a, b) => a.name.localeCompare(b.name))
      const inArea = (c: TileChoice, area?: Area) => (area ? c.area === area.id : !areas.some((a) => a.id === c.area))
      return [...areas, undefined].flatMap((area) => {
        const tiles = all.filter((c) => inArea(c, area))
        return tiles.length ? [{ area, tiles }] : []
      })
    },
  }
}

// The place of occupant key at spot, on a Layout.
const at = (key: string, { col, row, width, height }: Place) => ({ key, col, row, width, height })

// Whether a Function shows on a Device's Tile: a camera has a Tile though it has no Capability.
const hasTile = (fn: Fn) => tileCaps(fn).length > 0 || fn.kind === 'camera'

// A Function's name on its own Tile: its Device's, then its key.
const functionName = (d: Device, key: string) => `${d.name} · ${key}`

// The custom Dashboard hash opens, #dashboard/<id>, if it is one of dashboards; undefined for the
// built-in one, which any other address opens.
export function shown(hash: string, dashboards: CustomDashboard[]) {
  const id = hash.match(/^#dashboard\/(.+)$/)?.[1]
  return dashboards.find((d) => d.id === id)
}
