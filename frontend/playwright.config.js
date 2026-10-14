import { defineConfig, devices } from "@playwright/test";

// WebKit runs only the attachments specs: Commander ships in WKWebView on
// macOS, and paste/drop event shapes are where the engines differ. Set
// PW_SKIP_WEBKIT=1 on a machine where `npx playwright install webkit` fails.
const projects = [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }];
if (!process.env.PW_SKIP_WEBKIT) {
  projects.push({ name: "webkit", testMatch: /attachments\.spec\.js/, use: { ...devices["Desktop Safari"] } });
}

export default defineConfig({
  testDir: "./tests",
  webServer: {
    command: "npm run build && npx vite preview --port 4173 --strictPort",
    url: "http://localhost:4173",
    reuseExistingServer: false,
    timeout: 120000,
  },
  use: {
    baseURL: "http://localhost:4173",
    contextOptions: { reducedMotion: "reduce" },
  },
  projects,
});
