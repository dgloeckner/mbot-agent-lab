import { defineConfig } from '@playwright/test'

// Browser end-to-end tests (`npx nx run web:e2e`): the built labserver with
// the scripted model and simulated robots, driven through the team page.
// Each test file starts its own lab (web/e2e/lab.ts).
export default defineConfig({
  testDir: 'e2e',
  outputDir: 'e2e/.output',
  timeout: 90_000,
  workers: 1,
  reporter: [['list']],
  use: {
    viewport: { width: 1440, height: 1100 },
    deviceScaleFactor: 2,
    locale: 'de-DE',
    trace: 'off',
  },
})
