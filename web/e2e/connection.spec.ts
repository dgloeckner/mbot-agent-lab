// A dead connection: the server freezes (like a dead Wi-Fi, the connection
// stays open and nothing arrives). The band says so, the robot card shows old
// values as old, "Stopp" says the robots were not stopped, and everything
// recovers without a reload.

import { expect, test } from '@playwright/test'
import { join } from 'node:path'
import { Lab, SHOTS, type Team } from './lab'

let lab: Lab
let team: Team

test.beforeAll(async () => {
  lab = await Lab.start({ robots: 1 })
  team = await lab.team('Funkloch', ['mbot-1'])
})

test.afterAll(async () => {
  await lab?.stop()
})

test('a frozen server: the band notices, Stopp tells the truth, all recovers', async ({ page }) => {
  test.setTimeout(120_000)
  await page.setViewportSize({ width: 1366, height: 768 })
  await page.goto(lab.page(team))
  const card = page.locator('.robot')
  await expect(card).toContainText('online')
  const lost = page.locator('.conn[data-lost]')

  lab.signalServer('SIGSTOP')
  try {
    // Three keep-alive intervals (10 s each) without any event.
    await expect(lost).toBeVisible({ timeout: 40_000 })
    await expect(card).toHaveAttribute('data-status', 'stale')
    await expect(card).toContainText(/Stand vor \d+ s/)
    await page.locator('.band-actions').getByRole('button', { name: 'Stopp' }).click()
    await expect(page.locator('.toast-error')).toContainText('NICHT angehalten')
    await page.waitForTimeout(300) // the toast has faded in
    await page.screenshot({ path: join(SHOTS, 'connection-lost.png') })
  } finally {
    lab.signalServer('SIGCONT')
  }
  await expect(lost).toHaveCount(0, { timeout: 40_000 })
  await expect(card).toHaveAttribute('data-status', 'ok')
  await expect(card).toContainText('online')

  // The browser offline: the hint comes at once.
  await page.context().setOffline(true)
  await expect(lost).toBeVisible({ timeout: 5_000 })
  await page.context().setOffline(false)
  await expect(lost).toHaveCount(0, { timeout: 20_000 })
})
