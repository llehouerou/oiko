// The harness of the browser scripts: builds Oiko, starts it on a free port with a fresh data
// directory, never data/, and a headless Chromium; claim makes Alice its first Admin, with a
// Passkey in Chromium's virtual authenticator. Run after `npm run build`, from the dev shell, which
// sets CHROMIUM.
import { execFileSync, spawn } from 'node:child_process'
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { createServer } from 'node:net'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright-core'

// A port nothing listens on.
export const freePort = () =>
  new Promise((resolve) => {
    const s = createServer().listen(0, '127.0.0.1', () => {
      const { port } = s.address()
      s.close(() => resolve(port))
    })
  })

// Waits for check to hold, ten seconds at most; what names what it waits for.
export async function until(check, what) {
  for (let tries = 0; !(await check()); tries++) {
    if (tries === 100) throw new Error(`${what}: still not after 10 s`)
    await new Promise((r) => setTimeout(r, 100))
  }
}

// Oiko with config.json holding config, if given. base is where sign-in works without a Public URL.
export async function start(config) {
  if (!process.env.CHROMIUM) throw new Error('CHROMIUM is unset: run from the dev shell')
  const root = fileURLToPath(new URL('../..', import.meta.url))
  const dir = mkdtempSync(join(tmpdir(), 'oiko-e2e-'))
  const data = join(dir, 'data')
  mkdirSync(data)
  if (config) writeFileSync(join(data, 'config.json'), JSON.stringify(config))
  execFileSync('go', ['build', '-o', join(dir, 'oiko'), './cmd/oiko'], { cwd: root, stdio: 'inherit' })
  const port = await freePort()
  const base = `http://localhost:${port}`
  // GOPROXY=off: no Release check, so nothing from the network.
  const oiko = spawn(join(dir, 'oiko'), ['-listen', `127.0.0.1:${port}`, '-data', data], {
    stdio: ['ignore', 'ignore', 'pipe'],
    env: { ...process.env, GOPROXY: 'off' },
  })
  // Oiko's log shows the Setup link that claims it.
  const setupLink = new Promise((resolve) => {
    let log = ''
    oiko.stderr.on('data', (d) => {
      log += d
      const m = log.match(/open (\S+\/setup#\S+)/)
      if (m) resolve(m[1])
    })
  })
  let browser
  const stop = async () => {
    await browser?.close()
    oiko.kill()
    rmSync(dir, { recursive: true, force: true })
  }
  try {
    const up = () =>
      fetch(`http://127.0.0.1:${port}/api/me`).then(
        (r) => r.ok,
        () => false,
      )
    await until(up, `oiko answering on ${base}`)
    browser = await chromium.launch({ executablePath: process.env.CHROMIUM })
  } catch (e) {
    await stop()
    throw e
  }

  // Alice claims Oiko on page through the Setup link, which signs her in, and adds a Passkey.
  const claim = async (page) => {
    const cdp = await page.context().newCDPSession(page)
    await cdp.send('WebAuthn.enable')
    await cdp.send('WebAuthn.addVirtualAuthenticator', {
      options: { protocol: 'ctap2', transport: 'internal', hasResidentKey: true, hasUserVerification: true, isUserVerified: true },
    })
    await page.goto(base)
    await page.getByText('no Admin yet').waitFor()
    await page.goto(await setupLink)
    await page.getByLabel('Your name').fill('Alice')
    await page.getByRole('button', { name: 'Continue' }).click()
    await page.getByRole('button', { name: 'Add a Passkey' }).click()
    await page.locator('nav a[href="#automations"]').waitFor()
  }
  return { base, browser, claim, stop }
}
