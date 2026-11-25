// The task option "Agent darf nur eigene Werkzeuge des Teams nutzen": the
// control room sets it on a draft, the team page names the rule.

import { expect, test } from '@playwright/test'
import { join } from 'node:path'
import { Lab, SHOTS } from './lab'

let lab: Lab

test.beforeAll(async () => {
  lab = await Lab.start({ robots: 1 })
})

test.afterAll(async () => {
  await lab?.stop()
})

test('the control room sets the tool set, the team page shows it', async ({ page }) => {
  await page.goto(lab.base + '/admin')
  await page.locator('input[type="password"]').fill(lab.adminPassword)
  await page.getByRole('button', { name: 'Anmelden' }).click()
  const draft = await lab.adminCall('POST', '/admin/api/tasks', { title: 'Challenge: Labyrinth', body: 'Kommt mit euren eigenen Werkzeugen zum blauen Feld.' })
  await page.locator('.task-item', { hasText: 'Challenge: Labyrinth' }).locator('.task-open').click()
  const box = page.getByLabel('Agent darf nur eigene Werkzeuge des Teams nutzen')
  await box.check()
  await page.locator('.task-dialog').screenshot({ path: join(SHOTS, 'toolset-task-editor.png') })
  await page.getByRole('button', { name: 'Speichern' }).click()
  await expect.poll(async () => (await lab.adminCall('GET', `/admin/api/tasks/${draft.id}`)).agent_tools).toBe('team_tools')

  await lab.adminCall('POST', `/admin/api/tasks/${draft.id}/publish`)
  const team = await lab.team('Herausforderer', ['mbot-1'], 'Vorbereitung')
  await lab.adminCall('POST', `/admin/api/tasks/${draft.id}/assign`, { teams: [team.id] })
  await lab.teamCall(team, 'POST', `/tasks/${draft.id}/accept`, { prompt: 'empty' })

  await page.goto(lab.page(team))
  await page.getByRole('tab', { name: 'Aufgabe' }).click()
  await expect(page.locator('.task-panel .task-tool-rule')).toContainText('nur eure eigenen Werkzeuge')
  await page.getByRole('tab', { name: 'Werkzeuge' }).click()
  await expect(page.locator('.tools-panel .task-tool-rule')).toContainText('read_sensors')
  await expect(page.locator('.tool', { hasText: 'drive_until' }).locator('.tag-missing')).toBeVisible()
  await expect(page.locator('.tool', { hasText: 'read_sensors' }).locator('.tag-missing')).toHaveCount(0)
  await page.locator('.side-pane').screenshot({ path: join(SHOTS, 'toolset-team-page.png') })
})
