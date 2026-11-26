// The protocol with two robots and lab events: team-tool steps name their
// robot, lab events show on every tab, a scrolled-up log says when new lines
// arrive, Stopp always answers, and a read answer stays read after a reload.

import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { Lab, ROOT, SHOTS, type Team } from './lab'

let lab: Lab
let team: Team

test.beforeAll(async () => {
  lab = await Lab.start({ robots: 2 })
  team = await lab.team('Doppelt', ['mbot-1', 'mbot-2'])
})

test.afterAll(async () => {
  await lab?.stop()
})

test('steps of a team tool name their robot when the team has two', async ({ page }) => {
  test.setTimeout(90_000)
  const ex = JSON.parse(readFileSync(join(ROOT, 'docs', 'examples', 'programs', 'quadrat.json'), 'utf8'))
  const tool = await lab.teamCall(team, 'POST', '/programs', { name: 'quadrat', description: 'Fährt ein Quadrat.', blockly: ex.blockly })
  await page.goto(lab.page(team))
  await expect(page.locator('.robot')).toHaveCount(2)
  await new Promise((r) => setTimeout(r, 2500)) // the robots learn their owner from the next heartbeat
  const robots = (await lab.teamCall(team, 'GET', '/api/session')).robots.map((r: any) => r.alias)
  await lab.teamCall(team, 'POST', `/programs/${tool.id}/run`, { robot: robots[1], step: false })
  const steps = page.locator('.log-line[data-kind="tool"]')
  await expect(steps.first()).toBeVisible({ timeout: 20_000 })
  await expect.poll(async () => (await steps.allTextContents()).length, { timeout: 30_000 }).toBeGreaterThanOrEqual(8)
  for (const t of await steps.allTextContents()) expect(t).toContain(`robot: ${robots[1]}`)
  await page.locator('.side-pane').screenshot({ path: join(SHOTS, 'protocol-robot-names.png') })
})

test('lab events on every tab, new entries below, Stopp answers, read stays read', async ({ page }) => {
  test.setTimeout(90_000)
  await page.setViewportSize({ width: 1366, height: 768 }) // the steps of the test before fill the log
  await page.goto(lab.page(team))
  await page.getByRole('tab', { name: 'Werkzeuge' }).click()

  // The lab lead assigns a task: a toast on the "Werkzeuge" tab.
  const t = await lab.adminCall('POST', '/admin/api/tasks', { title: 'Zweite Aufgabe', body: 'Noch eine.' })
  await lab.adminCall('POST', `/admin/api/tasks/${t.id}/publish`)
  await lab.adminCall('POST', `/admin/api/tasks/${t.id}/assign`, { teams: [team.id] })
  await expect(page.locator('.toast')).toContainText('Ihr habt eine neue Aufgabe bekommen: „Zweite Aufgabe“')

  // Stopp outside a run answers visibly.
  await page.locator('.band-actions').getByRole('button', { name: 'Stopp' }).click()
  await expect(page.locator('.toast')).toContainText('Alle Roboter angehalten.')

  // Scrolled up in the log, new lines get a button.
  await page.getByRole('tab', { name: 'Protokoll' }).click()
  const log = page.locator('.log')
  await expect.poll(async () => log.evaluate((el) => el.scrollHeight > el.clientHeight + 100)).toBe(true)
  await log.evaluate((el) => el.scrollTo({ top: 0 }))
  await log.dispatchEvent('scroll')
  const robots = (await lab.teamCall(team, 'GET', '/api/session')).robots.map((r: any) => r.alias)
  await lab.teamCall(team, 'POST', '/tools/read_sensors', { robot: robots[0] })
  const below = page.locator('.new-below')
  await expect(below).toBeVisible()
  await below.click()
  await expect(below).toHaveCount(0)

  // A conversation answer, read on its tab, is not new again after a reload.
  await lab.teamCall(team, 'POST', '/message', { text: 'Was kannst du?' })
  const badge = page.getByRole('tab', { name: /Gespräch/ }).locator('.tab-badge')
  await expect(badge).toBeVisible({ timeout: 20_000 })
  await page.getByRole('tab', { name: /Gespräch/ }).click()
  await expect(badge).toHaveCount(0)
  await page.getByRole('tab', { name: 'Protokoll' }).click()
  await page.reload()
  await expect(page.locator('.band')).toBeVisible()
  await page.waitForTimeout(1500) // the stream replays the conversation
  expect(await page.locator('.toast').count()).toBe(0) // replayed lab events do not toast again
  await expect(page.getByRole('tab', { name: /Gespräch/ }).locator('.tab-badge')).toHaveCount(0)
})
