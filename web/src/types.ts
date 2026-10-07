// Mirrors the JSON of internal/home.

import type { Place } from './layout'

export type Availability = 'online' | 'offline' | 'unknown'

export interface Capability {
  key: string
  label: string
  type: 'binary' | 'numeric' | 'enum' | 'text' | 'composite' | 'list'
  unit?: string
  min?: number
  max?: number
  step?: number
  options?: string[]
  fields?: Capability[]
  access: { observable: boolean; settable: boolean; queryable: boolean }
  category: 'primary' | 'config' | 'diagnostic'
  stateless?: boolean
  counter?: boolean // a numeric running total, rising except on reset
}

export interface Fn {
  key: string
  kind: string
  area?: string // its Device's when absent
  capabilities: Capability[]
}

// A room or zone of the home; Areas come in the order an Admin set.
export interface Area {
  id: string
  name: string
  icon?: string // none: its header shows its Name alone
  hidden?: Target[] // tiles the dashboard folds away
  hiddenAggregates?: string[] // kinds whose Area Aggregate its header leaves out
  columns?: number // of its Layout; the dashboard's default when absent
  layout?: Placement[] // the tiles an Admin placed
}

// Where a tile sits in its Area's Layout: its first cell, from 0, and how many columns and rows it spans.
export interface Placement {
  tile: Target
  col: number
  row: number
  width: number
  height?: number // one row when absent
}

export interface Device {
  id: string
  name: string
  icon?: string // the dashboard's default when absent
  area?: string
  nativeAddress: string
  model?: string
  vendor?: string
  detached?: boolean
  functions: Fn[] | null
  capabilities: Capability[] | null
}

export type { Target } from './targets'
import type { Target } from './targets'

// A Capability of a Target.
export interface Ref {
  target: Target
  capability: string
}

export interface Aggregate {
  id: string
  name: string
  icon?: string // the dashboard's default when absent
  area?: string
  derived?: boolean // an Area Aggregate: it follows its Area, members and name included
  members: Target[] // Functions, Flags or nested Aggregates
  binary: 'any' | 'all'
  numeric: 'mean' | 'min' | 'max' | 'sum'
  kind?: string
  capabilities?: Capability[]
}

// A binary Function held by Oiko itself: kind 'flag', one Capability 'on'.
export interface Flag {
  id: string
  name: string
  area?: string
  kind: string
  capabilities: Capability[]
}

export interface Value {
  data: unknown
  at: string // when last reported
  since?: string // when it took its data: how long a state has held; an Event has none
}

export type CommandStatus = 'pending' | 'confirmed' | 'failed' | 'timed_out' | 'superseded'

export interface CommandState {
  id: string
  target: Target
  status: CommandStatus
  origin?: Origin
  error?: string
}

// Mirrors home.Origin: what issued a Command, by identity (ADR 0031): a
// Person, a Kiosk, a Program, a Step of an Automation in one of its Runs, or
// unknown (recorded before sign-in, or sent without credentials).
export type Origin = 'unknown' | { person: string } | { kiosk: string } | { program: string } | { automation: string; step: string; run: string }

// Mirrors home.CommandRecord: a Command as the Command history keeps it.
export interface CommandRecord extends CommandState {
  values?: Record<string, unknown>
  transition?: number
  time: string
}

// Mirrors history.LiveView: who watched a camera, from start to end.
export interface LiveView {
  target: Target
  origin: Origin
  start: string
  end: string
}

// A Recording as /api/recordings lists it: its duration in ms.
export interface Recording {
  id: string
  start: string
  duration: number
  trigger?: string
}

// Mirrors home.RunEnd.
export interface RunEnd {
  automation: string
  run: string
  time: string
  outcome: 'acted' | 'nothing' | 'error'
  trigger: RunTrigger
  commands: number // how many it issued
}

// Mirrors home.Trigger: what started a Run.
export interface RunTrigger {
  step: string // the trigger, or the timing Step whose deadline came due
  kind: string
  target: Target // '' for a time trigger
  capability: string
  value: unknown // the Value, or the Event for an event trigger
  time: string // when it happened, or was scheduled
  catchUp?: boolean
  skipped?: number
  by?: Origin // who started it, for a Manual trigger
}

// Mirrors automation.Trace: a Run's trigger, then each Step it reached, in order.
export interface Trace {
  run: string
  automation: string
  time: string
  outcome: RunEnd['outcome']
  trigger: RunTrigger
  steps: Reached[]
}

// Mirrors automation.Reached: a Step a Run went through, with its evidence.
export interface Reached {
  step: string
  fired: string[] | null
  read?: unknown
  at?: string // when the Value read was reported
  unknown?: boolean
  action?: string
  until?: string
  commands?: { target: Target; id?: string; values?: Record<string, unknown>; refused?: string; status?: CommandStatus }[]
  error?: string
  print?: string
  notification?: { title: string; message: string; recording?: { camera: Target; start: string } } // what a notify Step sent
}

// Mirrors automation.StepState: what a Step remembers between Runs.
export interface StepState {
  step: string
  kind: string
  deadline?: string
  passed?: { target?: Target; at: string }[]
  last?: string
  enabled?: boolean
  schedule?: { at: string; on: boolean }[]
  state?: unknown
}

// Mirrors home.AutomationStatus: an Automation's status and what its Tile shows. A Guest gets only
// those with Manual triggers, without reason, step or since.
export interface AutomationStatus {
  id: string
  name: string
  status: 'enabled' | 'disabled' | 'broken' | 'runaway'
  reason?: string // why it is broken
  step?: string // the Step it is broken at
  since?: string // when it became runaway
  manualTriggers?: { step: string; name: string }[] // in document order
}

// Mirrors build.Build, served with Oiko's Install and the type of each Bridge of the configuration.
export interface Build {
  version?: string // Oiko's; none when unknown
  types: { type: string; builtIn: boolean; package: string; module?: string; version?: string }[]
  install: 'nixos' | 'docker' | 'binary' // how this Oiko runs on its host (OIKO_INSTALL)
  bridges: Record<string, string> // each one's type, by name
}
// Mirrors release.Status: what the module proxy lists of a module built into Oiko, Oiko's first.
export interface ReleaseStatus {
  module: string
  current?: string // the version built in; none when unknown
  newest?: string // its newest Release that cannot break current; none when unknown
  breaking?: string // its newest Release that may break current, if any
  newer: boolean // newest is newer than current
}

export interface Snapshot {
  kind: 'snapshot'
  seq: number
  bridges: Record<string, boolean> // whether each is online, by name
  devices: Device[]
  aggregates: Aggregate[]
  flags: Flag[]
  areas: Area[]
  availability: Record<Target, Availability> // of each Device, Aggregate and Flag
  values: { ref: Ref; value: Value }[]
  events: { ref: Ref; value: Value }[]
  automations: AutomationStatus[]
  releases: ReleaseStatus[]
  dashboards?: CustomDashboard[] // those a Person sees, as they see them; none for a Kiosk or a Program
  list?: ListEntry[] // a Person's
}

// Sent again each time a check finds something else; not an Update.
export interface Releases {
  kind: 'releases'
  releases: ReleaseStatus[]
}

// The Dashboards a Person sees and their list, sent again whole each time either changes; not an
// Update.
export interface Dashboards {
  kind: 'dashboards'
  dashboards: CustomDashboard[]
  list: ListEntry[]
}

// Mirrors dashboard.Entry: a Dashboard in a Person's list, by id ('builtin' for the built-in one),
// and whether they hide it from their menu. The list holds every Dashboard they see, in their order.
export interface ListEntry {
  id: string
  hidden?: true
}

// Mirrors dashboard.Dashboard: a custom Dashboard, shared (an Admin's to edit, every Person's to
// see) or personal, its Sections on a Layout of its columns.
export interface CustomDashboard {
  id: string
  shared?: true
  owner?: string // the Person whose personal Dashboard it is
  name: string
  columns: number
  sections: CustomSection[]
}

// An Area's Section, by its id, or one of the Dashboard's own: an optional Name and Icon, and
// its Tiles on a Layout of its columns.
export type CustomSection = Place & (AreaSection | OwnSection)
export interface AreaSection {
  area: string
}
export interface OwnSection {
  area?: undefined
  id: string
  name?: string
  icon?: string
  columns: number
  tiles?: PlacedTile[]
}

// A Tile placed in an own Section: a Target's or an Automation's.
export type PlacedTile = Place & TileRef
export type TileRef = { target: Target; automation?: undefined } | { target?: undefined; automation: string }

export interface Update {
  seq: number
  kind:
    | 'devices'
    | 'aggregates'
    | 'flags'
    | 'areas'
    | 'value'
    | 'refresh'
    | 'event'
    | 'availability'
    | 'bridge'
    | 'command'
    | 'automations'
    | 'run'
    | 'deleted'
    | 'replaced'
  ref?: Ref
  value?: Value
  target?: Target // with availability: a Device, an Aggregate or a Flag
  availability?: Availability
  bridge?: string // with bridgeOnline
  bridgeOnline?: boolean
  command?: CommandState
  devices?: Device[]
  aggregates?: Aggregate[]
  flags?: Flag[]
  areas?: Area[]
  automations?: AutomationStatus[]
  run?: RunEnd
}
