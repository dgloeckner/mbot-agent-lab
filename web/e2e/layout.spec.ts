// Laptop widths between the one-column layout (960 px) and the wide layout:
// no horizontal page scroll, also with the splitter at both ends, and a double
// click puts the splitter back.

import { expect, test, type Page } from '@playwright/test'
import { join } from 'node:path'
import { Lab, SHOTS, type Team } from './lab'

let lab: Lab
let team: Team

test.beforeAll(async () => {
  lab = await Lab.start({ robots: 1 })
  team = await lab.team('Blitzschnell', ['mbot-1'])
})

test.afterAll(async () => {
  await lab?.stop()
})

const scrollWidth = (page: Page) => page.evaluate(() => document.documentElement.scrollWidth)

async function dragSplitter(page: Page, x: number) {
  const sp = (await page.locator('.splitter').boundingBox())!
  await page.mouse.move(sp.x + sp.width / 2, sp.y + sp.height / 2)
  await page.mouse.down()
  await page.mouse.move(x, sp.y + sp.height / 2, { steps: 5 })
  await page.mouse.up()
}

for (const width of [1024, 1093, 1366]) {
  test(`no horizontal scroll at ${width} px, also with the splitter at both ends`, async ({ page }) => {
    await page.setViewportSize({ width, height: 768 })
    await page.goto(lab.page(team))
    await expect(page.locator('.robot')).toBeVisible()
    expect(await scrollWidth(page)).toBe(width)
    const lastTab = page.locator('.side-pane .tab').last()

    for (const x of [0, width]) {
      await dragSplitter(page, x)
      expect(await scrollWidth(page)).toBe(width)
      // Both panes keep their minimum width; the side pane's tabs all fit.
      expect((await page.locator('.prompt-pane').boundingBox())!.width).toBeGreaterThanOrEqual(419)
      expect((await page.locator('.side-pane').boundingBox())!.width).toBeGreaterThanOrEqual(399)
      const tab = (await lastTab.boundingBox())!
      const pane = (await page.locator('.side-pane').boundingBox())!
      expect(tab.x + tab.width).toBeLessThanOrEqual(pane.x + pane.width + 1)
    }
    await page.screenshot({ path: join(SHOTS, `layout-${width}-narrow-side.png`) })

    await page.locator('.splitter').dblclick()
    await expect(page.locator('.splitter')).toHaveAttribute('aria-valuenow', '58')
  })
}
