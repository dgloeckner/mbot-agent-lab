// A team's second PC: the band shows a short code, another browser enters it
// on /join and lands on the team page. A wrong team link gets a page, not JSON.

import { expect, test } from '@playwright/test'
import { join } from 'node:path'
import { Lab, SHOTS, type Team } from './lab'

let lab: Lab
let team: Team

test.beforeAll(async () => {
  lab = await Lab.start({ robots: 1 })
  team = await lab.team('Zweitrechner', ['mbot-1'])
})

test.afterAll(async () => {
  await lab?.stop()
})

test('a second PC joins with the code the band shows', async ({ page, browser }) => {
  await page.setViewportSize({ width: 1366, height: 768 })
  await page.goto(lab.page(team))
  await page.getByRole('button', { name: 'Weiteren Rechner dazuholen' }).click()
  const code = (await page.locator('.link-code').textContent())!.trim()
  expect(code).toMatch(/^[A-Z0-9]{6}$/)
  await page.screenshot({ path: join(SHOTS, 'join-code-popover.png'), clip: { x: 700, y: 0, width: 666, height: 420 } })
  await page.keyboard.press('Escape')
  await expect(page.locator('.link-popover')).toHaveCount(0)

  const other = await browser.newContext({ locale: 'de-DE' })
  const pc2 = await other.newPage()
  await pc2.goto(lab.base + '/join')
  await pc2.getByRole('textbox', { name: 'Team-Code' }).fill(code.toLowerCase())
  await pc2.getByRole('button', { name: 'Dazukommen' }).click()
  await expect(pc2).toHaveURL(lab.page(team))
  await expect(pc2.locator('.band h1')).toContainText('Zweitrechner')

  // A wrong code says so.
  await pc2.goto(lab.base + '/join')
  await pc2.getByRole('textbox', { name: 'Team-Code' }).fill('ZZZZZZ')
  await pc2.getByRole('button', { name: 'Dazukommen' }).click()
  await expect(pc2.locator('.form-error')).toContainText('abgelaufen')

  // A wrong team link: a page that explains it and lists the team this browser joined.
  const resp = await pc2.goto(lab.base + '/t/vertippt')
  expect(resp!.status()).toBe(404)
  await expect(pc2.getByRole('heading', { name: 'Diesen Team-Link gibt es nicht (mehr)' })).toBeVisible()
  await expect(pc2.getByRole('link', { name: 'Zur Anmeldung' })).toHaveAttribute('href', '/join')
  await expect(pc2.getByRole('link', { name: 'Zweitrechner' })).toBeVisible()
  await pc2.screenshot({ path: join(SHOTS, 'join-unknown-link.png') })
  await other.close()
})
