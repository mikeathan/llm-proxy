import { defineConfig } from '@playwright/test'

// Visual regression (plan D23): screenshots of the dev-only /design reference
// (and, from Phase 2, each destination with fixture data) at 1440px and 390px
// in retro-dark and retro-paper. Runs the installed Chrome, so no browser
// download is needed: `npm run test:visual` (add `-- --update-snapshots` to
// accept an intended change).
const PORT = 5199

export default defineConfig({
  testDir: './e2e',
  snapshotPathTemplate: '{testDir}/__screenshots__/{projectName}/{arg}{ext}',
  fullyParallel: true,
  reporter: 'list',
  use: {
    baseURL: `http://127.0.0.1:${PORT}/admin/`,
    channel: 'chrome',
  },
  expect: { toHaveScreenshot: { maxDiffPixelRatio: 0.01, animations: 'disabled' } },
  webServer: {
    command: `npx vite --port ${PORT} --strictPort --host 127.0.0.1`,
    url: `http://127.0.0.1:${PORT}/admin/design`,
    reuseExistingServer: !process.env.CI,
  },
  projects: [
    { name: 'desktop', use: { viewport: { width: 1440, height: 900 } } },
    { name: 'mobile', use: { viewport: { width: 390, height: 844 } } },
  ],
})
