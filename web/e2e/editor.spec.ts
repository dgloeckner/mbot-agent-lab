// Stages 2 and 3 in the browser: a team builds "fahre ein Quadrat" from
// blocks in the tool editor, runs it and watches it step.

import { expect, test, type Locator, type Page } from '@playwright/test'
import { join } from 'node:path'
import { Lab, SHOTS, type Team } from './lab'

let lab: Lab
let team: Team

test.beforeAll(async () => {
  lab = await Lab.start({ robots: 1 })
  team = await lab.team('Baumeister', ['mbot-1'])
})

test.afterAll(async () => {
  await lab?.stop()
})

/** The flyout block of a category whose text starts with `text`. */
async function fromFlyout(page: Page, category: string, text: string): Promise<Locator> {
  await page.locator('.blocklyToolboxCategory', { hasText: category }).first().click()
  const block = page.locator('.blocklyFlyout .blocklyDraggable', { hasText: text }).first()
  await expect(block).toBeVisible()
  return block
}

/** Drags a block so that its top-left corner lands at (x, y). */
async function dragTo(page: Page, block: Locator, x: number, y: number) {
  const box = (await block.boundingBox())!
  const grab = { x: box.x + 8, y: box.y + 8 }
  await page.mouse.move(grab.x, grab.y)
  await page.mouse.down()
  await page.mouse.move(grab.x + 20, grab.y + 20, { steps: 4 })
  await page.mouse.move(x + 8, y + 8, { steps: 12 })
  await page.mouse.up()
  await page.waitForTimeout(400) // Blockly snaps and renders after the drop
}

/** The block canvas of the main workspace (not the flyout). */
function main(page: Page) {
  return page.locator('svg.blocklySvg > g.blocklyWorkspace > g.blocklyBlockCanvas').first()
}

test('build a square from blocks, run it and watch it step', async ({ page }) => {
  const foreign: string[] = []
  page.on('request', (req) => {
    if (!req.url().startsWith(lab.base) && !req.url().startsWith('data:')) foreign.push(req.url())
  })
  await page.goto(lab.page(team))
  await page.getByRole('tab', { name: 'Werkzeuge' }).click()
  await page.locator('.own-tools').getByRole('button', { name: 'Neues Werkzeug' }).click()
  await expect(page.locator('.blockly-host .blocklyMainBackground').first()).toBeVisible()
  const start = main(page).locator('.blocklyDraggable', { hasText: 'Wenn das Werkzeug startet' }).first()
  await expect(start).toBeVisible()
  await page.screenshot({ path: join(SHOTS, 'editor-empty.png') })

  // wiederhole 4 mal under the start block.
  let s = (await start.boundingBox())!
  await dragTo(page, await fromFlyout(page, 'Wiederholen', 'wiederhole'), s.x, s.y + s.height)
  const repeat = main(page).locator('.blocklyDraggable', { hasText: 'mal' }).first()
  await expect(repeat).toBeVisible()
  const r = (await repeat.boundingBox())!
  // fahre … cm into the loop, drehe … Grad below it.
  await dragTo(page, await fromFlyout(page, 'Fahren', 'fahre'), r.x + 24, r.y + 46)
  const drive = main(page).locator('.blocklyDraggable', { hasText: /^fahre/ }).first()
  const d = (await drive.boundingBox())!
  await dragTo(page, await fromFlyout(page, 'Fahren', 'drehe'), d.x, d.y + d.height)

  await page.getByRole('textbox', { name: 'Name', exact: true }).fill('quadrat')
  await page.getByLabel('Beschreibung für den Agenten').fill('Fährt ein Quadrat und steht danach wieder am Start.')
  await expect(page.locator('.tool-picker select').first()).toContainText('quadrat', { timeout: 10_000 })
  await expect(page.locator('.own-tool', { hasText: 'quadrat' }).locator('.tag-ok')).toBeVisible({ timeout: 10_000 })
  await page.screenshot({ path: join(SHOTS, 'editor-quadrat.png') })

  // Run it: the running block is marked and the Ablauf fills.
  await page.locator('.tool-head').getByRole('button', { name: 'Ausführen' }).click()
  await expect(main(page).locator('.blocklyHighlighted').first()).toBeVisible({ timeout: 10_000 })
  await expect(page.locator('.ablauf .ablauf-step').first()).toBeVisible({ timeout: 10_000 })
  await page.screenshot({ path: join(SHOTS, 'editor-run.png') })
  await expect(page.locator('.ablauf .ablauf-end')).toContainText('fertig nach 8 Schritten', { timeout: 30_000 })
  await expect(page.locator('.ablauf .ablauf-round')).toHaveCount(4)
  await page.screenshot({ path: join(SHOTS, 'editor-ablauf.png') })

  // Single step: it waits at "Weiter" before each call.
  await page.locator('.tool-head').getByRole('button', { name: 'Einzelschritt' }).click()
  await expect(page.locator('.ablauf-wait')).toBeVisible({ timeout: 10_000 })
  await expect(page.locator('.tool-head').getByRole('button', { name: 'Weiter' })).toBeVisible()
  await page.screenshot({ path: join(SHOTS, 'editor-single-step.png') })
  await page.locator('.tool-head').getByRole('button', { name: 'Weiter' }).click()
  await expect(page.locator('.ablauf .ablauf-step')).toHaveCount(1, { timeout: 10_000 })
  await expect(page.locator('.ablauf-wait')).toBeVisible({ timeout: 10_000 })
  await page.locator('.tool-head').getByRole('button', { name: 'Stopp' }).click()
  await expect(page.locator('.ablauf .ablauf-end')).toContainText('gestoppt', { timeout: 10_000 })

  // A loose block is refused: red outline and its reason.
  const free = (await page.locator('.blockly-host').boundingBox())!
  await dragTo(page, await fromFlyout(page, 'Licht und Anzeige', 'zeige'), free.x + 300, free.y + free.height - 200)
  const loose = main(page).locator('.blocklyDraggable.block-problem')
  await expect(loose).toHaveCount(1, { timeout: 10_000 })
  await expect(page.locator('.own-tool', { hasText: 'quadrat' }).locator('.tag-bad')).toBeVisible()
  await loose.first().locator('.blocklyIconGroup').first().click()
  await expect(page.getByText('hängt an nichts').first()).toBeVisible()
  await page.screenshot({ path: join(SHOTS, 'editor-refused.png') })
  await loose.first().click()
  await page.keyboard.press('Delete')
  await expect(page.locator('.own-tool', { hasText: 'quadrat' }).locator('.tag-ok')).toBeVisible({ timeout: 10_000 })

  // After a reload the tool is under "Eigene Werkzeuge".
  await page.reload()
  await page.getByRole('tab', { name: 'Werkzeuge' }).click()
  await expect(page.locator('.own-tool', { hasText: 'quadrat' })).toContainText('Fährt ein Quadrat')
  await page.locator('.side-pane').screenshot({ path: join(SHOTS, 'editor-own-tools.png') })
  expect(foreign).toEqual([])
})

test('on a laptop screen the canvas has room and follows the splitter', async ({ page }) => {
  await page.setViewportSize({ width: 1366, height: 768 })
  await page.goto(lab.page(team))
  await page.getByRole('tab', { name: 'Werkzeug bauen' }).click()
  await page.locator('.tool-picker select').first().selectOption({ label: 'quadrat' })
  await expect(page.locator('.blockly-host .blocklyMainBackground').first()).toBeVisible()
  const host = page.locator('.blockly-host')
  const svg = page.locator('.blockly-host svg.blocklySvg').first()
  const width = async (l: Locator) => Math.round((await l.boundingBox())!.width)
  const right = async (l: Locator) => {
    const b = (await l.boundingBox())!
    return b.x + b.width
  }
  // Blockly's trash group is larger than the can it draws; its middle must be on the canvas.
  const trashInside = async () => {
    const t = (await page.locator('.blocklyTrash').boundingBox())!
    const h = (await host.boundingBox())!
    return t.x + t.width / 2 < h.x + h.width && t.y + t.height / 2 < h.y + h.height
  }

  // Room for blocks: at least 600 px next to the toolbox, trash and the widest flyout block inside.
  expect((await width(host)) - (await width(page.locator('.blocklyToolbox')))).toBeGreaterThanOrEqual(600)
  await expect(page.locator('.blocklyTrash')).toBeInViewport()
  expect(await trashInside()).toBe(true)
  await page.locator('.blocklyToolboxCategory', { hasText: 'Fahren' }).first().click()
  const widest = await page.locator('.blocklyFlyout .blocklyDraggable').evaluateAll((els) => Math.max(...els.map((e) => e.getBoundingClientRect().right)))
  expect(widest).toBeLessThanOrEqual(await right(host))
  await page.keyboard.press('Escape')

  // Drag the splitter both ways: the canvas follows without a reload.
  const split = (await page.locator('.splitter').boundingBox())!
  for (const x of [split.x + 200, split.x - 150]) {
    await page.mouse.move(split.x + 4, split.y + split.height / 2)
    await page.mouse.down()
    await page.mouse.move(x, split.y + split.height / 2, { steps: 6 })
    await page.mouse.up()
    await expect.poll(async () => Math.abs((await width(svg)) - (await width(host)))).toBeLessThanOrEqual(1)
    expect(await trashInside()).toBe(true)
  }

  // "Groß": the editor takes the full width for a while.
  await page.locator('.tool-head').getByRole('button', { name: 'Groß' }).click()
  await expect(page.locator('.side-pane')).toBeHidden()
  await expect.poll(async () => width(svg)).toBeGreaterThan(1200)
  await page.screenshot({ path: join(SHOTS, 'editor-wide.png') })
  await page.locator('.tool-head').getByRole('button', { name: 'Normal' }).click()
  await expect(page.locator('.side-pane')).toBeVisible()
})
