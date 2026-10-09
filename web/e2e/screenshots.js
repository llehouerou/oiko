// Writes docs/images/dashboard.png, the README's picture of the built-in Dashboard, from the
// made-up home in home.json: a temporary Mosquitto holds its zigbee2mqtt messages, retained, for
// a fresh Oiko's zigbee2mqtt Bridge; its Areas are created and its Devices placed through the API.
// `make screenshots` runs it, from the dev shell.
import { execFileSync, spawn } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { connect } from 'node:net'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { freePort, start } from './oiko.js'

const home = JSON.parse(readFileSync(new URL('home.json', import.meta.url)))
const out = fileURLToPath(new URL('../../docs/images/dashboard.png', import.meta.url))

const dir = mkdtempSync(join(tmpdir(), 'oiko-mqtt-'))
const port = await freePort()
writeFileSync(join(dir, 'mosquitto.conf'), `listener ${port} 127.0.0.1\nallow_anonymous true\n`)
const mosquitto = spawn('mosquitto', ['-c', join(dir, 'mosquitto.conf')], { stdio: 'ignore' })
let oiko
try {
  const listening = () =>
    new Promise((resolve) => {
      const s = connect(port, '127.0.0.1', () => resolve(s.end() && true)).on('error', () => resolve(false))
    })
  for (let tries = 0; !(await listening()); tries++) {
    if (tries === 100) throw new Error(`mosquitto does not listen on ${port}`)
    await new Promise((r) => setTimeout(r, 100))
  }
  // What zigbee2mqtt would have left on the broker, before Oiko subscribes.
  const publish = (topic, message) =>
    execFileSync('mosquitto_pub', ['-h', '127.0.0.1', '-p', `${port}`, '-q', '1', '-r', '-t', `zigbee2mqtt/${topic}`, '-m', JSON.stringify(message)])
  const devices = home.areas.flatMap((a) => a.devices)
  publish('bridge/state', { state: 'online' })
  publish(
    'bridge/devices',
    devices.map((d) => ({ ieee_address: d.ieee, type: 'EndDevice', friendly_name: d.name, definition: { model: d.model, ...home.models[d.model] } })),
  )
  for (const d of devices) {
    publish(d.name, d.state)
    publish(`${d.name}/availability`, { state: 'online' })
  }

  oiko = await start({ bridges: { zigbee2mqtt: { broker: `mqtt://127.0.0.1:${port}` } } })
  const page = await (await oiko.browser.newContext({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 2, reducedMotion: 'reduce' })).newPage()
  await oiko.claim(page)
  await page.evaluate(async (areas) => {
    const ok = async (r) => {
      if (!r.ok) throw new Error(`${r.url}: ${r.status} ${await r.text()}`)
    }
    // The first event of the stream of updates is a snapshot of the home.
    const snapshot = async () => {
      const reader = (await fetch('/api/updates')).body.pipeThrough(new TextDecoderStream()).getReader()
      let text = ''
      while (!text.includes('\n\n')) text += (await reader.read()).value
      await reader.cancel()
      return JSON.parse(text.slice('data: '.length, text.indexOf('\n\n')))
    }
    for (const a of areas) await ok(await fetch('/api/areas', { method: 'POST', body: JSON.stringify({ name: a.name, icon: a.icon }) }))
    const snap = await snapshot()
    for (const a of areas) {
      const area = snap.areas.find((x) => x.name === a.name).id
      for (const d of a.devices) {
        const device = snap.devices.find((x) => x.name === d.name)
        if (!device) throw new Error(`no Device ${d.name}`)
        await ok(await fetch('/api/area', { method: 'PUT', body: JSON.stringify({ target: `device:${device.id}`, area }) }))
      }
    }
  }, home.areas)
  await page.goto(oiko.base)
  await page.getByText(home.areas.at(-1).devices.at(-1).name).waitFor()
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(1000) // the 24 h charts' History
  await page.screenshot({ path: out, animations: 'disabled', caret: 'hide' })
  console.log(`wrote ${out}`)
} finally {
  await oiko?.stop()
  mosquitto.kill()
  rmSync(dir, { recursive: true, force: true })
}
