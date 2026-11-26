// The accept dialog on a lab laptop: every prompt choice is visible when it
// opens, the chosen one is previewed, and the hint stands once.

import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { Lab, ROOT, SHOTS, type Team } from './lab'

let lab: Lab
let team: Team

test.beforeAll(async () => {
  lab = await Lab.start({ robots: 1 })
  team = await lab.team('Annehmer', ['mbot-1'])
})

test.afterAll(async () => {
  await lab?.stop()
})

/** Body and starter prompt of a German task file. */
function taskFile(name: string) {
  const src = readFileSync(join(ROOT, 'docs', 'lab', 'aufgaben', 'de', name), 'utf8')
  const [, body, starter] = /## Aufgabe\n([\s\S]*?)## Startprompt\n([\s\S]*)$/.exec(src)!
  return { body: body.trim(), starter: starter.trim() }
}

test('all prompt choices are in view at 1366 × 768, the hint stands once', async ({ page }) => {
  await page.setViewportSize({ width: 1366, height: 768 })
  const f = taskFile('erkunden.md')
  const t = await lab.adminCall('POST', '/admin/api/tasks', { title: 'Finde das blaue Feld', body: f.body, starter: f.starter })
  await lab.adminCall('POST', `/admin/api/tasks/${t.id}/publish`)
  await lab.adminCall('POST', `/admin/api/tasks/${t.id}/assign`, { teams: [team.id] })

  await page.goto(lab.page(team))
  await page.locator('.task-current').click()
  await page.getByRole('menuitem', { name: /Finde das blaue Feld/ }).click()
  const dlg = page.getByRole('dialog', { name: 'Finde das blaue Feld' })
  const radios = dlg.getByRole('radio')
  await expect(radios).toHaveCount(3)
  for (const r of await radios.all()) await expect(r).toBeInViewport()
  await expect(dlg.getByRole('radio', { name: 'Startprompt der Aufgabe' })).toBeChecked()
  await expect(dlg.locator('.accept-starter')).toContainText('Du steuerst einen kleinen Roboter')
  await expect(dlg.getByText(/sieht diesen Text nicht|kennt diesen Text nicht/)).toHaveCount(1)
  await page.screenshot({ path: join(SHOTS, 'accept-dialog-laptop.png') })

  // Another choice shows its own preview.
  await dlg.getByRole('radio', { name: 'Prompt aus einer anderen Aufgabe' }).check()
  await expect(dlg.getByRole('combobox', { name: 'Aufgabe, aus der der Prompt kommt' })).toBeVisible()
  await dlg.getByRole('radio', { name: 'Leer' }).check()
  await expect(dlg.locator('.accept-starter')).toHaveCount(0)
  await dlg.getByRole('button', { name: 'Aufgabe annehmen' }).click()
  await expect(dlg).toHaveCount(0)

  // The task tab: the hint once.
  await page.getByRole('tab', { name: 'Aufgabe' }).click()
  await expect(page.locator('.task-panel').getByText(/sieht diesen Text nicht|kennt diesen Text nicht/)).toHaveCount(1)
})
