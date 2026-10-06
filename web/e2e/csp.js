// Loads the built client, served by Oiko under its Content Security Policy
// (ADR 0034), in a headless Chromium, and fails on any violation or page
// error. Run after `npm run build`, from the dev shell, which sets CHROMIUM.
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
const base = `http://127.0.0.1:${port}`
const oiko = spawn(join(dir, 'oiko'), ['-listen', `127.0.0.1:${port}`, '-data', join(dir, 'data')], { stdio: 'ignore' })

const problems = []
let browser
try {
  const up = () =>
    fetch(`${base}/api/build`).then(
      (r) => r.ok,
      () => false,
    )
  for (let tries = 0; !(await up()); tries++) {
    if (tries === 100) throw new Error(`oiko does not answer on ${base}`)
    await new Promise((r) => setTimeout(r, 100))
  }
  // An Automation, for its editor.
  const { id } = await (await fetch(`${base}/api/automations`, { method: 'POST', body: '{"name": "Night"}' })).json()

  browser = await chromium.launch({ executablePath: process.env.CHROMIUM })
  const page = await browser.newPage()
  await page.addInitScript(() => {
    window.violations = []
    document.addEventListener('securitypolicyviolation', (e) =>
      window.violations.push(`${e.effectiveDirective} blocked ${e.blockedURI || 'inline'} at ${e.sourceFile}:${e.lineNumber}`),
    )
  })
  page.on('pageerror', (e) => problems.push(`page error: ${e.message}`))
  for (const hash of ['', '#history', '#automations', `#automations/${id}`]) {
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
