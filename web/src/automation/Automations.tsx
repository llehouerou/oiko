// The Automation editor: the list of Automations, and the graph of the open
// one. Edits stay in the editor until Save, which hot-reloads the Automation.
// Only an Admin edits; a Member reads them, their Runs and their Traces.

import '@xyflow/react/dist/style.css'
import {
  applyEdgeChanges,
  applyNodeChanges,
  Background,
  Controls,
  ReactFlow,
  ReactFlowProvider,
  useNodesInitialized,
  useReactFlow,
  type Edge as FlowEdge,
  type OnConnectEnd,
} from '@xyflow/react'
import { useEffect, useState } from 'react'
import { edit, runManual, useAutomationStatuses, useLiveRuns } from '../store'
import { confirm } from '../confirm'
import type { AutomationStatus, StepState, Trace } from '../types'
import { catalogue, connectionError, groups, kinds, newStep, reloadReport, type Kind } from './kinds'
import { ManualRun } from './kinds/manualTrigger'
import { flowEdge, spread, stepLabel, toDocument, toFlow, type Document } from './model'
import { Inspect, RunsDrawer } from './Runs'
import { onPath, pathOf } from './runtime'
import { BrokenStep, OpenStep, StepNode } from './StepNode'
import { titleIfTruncated } from '../truncated'
import { AppBar } from '../AppBar'
import { api, useAllows } from '../access'

const nodeTypes = { step: StepNode }

const dot = { enabled: 'bg-emerald-400', disabled: 'bg-neutral-500', broken: 'bg-red-500', runaway: 'bg-orange-500' }

export function Automations() {
  const statuses = useAutomationStatuses()
  const admin = useAllows('admin')
  const [docs, setDocs] = useState<Document[]>([])
  // #automations/<id>/<run> opens with that Run picked: a History marker links to it.
  const [, linkedId, linkedRun] = location.hash.split('/')
  const [openId, setOpenId] = useState<string | null>(linkedId ?? null)
  const [run, setRun] = useState<string | null>(linkedRun ?? null) // picked in the open Automation
  const [dirty, setDirty] = useState(false)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    api('/api/automations')
      .then((r) => r.json())
      .then(setDocs, (e) => setError(String(e)))
  }, [])
  const open = docs.find((d) => d.id === openId)
  const leave = async () => !dirty || confirm('Discard the unsaved changes?', 'Discard')
  const openRun = async (automation: string, r: string) => {
    if (!docs.some((d) => d.id === automation) || (automation !== openId && !(await leave()))) return // a deleted Automation has no Runs to show
    setOpenId(automation)
    setRun(r)
  }
  const automationName = (id: string) => docs.find((d) => d.id === id)?.name ?? 'deleted automation'
  const create = async () => {
    const name = prompt('Name of the new automation')
    if (!name || !(await leave())) return
    const doc = { id: '', name, enabled: false, steps: [], edges: [] }
    const res = await api('/api/automations', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(doc) })
    if (!res.ok) return setError((await res.text()).trim())
    const { id } = await res.json()
    setDocs([...docs, { ...doc, id }])
    setOpenId(id)
  }

  return (
    <div className="flex h-dvh flex-col">
      <AppBar page="#automations" onLeave={leave} />
      <div className="flex min-h-0 flex-1 text-sm">
        <aside className="flex w-56 shrink-0 flex-col gap-1 overflow-y-auto border-r border-neutral-800 p-3">
          <h1 className="mb-1 text-xs font-semibold tracking-wide text-neutral-500 uppercase">Automations</h1>
          {docs.map((d) => (
            <button
              key={d.id}
              onClick={async () => {
                if (d.id === openId || !(await leave())) return
                setOpenId(d.id)
                setRun(null)
              }}
              className={`flex items-center gap-2 rounded px-2 py-1 text-left ${d.id === openId ? 'bg-neutral-800' : 'hover:bg-neutral-900'}`}
            >
              <span className={`size-2 shrink-0 rounded-full ${dot[statuses.find((s) => s.id === d.id)?.status ?? 'disabled']}`} />
              <span className="truncate" onMouseEnter={titleIfTruncated}>
                {d.name}
              </span>
            </button>
          ))}
          {admin && (
            <button onClick={create} className="mt-2 text-left text-amber-400 hover:text-amber-300">
              + New automation
            </button>
          )}
          {error && <p className="text-red-400">{error}</p>}
        </aside>
        {open ? (
          <ReactFlowProvider key={open.id}>
            <Editor
              doc={open}
              status={statuses.find((s) => s.id === open.id)}
              run={run}
              onPickRun={setRun}
              openRun={openRun}
              automationName={automationName}
              onDirty={setDirty}
              onSaved={(d) => setDocs(docs.map((x) => (x.id === d.id ? d : x)))}
              onDeleted={() => {
                setDocs(docs.filter((x) => x.id !== open.id))
                setOpenId(null)
                setDirty(false)
              }}
            />
          </ReactFlowProvider>
        ) : (
          <p className="m-auto text-neutral-500">{admin ? 'Pick an automation, or create one.' : 'Pick an automation.'}</p>
        )}
      </div>
    </div>
  )
}

function Editor({
  doc,
  status,
  run,
  onPickRun,
  openRun,
  automationName,
  onDirty,
  onSaved,
  onDeleted,
}: {
  doc: Document
  status?: AutomationStatus
  run: string | null
  onPickRun: (run: string | null) => void
  openRun: (automation: string, run: string) => void
  automationName: (id: string) => string
  onDirty: (dirty: boolean) => void
  onSaved: (d: Document) => void
  onDeleted: () => void
}) {
  const rf = useReactFlow()
  const admin = useAllows('admin')
  const [saved, setSaved] = useState(() => spread(doc))
  const [name, setName] = useState(doc.name)
  const [nodes, setNodes] = useState(() => toFlow(saved).nodes)
  const [edges, setEdges] = useState<FlowEdge[]>(() => toFlow(saved).edges)
  const [toast, setToast] = useState<{ text: string; error?: boolean } | null>(null)
  const [openStep, setOpenStep] = useState<string | null>(null)
  const draft = toDocument({ ...saved, name }, nodes, edges)
  const dirty = JSON.stringify(draft) !== JSON.stringify(saved)
  useEffect(() => onDirty(dirty), [dirty, onDirty])
  // The fitView prop fits as soon as the first node is measured, missing the others:
  // fit once all of them are. The Editor remounts per Automation, so this runs on each load.
  const measured = useNodesInitialized()
  const [fitted, setFitted] = useState(false)
  useEffect(() => {
    if (!measured || fitted) return
    rf.fitView()
    setFitted(true)
  }, [measured, fitted, rf])
  useEffect(() => {
    if (!toast) return
    const t = setTimeout(() => setToast(null), 8000)
    return () => clearTimeout(t)
  }, [toast])

  // What the Steps remember now, refreshed after each Run and each Save.
  const liveRuns = useLiveRuns(doc.id)
  const [states, setStates] = useState<StepState[]>([])
  useEffect(() => {
    api(`/api/automations/${doc.id}/state`)
      .then((r) => (r.ok ? r.json() : []))
      .then(setStates, () => {})
  }, [doc.id, liveRuns, saved])
  // The Trace of the picked Run: its path lights up, the rest dims.
  const [trace, setTrace] = useState<Trace>()
  useEffect(() => {
    if (!run) return
    let stale = false
    api(`/api/runs/${run}`).then(async (r) => {
      const body = r.ok ? await r.json() : (await r.text()).trim()
      if (stale) return
      if (r.ok) setTrace(body)
      else setToast({ text: `No Trace of this Run: ${body}`, error: true })
    })
    return () => {
      stale = true
    }
  }, [run])
  const picked = run && trace?.run === run ? trace : undefined
  const path = picked && pathOf(picked)
  const shownEdges: FlowEdge[] = path
    ? edges.map((e) => (onPath(path, e) ? { ...e, animated: true, style: { stroke: '#fcd34d', strokeWidth: 2 } } : { ...e, style: { opacity: 0.2 } }))
    : edges
  const stepName = (id: string) => {
    const n = nodes.find((x) => x.id === id)
    return n ? stepLabel({ id, name: n.data.name }) : `${id} (deleted)`
  }
  // A Manual trigger Step runs the saved Automation, and shows its Run.
  const manual = {
    blocked: dirty ? 'save first' : status && status.status !== 'enabled' ? `it is ${status.status}` : null,
    run: async (step: string) => {
      const r = await runManual(doc.id, step)
      setToast({ text: `${stepName(step)}: ${r.text}`, error: r.error })
      if (r.run) onPickRun(r.run)
    },
  }

  // Replaces the Automation with d, which hot-reloads it.
  const put = async (d: Document) => {
    const err = await edit('PUT', `automations/${d.id}`, d)
    if (err) {
      setToast({ text: err, error: true })
      return false
    }
    setSaved(d)
    onSaved(d)
    return true
  }
  const save = async () => {
    if (!(await put(draft))) return
    if (!draft.enabled) return setToast({ text: 'Saved. It is disabled, so it keeps no Step state.' })
    const { kept, dropped, empty } = reloadReport(saved, draft)
    const parts = [
      kept.length && `kept for ${kept.join(', ')}`,
      dropped.length && `dropped for ${dropped.join(', ')}`,
      empty.length && `empty for ${empty.join(', ')}`,
    ]
    const report = parts.filter(Boolean).join('; ')
    setToast({ text: `Saved and reloaded.${report ? ` Step state ${report}.` : ''}` })
  }
  const revert = () => {
    const f = toFlow(saved)
    setNodes(f.nodes)
    setEdges(f.edges)
    setName(saved.name)
  }
  // Enabling or disabling applies at once, to the saved Automation; a
  // runaway one is re-enabled the same way.
  const setEnabled = (enabled: boolean) => put({ ...saved, enabled })
  const remove = async () => {
    if (!(await confirm(`Delete ${saved.name}?`, 'Delete'))) return
    const err = await edit('DELETE', `automations/${saved.id}`)
    if (err) setToast({ text: err, error: true })
    else onDeleted()
  }

  // Why a connection dropped at the end of a drag was refused: on a handle,
  // or on a trigger, which has none.
  const onConnectEnd: OnConnectEnd = (event, s) => {
    if (s.isValid || !s.fromHandle) return
    const point = 'changedTouches' in event ? event.changedTouches[0]! : event
    const to = s.toHandle?.nodeId ?? document.elementFromPoint(point.clientX, point.clientY)?.closest('.react-flow__node')?.getAttribute('data-id')
    if (!to) return
    const from = s.fromHandle
    const toHandle = s.toHandle?.id ?? null
    const c =
      from.type === 'source'
        ? { source: from.nodeId, sourceHandle: from.id ?? null, target: to, targetHandle: toHandle }
        : { source: to, sourceHandle: toHandle, target: from.nodeId, targetHandle: from.id ?? null }
    setToast({ text: connectionError(c, nodes, edges) ?? 'Outputs join inputs only.', error: true })
  }

  return (
    <section className="flex min-w-0 flex-1 flex-col">
      <header className="flex items-center gap-3 border-b border-neutral-800 px-3 py-2">
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          readOnly={!admin}
          aria-label="Automation name"
          className="min-w-0 flex-1 rounded bg-transparent px-1 text-base font-medium hover:bg-neutral-900 focus:bg-neutral-900 focus:outline-none"
        />
        {admin && (
          <>
            <label className="flex items-center gap-1 text-neutral-400">
              <input type="checkbox" checked={saved.enabled} onChange={(e) => setEnabled(e.target.checked)} className="accent-amber-400" />
              enabled
            </label>
            <button onClick={remove} className="text-red-400 hover:text-red-300">
              Delete
            </button>
            <button disabled={!dirty} onClick={revert} className="text-neutral-400 hover:text-white disabled:opacity-30">
              Revert
            </button>
            <button
              disabled={!dirty}
              onClick={save}
              className="rounded bg-amber-400 px-3 py-1 font-medium text-neutral-900 hover:bg-amber-300 disabled:bg-neutral-700 disabled:text-neutral-400"
            >
              {dirty ? 'Save' : 'Saved'}
            </button>
          </>
        )}
      </header>
      <StatusBanner status={status} onEnable={admin ? () => setEnabled(true) : undefined} />
      {/* The Runs sit beside the canvas when the screen is wide enough, below it otherwise. */}
      <div className="flex min-h-0 flex-1 flex-col 2xl:flex-row">
        <div className="flex min-h-0 min-w-0 flex-1">
          <aside hidden={!admin} className="w-40 shrink-0 space-y-3 overflow-y-auto border-r border-neutral-800 p-2 text-xs">
            {groups.map((g) => (
              <div key={g}>
                <div className="mb-1 font-semibold tracking-wide text-neutral-500 uppercase">{g}</div>
                {kinds
                  .filter((k) => catalogue[k].group === g)
                  .map((k) => (
                    <div
                      key={k}
                      draggable
                      onDragStart={(e) => e.dataTransfer.setData('application/x-oiko-step', k)}
                      className="mb-1 cursor-grab rounded bg-neutral-800 px-2 py-1 hover:bg-neutral-700"
                    >
                      {catalogue[k].label}
                    </div>
                  ))}
              </div>
            ))}
            <p className="text-neutral-500">Drag a Step onto the canvas.</p>
          </aside>
          <div
            className="relative min-w-0 flex-1"
            onDragOver={(e) => e.preventDefault()}
            onDrop={(e) => {
              const kind = admin && (e.dataTransfer.getData('application/x-oiko-step') as Kind)
              if (!kind) return
              const position = rf.screenToFlowPosition({ x: e.clientX, y: e.clientY })
              const step = newStep(
                kind,
                nodes.map((n) => n.id),
                position,
              )
              setNodes((ns) => [...ns, step])
              setOpenStep(step.id)
            }}
          >
            <Inspect value={{ trace: picked, path, states, openRun, automationName }}>
              <BrokenStep value={status?.status === 'broken' ? status.step : undefined}>
                <OpenStep value={{ open: openStep, setOpen: setOpenStep }}>
                  <ManualRun value={manual}>
                    <ReactFlow
                      nodes={nodes}
                      edges={shownEdges}
                      nodeTypes={nodeTypes}
                      onNodesChange={(changes) => setNodes((ns) => applyNodeChanges(changes, ns))}
                      onEdgesChange={(changes) => setEdges((es) => applyEdgeChanges(changes, es))}
                      onConnect={(c) => setEdges((es) => [...es, flowEdge(c)])}
                      isValidConnection={(c) => !connectionError(c, nodes, edges)}
                      nodesDraggable={admin}
                      nodesConnectable={admin}
                      deleteKeyCode={admin ? 'Backspace' : null}
                      onConnectEnd={onConnectEnd}
                      colorMode="dark"
                      minZoom={0.1}
                      proOptions={{ hideAttribution: true }}
                    >
                      <Background />
                      <Controls />
                    </ReactFlow>
                  </ManualRun>
                </OpenStep>
              </BrokenStep>
            </Inspect>
            {toast && (
              <button
                onClick={() => setToast(null)}
                className={`absolute top-3 left-1/2 z-10 max-w-xl -translate-x-1/2 rounded px-3 py-2 text-left shadow-lg ${toast.error ? 'bg-red-950 text-red-200' : 'bg-neutral-800'}`}
              >
                {toast.text}
              </button>
            )}
          </div>
        </div>
        <RunsDrawer automation={doc.id} picked={run} onPick={onPickRun} stepName={stepName} />
      </div>
    </section>
  )
}

// Why the Automation does not run, if it does not; an Admin may enable it again.
function StatusBanner({ status, onEnable }: { status?: AutomationStatus; onEnable?: () => void }) {
  if (!status || status.status === 'enabled') return null
  const button = (label: string) =>
    onEnable && (
      <button onClick={onEnable} className="rounded bg-neutral-700 px-2 py-0.5 text-xs text-white hover:bg-neutral-600">
        {label}
      </button>
    )
  const [text, action] = {
    broken: [`Broken: ${status.reason}. It won't run until it is edited.`, null],
    runaway: [`Runaway since ${new Date(status.since ?? '').toLocaleString()}: more than 20 Runs within 1 s. Stopped until re-enabled.`, button('Re-enable')],
    disabled: ['Disabled: it never runs.', button('Enable')],
  }[status.status]
  return (
    <div className={`flex items-center justify-between gap-3 px-3 py-2 ${status.status === 'disabled' ? 'bg-neutral-800' : 'bg-red-950 text-red-200'}`}>
      <span>{text}</span>
      {action}
    </div>
  )
}
