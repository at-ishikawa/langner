import { defineConfig, devices } from "@playwright/test";

const FRONTEND_PORT = 3100;
const BACKEND_PORT = 8080;

export default defineConfig({
  testDir: "./e2e-integration",
  timeout: 60000,
  use: {
    baseURL: `http://localhost:${FRONTEND_PORT}`,
  },
  projects: [
    {
      name: "integration",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: [
    {
      command:
        "cd .. && make -C backend build && OPENAI_API_KEY=test-key ./langner-server --config config.example.yml",
      port: BACKEND_PORT,
      reuseExistingServer: !process.env.CI,
      timeout: 60000,
    },
    {
      command: `pnpm dev --port ${FRONTEND_PORT}`,
      url: `http://localhost:${FRONTEND_PORT}`,
      reuseExistingServer: !process.env.CI,
      timeout: 60000,
      env: {
        NEXT_PUBLIC_API_BASE_URL: `http://localhost:${BACKEND_PORT}`,
        NEXT_PUBLIC_CONNECT_JSON: "true",
      },
    },
  ],
  workers: 1,
});
