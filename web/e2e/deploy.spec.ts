// Stage 4 in the browser: one deploy is one version of prompt and team
// tools, and "Diese Version wiederherstellen" brings both back.

import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { Lab, ROOT, SHOTS, type Team } from './lab'

let lab: Lab
let team: Team

test.beforeAll(async () => {
  lab = await Lab.start({ robots: 1 })
  team = await lab.team('Versionen', ['mbot-1'])
  const ex = JSON.parse(readFileSync(join(ROOT, 'docs', 'examples', 'programs', 'quadrat.json'), 'utf8'))
  await lab.teamCall(team, 'POST', '/programs', { name: 'quadrat', description: 'Fährt ein Quadrat.', blockly: ex.blockly })
})

test.afterAll(async () => {
  await lab?.stop()
})

/** The history entry of a deployed version (not a browser snapshot that mentions it). */
const version = (page: Page, n: number) =>
  page.locator('.history-item').filter({ has: page.locator('strong', { hasText: new RegExp(`^Version ${n}$`) }) })

test('deploy from the tool editor, change both, restore the first version', async ({ page }) => {
  await page.goto(lab.page(team))
  const prompt = page.locator('.editor .cm-content')
  await prompt.click()
  await page.keyboard.type('Fahre ein Quadrat mit `quadrat`.')

  // Deploy from the tool editor: version 1 holds the prompt and the tool.
  await page.getByRole('tab', { name: 'Werkzeuge' }).click()
  await page.locator('.own-tool', { hasText: 'quadrat' }).getByRole('button', { name: 'Bearbeiten' }).click()
  await expect(page.locator('.blockly-host .blocklyMainBackground').first()).toBeVisible()
  await page.locator('.tool-head').getByRole('button', { name: 'Deployen' }).click()
  await expect(page.locator('.toast')).toContainText('Version 1 deployt mit 1 eigenen Werkzeug')
  await expect(page.locator('.own-tool .tag-agent')).toHaveText('beim Agenten')

  // Change the tool and the prompt, deploy version 2.
  const desc = page.getByLabel('Beschreibung für den Agenten')
  await desc.fill('Fährt ein großes Quadrat.')
  await expect(page.locator('.own-tool .tag-agent')).toHaveText('beim Agenten, geändert', { timeout: 10_000 })
  await page.locator('.side-pane').screenshot({ path: join(SHOTS, 'deploy-own-tools.png') })
  await page.getByRole('tab', { name: 'Prompt' }).click()
  await prompt.click()
  await page.keyboard.press('End')
  await page.keyboard.type(' Danach finish.')
  await page.locator('.deploy-actions').getByRole('button', { name: /^Deployen Strg/ }).click()
  await expect(page.locator('.toast')).toContainText('Version 2 deployt')

  // Something not deployed yet: the restore asks first.
  await page.getByRole('tab', { name: 'Werkzeug bauen' }).click()
  await expect(desc).toHaveValue('Fährt ein großes Quadrat.') // the editor has loaded the tool
  await expect(page.locator('.blockly-host .blocklyMainBackground').first()).toBeVisible()
  await desc.fill('Noch nicht deployt.')
  await expect(page.locator('.own-tool .tag-agent')).toHaveText('beim Agenten, geändert', { timeout: 10_000 })
  await page.getByRole('tab', { name: 'Prompt' }).click()
  await page.getByRole('tab', { name: 'Verlauf' }).click()
  await version(page, 2).click()
  await expect(version(page, 2)).toContainText('quadrat: geändert')
  await expect(version(page, 1)).toContainText('quadrat: neu')
  await page.locator('.prompt-pane').screenshot({ path: join(SHOTS, 'deploy-history.png') })
  await version(page, 1).click()
  await page.getByRole('button', { name: 'Diese Version wiederherstellen' }).click()
  const ask = page.getByRole('alertdialog', { name: 'Wiederherstellen bestätigen' })
  await expect(ask).toContainText('die Änderungen an „quadrat“')
  await page.locator('.prompt-pane').screenshot({ path: join(SHOTS, 'deploy-restore-question.png') })
  await ask.getByRole('button', { name: 'Trotzdem wiederherstellen' }).click()

  // Both are as they were in version 1.
  await expect(prompt).toHaveText('Fahre ein Quadrat mit `quadrat`.')
  const tools = await lab.teamCall(team, 'GET', '/programs')
  expect(tools.map((t: any) => [t.name, t.description])).toEqual([['quadrat', 'Fährt ein Quadrat.']])
  await page.getByRole('tab', { name: 'Werkzeug bauen' }).click()
  await expect(desc).toHaveValue('Fährt ein Quadrat.')
})
