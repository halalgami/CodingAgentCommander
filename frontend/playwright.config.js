import { defineConfig } from "@playwright/test";
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
});
