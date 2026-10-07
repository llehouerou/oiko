import { useEffect, useState } from 'react'
import { mdiCheck, mdiContentCopy, mdiViewDashboardEditOutline } from '@mdi/js'
import { useAggregates, useAreas, useAutomationStatuses, useDashboardList, useDashboards, useDevices, useFlags } from './store'
import { useAllows, useMe } from './access'
import { copied, dashboard, shown } from './dashboard'
import { ActionSetting, AppBar, ChartsSetting, CreateButton } from './AppBar'
import { ReleaseBanner } from './About'
import { ChartsShown } from './MiniChart'
import { Svg } from './icons'
import { CustomDashboardView, Dashboard } from './Dashboard'
import { CreateDashboard, DashboardMenu } from './DashboardMenu'
import { DashboardEditor } from './DashboardEditor'
import { FlagPill } from './Tiles'
import { AggregatePanel, AreaPanel, DevicePanel, FlagPanel } from './HomePanels'

// The Home page: the Dashboard hash opens, at #dashboard/<id>, the Person's first one shown
// otherwise, and the panels an Admin opens from it. The built-in Dashboard is drawn from the
// Sections derived from the Areas, under the Flags without an Area as pills. A Person switches
// Dashboards from the menu that takes the Home tab's place.
export function Home({ hash }: { hash: string }) {
  const devices = useDevices()
  const aggregates = useAggregates()
  const flags = useFlags()
  const areas = useAreas()
  const automations = useAutomationStatuses() // their Tiles: those with a Manual trigger show
  const dashboards = useDashboards()
  const list = useDashboardList()
  const identity = useMe()?.identity
  const person = !!identity && 'person' in identity
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
  const home = dashboard(devices, aggregates, flags, areas, automations)
  const { pills, sections } = home
  const current = shown(hash, dashboards, list) // undefined: the built-in Dashboard
  // Whether the viewer changes the current Dashboard: an Admin arranges the built-in one (the
  // Areas' order and their Layouts) and edits a shared one, a Person edits their own.
  const editable = current ? !current.shared || admin : admin
  // The Dashboard being changed, by id, if one is. Leaving it, or its going, ends the change.
  const [editingId, setEditingId] = useState<string | null>(null)
  const editing = editable && editingId === (current?.id ?? 'builtin')
  const arrange = editing && !current
  // Whether the Person is duplicating the current Dashboard into a new one.
  const [duplicating, setDuplicating] = useState(false)
  return (
    <>
      <AppBar
        page="#"
        home={person && <DashboardMenu current={current} dashboards={dashboards} list={list} onCreated={setEditingId} />}
        settings={
          (member || person) && (
            <>
              {member && <ChartsSetting charts={charts} onCharts={toggleCharts} />}
              {editable && (
                <ActionSetting
                  icon={mdiViewDashboardEditOutline}
                  label={current ? 'Edit dashboard' : 'Arrange dashboard'}
                  onClick={() => setEditingId(current?.id ?? 'builtin')}
                />
              )}
              {person && <ActionSetting icon={mdiContentCopy} label="Duplicate…" onClick={() => setDuplicating(true)} />}
            </>
          )
        }
      />
      <main className="mx-auto max-w-[120rem] space-y-6 p-4 pb-24 lg:px-8">
        {admin && <ReleaseBanner />}
        {!current && pills.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {pills.map((f) => (
              <FlagPill key={f.id} flag={f} onOpen={admin ? () => setOpenId(f.id) : undefined} />
            ))}
          </div>
        )}
        <ChartsShown value={charts}>
          {current && editing ? (
            <DashboardEditor
              dashboard={current}
              sections={home.custom(current, true)}
              areas={areas}
              choices={home.choices}
              onDuplicate={() => setDuplicating(true)}
              onDone={() => setEditingId(null)}
              onResult={setToast}
            />
          ) : current ? (
            <CustomDashboardView
              id={current.id}
              columns={current.columns}
              sections={home.custom(current)}
              onOpen={admin ? setOpenId : undefined}
              onResult={setToast}
            />
          ) : (
            <Dashboard sections={sections} arranging={arrange} onOpen={admin ? setOpenId : undefined} onResult={setToast} />
          )}
        </ChartsShown>
      </main>
      {arrange ? (
        <button
          onClick={() => setEditingId(null)}
          className="fixed right-6 bottom-6 z-10 flex h-14 items-center gap-2 rounded-2xl bg-amber-400 px-5 font-medium text-neutral-950 shadow-xl shadow-amber-500/20 hover:bg-amber-300"
        >
          <Svg path={mdiCheck} className="size-6" />
          Done
        </button>
      ) : (
        admin && !editing && <CreateButton onCreate={(what) => setOpenId(what)} />
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
      {duplicating && (
        <CreateDashboard
          title={`Duplicate ${current?.name ?? 'Home'}`}
          copy={copied(current, areas)}
          onCreated={setEditingId}
          onClose={() => setDuplicating(false)}
        />
      )}
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
