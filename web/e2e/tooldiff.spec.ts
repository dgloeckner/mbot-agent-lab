// "Verlauf" compares a team tool between versions: chips per version, two
// read-only block pictures with marks for a changed value, a new, a removed
// and a moved block, a new and a removed tool, and two versions.

import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { Lab, ROOT, SHOTS, type Team } from './lab'

let lab: Lab
let team: Team

const example = (name: string) => JSON.parse(readFileSync(join(ROOT, 'docs', 'examples', 'programs', `${name}.json`), 'utf8'))

/** The block with an id anywhere in a block state. */
function find(v: any, id: string): any {
  if (Array.isArray(v)) {
    for (const e of v) {
      const b = find(e, id)
      if (b) return b
    }
  } else if (v && typeof v === 'object') {
    if (v.id === id) return v
    for (const k of Object.keys(v)) {
      const b = find(v[k], id)
      if (b) return b
    }
  }
  return null
}

let labyrinthId = ''
let wandId = ''

test.beforeAll(async () => {
  lab = await Lab.start({ robots: 1 })
  team = await lab.team('Vergleich', ['mbot-1'])
  const l = await lab.teamCall(team, 'POST', '/programs', {
    name: 'labyrinth',
    description: 'Fährt bis zum blauen Feld.',
    blockly: example('labyrinth').blockly,
  })
  const w = await lab.teamCall(team, 'POST', '/programs', {
    name: 'wand',
    description: 'Fährt zur Wand.',
    blockly: example('wand').blockly,
  })
  labyrinthId = l.id
  wandId = w.id
  await lab.teamCall(team, 'POST', '/prompt', {
    text: 'Version eins: nutze `labyrinth`.',
  })

  // The working state: a changed value, a new block, a removed block, a
  // moved block, a new description; wand deleted, a new tool.
  const b = example('labyrinth').blockly
  find(b, 'l_drive').fields.max_cm = 200
  const turn = find(b, 'l_turn')
  const ifBlock = find(b, 'l_if')
  ifBlock.inputs.DO = {
    block: { type: 'tool_set_leds', id: 'l_led', fields: { color: 'red' } },
  }
  ifBlock.next = { block: turn }
  delete find(b, 'l_loop').next // the return block
  await lab.teamCall(team, 'PUT', `/programs/${labyrinthId}`, {
    name: 'labyrinth',
    description: 'Fährt rechts an der Wand bis zum blauen Feld.',
    blockly: b,
  })
  await lab.teamCall(team, 'DELETE', `/programs/${wandId}`)
  await lab.teamCall(team, 'POST', '/programs', {
    name: 'quadrat',
    description: 'Fährt ein Quadrat.',
    blockly: example('quadrat').blockly,
  })
})

test.afterAll(async () => {
  await lab?.stop()
})

const version = (page: Page, n: number) =>
  page.locator('.history-item').filter({
    has: page.locator('strong', { hasText: new RegExp(`^Version ${n}$`) }),
  })

async function openHistory(page: Page) {
  await page.goto(lab.page(team))
  await page.getByRole('tab', { name: 'Verlauf' }).click()
  await version(page, 1).click()
}

test('a version against the working state, with every kind of mark', async ({ page }) => {
  await page.setViewportSize({ width: 1600, height: 1000 })
  await openHistory(page)
  const chips = page.locator('.history-chips')
  await expect(chips.getByRole('tab')).toHaveText(['Prompt', 'labyrinthgeändert', 'quadratneu', 'wandentfernt'])
  await page.locator('.history-detail').screenshot({ path: join(SHOTS, 'tooldiff-chips.png') })

  await chips.getByRole('tab', { name: /labyrinth/ }).click()
  const diff = page.locator('.tooldiff')
  await expect(diff).toHaveAttribute('data-layout', 'side')
  const left = diff.locator('.tooldiff-left')
  const right = diff.locator('.tooldiff-right')
  await expect(left.locator('header')).toContainText('Version 1')
  await expect(right.locator('header')).toContainText('Euer Stand jetzt')
  await expect(left.locator('.diff-removed')).toHaveCount(1)
  await expect(left.locator('.diff-new')).toHaveCount(0)
  await expect(right.locator('.diff-new')).toHaveCount(1)
  await expect(right.locator('.diff-removed')).toHaveCount(0)
  await expect(left.locator('.diff-moved')).toHaveCount(1)
  await expect(right.locator('.diff-moved')).toHaveCount(1)
  await expect(left.locator('.diff-field')).toHaveCount(1)
  await expect(right.locator('.diff-field')).toHaveCount(1)
  await expect(diff.locator('.tooldiff-text')).toContainText('Fährt rechts an der Wand')
  await expect(diff.locator('.tooldiff-legend li')).toHaveCount(4)
  await page.locator('.history-detail').screenshot({ path: join(SHOTS, 'tooldiff-marks.png') })

  // A new tool stands on the right only, a removed one on the left only.
  await chips.getByRole('tab', { name: /quadrat/ }).click()
  await expect(left.locator('.tooldiff-missing')).toHaveText('Gab es in Version 1 noch nicht.')
  await expect(right.locator('.blocklySvg')).toBeVisible()
  await page.locator('.history-detail').screenshot({ path: join(SHOTS, 'tooldiff-new-tool.png') })
  await chips.getByRole('tab', { name: /wand/ }).click()
  await expect(right.locator('.tooldiff-missing')).toContainText('Entfernt')
  await expect(left.locator('.blocklySvg')).toBeVisible()
  await page.locator('.history-detail').screenshot({ path: join(SHOTS, 'tooldiff-removed-tool.png') })
})

test('narrow: one side above the other', async ({ page }) => {
  await page.setViewportSize({ width: 560, height: 1100 })
  await openHistory(page)
  await page
    .locator('.history-chips')
    .getByRole('tab', { name: /labyrinth/ })
    .click()
  await expect(page.locator('.tooldiff')).toHaveAttribute('data-layout', 'stacked')
  await expect(page.locator('.tooldiff-right .diff-new')).toHaveCount(1)
  const l = await page.locator('.tooldiff-left').boundingBox()
  const r = await page.locator('.tooldiff-right').boundingBox()
  expect(r!.y).toBeGreaterThan(l!.y + l!.height - 1)
  await page.screenshot({
    path: join(SHOTS, 'tooldiff-narrow.png'),
    fullPage: true,
  })
})

test('two versions for the debrief', async ({ page }) => {
  await lab.teamCall(team, 'POST', '/prompt', {
    text: 'Version zwei: nutze `labyrinth` und `quadrat`.',
  })
  await page.setViewportSize({ width: 1600, height: 1000 })
  await openHistory(page)
  await page.getByLabel('Vergleichen mit').selectOption('2')
  await expect(page.locator('.history-toolbar')).toContainText('Version 1 im Vergleich zu Version 2')
  const chips = page.locator('.history-chips')
  await expect(chips.getByRole('tab')).toHaveText(['Prompt', 'labyrinthgeändert', 'quadratneu', 'wandentfernt'])
  await chips.getByRole('tab', { name: /labyrinth/ }).click()
  await expect(page.locator('.tooldiff-right header')).toContainText('Version 2')
  await expect(page.locator('.tooldiff-right .diff-new')).toHaveCount(1)
  await page.locator('.history-detail').screenshot({ path: join(SHOTS, 'tooldiff-two-versions.png') })
  // The prompt chip compares the two version texts.
  await chips.getByRole('tab', { name: 'Prompt' }).click()
  await expect(page.locator('.diff-legend')).toContainText('Version 2')
})
