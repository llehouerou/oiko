import { useEffect, useState } from 'react'
import { mdiCheck } from '@mdi/js'
import { useAggregates, useAreas, useAutomationStatuses, useDevices, useFlags } from './store'
import { useAllows } from './access'
import { dashboard } from './dashboard'
import { AppBar, ArrangeSetting, ChartsSetting, CreateButton } from './AppBar'
import { ReleaseBanner } from './About'
import { ChartsShown } from './MiniChart'
import { Svg } from './icons'
import { Dashboard } from './Dashboard'
import { FlagPill } from './Tiles'
import { AggregatePanel, AreaPanel, DevicePanel, FlagPanel } from './HomePanels'

// The Home page: the built-in Dashboard, drawn from the Sections derived from the Areas, under the
// Flags without an Area as pills, and the panels an Admin opens from it.
export function Home() {
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
  // Whether an Admin is arranging the dashboard: the Areas' order and their Layouts.
  const [arranging, setArranging] = useState(false)
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
          <Dashboard sections={sections} arranging={arranging} onOpen={admin ? setOpenId : undefined} onResult={setToast} />
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
