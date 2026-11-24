// Stage 1 in the browser: a team runs built-in tools by hand in the
// "Werkzeuge" tab and sees them as "Ihr" in the protocol.

import { expect, test } from '@playwright/test'
import { join } from 'node:path'
import { Lab, SHOTS, type Team } from './lab'

let lab: Lab
let team: Team

test.beforeAll(async () => {
  lab = await Lab.start({ robots: 2 })
  team = await lab.team('Blaue Blitze', ['mbot-1'])
})

test.afterAll(async () => {
  await lab?.stop()
})

test('drive by hand in the Werkzeuge tab, then the console is off during a run', async ({ page }) => {
  await page.goto(lab.page(team))
  await page.getByRole('tab', { name: 'Werkzeuge' }).click()
  const panel = page.locator('.tools-panel')
  await expect(panel.locator('.tool-tag', { hasText: 'nur Agent' }).first()).toBeVisible()
  await page.locator('.side-pane').screenshot({ path: join(SHOTS, 'console-list.png') })

  // Open drive, enter 20, run it.
  await panel.getByRole('button', { name: /^drive\b/ }).first().click()
  const form = panel.getByRole('form', { name: 'drive ausführen' })
  await form.getByLabel('cm', { exact: true }).fill('20')
  await form.getByRole('button', { name: 'Ausführen' }).click()
  await expect(form.locator('.console-result p')).toHaveText(/Fertig, \d+ cm gefahren/)

  // A second drive across the line, for a floor band with a stripe.
  await form.getByLabel('cm', { exact: true }).fill('30')
  await form.getByRole('button', { name: 'Ausführen' }).click()
  await expect(form.getByRole('button', { name: 'Läuft …' })).toBeVisible()
  await expect(form.getByRole('button', { name: 'Ausführen' })).toBeEnabled()
  await expect(form.locator('.console-keys')).toContainText('driven_cm: ')
  await expect(form.locator('.floor-band svg')).toBeVisible()
  await form.locator('details summary').click()
  await page.locator('.side-pane').screenshot({ path: join(SHOTS, 'console-result.png') })

  // The protocol shows both calls as "Ihr".
  await page.getByRole('tab', { name: 'Protokoll' }).click()
  const ihr = page.locator('.log-line[data-group="team"][data-kind="tool"]')
  await expect(ihr).toHaveCount(2)
  await expect(ihr.first().locator('.log-who')).toHaveText('Ihr')
  await expect(ihr.first()).toContainText('drive(cm: 20)')
  await expect(page.getByRole('button', { name: 'Selbst steuern' })).toBeVisible()
  await page.locator('.side-pane').screenshot({ path: join(SHOTS, 'console-protocol.png') })

  // During a run the console is off.
  await lab.teamCall(team, 'POST', '/prompt', { text: 'Warte auf dein Team.' })
  await lab.teamCall(team, 'POST', '/start')
  await page.getByRole('tab', { name: 'Werkzeuge' }).click()
  await panel.getByRole('button', { name: /^drive\b/ }).first().click()
  await expect(page.locator('.console-hint')).toContainText('Erst anhalten')
  await expect(form.getByRole('button', { name: 'Ausführen' })).toBeDisabled()
  await page.locator('.side-pane').screenshot({ path: join(SHOTS, 'console-disabled.png') })
  await lab.teamCall(team, 'POST', '/stop')
  await expect(form.getByRole('button', { name: 'Ausführen' })).toBeEnabled()

  // With two robots a picker sits above the list.
  await lab.adminCall('POST', '/admin/api/assign', { robot: 'mbot-2', team: team.id })
  const picker = page.locator('.robot-picker')
  await expect(picker.getByRole('button')).toHaveCount(2)
  await picker.getByRole('button').nth(1).click()
  // The robot learns its new owner from the next heartbeat (once a second) and
  // ends a command it got before with "reassigned"; give it that heartbeat.
  await page.waitForTimeout(2500)
  await form.getByLabel('cm', { exact: true }).fill('10')
  await form.getByRole('button', { name: 'Ausführen' }).click()
  await expect(form.locator('.console-result p')).toHaveText(/Fertig/)
  await page.locator('.side-pane').screenshot({ path: join(SHOTS, 'console-robot-picker.png') })

  for (const line of lab.serverLines('tool by hand')) console.log('server: ' + line)
})
