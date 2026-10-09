// Loads the built client, served by Oiko under its Content Security Policy
// (ADR 0034), in a headless Chromium, and fails on any violation or page
// error. On the way, its first Admin claims it with a Passkey in Chromium's
// virtual authenticator and signs in with it again, then invites a Person, who
// signs in by their Sign-in link, and pairs a screen as a Kiosk. Run after
// `npm run build`, from the dev shell, which sets CHROMIUM.
import { start } from './oiko.js'

const { base, browser, claim, stop } = await start()
const problems = []
try {
  // Every violation and page error of page, on whichever page it navigates to.
  const watch = async (page) => {
    await page.exposeFunction('violation', (v) => problems.push(v))
    await page.addInitScript(() =>
      document.addEventListener('securitypolicyviolation', (e) =>
        window.violation(`${e.effectiveDirective} blocked ${e.blockedURI || 'inline'} at ${e.sourceFile}:${e.lineNumber}`),
      ),
    )
    page.on('pageerror', (e) => problems.push(`page error: ${e.message}`))
    return page
  }
  const page = await watch(await browser.newPage())
  // Sign-in, then the Setup page, which signs in its first Admin and offers a Passkey.
  await claim(page)
  // Signed out, then in again with that Passkey.
  await page.evaluate(() => fetch('/api/sign-out', { method: 'POST' }))
  await page.goto(base)
  await page.getByRole('button', { name: 'Sign in with a Passkey' }).click()
  await page.locator('nav a[href="#automations"]').waitFor()
  // An Automation, for its editor.
  const id = await page.evaluate(async () => (await (await fetch('/api/automations', { method: 'POST', body: '{"name": "Night"}' })).json()).id)

  for (const hash of ['', '#history', '#automations', `#automations/${id}`, '#persons', '#programs', '#account']) {
    await page.goto(`${base}/${hash}`)
    await page.locator('#root > *').first().waitFor()
    await page.waitForTimeout(1000) // ponytail: a fixed settle; wait on each page's content if it proves flaky
  }
  // A Live view plays through a MediaSource attached by a blob: URL (ADR 0037): one opens under the policy.
  const opened = await page.evaluate(
    () =>
      new Promise((resolve) => {
        const ms = new MediaSource()
        ms.addEventListener('sourceopen', () => resolve(true), { once: true })
        const v = document.createElement('video')
        v.addEventListener('error', () => resolve(false), { once: true })
        v.src = URL.createObjectURL(ms)
        setTimeout(() => resolve(false), 5000)
      }),
  )
  if (!opened) problems.push('a MediaSource does not open under the policy')

  // Alice invites Bob: a Person, then a Sign-in link, its QR code drawn inline.
  await page.goto(`${base}/#persons`)
  await page.getByPlaceholder('Name').fill('Bob')
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  await page.getByRole('button', { name: 'Create a Sign-in link' }).nth(1).click() // Alice's is first
  await page.getByRole('img', { name: 'QR code of the Sign-in link' }).waitFor()
  const link = await page.locator('code', { hasText: '/sign-in#' }).textContent()
  // Bob opens it on his own device: Continue, then the Passkey offer, declined.
  const bob = await watch(await (await browser.newContext()).newPage())
  await bob.goto(link)
  await bob.getByText("You've been invited to Oiko as Bob").waitFor()
  await bob.getByRole('button', { name: 'Continue' }).click()
  await bob.getByRole('button', { name: 'Not now' }).click()
  await bob.getByRole('button', { name: 'Open Oiko' }).click()
  await bob.locator('nav a[href="#automations"]').waitFor() // a Member
  // Spent, it says so.
  await bob.goto(link)
  await bob.getByText('expired or was already used').waitFor()

  // A wall tablet asks to be a Kiosk; Alice approves it from the page its QR code opens.
  const wall = await watch(await (await browser.newContext()).newPage())
  await wall.goto(base)
  const request = wall.waitForResponse((r) => r.url().endsWith('/api/kiosk-pairing'))
  await wall.getByRole('button', { name: 'Use this screen as a Kiosk' }).click()
  await wall.getByRole('img', { name: 'QR code to pair this screen' }).waitFor()
  const { secret } = await (await request).json()
  await page.goto(`${base}/pair#${secret}`)
  await page.getByText('shared screen').waitFor()
  await page.getByLabel('Name').fill('Hall tablet')
  await page.getByRole('button', { name: 'Approve' }).click()
  await page.getByText('signs in as Hall tablet').waitFor()
  await wall.locator('nav').waitFor() // the dashboard, as the Kiosk, a Guest
  await page.goto(`${base}/#kiosks`)
  await page.getByRole('button', { name: 'Sign out' }).waitFor()
} finally {
  await stop()
}
if (problems.length > 0) {
  console.error(problems.join('\n'))
  process.exit(1)
}
console.log('no CSP violation')
