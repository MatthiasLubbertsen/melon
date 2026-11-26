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

    // Homework on a lesson that drops out: written with the editor's buttons,
    // shown on the lesson, and listed in the homework menu to move along.
    for (const d of await page.locator('nav [data-day]').all()) {
      await d.click()
      if (await page.locator('[data-status="cancelled"]:visible').count()) break
    }
    await page.locator('[data-status="cancelled"]:visible').first().click()
    const drawer = page.locator('[data-slot="drawer-popup"]')
    await drawer.waitFor()
    await drawer.getByRole('button', { name: 'Huiswerk toevoegen' }).click()
    const box = drawer.getByRole('textbox', { name: 'Huiswerk toevoegen' })
    await box.fill('Maak opgaven 12 t/m 18\nLeer de woordjes van hoofdstuk 4')
    await box.press('ControlOrMeta+a')
    await drawer.getByRole('button', { name: 'Takenlijst' }).click()
    await box.press('ControlOrMeta+End')
    await box.press('Enter')
    await box.press('Enter')
    await box.pressSequentially('Inleveren via ')
    await drawer.getByRole('button', { name: 'Link' }).click()
    await box.pressSequentially('de ELO')
    await box.press('End')
    await box.press('ArrowLeft')
    await box.pressSequentially('elo.example.nl')
    await shot(`23-homework-editor-${theme}`)
    await drawer.locator('section[aria-labelledby="homework-title"]').getByRole('button', { name: 'Opslaan' }).click()
    await drawer.locator('li[data-task] [role="checkbox"]').first().click()
    await drawer.locator('li[data-task="done"]').waitFor()
    await shot(`24-homework-lesson-${theme}`)
    await page.keyboard.press('Escape')
    await drawer.waitFor({ state: 'detached' })
    await page.getByRole('button', { name: 'Huiswerk', exact: true }).click()
    await page.getByRole('heading', { name: 'Zonder les' }).waitFor()
    await shot(`25-homework-menu-${theme}`)
    await page.getByRole('button', { name: 'Naar volgende les' }).click()
    await page.getByRole('heading', { name: 'Komt nog' }).waitFor()
    await page.getByRole('heading', { name: 'Zonder les' }).waitFor({ state: 'detached' })
    await shot(`26-homework-moved-${theme}`)
    await page.locator('[data-slot="drawer-popup"]').locator('li button').first().click()
    await page.locator('[data-slot="drawer-popup"]').waitFor({ state: 'detached' })
    await page.getByText('Huiswerk', { exact: true }).locator('visible=true').first().waitFor()
    await shot(`27-homework-on-lesson-${theme}`)

    // A week plan link for the subject, shown on each of its lessons that week.
    await page.locator('[data-status]:visible', { hasText: 'Huiswerk' }).first().click()
    await drawer.waitFor()
    await drawer.getByRole('button', { name: 'Weekplanning toevoegen' }).click()
    await drawer.getByRole('textbox', { name: /Weekplanning/ }).fill('https://jfc.itslearning.com/plans/courses/32813/plan/413396')
    await drawer.locator('form').filter({ has: page.locator('#week-plan') }).getByRole('button', { name: 'Opslaan' }).click()
    await drawer.getByRole('link', { name: /Weekplanning week/ }).waitFor()
    await shot(`28-week-plan-${theme}`)
    const planned = (await drawer.getByRole('heading').first().innerText()).toLowerCase()
    await page.keyboard.press('Escape')
    await drawer.waitFor({ state: 'detached' })

    // Search: the button grows into a bar that reads plain questions, typos and all.
    await page.getByRole('button', { name: 'Zoeken', exact: true }).click()
    const search = page.getByRole('combobox', { name: 'Zoeken' })
    await search.fill(`volgnde les ${planned}`)
    await page.locator('#schedule-search-results [role="option"]').first().waitFor()
    await shot(`30-search-next-${theme}`)
    await search.press('Enter')
    await search.waitFor({ state: 'detached' })
    await page.getByRole('button', { name: 'Zoeken', exact: true }).click()
    await search.fill(`weekplanning ${planned}`)
    await page.locator('#schedule-search-results [role="option"]', { hasText: 'Weekplanning' }).first().waitFor()
    await shot(`31-search-week-plan-${theme}`)
    await search.press('Escape')
    await search.waitFor({ state: 'detached' })

    // The calendar feed links every lesson to Melon: that link opens it, even in another week.
    const target = await page.evaluate(async () => {
      const session = (await (await fetch('/api/app/session')).json()).data
      const account = session.accounts.find((a) => a.role === 'student').id
      const next = new Date(Date.now() + 7 * 86400000).toISOString().slice(0, 10)
      const sched = (await (await fetch(`/api/v1/accounts/${account}/schedule?from=${next}&to=${next}`)).json()).data
      const l = sched.lessons.find((x) => x.status !== 'cancelled') ?? sched.lessons[0]
      return l && { account, day: l.date, lesson: l.instance, title: l.title }
    })
    if (target) {
      await page.goto(`${BASE}/?account=${target.account}&day=${target.day}&lesson=${target.lesson}`)
      await drawer.waitFor()
      await drawer.getByRole('heading', { name: target.title }).waitFor()
      await page.waitForURL(`${BASE}/`)
      await shot(`29-lesson-link-${theme}`)
      await page.keyboard.press('Escape')
      await drawer.waitFor({ state: 'detached' })
    }
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

    // An account: set a PIN, sign out, and sign back in with number + PIN.
    // This also proves the whole round trip works in a real browser.
    await page.goto(`${BASE}/settings`)
    await page.getByRole('button', { name: 'Pincode instellen' }).click()
    await page.locator('#new-pin').fill('4821')
    await shot(`13-set-pin-${theme}`)
    await page.getByRole('button', { name: 'Opslaan' }).click()
    await page.getByRole('button', { name: 'Uitloggen' }).waitFor()
    await page.locator('[data-sonner-toast]').waitFor({ state: 'detached' })
    await shot(`14-settings-signed-in-${theme}`)
    await page.getByRole('button', { name: 'Uitloggen' }).click()
    await page.getByRole('alertdialog').or(page.getByRole('dialog')).getByRole('button', { name: 'Uitloggen' }).click()
    await page.getByRole('button', { name: /Al een Melon-account/ }).click()
    await page.getByLabel('Gebruikersnummer').fill('100001')
    await page.getByLabel('Pincode').fill('4821')
    await shot(`15-sign-in-${theme}`)
    await page.getByRole('button', { name: 'Inloggen', exact: true }).click()
    await page.locator('[data-status]:visible').first().waitFor()
    await context.close()
  }

  // A teacher whose Zermelo token cannot list people gets a code field instead
  // of name search. The demo teacher can list people, so the session answer is
  // rewritten in the browser to show that variant.
  for (const theme of ['light', 'dark']) {
    console.log(`teacher without name search, ${theme}`)
    const { context, page, shot } = await session({ viewport: phone, theme })
    await page.route('**/api/app/session', async (route) => {
      const res = await route.fetch()
      const body = await res.json()
      for (const a of body.data?.accounts ?? []) if (a.role === 'employee') a.canViewOthers = false
      await route.fulfill({ response: res, json: body })
    })
    await page.goto(BASE)
    await page.getByRole('button', { name: 'Bekijk de demo' }).click()
    await page.locator('[data-status]:visible').first().waitFor()
    await page.getByRole('button', { name: 'Wissel van account' }).click()
    await page.getByRole('menuitem', { name: /Noor Bakker/ }).click()
    const field = page.getByLabel('Rooster opzoeken met een code (16528, dh)')
    await field.waitFor()
    await field.fill('16528')
    await shot(`16-teacher-code-lookup-${theme}`)
    await context.close()
  }

  // A parent: linking gives one account per child, which can each get a nickname.
  for (const theme of ['light', 'dark']) {
    console.log(`parent, ${theme}`)
    const { context, page, shot } = await session({ viewport: phone, theme })
    await page.goto(BASE)
    await page.locator('#school').fill('demo')
    await page.locator('#secret').fill('ouder')
    await page.getByRole('button', { name: 'Koppelen' }).click()
    await page.locator('[data-status]:visible').first().waitFor()
    // Look a week ahead, then switch child: the day stays where it was.
    await page.getByRole('button', { name: 'Volgende week' }).click()
    const selectedDay = () => page.locator('[aria-current="date"]:visible').first().getAttribute('data-day')
    const picked = await selectedDay()
    await page.getByRole('button', { name: 'Wissel van account' }).click()
    await page.getByRole('menuitem', { name: /Finn Visser/ }).waitFor()
    await shot(`17-parent-children-${theme}`)
    await page.getByRole('menuitem', { name: /Finn Visser/ }).click()
    await page.locator('[data-status]:visible').first().waitFor()
    const after = await selectedDay()
    if (after !== picked) throw new Error(`day changed on switching child: ${picked} -> ${after}`)
    await shot(`21-parent-same-day-${theme}`)

    // Feedback from a parent is from the parent, about the child that was
    // open. CI makes the demo parent an admin (MELON_ADMINS), so it can look.
    await page.getByRole('button', { name: 'Ideeën en bugs' }).click()
    await page.getByRole('textbox', { name: 'Nieuw bericht' }).fill('Als ik van kind wissel zou de dag gelijk moeten blijven.')
    await page.getByRole('button', { name: 'Versturen' }).click()
    await page.getByText('Als ik van kind wissel').first().waitFor()
    await page.keyboard.press('Escape')
    // The "sent" toast covers the header on a phone; let it fade first.
    await page.mouse.move(phone.width / 2, phone.height - 10)
    await page.locator('[data-sonner-toast]').waitFor({ state: 'detached' })
    await page.getByRole('link', { name: 'Instellingen' }).click()
    await page.getByRole('link', { name: 'Beheer' }).click()
    await page.getByRole('tab', { name: /Feedback/ }).click()
    await page.getByRole('button', { name: /Als ik van kind wissel/ }).first().click()
    await page.getByText('Van Eva de Vries', { exact: true }).waitFor()
    await shot(`22-parent-feedback-admin-${theme}`)
    await page.keyboard.press('Escape')

    // And back from the admin dashboard, the schedule is still on that day.
    await page.getByRole('link', { name: 'Terug' }).click()
    await page.getByRole('heading', { name: 'Instellingen' }).waitFor()
    await page.getByRole('link', { name: 'Terug' }).click()
    await page.locator('[data-status]:visible').first().waitFor()
    if ((await selectedDay()) !== picked) throw new Error(`day changed after visiting admin: ${picked} -> ${await selectedDay()}`)

    // The "linked" toast sits on top of the header on a phone until it fades.
    // Sonner pauses a toast while the mouse is over it, and the mouse is still
    // up in the header from the account switcher, so move it out of the way.
    await page.mouse.move(phone.width / 2, phone.height - 10)
    await page.locator('[data-sonner-toast]').waitFor({ state: 'detached' })
    await page.getByRole('link', { name: 'Instellingen' }).click()
    await page.getByRole('heading', { name: 'Instellingen' }).waitFor()
    await page.getByRole('button', { name: 'Naam geven' }).nth(1).click()
    await page.locator('#nickname').fill('Jongste')
    await shot(`18-parent-nickname-${theme}`)
    await page.getByRole('button', { name: 'Opslaan' }).click()
    await page.getByText('Jongste').first().waitFor()
    await page.locator('[data-sonner-toast]').waitFor({ state: 'detached' })
    await shot(`19-parent-settings-${theme}`)
    await context.close()
  }

  // Release notes: a profile that last saw an older note gets everything
  // since, once. A new profile (like the demo) starts caught up, so the
  // screenshots rewind it first. Light shows a long catch-up, dark a single note.
  for (const [theme, seen] of [['light', 'overlap-cards'], ['dark', 'parents']]) {
    console.log(`release notes, ${theme}`)
    const { context, page, shot } = await session({ viewport: phone, theme })
    await page.goto(BASE)
    await page.getByRole('button', { name: 'Bekijk de demo' }).click()
    await page.locator('[data-status]:visible').first().waitFor()
    const status = await page.evaluate((id) => fetch('/api/app/releases/seen', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ id }) }).then((r) => r.status), seen)
    if (status !== 204) throw new Error(`rewinding release notes: ${status}`)
    await page.reload()
    const dialog = page.getByRole('dialog', { name: 'Nieuw in Melon' })
    await dialog.waitFor()
    await shot(`20-release-notes-${theme}`)
    await dialog.getByRole('button', { name: 'Top, verder' }).click()
    await dialog.waitFor({ state: 'detached' })
    // Once seen, it stays away after a reload.
    await page.reload()
    await page.locator('[data-status]:visible').first().waitFor()
    await page.waitForTimeout(500)
    if (await dialog.isVisible()) problems.push('release notes showed again after being dismissed')
    await context.close()
  }

  // Detention: drag across periods on the schedule from the add dialog, then see it share the
  // card of the lesson in that period (or take a free period whole).
  for (const theme of ['light', 'dark']) {
    console.log(`detention, ${theme}`)
    const { context, page, shot } = await session({ viewport: phone, theme })
    await page.goto(BASE)
    await page.getByRole('button', { name: 'Bekijk de demo' }).click()
    await page.locator('[data-status]:visible').first().waitFor()
    await page.getByRole('button', { name: 'Nakomen toevoegen' }).click()
    const dialog = page.getByRole('dialog', { name: 'Nakomen toevoegen' })
    await dialog.waitFor()
    // The dialog steps aside; the periods are dragged over on the schedule itself.
    await dialog.getByRole('button', { name: 'Kies de uren op je rooster' }).click()
    await dialog.waitFor({ state: 'detached' })
    const bar = page.getByRole('region', { name: 'Nakomen toevoegen' })
    await bar.waitFor()
    const cells = page.locator('[data-pick-day]:visible')
    if ((await cells.count()) < 3) throw new Error('no periods to pick a detention from')
    const a = await cells.nth(1).boundingBox()
    const b = await cells.nth(2).boundingBox()
    await page.mouse.move(a.x + a.width / 2, a.y + a.height / 2)
    await page.mouse.down()
    await page.mouse.move(b.x + b.width / 2, b.y + b.height / 2, { steps: 4 })
    await page.mouse.up()
    await shot(`22-detention-pick-${theme}`)
    await bar.getByRole('button', { name: 'Klaar' }).click()
    await dialog.waitFor()
    if (!/t\/m/.test(await dialog.getByRole('button', { name: /uur, kies opnieuw/ }).innerText())) problems.push('dragging across two periods on the schedule did not pick both')
    await dialog.getByRole('radio').nth(1).click()
    await dialog.locator('#detention-location').fill('B014')
    await dialog.locator('#detention-date').focus()
    await shot(`23-detention-add-${theme}`)
    await dialog.getByRole('button', { name: 'Toevoegen' }).click()
    await dialog.waitFor({ state: 'detached' })
    await page.locator('[data-status="detention"]:visible').first().waitFor()
    await page.mouse.move(phone.width / 2, phone.height - 10)
    await page.waitForFunction(() => !document.querySelector('[data-sonner-toast]'))
    await page.locator('[data-status="detention"]:visible').first().scrollIntoViewIfNeeded()
    await shot(`24-detention-schedule-${theme}`)
    // Change it again from the lesson sheet: the form comes back filled in.
    await page.locator('[data-status="detention"]:visible').first().click()
    await page.getByRole('button', { name: 'Aanpassen' }).click()
    const edit = page.getByRole('dialog', { name: 'Nakomen aanpassen' })
    await edit.waitFor()
    if ((await edit.locator('#detention-location').inputValue()) !== 'B014') problems.push('editing a detention did not fill in its location')
    await edit.locator('#detention-location').fill('B015')
    await edit.locator('#detention-date').focus()
    await shot(`25-detention-edit-${theme}`)
    await edit.getByRole('button', { name: 'Opslaan' }).click()
    await edit.waitFor({ state: 'detached' })
    await page.getByText('B015').first().waitFor()
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

  // The admin dashboard's homework and detention numbers. CI makes the demo parent an admin.
  {
    const { context, page, shot } = await session({ viewport: desktop, theme: 'light' })
    await page.goto(BASE)
    await page.locator('#school').fill('demo')
    await page.locator('#secret').fill('ouder')
    await page.getByRole('button', { name: 'Koppelen' }).click()
    await page.locator('[data-status]:visible').first().waitFor()
    await page.goto(`${BASE}/admin`)
    const planner = page.getByText('Huiswerk en nakomen', { exact: true })
    await planner.waitFor()
    await planner.evaluate((el) => el.closest('[data-slot="card"]')?.scrollIntoView({ block: 'start' }) ?? el.scrollIntoView({ block: 'start' }))
    await shot('13-admin-planner-light')
    await context.close()
  }

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
