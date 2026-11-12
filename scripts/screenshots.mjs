// README screenshots and a browser smoke test, against a demo instance (DEMO_DATA=true: three places
// around Sarajevo, eight months of observing log, a made-up cloud forecast):
//
//   DATA_DIR=/tmp/pskydemo DEMO_DATA=true ADMIN_EMAIL=demo@example.com ADMIN_PASSWORD=demodemo1 ADMIN_NAME=Amar PORT=18309 TZ=Europe/Sarajevo ./pocket-sky
//   CDP=http://127.0.0.1:9336 BASE=http://127.0.0.1:18309 node scripts/screenshots.mjs   # any Chrome with remote debugging
//   SHOTS=0 … node scripts/screenshots.mjs                                                # checks only
// playwright-core comes from web/node_modules, or set PW_FROM to a package.json next to an install of it.
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'

const require = createRequire(process.env.PW_FROM || new URL('../web/package.json', import.meta.url))
let pw
try { pw = require('playwright') } catch { pw = require('playwright-core') }
const { chromium } = pw
const base = process.env.BASE || 'http://127.0.0.1:18309'
const shots = process.env.SHOTS !== '0'
const dir = new URL('../docs/screenshots/', import.meta.url).pathname
const browser = process.env.CDP ? await chromium.connectOverCDP(process.env.CDP) : await chromium.launch()
const errors = []

async function session({ phone = false, dark = false } = {}) {
  const ctx = await browser.newContext({
    viewport: phone ? { width: 390, height: 844 } : { width: 1440, height: 900 },
    deviceScaleFactor: phone ? 3 : 2, isMobile: phone, hasTouch: phone, colorScheme: dark ? 'dark' : 'light', timezoneId: 'Europe/Sarajevo', locale: 'en-GB',
    userAgent: phone ? 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1' : undefined,
  })
  const page = await ctx.newPage()
  page.on('pageerror', e => errors.push(`pageerror: ${e}`))
  page.on('console', m => { if (m.type() === 'error' && !/Failed to load resource/.test(m.text())) errors.push(m.text()) })
  await page.goto(base + '/login')
  const res = await page.evaluate(() => fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: 'demo@example.com', password: 'demodemo1' }) }).then(r => r.status))
  assert.equal(res, 200, 'demo sign-in')
  return { ctx, page }
}
async function go(page, path, wait = 1400, frozen = true) {
  await page.goto(base + path, { waitUntil: 'networkidle' })
  // headless Chrome freezes CSS fade-ins: show everything at once
  if (frozen) await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' })
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(wait)
}
async function shot(page, name) { if (shots) await page.screenshot({ path: dir + name + '.png' }) }
/** "Point at the sky" without a phone: compass readings sent by hand. */
async function point(page, alpha, beta, gamma = 0) {
  await page.evaluate(([a, b, g]) => {
    for (let i = 0; i < 40; i++) window.dispatchEvent(new DeviceOrientationEvent('deviceorientationabsolute', { alpha: a, beta: b, gamma: g, absolute: true }))
  }, [alpha, beta, gamma])
  await page.waitForTimeout(500)
}

// ---- desktop ----
{
  const { ctx, page } = await session()
  await go(page, '/', 2000)
  const html = await page.content()
  assert.ok(html.includes('Through the night') && html.includes('The coming nights') && html.includes('Planets') && html.includes('Best sights tonight'), 'tonight shows the night, the coming nights, planets and sights')
  await shot(page, 'desktop-tonight')
  await go(page, '/map?object=dso:M31', 1800)
  assert.ok(await page.locator('.sky-sheet').isVisible(), 'the map opens an object')
  await shot(page, 'desktop-map')
  await go(page, '/events', 1800)
  assert.ok((await page.content()).includes('meteor shower'), 'events list a meteor shower')
  await shot(page, 'desktop-events')
  await go(page, '/log')
  assert.ok((await page.locator('.log-row').count()) > 5, 'the log has nights')
  await shot(page, 'desktop-log')
  await go(page, '/lists')
  await page.click('text=Messier list')
  await page.waitForTimeout(600)
  await shot(page, 'desktop-lists')
  await go(page, '/settings/map')
  await shot(page, 'desktop-settings-map')
  await ctx.close()
}

// ---- phone ----
{
  const { ctx, page } = await session({ phone: true })
  await go(page, '/', 2000)
  await shot(page, 'phone-tonight')
  await go(page, '/map', 1500)
  await shot(page, 'phone-map')
  await page.getByRole('button', { name: 'Point at the sky' }).click()
  await page.waitForTimeout(800)
  // there is no camera in a headless browser: the stars on their own
  const cam = page.locator('button:has-text("Camera on")')
  if (await cam.count()) await cam.click()
  await point(page, 200, 125)
  const where = await page.locator('.sky-aim-dir').textContent()
  assert.ok(/35° up/.test(where || ''), `pointing reads the tilt: ${where}`)
  await shot(page, 'phone-point')
  await ctx.close()
}
{
  const { ctx, page } = await session({ phone: true, dark: true })
  await go(page, '/', 2000)
  await shot(page, 'phone-tonight-dark')
  await ctx.close()
}

await browser.close()
if (errors.length) { console.log(errors.join('\n')); process.exit(1) }
console.log('screenshots ok')
