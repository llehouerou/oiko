// Loads the built client, served by Oiko under its Content Security Policy
// (ADR 0034), in a headless Chromium, and fails on any violation or page
// error. On the way, its first Admin adds a Passkey to Chromium's virtual
// authenticator and signs in with it again. Run after `npm run build`, from
// the dev shell, which sets CHROMIUM.
import { execFileSync, spawn } from 'node:child_process'
import { mkdtempSync, rmSync } from 'node:fs'
import { createServer } from 'node:net'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright-core'

if (!process.env.CHROMIUM) throw new Error('CHROMIUM is unset: run from the dev shell')
const root = fileURLToPath(new URL('../..', import.meta.url))
const dir = mkdtempSync(join(tmpdir(), 'oiko-csp-'))
execFileSync('go', ['build', '-o', join(dir, 'oiko'), './cmd/oiko'], { cwd: root, stdio: 'inherit' })
const port = await new Promise((resolve) => {
  const s = createServer().listen(0, '127.0.0.1', () => {
    const { port } = s.address()
    s.close(() => resolve(port))
  })
})
const base = `http://localhost:${port}` // where sign-in works without a Public URL
const oiko = spawn(join(dir, 'oiko'), ['-listen', `127.0.0.1:${port}`, '-data', join(dir, 'data')], { stdio: ['ignore', 'ignore', 'pipe'] })
// Oiko's log shows the Setup link that claims it.
const setupLink = new Promise((resolve) => {
  let log = ''
  oiko.stderr.on('data', (d) => {
    log += d
    const m = log.match(/open (\S+\/setup#\S+)/)
    if (m) resolve(m[1])
  })
})

const problems = []
let browser
try {
  const up = () =>
    fetch(`http://127.0.0.1:${port}/api/me`).then(
      (r) => r.ok,
      () => false,
    )
  for (let tries = 0; !(await up()); tries++) {
    if (tries === 100) throw new Error(`oiko does not answer on ${base}`)
    await new Promise((r) => setTimeout(r, 100))
  }

  browser = await chromium.launch({ executablePath: process.env.CHROMIUM })
  const page = await browser.newPage()
  await page.addInitScript(() => {
    window.violations = []
    document.addEventListener('securitypolicyviolation', (e) =>
      window.violations.push(`${e.effectiveDirective} blocked ${e.blockedURI || 'inline'} at ${e.sourceFile}:${e.lineNumber}`),
    )
  })
  page.on('pageerror', (e) => problems.push(`page error: ${e.message}`))
  const cdp = await page.context().newCDPSession(page)
  await cdp.send('WebAuthn.enable')
  await cdp.send('WebAuthn.addVirtualAuthenticator', {
    options: { protocol: 'ctap2', transport: 'internal', hasResidentKey: true, hasUserVerification: true, isUserVerified: true },
  })
  // Sign-in, then the Setup page, which signs in its first Admin and offers a Passkey.
  await page.goto(base)
  await page.getByText('no Admin yet').waitFor()
  await page.goto(await setupLink)
  await page.getByLabel('Your name').fill('Alice')
  await page.getByRole('button', { name: 'Continue' }).click()
  await page.getByRole('button', { name: 'Add a Passkey' }).click()
  await page.locator('nav a[href="#automations"]').waitFor()
  // Signed out, then in again with that Passkey.
  await page.evaluate(() => fetch('/api/sign-out', { method: 'POST' }))
  await page.goto(base)
  await page.getByRole('button', { name: 'Sign in with a Passkey' }).click()
  await page.locator('nav a[href="#automations"]').waitFor()
  // An Automation, for its editor.
  const id = await page.evaluate(async () => (await (await fetch('/api/automations', { method: 'POST', body: '{"name": "Night"}' })).json()).id)

  for (const hash of ['', '#history', '#automations', `#automations/${id}`, '#programs', '#account']) {
    await page.goto(`${base}/${hash}`)
    await page.locator('#root > *').first().waitFor()
    await page.waitForTimeout(1000) // ponytail: a fixed settle; wait on each page's content if it proves flaky
  }
  problems.push(...(await page.evaluate(() => window.violations)))
} finally {
  await browser?.close()
  oiko.kill()
  rmSync(dir, { recursive: true, force: true })
}
if (problems.length > 0) {
  console.error(problems.join('\n'))
  process.exit(1)
}
console.log('no CSP violation')
