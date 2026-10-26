import { defineConfig, devices } from '@playwright/test'
import { mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const data = mkdtempSync(join(tmpdir(), 'aint-e2e-'))

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  timeout: 30000,
  use: { baseURL: 'http://127.0.0.1:5179', trace: 'retain-on-failure' },
  projects: [
    { name: 'narrow', use: { ...devices['iPhone SE'], viewport: { width: 320, height: 740 }, defaultBrowserType: 'chromium' } },
    { name: 'desktop', use: { ...devices['Desktop Chrome'] } },
    { name: 'mobile', use: { ...devices['iPhone 13'], defaultBrowserType: 'chromium' } },
  ],
  webServer: [
    {
      command: `go run ./cmd/aint serve --data "${data}" --port 7879 --no-browser --fake-llm testdata/fake/m5.yaml`,
      cwd: '..',
      url: 'http://127.0.0.1:7879/api/v1/works',
      reuseExistingServer: false,
      timeout: 120000,
    },
    {
      command: 'pnpm dev --host 127.0.0.1 --port 5179 --strictPort',
      url: 'http://127.0.0.1:5179',
      env: { AINT_API: 'http://127.0.0.1:7879' },
      reuseExistingServer: false,
    },
  ],
})
