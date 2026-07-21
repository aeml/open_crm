import { defineConfig, devices } from '@playwright/test'

const webURL = process.env.OPEN_CRM_E2E_WEB_URL || 'http://127.0.0.1:4173'
const apiURL = process.env.OPEN_CRM_E2E_API_URL || 'http://127.0.0.1:8081'
const databaseURL = process.env.OPEN_CRM_E2E_DATABASE_URL
const reuseExistingServer = process.env.OPEN_CRM_E2E_REUSE_SERVER === 'true'
const outputDir = process.env.OPEN_CRM_E2E_OUTPUT_DIR || 'test-results'
const smtpCaptureURL = process.env.OPEN_CRM_E2E_SMTP_CAPTURE_URL || 'http://127.0.0.1:2526'
const stripeSandboxURL = process.env.OPEN_CRM_E2E_STRIPE_SANDBOX_URL || 'http://127.0.0.1:2527'
const billingProvider = process.env.OPEN_CRM_E2E_BILLING_PROVIDER || 'fake'
const hostedBilling = billingProvider === 'stripe'
const testCredentialKey = 'MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY='
const stripeSecretKey = process.env.OPEN_CRM_E2E_STRIPE_SECRET_KEY || 'sk_test_open_crm_e2e'
const stripeWebhookSecret = process.env.OPEN_CRM_E2E_STRIPE_WEBHOOK_SECRET || 'whsec_open_crm_e2e'

if (!databaseURL) {
  throw new Error('OPEN_CRM_E2E_DATABASE_URL must point to a disposable PostgreSQL database')
}

export default defineConfig({
  testDir: './e2e',
  outputDir,
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['line'], ['html', { open: 'never', outputFolder: `${outputDir}-report` }]] : 'line',
  timeout: 60_000,
  expect: {
    timeout: 10_000
  },
  use: {
    baseURL: webURL,
    actionTimeout: 10_000,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure'
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] }
    }
  ],
  webServer: [
    {
      name: 'Open CRM API',
      command: 'go run ./cmd/migrate && go run ./cmd/open_crm_api',
      cwd: '../api',
      url: `${apiURL}/readyz`,
      timeout: 120_000,
      reuseExistingServer,
      env: {
        ...process.env,
        DATABASE_URL: databaseURL,
        API_PORT: new URL(apiURL).port,
        GO_ENV: 'test',
        ALLOWED_ORIGINS: webURL,
        WEB_BASE_URL: webURL,
        API_BASE_URL: apiURL,
        BILLING_PROVIDER: billingProvider,
        STRIPE_SECRET_KEY: hostedBilling ? stripeSecretKey : '',
        STRIPE_WEBHOOK_SECRET: hostedBilling ? stripeWebhookSecret : '',
        STRIPE_PRICE_PRO: hostedBilling ? 'price_open_crm_e2e_pro' : '',
        OPEN_CRM_TEST_STRIPE_API_BASE_URL: hostedBilling ? stripeSandboxURL : '',
        EMAIL_PROVIDER: 'fake',
        TELEPHONY_PROVIDER: 'fake',
        CALENDAR_PROVIDER: 'fake',
        CREDENTIAL_ENCRYPTION_KEY: process.env.CREDENTIAL_ENCRYPTION_KEY || testCredentialKey
      }
    },
    {
      name: 'Open CRM web',
      command: `npm run dev -- --host 127.0.0.1 --port ${new URL(webURL).port}`,
      url: webURL,
      timeout: 120_000,
      reuseExistingServer,
      env: {
        ...process.env,
        VITE_API_BASE_URL: apiURL
      }
    },
    {
      name: 'Open CRM SMTP provider sandbox',
      command: 'node e2e/support/smtp_capture.mjs',
      url: `${smtpCaptureURL}/health`,
      timeout: 30_000,
      reuseExistingServer,
      env: {
        ...process.env,
        OPEN_CRM_E2E_SMTP_PORT: process.env.OPEN_CRM_E2E_SMTP_PORT || '2525',
        OPEN_CRM_E2E_SMTP_HTTP_PORT: new URL(smtpCaptureURL).port
      }
    },
    ...(hostedBilling ? [{
      name: 'Open CRM Stripe provider sandbox',
      command: 'node e2e/support/stripe_sandbox.mjs',
      url: `${stripeSandboxURL}/health`,
      timeout: 30_000,
      reuseExistingServer,
      env: {
        ...process.env,
        OPEN_CRM_E2E_STRIPE_HTTP_PORT: new URL(stripeSandboxURL).port,
        OPEN_CRM_E2E_STRIPE_SECRET_KEY: stripeSecretKey
      }
    }] : [])
  ]
})
