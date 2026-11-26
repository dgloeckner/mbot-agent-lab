// The team band on a lab laptop (1366 × 768): one height whatever the agent
// does, buttons that keep their place, and room for three robots in one row.

import { expect, test, type Page } from '@playwright/test'
import { join } from 'node:path'
import { Lab, SHOTS, type Team } from './lab'

let lab: Lab
let team: Team

test.beforeAll(async () => {
  lab = await Lab.start({ robots: 3 })
  team = await lab.team('Blitzschnell', ['mbot-1'])
})

test.afterAll(async () => {
  await lab?.stop()
})

const bandHeight = async (page: Page) => Math.round((await page.locator('.band').boundingBox())!.height)
const left = async (page: Page, sel: string) => Math.round((await page.locator(sel).first().boundingBox())!.x)

test('the band keeps its height through a run and a conversation, and fits three robots', async ({ page }) => {
  test.setTimeout(120_000)
  await page.setViewportSize({ width: 1366, height: 768 })
  await page.goto(lab.page(team))
  await expect(page.locator('.robot')).toHaveCount(1)
  const start = page.locator('.band-actions').getByRole('button', { name: 'Lauf starten' })
  const before = { band: await bandHeight(page), card: await left(page, '.robot'), start: await left(page, '.band-actions .btn-on-band') }
  expect(before.band).toBeLessThanOrEqual(165)

  await lab.teamCall(team, 'POST', '/prompt', { text: 'Fahre mit `drive` 20 cm und dann `finish`.' })
  await new Promise((r) => setTimeout(r, 2500)) // the robot learns its owner from the next heartbeat
  await lab.teamCall(team, 'POST', '/start')
  await expect(page.locator('.agent-label')).toHaveText(/^Agent /)
  await expect(page.locator('.run-usage')).toContainText('dieser Lauf')
  // During a run "Lauf starten" stays where it was, disabled.
  await expect(start).toBeDisabled()
  expect(await bandHeight(page)).toBe(before.band)
  expect(await left(page, '.robot')).toBe(before.card)
  expect(await left(page, '.band-actions .btn-on-band')).toBe(before.start)

  await expect(page.locator('.agent-label')).toHaveText('Kein Lauf', { timeout: 40_000 })
  await lab.teamCall(team, 'POST', '/message', { text: 'Was kannst du?' })
  await expect(page.locator('.run-usage')).toContainText('gesamt', { timeout: 20_000 })
  await expect(page.locator('.run-usage')).toHaveAttribute('title', /davon Gespräche/, { timeout: 20_000 })
  expect(await bandHeight(page)).toBe(before.band)
  expect(await left(page, '.robot')).toBe(before.card)

  await lab.adminCall('POST', '/admin/api/assign', { robot: 'mbot-2', team: team.id })
  await lab.adminCall('POST', '/admin/api/assign', { robot: 'mbot-3', team: team.id })
  await expect(page.locator('.robot')).toHaveCount(3)
  const tops = await page.locator('.robot').evaluateAll((els) => els.map((e) => Math.round(e.getBoundingClientRect().top)))
  expect(new Set(tops).size).toBe(1) // one row
  expect(await bandHeight(page)).toBeLessThanOrEqual(200)
  await page.locator('.band').screenshot({ path: join(SHOTS, 'band-three-robots.png') })
})
