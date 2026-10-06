import { useEffect, useRef, useState, type CSSProperties, type FormEvent, type PointerEvent, type ReactNode } from 'react'
import {
  connect,
  edit,
  runManual,
  sendCommand,
  useAggregates,
  useCatalogue,
  useAutomationStatuses,
  useAreas,
  useFlags,
  useAvailability,
  useCommand,
  useControl,
  useDevices,
  useLastEvent,
  useNow,
  useValue,
  useOnCount,
} from './store'
import type { Aggregate, Area, AutomationStatus, Capability, CommandState, Device, Flag, Fn, Ref, Target } from './types'
import { aggregateTarget, deviceTarget, flagTarget, parseTarget, targetKind } from './targets'
import { Automations } from './automation/Automations'
import { confirm } from './confirm'
import { titleIfTruncated } from './truncated'
import { ColorPicker, display, format, RangeSlider, Switch, type HS } from './controls'
import {
  mdiArrowExpandHorizontal,
  mdiArrowExpandVertical,
  mdiCheck,
  mdiChevronDown,
  mdiCogOutline,
  mdiDrag,
  mdiDoorClosed,
  mdiDoorOpen,
  mdiBrightness5,
  mdiGauge,
  mdiMoleculeCo2,
  mdiThermometer,
  mdiVolumeHigh,
  mdiWaterPercent,
  mdiWeatherRainy,
  mdiMotionSensor,
  mdiAlarmLight,
  mdiAlarmLightOutline,
  mdiGestureTapButton,
  mdiPowerPlug,
  mdiPowerPlugOutline,
  mdiFlag,
  mdiFlagOutline,
  mdiMotionSensorOff,
  mdiSmokeDetector,
  mdiSmokeDetectorAlert,
  mdiWaterAlert,
  mdiWaterOutline,
} from '@mdi/js'
import { createPortal } from 'react-dom'
import { IconPicker, LightIcon, Svg } from './icons'
import { ChartsShown, MiniChart } from './MiniChart'
import { Timeline } from './Timeline'
import { HistorySheet } from './HistorySheet'
import { co2Level, motion, roles, turns, type Roles } from './roles'
import { aggregateSummary, titled, unwell, type Part, type Shape } from './tiles'
import { dashboard, type DashboardTile, type TargetTile } from './dashboard'
import { maxColumns, maxRows, move, placements, reflow, resize, rows, stored, type Arranged, type Place } from './layout'
import { AppBar, ArrangeSetting, ChartsSetting, CreateButton } from './AppBar'
import { ReleaseBanner } from './About'
import { Setup } from './Setup'
import { SignIn } from './SignIn'
import { Programs } from './Programs'
import { Account } from './Account'
import { screen, useAllows, useMe } from './access'
import {
  DndContext,
  PointerSensor,
  pointerWithin,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type CollisionDetection,
  type DragEndEvent,
} from '@dnd-kit/core'

export function App() {
  const me = useMe()
  const [page, setPage] = useState(location.hash)
  const [setup, setSetup] = useState(location.pathname === '/setup') // a Setup link, its secret as the hash
  useEffect(() => {
    const follow = () => setPage(location.hash)
    addEventListener('hashchange', follow)
    return () => removeEventListener('hashchange', follow)
  }, [])
  const signedIn = !!me?.identity
  useEffect(() => (signedIn ? connect() : undefined), [signedIn])
  if (setup) {
    // Done: the dashboard, the secret out of the address and the browser's history.
    const done = () => (history.replaceState(null, '', '/'), setPage(''), setSetup(false))
    return <Setup onDone={done} />
  }
  switch (screen(me, page)) {
    case null:
      return null
    case 'sign-in':
      return <SignIn me={me!} />
    case 'programs':
      return <Programs />
    case 'account':
      return <Account />
    case 'automations':
      return <Automations />
    case 'history':
      return <Timeline />
    case 'home':
      return <Home />
  }
}

function Home() {
  const devices = useDevices()
  const aggregates = useAggregates()
  const flags = useFlags()
  const areas = useAreas()
  const automations = useAutomationStatuses() // their Tiles: those with a Manual trigger show
  const member = useAllows('member') // reads the home's past: its tiles' charts
  const admin = useAllows('admin') // edits the home: its panels, arranging it, creating in it
  const [toast, setToast] = useState<{ text: string; error?: boolean } | null>(null)
  // Whether the tiles show their 24 h charts; each browser remembers its choice.
  const [charts, setCharts] = useState(() => localStorage.getItem('oiko.charts') !== 'hidden')
  const toggleCharts = () => {
    localStorage.setItem('oiko.charts', charts ? 'hidden' : 'shown')
    setCharts(!charts)
  }
  useEffect(() => {
    if (!toast) return
    const t = setTimeout(() => setToast(null), 6000)
    return () => clearTimeout(t)
  }, [toast])
  // A Device's, an Aggregate's, a Flag's or an Area's ID, or 'new-aggregate' / 'new-flag' / 'new-area'
  // while creating one; back reopens what it was opened from, e.g. a light's sheet.
  const [open, setOpen] = useState<{ id: string; back?: () => void } | null>(null)
  const openId = open?.id
  const setOpenId = (id: string, back?: () => void) => setOpen({ id, back })
  const close = () => setOpen(null)
  const reopen = open?.back
  const back = reopen && (() => (close(), reopen()))
  const openDevice = devices.find((d) => d.id === openId)
  const openAggregate = aggregates.find((a) => a.id === openId)
  const openFlag = flags.find((f) => f.id === openId)
  const openArea = areas.find((a) => a.id === openId)
  const { pills, sections } = dashboard(devices, aggregates, flags, areas, automations)
  // A Tile's ⋯ opens its Device's, Aggregate's or Flag's panel, to an Admin.
  const opener = (t: TargetTile) => (admin ? (back?: () => void) => setOpenId(parseTarget(t.subject)!.id, back) : undefined)
  const tile = (t: DashboardTile): TileNode => ({
    ...t,
    node: t.kind === 'manual' ? <ManualTile key={t.key} automation={t.automation} onResult={setToast} /> : <Tile key={t.key} tile={t} onOpen={opener(t)} />,
  })
  // The sections this browser folds, by Area id ('' for Others).
  const [collapsed, setCollapsed] = useState<string[]>(() => JSON.parse(localStorage.getItem('oiko.collapsed') ?? '[]'))
  const collapse = (id: string) => {
    const next = collapsed.includes(id) ? collapsed.filter((c) => c !== id) : [...collapsed, id]
    localStorage.setItem('oiko.collapsed', JSON.stringify(next))
    setCollapsed(next)
  }
  // Whether an Admin is arranging the dashboard: the Areas' order and their Layouts.
  const [arranging, setArranging] = useState(false)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))
  const report = (err: string | null) => err && setToast({ text: err, error: true })
  const saveLayout = async (area: string, columns: number, layout: Arranged[]) =>
    report(await edit('PUT', `areas/${area}/layout`, { columns, layout: stored(layout) }))
  // A dragged Area takes the place of the one it lands on; a dragged tile, the cell it lands on.
  const dropped = async ({ active, over }: DragEndEvent) => {
    const from = active.data.current
    const to = over?.data.current
    if (!from || !to) return
    if (from.type === 'area') {
      if (to.area === from.area) return
      const at = areas.findIndex((a) => a.id === to.area)
      const ids = areas.map((a) => a.id).filter((id) => id !== from.area)
      ids.splice(at, 0, from.area)
      return report(await edit('PUT', 'areas', { order: ids }))
    }
    const s = sections.find((s) => s.area?.id === from.area)
    if (s?.columns) saveLayout(from.area, s.columns, move(placements(s.tiles), from.tile, to.col, to.row, s.columns))
  }
  return (
    <>
      <AppBar
        page="#"
        settings={
          member && (
            <>
              <ChartsSetting charts={charts} onCharts={toggleCharts} />
              {admin && <ArrangeSetting onArrange={() => setArranging(true)} />}
            </>
          )
        }
      />
      <main className="mx-auto max-w-[120rem] space-y-6 p-4 pb-24 lg:px-8">
        {admin && <ReleaseBanner />}
        {pills.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {pills.map((f) => (
              <FlagPill key={f.id} flag={f} onOpen={admin ? () => setOpenId(f.id) : undefined} />
            ))}
          </div>
        )}
        <ChartsShown value={charts}>
          <DndContext sensors={sensors} collisionDetection={landing} onDragEnd={dropped}>
            {/* A wide screen sets the sections in two columns, alternately: the first on the left, the
                second on the right, and so on, whatever their heights. Narrower, the columns melt
                away and the sections come one under another, in their order. */}
            <div className="flex flex-col gap-6 xl:flex-row xl:items-start">
              {[0, 1].map((column) => (
                <div key={column} className="contents xl:flex xl:min-w-0 xl:flex-1 xl:flex-col xl:gap-6">
                  {sections.map((s, i) => {
                    const id = s.area?.id ?? ''
                    return (
                      i % 2 === column && (
                        <div key={id} style={{ order: i }}>
                          <Section
                            title={s.area?.name ?? (areas.length ? 'Others' : undefined)}
                            collapsed={collapsed.includes(id)}
                            onCollapse={() => collapse(id)}
                            onSettings={admin && s.area ? () => setOpenId(id) : undefined}
                            status={
                              <>
                                <Climate aggregates={s.climate} />
                                {s.presence && <AreaState aggregate={s.presence} />}
                                {s.doors && <AreaState aggregate={s.doors} />}
                              </>
                            }
                            bar={s.bar && <Tile tile={s.bar} compact onOpen={opener(s.bar)} />}
                            columns={s.columns}
                            tiles={s.tiles.map(tile)}
                            hidden={s.area?.hidden}
                            arranging={arranging && s.area ? { area: id, onLayout: (columns, layout) => saveLayout(id, columns, layout) } : undefined}
                          />
                        </div>
                      )
                    )
                  })}
                </div>
              ))}
            </div>
          </DndContext>
        </ChartsShown>
      </main>
      {arranging ? (
        <button
          onClick={() => setArranging(false)}
          className="fixed right-6 bottom-6 z-10 flex h-14 items-center gap-2 rounded-2xl bg-amber-400 px-5 font-medium text-neutral-950 shadow-xl shadow-amber-500/20 hover:bg-amber-300"
        >
          <Svg path={mdiCheck} className="size-6" />
          Done
        </button>
      ) : (
        admin && <CreateButton onCreate={(what) => setOpenId(what)} />
      )}
      {openDevice && <DevicePanel key={openDevice.id} device={openDevice} devices={devices} onClose={close} onBack={back} />}
      {openAggregate && <AggregatePanel key={openAggregate.id} aggregate={openAggregate} aggregates={aggregates} onClose={close} onBack={back} />}
      {openId === 'new-aggregate' && <AggregatePanel aggregates={aggregates} onClose={close} />}
      {openFlag && <FlagPanel key={openFlag.id} flag={openFlag} onClose={close} />}
      {openId === 'new-flag' && <FlagPanel onClose={close} />}
      {openArea && (
        <AreaPanel key={openArea.id} area={openArea} areas={areas} devices={devices} tiles={sections.find((s) => s.area === openArea)?.tiles} onClose={close} />
      )}
      {openId === 'new-area' && <AreaPanel areas={areas} devices={devices} onClose={close} />}
      {toast && (
        <button
          onClick={() => setToast(null)}
          className={`fixed bottom-24 left-1/2 z-20 max-w-xl -translate-x-1/2 rounded px-3 py-2 text-left text-sm shadow-lg ${toast.error ? 'bg-red-950 text-red-200' : 'bg-neutral-800'}`}
        >
          {toast.text}
        </button>
      )}
    </>
  )
}

// A modal panel; it closes on ✕, Escape or a click on the backdrop. With onBack,
// ←, Escape and a phone's back gesture go back instead.
function Panel({ title, onClose, onBack, children }: { title: ReactNode; onClose: () => void; onBack?: () => void; children: ReactNode }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const backing = useRef(false)
  useEffect(() => dialog.current?.showModal(), [])
  return (
    <dialog
      ref={dialog}
      onCancel={() => (backing.current = true)}
      onClose={() => (backing.current && onBack ? onBack() : onClose())}
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

const clamp = (f: number) => Math.min(1, Math.max(0, f))

// A bar's icon, off and on, outside lights: those have their own, picked per Device.
const barIcons: Record<string, [off: string, on: string]> = {
  switch: [mdiPowerPlugOutline, mdiPowerPlug],
  alarm: [mdiAlarmLightOutline, mdiAlarmLight],
  flag: [mdiFlagOutline, mdiFlag],
}

// A light's, a plug's or a siren's tile is a single control, a bar: a tap toggles it, a sideways
// drag sets a light's brightness from where it was, a vertical one scrolls the page. ⋯ opens its
// other controls and readings, and from there its Device's or Aggregate's panel, which goes back
// to them.
function BarTile({
  name,
  icon,
  members,
  note,
  badges,
  target,
  toggle: key,
  fn,
  dimmed,
  compact = false,
  onSettings,
}: {
  name: string
  icon?: string
  members?: Target[] // an Aggregate's lights
  note: string // detached, offline…
  badges?: ReactNode // its health, when wrong
  target: Target
  toggle: string // its main control, an on/off
  fn: Fn
  dimmed: boolean
  compact?: boolean // in an Area's header: the bar alone, its name only on its sheet
  onSettings?: (back: () => void) => void // an Admin's: its Device's or Aggregate's panel
}) {
  const cap = fn.capabilities.find((c) => c.key === 'brightness' && c.access.settable && c.min != null && c.max != null)
  const on = useValue({ target, capability: key })?.data === true
  const powerCap = fn.capabilities.find((c) => c.key === 'power')
  const power = useValue({ target, capability: 'power' })?.data // a plug's, while on
  const command = useCommand(target)
  const { value: picked, set, picking, hold } = useControl<number>(target, 'brightness', { fade: true })
  const brightness = typeof picked === 'number' ? picked : undefined
  const [sheet, setSheet] = useState(false)
  const gesture = useRef<{ x: number; y: number; from: number; width: number; moving: boolean } | null>(null)
  const lit = on || picking
  const group = members !== undefined
  const litCount = useOnCount((members ?? []).map((m) => ({ target: m, capability: 'state' })))
  const min = Math.max(1, cap?.min ?? 1) // brightness 0 switches the light off: that is the tap's job
  const max = cap?.max ?? 1
  const level = !lit ? 0 : cap && brightness !== undefined ? clamp((brightness - min) / (max - min)) : 1
  const toggle = () => sendCommand(target, { [key]: 'toggle' })
  const dim = (f: number) => {
    if (!cap) return
    set(Math.round(min + clamp(f) * (max - min)))
  }

  const down = (e: PointerEvent<HTMLDivElement>) => {
    gesture.current = { x: e.clientX, y: e.clientY, from: level, width: e.currentTarget.clientWidth, moving: false }
  }
  const move = (e: PointerEvent<HTMLDivElement>) => {
    const g = gesture.current
    if (!g) return
    const dx = e.clientX - g.x
    if (!g.moving) {
      if (Math.abs(e.clientY - g.y) > 10) return void (gesture.current = null) // a scroll
      if (Math.abs(dx) < 8) return
      g.moving = true
      e.currentTarget.setPointerCapture(e.pointerId)
      hold(true)
    }
    dim(g.from + dx / g.width)
  }
  const up = () => {
    const g = gesture.current
    gesture.current = null
    hold(false)
    if (g && !g.moving) toggle()
  }

  const status = (
    <>
      {[
        lit && group && `${litCount}/${members.length}`,
        !lit ? 'off' : cap && brightness !== undefined ? display(brightness, cap) : 'on',
        lit && powerCap && power !== undefined && display(power, powerCap),
        note,
      ]
        .filter(Boolean)
        .join(' · ')}
      <CommandNote command={command} />
    </>
  )
  return (
    <>
      <section
        title={compact ? name : undefined}
        className={`${compact ? 'min-w-36 flex-1 sm:w-56 sm:flex-none' : 'rounded-xl bg-neutral-900 p-1.5'} ${dimmed ? 'opacity-50' : ''}`}
      >
        <div
          role="switch"
          aria-checked={lit}
          aria-label={name}
          tabIndex={0}
          onPointerDown={down}
          onPointerMove={move}
          onPointerUp={up}
          onPointerCancel={() => ((gesture.current = null), hold(false))}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') toggle()
            else if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') dim(level + (e.key === 'ArrowRight' ? 0.1 : -0.1))
            else return
            e.preventDefault()
          }}
          className={`relative ${compact ? 'h-9' : 'h-14'} cursor-pointer touch-pan-y overflow-hidden rounded-lg bg-neutral-800 select-none`}
        >
          <div className="absolute inset-y-0 left-0 bg-amber-400/25" style={{ width: `${lit ? Math.max(level, 0.04) * 100 : 0}%` }} />
          <div className={`relative flex h-full items-center ${compact ? 'gap-2 pl-2.5' : 'gap-3 pl-3'}`}>
            {fn.kind === 'light' ? (
              <LightIcon
                icon={icon}
                group={group}
                lit={lit}
                className={`${compact ? 'size-5' : 'size-6'} shrink-0 ${lit ? 'text-amber-300' : 'text-neutral-500'}`}
              />
            ) : (
              <Svg
                path={(barIcons[fn.kind] ?? barIcons.switch!)[lit ? 1 : 0]}
                className={`${compact ? 'size-5' : 'size-6'} shrink-0 ${lit ? 'text-amber-300' : 'text-neutral-500'}`}
              />
            )}
            {compact ? (
              <p className="min-w-0 flex-1 truncate text-sm text-neutral-300">{status}</p>
            ) : (
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2 text-xs">
                  <p className="truncate text-base font-medium" onMouseEnter={titleIfTruncated}>
                    {name}
                  </p>
                  {badges}
                </div>
                <p className="truncate text-xs text-neutral-400">{status}</p>
              </div>
            )}
            <button
              onPointerDown={(e) => e.stopPropagation()}
              onKeyDown={(e) => e.stopPropagation()}
              onClick={() => setSheet(true)}
              aria-label="More"
              className={`self-stretch ${compact ? 'px-3' : 'px-4'} text-lg text-neutral-500 hover:text-white`}
            >
              ⋯
            </button>
          </div>
        </div>
        {!compact && (
          <div className="mt-2 px-2.5 pb-1 empty:hidden">
            <MiniChart target={target} fn={fn} />
          </div>
        )}
      </section>
      {sheet && (
        <Panel
          title={
            <>
              <h2 className="truncate text-lg font-medium" onMouseEnter={titleIfTruncated}>
                {name}
              </h2>
              <p className="text-xs text-neutral-500">{status}</p>
            </>
          }
          onClose={() => setSheet(false)}
        >
          <FunctionControls target={target} fn={fn} />
          {onSettings && (
            <div className="mt-6 flex justify-end border-t border-neutral-800 pt-4">
              <button
                onClick={() => (setSheet(false), onSettings(() => setSheet(true)))}
                className="flex items-center gap-1.5 rounded-full bg-neutral-800 py-1.5 pr-3.5 pl-2.5 text-sm text-neutral-300 hover:bg-neutral-700 hover:text-white"
              >
                <Svg path={mdiCogOutline} className="size-4" />
                Settings
              </button>
            </div>
          )}
        </Panel>
      )}
    </>
  )
}

// A Flag without an Area, as a pill under the page header: a tap toggles it, ⋯ opens its panel, to an Admin.
function FlagPill({ flag, onOpen }: { flag: Flag; onOpen?: () => void }) {
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

// An Automation's Tile with Manual triggers: a button for each, named after it.
// Only an enabled Automation runs; otherwise the tile is dimmed and says why.
function ManualTile({ automation, onResult }: { automation: AutomationStatus; onResult: (r: { text: string; error?: boolean }) => void }) {
  const off = automation.status !== 'enabled' ? automation.status : null
  return (
    <section className={`space-y-3 rounded-xl bg-neutral-900 p-4 ${off ? 'opacity-50' : ''}`}>
      <div className="min-w-0">
        <p className="truncate font-medium" onMouseEnter={titleIfTruncated}>
          {automation.name}
        </p>
        <p className="text-xs text-neutral-500">{['automation', off].filter(Boolean).join(' · ')}</p>
      </div>
      <div className="flex flex-wrap gap-2">
        {automation.manualTriggers?.map((s) => (
          <button
            key={s.step}
            disabled={!!off}
            onClick={async () => {
              const r = await runManual(automation.id, s.step)
              onResult({ ...r, text: `${automation.name} · ${s.name || 'Run'}: ${r.text}` })
            }}
            className="rounded bg-neutral-800 px-3 py-1 hover:bg-neutral-700 disabled:cursor-not-allowed disabled:hover:bg-neutral-800"
          >
            {s.name || 'Run'}
          </button>
        ))}
      </div>
    </section>
  )
}

// A tile of the dashboard, drawn: key is its Target, which an Area may hide, or its Automation's id.
interface TileNode {
  key: string
  label: string
  place?: Place // in an Area's Layout
  node: ReactNode
}

// The narrowest a column of a Layout gets, in px (14rem), and the gap between two.
const minColumn = 224
const columnGap = 12

// Where a drag lands: an Area on another Area, a tile in a cell of its own Area's grid.
const landing: CollisionDetection = (args) => {
  const from = args.active.data.current
  const fits = (to?: Record<string, unknown>) => to?.type === from?.type && (from?.type === 'area' || to?.area === from?.area)
  return pointerWithin({ ...args, droppableContainers: args.droppableContainers.filter((d) => fits(d.data.current)) })
}

// A place's cells in the grid of a Layout.
// A place's cells in the grid of a Layout. A tile several rows tall fills them, whatever its content.
const cells = (p: Place): CSSProperties => ({
  gridColumn: `${p.col + 1} / span ${p.width}`,
  gridRow: `${p.row + 1} / span ${rows(p)}`,
  ...(rows(p) > 1 && { alignSelf: 'stretch', display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)' }),
})

// What arranging the dashboard hands an Area's section: its id, and how to save its Layout.
interface Arranging {
  area: string
  onLayout: (columns: number, layout: Arranged[]) => void
}

// An Area's part of the dashboard, a card of its own, or that of the tiles without one. An Area's
// header is a banner: its name large, its status (climate, presence, doors) on a line under it, its
// light bar on the right, which a folded section keeps. The bar has no chart: its ⋯ sheet has its
// History. An Area's tiles sit where its Layout places them, gaps included, while its columns fit;
// narrower, they come one after another, as Others' do. The tiles it hides wait behind a link; a
// tap on its name folds it all away. While arranging, an Area's section is open, its header drags
// it among the others, and its grid shows every cell, hidden tiles dimmed in theirs.
function Section({
  title,
  collapsed,
  onCollapse,
  onSettings,
  status,
  bar,
  columns,
  tiles,
  hidden = [],
  arranging,
}: {
  title?: string
  collapsed: boolean
  onCollapse: () => void
  onSettings?: () => void
  status?: ReactNode
  bar?: ReactNode
  columns?: number // of an Area's Layout
  tiles: TileNode[]
  hidden?: string[]
  arranging?: Arranging
}) {
  const [showHidden, setShowHidden] = useState(false)
  const [width, setWidth] = useState(0)
  const measure = (el: HTMLDivElement | null) => {
    if (!el) return
    const o = new ResizeObserver(([e]) => setWidth(e!.contentRect.width))
    o.observe(el)
    return () => o.disconnect()
  }
  const area = arranging?.area ?? ''
  const drag = useDraggable({ id: `area:${area}`, data: { type: 'area', area }, disabled: !arranging })
  const drop = useDroppable({ id: `area:${area}`, data: { type: 'area', area }, disabled: !arranging })
  const shown = tiles.filter((t) => !hidden.includes(t.key))
  const folded = tiles.length - shown.length
  const visible = showHidden ? tiles : shown
  const open = !collapsed || !title || !!arranging
  const grid = columns !== undefined && width >= columns * minColumn + (columns - 1) * columnGap
  const layout = placements(tiles)
  return (
    <section
      ref={(el) => (drag.setNodeRef(el), drop.setNodeRef(el))}
      style={drag.transform ? { transform: `translate3d(${drag.transform.x}px, ${drag.transform.y}px, 0)` } : undefined}
      className={`overflow-hidden rounded-2xl border ${drag.isDragging ? 'relative z-20 border-amber-400 bg-neutral-900 shadow-2xl' : drop.isOver ? 'border-amber-400' : 'border-neutral-800 bg-neutral-900/30'}`}
    >
      {title && (
        <header className="flex flex-wrap items-center gap-x-4 gap-y-3 bg-linear-to-r from-neutral-800/60 to-transparent px-4 py-3">
          {/* too narrow for both, the bar goes under the name, as wide as the card */}
          <div className="min-w-48 flex-1">
            <div className="flex items-center gap-1">
              {arranging && (
                <button
                  {...drag.listeners}
                  {...drag.attributes}
                  aria-label={`Move ${title}`}
                  title="Drag to move"
                  className="-ml-2 grid size-8 shrink-0 cursor-grab touch-none place-items-center rounded-full text-amber-300 hover:bg-neutral-800"
                >
                  <Svg path={mdiDrag} className="size-5" />
                </button>
              )}
              <button
                onClick={onCollapse}
                disabled={!!arranging}
                aria-expanded={open}
                className="flex min-w-0 items-center gap-1.5 enabled:hover:text-amber-200"
              >
                <h2 className="truncate text-xl font-semibold tracking-tight">{title}</h2>
                {!arranging && <Svg path={mdiChevronDown} className={`size-5 shrink-0 text-neutral-500 transition-transform ${open ? '' : '-rotate-90'}`} />}
              </button>
              {onSettings && !arranging && (
                <button
                  onClick={onSettings}
                  aria-label={`${title} settings`}
                  title="Settings"
                  className="grid size-7 shrink-0 place-items-center rounded-full text-neutral-500 hover:bg-neutral-800 hover:text-white"
                >
                  <Svg path={mdiCogOutline} className="size-5" />
                </button>
              )}
              {arranging && columns && (
                <Stepper
                  value={columns}
                  max={maxColumns}
                  label="columns"
                  onChange={(n) => arranging.onLayout(n, reflow(layout, n))}
                  className="ml-auto text-sm"
                />
              )}
            </div>
            <div className="mt-0.5 flex flex-wrap gap-x-4 gap-y-1 text-sm empty:hidden">{status}</div>
          </div>
          {bar}
        </header>
      )}
      {open && (
        <div ref={measure} className="space-y-3 p-3 empty:hidden">
          {arranging && columns ? (
            <div className="grid auto-rows-[minmax(3.5rem,auto)] items-start gap-3" style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}>
              {/* every cell, one row more than the tiles take, for a tile to land in */}
              {[...Array((Math.max(0, ...layout.map((p) => p.row + rows(p))) + 1) * columns).keys()].map((i) => (
                <Cell key={i} area={area} col={i % columns} row={Math.floor(i / columns)} />
              ))}
              {tiles.map((t) => (
                <ArrangedTile
                  key={t.key}
                  area={area}
                  tile={t}
                  hidden={hidden.includes(t.key)}
                  columns={columns}
                  onSize={(size) => arranging.onLayout(columns, resize(layout, t.key, size, columns))}
                />
              ))}
            </div>
          ) : visible.length > 0 && grid ? (
            <div className="grid auto-rows-[minmax(3.5rem,auto)] items-start gap-3" style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}>
              {visible.map((t) => (
                <div key={t.key} style={t.place && cells(t.place)}>
                  {t.node}
                </div>
              ))}
            </div>
          ) : visible.length > 0 ? (
            <div className="grid grid-flow-dense grid-cols-[repeat(auto-fill,minmax(16rem,1fr))] items-start gap-3">{visible.map((t) => t.node)}</div>
          ) : (
            !bar && !folded && <p className="text-sm text-neutral-600">Nothing here yet: add devices from its settings.</p>
          )}
          {folded > 0 && !arranging && (
            <button onClick={() => setShowHidden(!showHidden)} className="text-xs text-neutral-500 hover:text-white">
              {showHidden ? `Hide ${folded} again` : `${folded} hidden`}
            </button>
          )}
        </div>
      )}
    </section>
  )
}

// A cell of an Area's grid while arranging, where one of its tiles may land.
function Cell({ area, col, row }: { area: string; col: number; row: number }) {
  const { setNodeRef, isOver } = useDroppable({ id: `cell:${area}:${col}:${row}`, data: { type: 'tile', area, col, row } })
  return (
    <div
      ref={setNodeRef}
      style={{ gridColumn: col + 1, gridRow: row + 1 }}
      className={`min-h-14 self-stretch rounded-xl border border-dashed ${isOver ? 'border-amber-400 bg-amber-400/10' : 'border-neutral-700/60'}`}
    />
  )
}

// A tile while arranging: a veil over it keeps its controls from a tap and drags it to another
// cell; its width steps from one column to them all, its height from one row to maxRows.
function ArrangedTile({
  area,
  tile,
  hidden,
  columns,
  onSize,
}: {
  area: string
  tile: TileNode
  hidden: boolean
  columns: number
  onSize: (size: { width?: number; height?: number }) => void
}) {
  const { setNodeRef, listeners, attributes, transform, isDragging } = useDraggable({
    id: `tile:${area}:${tile.key}`,
    data: { type: 'tile', area, tile: tile.key },
  })
  const place = tile.place!
  return (
    <div
      ref={setNodeRef}
      style={{ ...cells(place), transform: transform ? `translate3d(${transform.x}px, ${transform.y}px, 0)` : undefined }}
      className={`relative ${isDragging ? 'z-20 shadow-2xl' : ''}`}
    >
      <div className={`grid grid-cols-[minmax(0,1fr)] ${hidden ? 'opacity-30' : ''}`}>{tile.node}</div>
      <div
        {...listeners}
        {...attributes}
        aria-label={`Move ${tile.label}`}
        className="absolute inset-0 flex cursor-grab touch-none items-end justify-end gap-1 rounded-xl p-1.5 ring-1 ring-amber-400/50 hover:bg-amber-400/5"
      >
        <Stepper
          icon={mdiArrowExpandHorizontal}
          value={place.width}
          max={columns}
          label="columns wide"
          onChange={(width) => onSize({ width })}
          className="bg-neutral-950/90 text-xs"
        />
        <Stepper
          icon={mdiArrowExpandVertical}
          value={rows(place)}
          max={maxRows}
          label="rows tall"
          onChange={(height) => onSize({ height })}
          className="bg-neutral-950/90 text-xs"
        />
      </div>
    </div>
  )
}

// − value +, from 1 to max, after an icon telling what it counts; a press on it never starts a drag.
function Stepper({
  icon,
  value,
  max,
  label,
  onChange,
  className = '',
}: {
  icon?: string
  value: number
  max: number
  label: string
  onChange: (n: number) => void
  className?: string
}) {
  const step = 'grid size-7 place-items-center rounded-full hover:bg-neutral-700 disabled:opacity-30 disabled:hover:bg-transparent'
  return (
    <span onPointerDown={(e) => e.stopPropagation()} className={`flex items-center rounded-full bg-neutral-800 text-neutral-200 ${className}`}>
      <button disabled={value <= 1} onClick={() => onChange(value - 1)} aria-label={`Fewer ${label}`} className={step}>
        −
      </button>
      <span title={label} className="flex min-w-4 items-center justify-center gap-0.5 tabular-nums">
        {icon && <Svg path={icon} className="size-3.5 text-neutral-400" />}
        {value}
      </span>
      <button disabled={value >= max} onClick={() => onChange(value + 1)} aria-label={`More ${label}`} className={step}>
        +
      </button>
    </span>
  )
}

// An Area's presence or doors in its header: the state of its Aggregate of occupancy or of contacts.
function AreaState({ aggregate }: { aggregate: Aggregate }) {
  const cap = aggregate.capabilities?.find((c) => c.type === 'binary')
  return cap ? <StateText target={aggregateTarget(aggregate.id)} cap={cap} /> : null
}

// An Area's climate in its header: its mean temperature and humidity, its worst CO₂, each after its
// icon. A tap on one opens its History.
function Climate({ aggregates }: { aggregates: Aggregate[] }) {
  return aggregates.flatMap((a) => {
    const cap = a.capabilities?.find((c) => c.key === a.kind)
    return cap
      ? [
          <span key={a.id} className="flex items-center gap-1 whitespace-nowrap">
            <Svg path={readingIcons[cap.key] ?? mdiGauge} className="size-4 text-neutral-500" />
            <ReadingText target={aggregateTarget(a.id)} cap={cap} />
          </span>,
        ]
      : []
  })
}

// Creates an Area, or renames and deletes one and gathers Devices in it at once. Its place among the
// others is arranged on the dashboard.
function AreaPanel({
  area,
  areas,
  devices,
  tiles = [],
  onClose,
}: {
  area?: Area
  areas: Area[]
  devices: Device[]
  tiles?: { key: string; label: string }[]
  onClose: () => void
}) {
  const [error, setError] = useState<string | null>(null)
  // Its Devices first, then those without an Area; the order is the one at opening, so ticking a box does not move it.
  const [order] = useState(() => {
    const rank = (d: Device) => (d.area === area?.id ? 0 : d.area ? 2 : 1)
    return devices
      .filter((d) => d.functions?.length)
      .sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name))
      .map((d) => d.id)
  })
  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const body = { name: new FormData(e.currentTarget).get('name') }
    const err = area ? await edit('PUT', `areas/${area.id}`, body) : await edit('POST', 'areas', body)
    if (err) setError(err)
    else onClose()
  }
  // Hides or shows item of one of the Area's display lists, at once.
  const display = async (list: 'hidden' | 'hiddenAggregates', item: string, hide: boolean) => {
    if (!area) return
    const now = { hidden: area.hidden ?? [], hiddenAggregates: area.hiddenAggregates ?? [] }
    now[list] = hide ? [...now[list], item] : now[list].filter((x) => x !== item)
    setError(await edit('PUT', `areas/${area.id}/display`, now))
  }
  const legend = 'mb-2 text-xs font-semibold tracking-wide text-neutral-500 uppercase'
  return (
    <Panel
      title={
        <h2 className="truncate text-lg font-medium" onMouseEnter={titleIfTruncated}>
          {area?.name ?? 'New area'}
        </h2>
      }
      onClose={onClose}
    >
      <form onSubmit={submit} className="space-y-4 text-sm">
        <input name="name" defaultValue={area?.name} placeholder="Name" aria-label="Name" className="w-full rounded bg-neutral-800 px-2 py-1" />
        {error && <p className="text-red-400">{error}</p>}
        <div className="flex flex-wrap gap-2">
          <button type="submit" className="rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300">
            {area ? 'Save' : 'Create'}
          </button>
          {area && (
            <button
              type="button"
              onClick={async () =>
                (await confirm(`Delete ${area.name}? What it holds is left without an area.`, 'Delete')) && setError(await edit('DELETE', `areas/${area.id}`))
              }
              className="ml-auto rounded bg-neutral-800 px-3 py-1 text-red-400 hover:bg-neutral-700"
            >
              Delete
            </button>
          )}
        </div>
      </form>
      {area && (
        <fieldset className="space-y-1 text-sm">
          <legend className={legend}>Header</legend>
          {[
            ['light', 'Lights'],
            ['occupancy', 'Presence'],
            ['contact', 'Doors'],
            ['temperature', 'Temperature'],
            ['humidity', 'Humidity'],
            ['co2', 'CO₂'],
          ].map(([kind, label]) => (
            <label key={kind} className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={!area.hiddenAggregates?.includes(kind!)}
                onChange={(e) => display('hiddenAggregates', kind!, !e.target.checked)}
                className="accent-amber-400"
              />
              {label}
            </label>
          ))}
        </fieldset>
      )}
      {area && tiles.length > 0 && (
        <fieldset className="space-y-1 text-sm">
          <legend className={legend}>Tiles shown</legend>
          {tiles.map((t) => (
            <label key={t.key} className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={!area.hidden?.includes(t.key)}
                onChange={(e) => display('hidden', t.key, !e.target.checked)}
                className="accent-amber-400"
              />
              <span className="truncate">{t.label}</span>
            </label>
          ))}
        </fieldset>
      )}
      {area && (
        <fieldset className="max-h-80 space-y-1 overflow-y-auto text-sm">
          <legend className="mb-2 text-xs font-semibold tracking-wide text-neutral-500 uppercase">Devices</legend>
          {order.flatMap((id) => {
            const d = devices.find((d) => d.id === id)
            if (!d) return []
            const elsewhere = d.area !== area.id && areas.find((a) => a.id === d.area)?.name
            return [
              <label key={d.id} className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={d.area === area.id}
                  onChange={async (e) => setError(await edit('PUT', 'area', { target: deviceTarget(d.id), area: e.target.checked ? area.id : '' }))}
                  className="accent-amber-400"
                />
                <span className="truncate">{d.name}</span>
                {elsewhere && <span className="shrink-0 text-neutral-500">· {elsewhere}</span>}
              </label>,
            ]
          })}
        </fieldset>
      )}
    </Panel>
  )
}

// Sets the Area of target at once. With inherited, the Area of a Function's Device,
// "" stands for following it rather than for none.
function AreaSelect({
  target,
  area,
  inherited,
  onError,
  className = '',
}: {
  target: Target
  area?: string
  inherited?: string
  onError: (e: string | null) => void
  className?: string
}) {
  const areas = useAreas()
  const name = (id?: string) => areas.find((a) => a.id === id)?.name ?? 'none'
  return (
    <select
      value={area ?? ''}
      aria-label="Area"
      onChange={async (e) => onError(await edit('PUT', 'area', { target, area: e.target.value }))}
      className={`rounded bg-neutral-800 px-2 py-1 ${className}`}
    >
      <option value="">{inherited === undefined ? 'No area' : `As the device (${name(inherited)})`}</option>
      {areas.map((a) => (
        <option key={a.id} value={a.id}>
          {a.name}
        </option>
      ))}
    </select>
  )
}

// The Area of a Device, a Flag or an Aggregate; children add those of a Device's Functions.
function AreaField({ target, area, onError, children }: { target: Target; area?: string; onError: (e: string | null) => void; children?: ReactNode }) {
  return (
    <section className="space-y-2 text-sm">
      <h3 className="text-xs font-semibold tracking-wide text-neutral-500 uppercase">Area</h3>
      <AreaSelect target={target} area={area} onError={onError} className="w-full" />
      {children}
    </section>
  )
}

// Creates a Flag, or renames and deletes an existing one.
function FlagPanel({ flag, onClose }: { flag?: Flag; onClose: () => void }) {
  const [error, setError] = useState<string | null>(null)
  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const body = { name: new FormData(e.currentTarget).get('name') }
    const err = flag ? await edit('PUT', `flags/${flag.id}`, body) : await edit('POST', 'flags', body)
    if (err) setError(err)
    else onClose()
  }
  return (
    <Panel
      title={
        <h2 className="truncate text-lg font-medium" onMouseEnter={titleIfTruncated}>
          {flag?.name ?? 'New flag'}
        </h2>
      }
      onClose={onClose}
    >
      <form onSubmit={submit} className="space-y-4 text-sm">
        <input name="name" defaultValue={flag?.name} placeholder="Name" aria-label="Name" className="w-full rounded bg-neutral-800 px-2 py-1" />
        {flag && <AreaField target={flagTarget(flag.id)} area={flag.area} onError={setError} />}
        {error && <p className="text-red-400">{error}</p>}
        <div className="flex justify-between gap-2">
          <button type="submit" className="rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300">
            {flag ? 'Save' : 'Create'}
          </button>
          {flag && (
            <button
              type="button"
              onClick={async () => (await confirm(`Delete ${flag.name}?`, 'Delete')) && setError(await edit('DELETE', `flags/${flag.id}`))}
              className="rounded bg-neutral-800 px-3 py-1 text-red-400 hover:bg-neutral-700"
            >
              Delete
            </button>
          )}
        </div>
      </form>
    </Panel>
  )
}

// Creates an Aggregate, or edits and deletes an existing one. Members are offered
// among the Functions, Flags and Aggregates of one kind at a time, Aggregates first.
function AggregatePanel({
  aggregate,
  aggregates,
  onClose,
  onBack,
}: {
  aggregate?: Aggregate
  aggregates: Aggregate[]
  onClose: () => void
  onBack?: () => void
}) {
  const targets = useCatalogue()
  const fns = targets.list
    .filter((e) => targetKind(e.target) === 'function' || targetKind(e.target) === 'flag')
    .map((e) => ({ member: e.target, kind: e.kind, label: e.name }))
  const kinds = [...new Set(fns.map((f) => f.kind))].sort()
  const [kind, setKind] = useState(aggregate?.kind || kinds[0] || '')
  const [error, setError] = useState<string | null>(null)
  // Whether a contains the Aggregate id, directly or not; Oiko refuses such cycles.
  const contains = (a: Aggregate, id: string): boolean =>
    a.members.some((m) => m === aggregateTarget(id) || aggregates.some((n) => m === aggregateTarget(n.id) && contains(n, id)))
  const candidates: { member: Target; label: string }[] = [
    ...aggregates
      .filter((a) => a.kind === kind && a.id !== aggregate?.id && !(aggregate && contains(a, aggregate.id)))
      .map((a) => ({ member: aggregateTarget(a.id), label: `${a.name} · aggregate` })),
    ...fns.filter((f) => f.kind === kind),
  ]
  const isMember = (m: Target) => aggregate?.members.includes(m) ?? false
  // Current members first, then Aggregates; the order is the one at opening, so ticking a box does not move it.
  candidates.sort(
    (a, b) =>
      Number(isMember(b.member)) - Number(isMember(a.member)) ||
      Number(parseTarget(b.member)?.kind === 'aggregate') - Number(parseTarget(a.member)?.kind === 'aggregate') ||
      a.label.localeCompare(b.label),
  )
  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const form = new FormData(e.currentTarget)
    const members = form.getAll('member').map(String)
    const body = { name: form.get('name'), binary: form.get('binary'), numeric: form.get('numeric'), members }
    const err = aggregate ? await edit('PUT', `aggregates/${aggregate.id}`, body) : await edit('POST', 'aggregates', body)
    if (err) setError(err)
    else onClose()
  }
  const field = 'rounded bg-neutral-800 px-2 py-1'
  const title = aggregate ? (
    <>
      <h2 className="truncate text-lg font-medium" onMouseEnter={titleIfTruncated}>
        {aggregate.name}
      </h2>
      <p className="text-xs text-neutral-500">{aggregateSummary(aggregate)}</p>
    </>
  ) : (
    <h2 className="text-lg font-medium">New aggregate</h2>
  )
  // An Area Aggregate follows its Area: nothing to edit, only who is in it.
  if (aggregate?.derived) {
    const label = targets.name
    return (
      <Panel title={title} onClose={onClose} onBack={onBack}>
        <p className="text-sm text-neutral-400">Every {aggregate.kind} of its area, as the area changes.</p>
        <ul className="space-y-1 text-sm">
          {aggregate.members.map((m) => (
            <li key={m}>{label(m)}</li>
          ))}
        </ul>
      </Panel>
    )
  }
  return (
    <Panel title={title} onClose={onClose} onBack={onBack}>
      {aggregate && <AreaField target={aggregateTarget(aggregate.id)} area={aggregate.area} onError={setError} />}
      {aggregate?.kind === 'light' && <IconPicker target={aggregateTarget(aggregate.id)} icon={aggregate.icon} group onError={setError} />}
      <form onSubmit={submit} className="space-y-4 text-sm">
        <input name="name" defaultValue={aggregate?.name} placeholder="Name" aria-label="Name" className={`w-full ${field}`} />
        <div className="flex gap-2">
          <select value={kind} onChange={(e) => setKind(e.target.value)} aria-label="Kind" className={`flex-1 ${field}`}>
            {kinds.map((k) => (
              <option key={k}>{k}</option>
            ))}
          </select>
          <select name="binary" defaultValue={aggregate?.binary ?? 'any'} aria-label="Binary rule" className={field}>
            <option value="any">on when any member is</option>
            <option value="all">on when all members are</option>
          </select>
          <select name="numeric" defaultValue={aggregate?.numeric ?? 'mean'} aria-label="Numeric rule" className={field}>
            <option value="mean">mean</option>
            <option value="min">min</option>
            <option value="max">max</option>
            <option value="sum">sum</option>
          </select>
        </div>
        <fieldset className="max-h-64 space-y-1 overflow-y-auto">
          <legend className="mb-2 text-xs font-semibold tracking-wide text-neutral-500 uppercase">Members</legend>
          {candidates.map((c) => {
            const value = c.member
            return (
              <label key={value} className="flex items-center gap-2">
                <input type="checkbox" name="member" value={value} defaultChecked={isMember(c.member)} className="accent-amber-400" />
                {c.label}
              </label>
            )
          })}
        </fieldset>
        {error && <p className="text-red-400">{error}</p>}
        <div className="flex justify-between gap-2">
          <button type="submit" className="rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300">
            {aggregate ? 'Save' : 'Create'}
          </button>
          {aggregate && (
            <button
              type="button"
              onClick={async () => (await confirm(`Delete ${aggregate.name}?`, 'Delete')) && setError(await edit('DELETE', `aggregates/${aggregate.id}`))}
              className="rounded bg-neutral-800 px-3 py-1 text-red-400 hover:bg-neutral-700"
            >
              Delete
            </button>
          )}
        </div>
      </form>
    </Panel>
  )
}

// fade: numeric changes glide over a short transition (a light's primary controls).
interface CapProps {
  target: Target
  cap: Capability
  fade?: boolean
}

// The Roles of a Device's own Capabilities and of each of its Functions'.
function deviceRoles(device: Device): { target: Target; fn?: Fn; r: Roles }[] {
  return [
    { target: deviceTarget(device.id), r: roles('', device.capabilities ?? []) },
    ...(device.functions ?? []).map((fn) => ({ target: deviceTarget(device.id, fn.key), fn, r: roles(fn.kind, fn.capabilities) })),
  ]
}

// A Target's Tile, as dashboard.ts resolves it, drawn as its shape says: a bar, a sensor's, or its
// Functions' controls. A lone Function shares the Tile's header; a Function's kind or key shows as
// a heading only where it adds something. Dimmed while its Device is detached or it is offline.
function Tile({ tile, compact, onOpen }: { tile: TargetTile; compact?: boolean; onOpen?: (back?: () => void) => void }) {
  const offline = useAvailability(tile.subject) === 'offline'
  const { shape, name, fns } = tile
  const dimmed = tile.detached || offline
  const note = [tile.detached && 'detached', offline && 'offline'].filter(Boolean).join(' · ')
  const badges = tile.health.map((h) => <HealthBadge key={`${h.target}|${h.cap.key}`} {...h} />)
  if (shape.kind === 'bar')
    return (
      <BarTile
        name={name}
        icon={tile.icon}
        members={tile.members}
        note={note}
        badges={badges}
        target={shape.target}
        fn={shape.fn}
        toggle={shape.toggle}
        dimmed={dimmed}
        compact={compact}
        onSettings={onOpen}
      />
    )
  if (shape.kind !== 'controls') return <SensorTile shape={shape} name={name} note={note} badges={badges} dimmed={dimmed} onOpen={onOpen && (() => onOpen())} />
  const header = (command?: ReactNode) => (
    <div className="min-w-0">
      {onOpen ? (
        <button onClick={() => onOpen()} onMouseEnter={titleIfTruncated} className="block max-w-full truncate text-left font-medium hover:underline">
          {name}
        </button>
      ) : (
        <p onMouseEnter={titleIfTruncated} className="truncate font-medium">
          {name}
        </p>
      )}
      <p className="flex items-center gap-2 text-xs text-neutral-500">
        <span>
          {[tile.summary, note].filter(Boolean).join(' · ')}
          {command}
        </span>
        {badges}
      </p>
    </div>
  )
  const heading = (fn: Fn) =>
    titled(fn)
      ? (command: ReactNode) => (
          <span className="text-sm text-neutral-400">
            {fn.capabilities.length === 1 ? fn.capabilities[0]?.label : fn.key}
            <span className="text-xs text-neutral-500">{command}</span>
          </span>
        )
      : undefined
  const [only] = fns
  return (
    <section className={`space-y-3 rounded-xl bg-neutral-900 p-4 ${dimmed ? 'opacity-50' : ''}`}>
      {fns.length === 1 && only ? (
        <FunctionControls {...only} heading={header} />
      ) : (
        <>
          {header()}
          {fns.map((f) => (
            <FunctionControls key={f.target} {...f} heading={heading(f.fn)} />
          ))}
        </>
      )}
    </section>
  )
}

// A button's Tile, a bar with no chart: its last Event and how long ago. A tap opens its History, ⋯ its
// Device's panel.
function EventTile({ name, note, badges, item, dimmed, onOpen }: SensorProps & { item: Part }) {
  const event = useLastEvent(ref(item))
  const now = useNow()
  const [history, setHistory] = useState(false)
  return (
    <section className={`rounded-xl bg-neutral-900 p-1.5 ${dimmed ? 'opacity-50' : ''}`}>
      <div
        role="button"
        tabIndex={0}
        aria-label={name}
        // its History sheet is portaled out of the bar, but React still bubbles its events here
        onClick={(e) => e.currentTarget.contains(e.target as Node) && setHistory(true)}
        onKeyDown={(e) => e.currentTarget.contains(e.target as Node) && (e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), setHistory(true))}
        className="flex h-14 cursor-pointer items-center gap-3 rounded-lg bg-neutral-800 pl-3 select-none hover:bg-neutral-700/60"
      >
        <Svg path={mdiGestureTapButton} className="size-6 shrink-0 text-neutral-500" />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 text-xs">
            <p className="truncate text-base font-medium" onMouseEnter={titleIfTruncated}>
              {name}
            </p>
            {badges}
          </div>
          <p className="truncate text-xs text-neutral-400" onMouseEnter={titleIfTruncated}>
            {[event ? `${String(event.data)} · ${age(event.at, now)}` : 'none yet', note].filter(Boolean).join(' · ')}
          </p>
        </div>
        <More onOpen={onOpen} />
      </div>
      {history && createPortal(<HistorySheet target={item.target} onClose={() => setHistory(false)} />, document.body)}
    </section>
  )
}

interface SensorProps {
  name: string
  note: string // detached, offline…
  badges?: ReactNode // its health, when wrong
  dimmed: boolean
  onOpen?: () => void // an Admin's: its Device's or Aggregate's panel
}

// A sensor bar's ⋯, opening its panel, if anything does.
function More({ onOpen }: { onOpen?: () => void }) {
  if (!onOpen) return null
  return (
    <button
      onClick={(e) => (e.stopPropagation(), onOpen())}
      onKeyDown={(e) => e.stopPropagation()}
      aria-label="More"
      className="self-stretch px-4 text-lg text-neutral-500 hover:text-white"
    >
      ⋯
    </button>
  )
}

// A Tile with nothing to command, drawn as its shape says: its last Event, a bar for its state or
// its single reading, or a cell per reading.
function SensorTile({ shape, ...props }: SensorProps & { shape: Extract<Shape, { kind: 'event' | 'sensor' | 'readings' }> }) {
  if (shape.kind === 'event') return <EventTile {...props} item={shape.event} />
  const icon = readingIcons[shape.readings[0]?.cap.key ?? ''] ?? mdiGauge
  if (shape.kind === 'readings') return <ReadingsTile {...props} icon={icon} readings={shape.readings} />
  if (shape.state) return <StateTile {...props} state={shape.state} readings={shape.readings} />
  return <SensorBar {...props} icon={icon} readings={shape.readings} />
}

// A cell per reading: its value large, its label and its 24 h, each opening its History. With three
// or more, the Tile takes two columns of an automatic grid; in a Layout, its place's.
function ReadingsTile({ name, note, badges, icon, readings, dimmed, onOpen }: SensorProps & { icon: string; readings: Part[] }) {
  const wide = readings.length >= 3
  return (
    <section className={`@container space-y-3 rounded-xl bg-neutral-900 p-3 ${wide ? 'sm:col-span-2' : ''} ${dimmed ? 'opacity-50' : ''}`}>
      <div className="flex items-center gap-3 px-1.5">
        <Svg path={icon} className="size-6 shrink-0 text-neutral-500" />
        <div className="flex min-w-0 flex-1 items-center gap-2 text-xs">
          <p className="truncate text-base font-medium" onMouseEnter={titleIfTruncated}>
            {name}
          </p>
          {badges}
          {note && <span className="text-neutral-500">{note}</span>}
        </div>
        {onOpen && (
          <button onClick={onOpen} aria-label="More" className="px-2 text-lg text-neutral-500 hover:text-white">
            ⋯
          </button>
        )}
      </div>
      {/* three readings side by side only where its place is wide enough for them */}
      <div className={`grid grid-cols-2 gap-2 ${wide ? '@lg:grid-cols-3' : ''}`}>
        {readings.map((r) => (
          <div key={`${r.target}|${r.cap.key}`} className="min-w-0 space-y-1 rounded-lg bg-neutral-800/60 p-2.5">
            <ReadingText {...r} className="block max-w-full truncate text-xl font-medium" />
            <p className="truncate text-xs text-neutral-400">{r.cap.label}</p>
            {/* a reading other than its Function's charted one has no 24 h loaded */}
            {roles(r.fn.kind, r.fn.capabilities).charted === r.cap && (
              <div className="px-1">
                <MiniChart target={r.target} fn={r.fn} />
              </div>
            )}
          </div>
        ))}
      </div>
    </section>
  )
}

function StateTile({ state, ...props }: SensorProps & { state: Part; readings: Part[] }) {
  const held = useHeld(state)
  return <SensorBar {...props} state={state} active={held.active} icon={held.icon} lead={held.text} />
}

const readingIcons: Record<string, string> = {
  temperature: mdiThermometer,
  humidity: mdiWaterPercent,
  co2: mdiMoleculeCo2,
  noise: mdiVolumeHigh,
  pressure: mdiGauge,
  illuminance: mdiBrightness5,
  rain: mdiWeatherRainy,
}

// Shaped as a light's Tile: a bar with a big icon, the name and a status line, its chart below. With
// a state, the bar takes its colour while active and the status leads with how long it has held. A
// tap on the bar opens the History of what the chart shows: the state's, or the first reading's.
function SensorBar({
  name,
  note,
  badges,
  readings,
  dimmed,
  onOpen,
  state,
  active = false,
  icon,
  lead,
}: SensorProps & { readings: Part[]; state?: Part; active?: boolean; icon: string; lead?: string }) {
  const [history, setHistory] = useState(false)
  const charted = state ?? readings[0]
  const status = [
    lead && (
      <span key="lead" className={active ? 'text-emerald-300' : ''}>
        {lead}
      </span>
    ),
    ...readings.map((p) => <ReadingText key={`${p.target}|${p.cap.key}`} {...p} />),
    note && <span key="note">{note}</span>,
  ].filter(Boolean)
  return (
    <section className={`rounded-xl bg-neutral-900 p-1.5 ${dimmed ? 'opacity-50' : ''}`}>
      <div
        role="button"
        tabIndex={0}
        aria-label={name}
        // A reading's History sheet is portaled out of the bar, but React still bubbles its events here.
        onClick={(e) => e.currentTarget.contains(e.target as Node) && setHistory(true)}
        onKeyDown={(e) => e.currentTarget.contains(e.target as Node) && (e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), setHistory(true))}
        className={`flex h-14 cursor-pointer items-center gap-3 rounded-lg pl-3 select-none ${active ? 'bg-emerald-500/20' : 'bg-neutral-800 hover:bg-neutral-700/60'}`}
      >
        <Svg path={icon} className={`size-6 shrink-0 ${active ? 'text-emerald-300' : 'text-neutral-500'}`} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 text-xs">
            <p className="truncate text-base font-medium" onMouseEnter={titleIfTruncated}>
              {name}
            </p>
            {badges}
          </div>
          <p className="truncate text-xs text-neutral-400">{status.flatMap((s, i) => (i ? [' · ', s] : [s]))}</p>
        </div>
        <More onOpen={onOpen} />
      </div>
      {charted && (
        <div className="mt-2 px-2.5 pb-1 empty:hidden">
          <MiniChart target={charted.target} fn={charted.fn} />
        </div>
      )}
      {/* outside the tile, which an offline Device dims */}
      {history && charted && createPortal(<HistorySheet target={charted.target} onClose={() => setHistory(false)} />, document.body)}
    </section>
  )
}

// A reading in a status line: its unit says what it is, or else its label does. CO₂ that calls for
// airing takes a colour. A tap opens its own History, not the bar's.
function ReadingText(props: CapProps & { className?: string }) {
  const { target, cap, className = '' } = props
  const value = useValue(ref(props))
  const [open, setOpen] = useState(false)
  const level = co2Level(cap.key, value?.data)
  return (
    <>
      <button
        onClick={(e) => (e.stopPropagation(), setOpen(true))}
        onKeyDown={(e) => e.stopPropagation()}
        title={`${cap.label} history`}
        className={`-my-1 py-1 text-left underline-offset-2 hover:text-white hover:underline ${level === 'high' ? 'text-red-400' : level === 'raised' ? 'text-amber-300' : ''} ${className}`}
      >
        {!cap.unit && `${cap.label} `}
        {value ? display(value.data, cap) : '—'}
      </button>
      {open && createPortal(<HistorySheet target={target} onClose={() => setOpen(false)} />, document.body)}
    </>
  )
}

const stateIcons: Record<string, [idle: string, active: string]> = {
  occupancy: [mdiMotionSensorOff, mdiMotionSensor],
  presence: [mdiMotionSensorOff, mdiMotionSensor],
  contact: [mdiDoorClosed, mdiDoorOpen],
  water_leak: [mdiWaterOutline, mdiWaterAlert],
  smoke: [mdiSmokeDetector, mdiSmokeDetectorAlert],
}

// A state: whether it is active, its icon, and how long it has held, as Home tells it (an
// Aggregate's from its members'). Motion that is on is "now".
function useHeld({ target, cap }: CapProps) {
  const value = useValue({ target, capability: cap.key })
  const now = useNow()
  const active = value?.data === true
  const [idle, lit] = stateIcons[cap.key] ?? [mdiMotionSensorOff, mdiMotionSensor]
  const text = active && motion(cap) ? 'now' : value?.since ? age(value.since, now) : '?'
  return { active, icon: active ? lit : idle, text, title: `${cap.label} · ${value ? format(value.data) : 'unknown'}` }
}

// A state as an Area's header tells its presence or doors: its icon, what it is, how long it has
// held, lit while active. A tap opens its History.
function StateText(props: CapProps) {
  const held = useHeld(props)
  const [open, setOpen] = useState(false)
  return (
    <>
      <button onClick={() => setOpen(true)} title={held.title} className="flex items-center gap-1 whitespace-nowrap hover:underline">
        <Svg path={held.icon} className={`size-4 ${held.active ? 'text-emerald-300' : 'text-neutral-500'}`} />
        <span className={held.active ? 'text-emerald-300' : ''}>{held.active ? turns(props.cap)[0] : props.cap.key === 'contact' ? 'closed' : 'clear'}</span>
        <span className="text-neutral-500">{held.text}</span>
      </button>
      {open && createPortal(<HistorySheet target={props.target} onClose={() => setOpen(false)} />, document.body)}
    </>
  )
}

// The controls of one Function: its main control and adjustments, its state,
// readings and Events. Its toggles sit beside its heading, which shows the
// Function's pending Command; settings and diagnostics live in the DevicePanel.
function FunctionControls({ target, fn, heading }: { target: Target; fn: Fn; heading?: (note: ReactNode) => ReactNode }) {
  const command = useCommand(target)
  const fade = fn.kind === 'light'
  const r = roles(fn.kind, fn.capabilities)
  const controls = [r.control, ...r.adjustments].filter((c) => c !== undefined)
  const toggles = controls.filter((c) => c.type === 'binary')
  const sliders = controls.filter((c) => c.type === 'numeric' && c.min != null && c.max != null)
  const colors = controls.filter((c) => c.type === 'composite') // a light's color_hs
  const selects = controls.filter((c) => c.type === 'enum')
  const readings = [r.state, ...r.readings].filter((c) => c !== undefined)
  const events = r.events

  return (
    <>
      {heading && (
        <div className="flex items-start justify-between gap-3">
          {heading(<CommandNote command={command} />)}
          {toggles.map((c) => (
            <Toggle key={c.key} target={target} cap={c} />
          ))}
        </div>
      )}
      {readings.map((c) => (
        <Reading key={c.key} target={target} cap={c} />
      ))}
      {sliders.map((c) => (
        <Slider key={c.key} target={target} cap={c} fade={fade} />
      ))}
      {colors.map((c) => (
        <Color key={c.key} target={target} cap={c} />
      ))}
      {selects.map((c) => (
        <Select key={c.key} target={target} cap={c} />
      ))}
      {events.map((c) => (
        <LastEvent key={c.key} target={target} cap={c} />
      ))}
      <MiniChart target={target} fn={fn} />
    </>
  )
}

// Only a Command that ended badly is worth mentioning.
function CommandNote({ command }: { command?: CommandState }) {
  switch (command?.status) {
    case 'timed_out':
      return <span className="text-amber-400"> · no response</span>
    case 'failed':
      return <span className="text-red-400"> · {command.error ?? 'failed'}</span>
  }
  return null
}

// Settings and diagnostics of a Device, whether they belong to it or to one of its Functions.
function DevicePanel({ device, devices, onClose, onBack }: { device: Device; devices: Device[]; onClose: () => void; onBack?: () => void }) {
  const [error, setError] = useState<string | null>(null)
  const target = deviceTarget(device.id)
  const availability = useAvailability(target)
  const command = useCommand(target)
  const all = deviceRoles(device)
  const settings = all.flatMap(({ target, fn, r }) => r.settings.map((cap) => ({ target, cap, fn })))
  const diagnostics = all.flatMap(({ target, fn, r }) => [...r.health, ...r.diagnostics].map((cap) => ({ target, cap, fn })))
  // A Function of its own Capability's name needs no prefix.
  const label = (p: (typeof settings)[number]) => (p.fn && p.fn.key !== p.cap.key ? `${p.fn.key} · ${p.cap.label}` : p.cap.label)
  const title = (
    <>
      <NameField device={device} onError={setError} />
      <p className="text-xs text-neutral-500">
        {[device.vendor, device.model, device.nativeAddress, availability].filter(Boolean).join(' · ')}
        <CommandNote command={command} />
      </p>
    </>
  )

  return (
    <Panel title={title} onClose={onClose} onBack={onBack}>
      {error && <p className="text-sm text-red-400">{error}</p>}
      {device.detached && <DetachedSection device={device} devices={devices} onError={setError} />}
      <AreaField target={target} area={device.area} onError={setError}>
        {(device.functions?.length ?? 0) > 1 &&
          device.functions!.map((fn) => (
            <label key={fn.key} className="flex items-center justify-between gap-3">
              <span className="truncate text-neutral-400">{fn.key}</span>
              <AreaSelect target={deviceTarget(device.id, fn.key)} area={fn.area} inherited={device.area ?? ''} onError={setError} />
            </label>
          ))}
      </AreaField>
      {device.functions?.some((f) => f.kind === 'light') && <IconPicker target={target} icon={device.icon} group={false} onError={setError} />}
      {settings.length > 0 && (
        <section className="space-y-3">
          <h3 className="text-xs font-semibold tracking-wide text-neutral-500 uppercase">Settings</h3>
          {settings.map((p) => (
            <Control key={`${p.target}|${p.cap.key}`} {...p} cap={{ ...p.cap, label: label(p) }} />
          ))}
        </section>
      )}
      {diagnostics.length > 0 && (
        <section className="space-y-2">
          <h3 className="text-xs font-semibold tracking-wide text-neutral-500 uppercase">Diagnostics</h3>
          {diagnostics.map((p) => (
            <Reading key={`${p.target}|${p.cap.key}`} {...p} cap={{ ...p.cap, label: label(p) }} />
          ))}
        </section>
      )}
    </Panel>
  )
}

// Renames on Enter or when the field loses focus; Escape closes the panel as usual.
function NameField({ device, onError }: { device: Device; onError: (e: string | null) => void }) {
  const save = async (name: string) => {
    if (name.trim() === device.name) return
    onError(await edit('PATCH', `devices/${device.id}`, { name }))
  }
  return (
    <input
      defaultValue={device.name}
      aria-label="Name"
      onBlur={(e) => save(e.target.value)}
      onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
      className="-ml-1 w-full rounded bg-transparent px-1 text-lg font-medium hover:bg-neutral-800 focus:bg-neutral-800 focus:outline-none"
    />
  )
}

// A Detached Device keeps its identity until new hardware takes it over (Replace) or it is deleted.
function DetachedSection({ device, devices, onError }: { device: Device; devices: Device[]; onError: (e: string | null) => void }) {
  // Same model first: the usual replacement for a dead device.
  const candidates = devices
    .filter((d) => !d.detached)
    .sort((a, b) => Number(b.model === device.model) - Number(a.model === device.model) || a.name.localeCompare(b.name))
  return (
    <section className="space-y-3 rounded-lg bg-amber-950/40 p-3 text-sm">
      <p className="text-amber-200">This device has left zigbee2mqtt. Pair its replacement, then pick it here to keep this identity.</p>
      <div className="flex gap-2">
        <select
          defaultValue=""
          aria-label="Replace with"
          onChange={async (e) => {
            const picked = e.target
            const name = picked.selectedOptions[0]?.text
            if (!(await confirm(`${name} takes over ${device.name}? This cannot be undone.`, 'Replace'))) {
              picked.value = ''
              return
            }
            onError(await edit('POST', `devices/${device.id}/replace`, { with: picked.value }))
          }}
          className="min-w-0 flex-1 rounded bg-neutral-800 px-2 py-1"
        >
          <option value="" disabled>
            Replace with…
          </option>
          {candidates.map((d) => (
            <option key={d.id} value={d.id}>
              {d.name}
              {d.model && ` (${d.model})`}
            </option>
          ))}
        </select>
        <button
          onClick={async () => (await confirm(`Delete ${device.name} for good?`, 'Delete')) && onError(await edit('DELETE', `devices/${device.id}`))}
          className="rounded bg-neutral-800 px-3 py-1 text-red-400 hover:bg-neutral-700"
        >
          Delete
        </button>
      </div>
    </section>
  )
}

function Control(props: CapProps) {
  const { cap } = props
  if (cap.access.settable && cap.type === 'binary') {
    return (
      <div className="flex items-center justify-between gap-2 text-sm">
        <span className="text-neutral-400">{cap.label}</span>
        <Toggle {...props} />
      </div>
    )
  }
  if (cap.access.settable && cap.type === 'numeric' && cap.min != null && cap.max != null) return <Slider {...props} />
  if (cap.access.settable && cap.type === 'enum') return <Select {...props} />
  return <Reading {...props} />
}

const ref = ({ target, cap }: CapProps): Ref => ({ target, capability: cap.key })

// Health on a Tile, only when wrong: a battery's level once low, a tamper or low flag once on.
function HealthBadge(props: CapProps) {
  const data = useValue(ref(props))?.data
  if (!unwell(data)) return null
  if (typeof data !== 'number') return <span className="text-red-400">{props.cap.label}</span>
  const level = Math.max(0, Math.min(100, data))
  return (
    <span title={props.cap.label} className="flex items-center gap-1 text-red-400">
      <svg viewBox="0 0 24 12" className="h-2.5 w-5" aria-hidden>
        <rect x="0.5" y="0.5" width="20" height="11" rx="2" fill="none" stroke="currentColor" />
        <rect x="21.5" y="3.5" width="2" height="5" rx="1" fill="currentColor" />
        <rect x="2" y="2" width={(17 * level) / 100} height="8" rx="1" fill="currentColor" />
      </svg>
      {Math.round(level)}%
    </span>
  )
}

function Toggle(props: CapProps) {
  const { target, cap } = props
  const on = useValue(ref(props))?.data === true
  return <Switch on={on} onChange={(v) => sendCommand(target, { [cap.key]: v })} label={cap.label} />
}

// Which of a colour light's modes it shows, if it tells: hs, xy or color_temp.
// The other mode's control shows a value derived from it, so it is dimmed.
function useColorMode(target: Target) {
  return useValue({ target, capability: 'color_mode' })?.data as string | undefined
}

function Slider(props: CapProps) {
  const { target, cap, fade } = props
  const { value, set, picking, hold } = useControl<number>(target, cap.key, { fade })
  const mode = useColorMode(target)
  return (
    <RangeSlider
      cap={cap}
      value={typeof value === 'number' ? value : cap.min!}
      dim={cap.key === 'color_temp' && mode !== undefined && mode !== 'color_temp' && !picking}
      onDragging={hold}
      onChange={set}
      className="text-sm"
    />
  )
}

function Color(props: CapProps) {
  const { target, cap } = props
  const { value, set, picking } = useControl<HS>(target, cap.key)
  const mode = useColorMode(target)
  return (
    <div className="flex items-center justify-between gap-2 text-sm">
      <span className="text-neutral-400">Color</span>
      <ColorPicker value={value ?? { hue: 0, saturation: 0 }} dim={mode === 'color_temp' && !picking} onChange={set} label={cap.label} />
    </div>
  )
}

function Select(props: CapProps) {
  const { target, cap } = props
  const value = useValue(ref(props))
  const current = typeof value?.data === 'string' ? value.data : ''
  return (
    <label className="flex items-center justify-between gap-2 text-sm">
      <span className="text-neutral-400">{cap.label}</span>
      <select value={current} onChange={(e) => sendCommand(target, { [cap.key]: e.target.value })} className="rounded bg-neutral-800 px-2 py-1">
        <option value="" disabled>
          —
        </option>
        {current !== '' && !cap.options?.includes(current) && (
          <option disabled>{current}</option> // reported, but not one a Command may set
        )}
        {cap.options?.map((o) => (
          <option key={o}>{o}</option>
        ))}
      </select>
    </label>
  )
}

function Reading(props: CapProps) {
  const { cap } = props
  const value = useValue(ref(props))
  const now = useNow()
  return (
    <div className="flex items-baseline justify-between gap-2 text-sm">
      <span className="text-neutral-400">{cap.label}</span>
      <span>
        {value ? format(value.data, cap.unit) : '—'}
        {value && now - new Date(value.at).getTime() > STALE_AFTER && <span className="ml-2 text-xs text-amber-400/80">{age(value.at, now)}</span>}
      </span>
    </div>
  )
}

function LastEvent(props: CapProps) {
  const { cap } = props
  const event = useLastEvent(ref(props))
  const now = useNow()
  return (
    <div className="flex items-baseline justify-between gap-2 text-sm">
      <span className="text-neutral-400">{cap.label}</span>
      {event ? (
        <span>
          {String(event.data)}
          <span className="ml-2 text-xs text-neutral-500">{age(event.at, now)}</span>
        </span>
      ) : (
        <span className="text-neutral-500">none yet</span>
      )}
    </div>
  )
}

const relative = new Intl.RelativeTimeFormat('en', { numeric: 'auto', style: 'narrow' })

// A Value's age is only worth showing once it is stale.
const STALE_AFTER = 60 * 60 * 1000

function age(at: string, now: number) {
  const seconds = Math.round((new Date(at).getTime() - now) / 1000)
  if (seconds > -10) return 'just now'
  if (seconds > -3600) return relative.format(Math.round(seconds / 60), 'minute')
  if (seconds > -86400) return relative.format(Math.round(seconds / 3600), 'hour')
  return relative.format(Math.round(seconds / 86400), 'day')
}
