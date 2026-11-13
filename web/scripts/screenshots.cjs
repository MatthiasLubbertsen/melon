// Takes screenshots of Melon's main screens against a server running in demo
// mode (MELON_DEMO=1). Only generated demo data is used, never a real account.
//
//   BASE_URL=http://127.0.0.1:8080 OUT=screenshots node web/scripts/screenshots.cjs
const { chromium } = require('playwright')
const fs = require('node:fs')
const path = require('node:path')

const BASE = process.env.BASE_URL || 'http://127.0.0.1:8080'
const OUT = path.resolve(process.env.OUT || 'screenshots')
const SCALE = Number(process.env.SCALE || 2)
const phone = { width: 390, height: 844 }
const desktop = { width: 1280, height: 800 }

async function main() {
  fs.mkdirSync(OUT, { recursive: true })
  const browser = await chromium.launch()
  const problems = []

  async function session({ viewport, theme, lang = 'nl' }) {
    const context = await browser.newContext({ viewport, deviceScaleFactor: SCALE, colorScheme: theme, locale: 'nl-NL', timezoneId: 'Europe/Amsterdam', hasTouch: viewport.width < 600 })
    await context.addInitScript(([l]) => localStorage.setItem('melon.lang', l), [lang])
    const page = await context.newPage()
    page.on('pageerror', (e) => problems.push(`pageerror: ${e.message}`))
    page.on('console', (m) => {
      // The API reference probes for eval(); Melon's CSP rightly refuses it and the page works without.
      if (m.type() === 'error' && !/script-src|Content Security Policy/.test(m.text())) problems.push(`console: ${m.text()}`)
    })
    const shot = async (name) => {
      await page.waitForLoadState('networkidle')
      await page.waitForTimeout(350) // let transitions settle
      await page.screenshot({ path: path.join(OUT, `${name}.png`) })
      console.log(`  ${name}.png`)
    }
    return { context, page, shot }
  }

  // Refuse to run against anything but a demo server.
  const info = await (await fetch(`${BASE}/api/app/session`)).json()
  if (!info.data.demo) throw new Error('The server is not in demo mode; refusing to take screenshots.')

  for (const theme of ['light', 'dark']) {
    console.log(`phone, ${theme}`)
    const { context, page, shot } = await session({ viewport: phone, theme })
    await page.goto(BASE)
    await page.getByRole('button', { name: 'Koppelen' }).waitFor()
    await shot(`01-welcome-${theme}`)

    await page.getByRole('button', { name: 'Bekijk de demo' }).click()
    await page.locator('[data-status]:visible').first().waitFor()
    await shot(`02-schedule-${theme}`)

    await page.locator('[data-status="changed"]:visible, [data-status="cancelled"]:visible').first().click()
    await page.locator('[data-slot="drawer-popup"]').waitFor()
    await shot(`03-lesson-${theme}`)
    await page.keyboard.press('Escape')
    await page.locator('[data-slot="drawer-popup"]').waitFor({ state: 'detached' })

    await page.getByRole('button', { name: 'Wissel van account' }).click()
    await page.getByRole('menuitem', { name: /Noor Bakker/ }).waitFor()
    await shot(`04-accounts-${theme}`)
    await page.getByRole('menuitem', { name: /Noor Bakker/ }).click()

    await page.getByRole('searchbox').fill('van')
    await page.getByRole('option').first().waitFor()
    await shot(`05-search-${theme}`)
    await page.getByRole('option', { name: /Daan van den Berg/ }).click()
    await page.getByText('Rooster van Daan van den Berg').waitFor()
    await shot(`06-student-view-${theme}`)

    await page.getByRole('link', { name: 'Instellingen' }).click()
    await page.getByRole('heading', { name: 'Instellingen' }).waitFor()
    await shot(`07-settings-accounts-${theme}`)
    await page.getByRole('tab', { name: 'Namen' }).click()
    await shot(`08-settings-names-${theme}`)
    await page.getByRole('tab', { name: 'Koppelingen' }).click()
    await page.getByRole('switch').click()
    await page.getByRole('link', { name: 'Toevoegen aan agenda' }).waitFor()
    await shot(`09-settings-connections-${theme}`)

    // OAuth consent, with a client registered just for this screenshot.
    const client = await (await fetch(`${BASE}/oauth/register`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ client_name: 'Claude', redirect_uris: ['https://claude.ai/api/mcp/auth_callback'] }) })).json()
    await page.goto(`${BASE}/oauth/authorize?response_type=code&client_id=${client.client_id}&redirect_uri=${encodeURIComponent('https://claude.ai/api/mcp/auth_callback')}&state=demo&code_challenge=E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM&code_challenge_method=S256`)
    await page.getByRole('button', { name: 'Toestaan' }).waitFor()
    await page.getByRole('checkbox').first().click()
    await shot(`10-consent-${theme}`)
    await context.close()
  }

  console.log('desktop')
  const { context, page, shot } = await session({ viewport: desktop, theme: 'light' })
  await page.goto(BASE)
  await page.getByRole('button', { name: 'Bekijk de demo' }).click()
  await page.locator('[data-status]:visible').first().waitFor()
  await shot('11-week-desktop-light')
  await page.goto(`${BASE}/docs`)
  await page.waitForLoadState('networkidle')
  await shot('12-api-docs-light')
  await context.close()

  await browser.close()
  if (problems.length) {
    console.error(`\nBrowser reported problems:\n${[...new Set(problems)].join('\n')}`)
    process.exit(1)
  }
}

main().catch((e) => {
  console.error(e)
  process.exit(1)
})
