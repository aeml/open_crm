import AxeBuilder from '@axe-core/playwright'
import { expect, test } from '@playwright/test'

const wcagTags = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa']

// The shared public signup budget permits three requests per client/hour. Keep
// this deterministic scan to one attempt so the functional journey retains
// its configured retry without a retry-only rate-limit failure.
test.describe.configure({ retries: 0 })

function uniqueRunID() {
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function formatViolations(violations) {
  return violations.map((violation) => ({
    id: violation.id,
    impact: violation.impact,
    help: violation.help,
    helpUrl: violation.helpUrl,
    nodes: violation.nodes.map((node) => ({
      target: node.target,
      html: node.html,
      failureSummary: node.failureSummary
    }))
  }))
}

async function expectNoAccessibilityViolations(page, surface) {
  const results = await new AxeBuilder({ page }).withTags(wcagTags).analyze()
  const violations = formatViolations(results.violations)

  await test.info().attach(`axe-${surface}`, {
    body: JSON.stringify({ url: page.url(), violations }, null, 2),
    contentType: 'application/json'
  })

  expect(violations, `${surface} must have no automated WCAG A/AA violations`).toEqual([])
}

async function bootstrapWorkspace(page) {
  const runID = uniqueRunID()

  await page.goto('/bootstrap')
  await expect(page.getByRole('heading', { name: 'Create your workspace' })).toBeVisible()
  await expectNoAccessibilityViolations(page, 'workspace-signup')

  await page.getByLabel('Company name').fill(`Accessibility Workspace ${runID}`)
  await page.getByLabel('Business type').selectOption('general')
  await page.getByLabel('First name').fill('Accessibility')
  await page.getByLabel('Last name').fill('Owner')
  await page.getByLabel('Email').fill(`accessibility-owner-${runID}@example.test`)
  await page.getByLabel('Password').fill('Accessible-Pilot-Secure-31!')
  await page.getByRole('button', { name: 'Create workspace' }).click()

  await expect(page.getByRole('heading', { name: 'Check your email' })).toBeVisible()
  await expectNoAccessibilityViolations(page, 'workspace-signup-confirmation')
  await page.getByRole('link', { name: 'Verify email locally' }).click()
  await expect(page).toHaveURL(/\/dashboard$/)
  await expect(page.getByText('Build the first useful CRM loop.')).toBeVisible()
}

async function visitAuthenticatedSurface(page, path, title, surface) {
  await page.goto(path)
  await expect(page).toHaveTitle(`${title} — Open CRM`)
  await expect(page.locator('.route-loading')).toHaveCount(0)
  await page.waitForLoadState('networkidle')
  await expect(page.locator('[role="alert"]')).toHaveCount(0)
  await expectNoAccessibilityViolations(page, surface)
}

test('critical public and authenticated surfaces meet automated WCAG A/AA rules', async ({ page }) => {
  await page.goto('/login')
  await expect(page.getByRole('heading', { name: 'Sign in to Open CRM' })).toBeVisible()
  await expectNoAccessibilityViolations(page, 'login')

  await page.goto('/forgot-password')
  await expect(page.getByRole('heading', { name: 'Reset your password' })).toBeVisible()
  await expectNoAccessibilityViolations(page, 'password-reset-request')

  await page.goto('/reset-password')
  await expect(page.getByText('Reset token is missing. Request a new reset link.')).toBeVisible()
  await expectNoAccessibilityViolations(page, 'password-reset-missing-token')

  await bootstrapWorkspace(page)
  await expectNoAccessibilityViolations(page, 'dashboard')

  await page.keyboard.press('Tab')
  await expect(page.getByRole('link', { name: 'Skip to main content' })).toBeFocused()

  const authenticatedSurfaces = [
    ['/companies', 'Companies', 'clients'],
    ['/deals', 'Deals', 'deals'],
    ['/tasks', 'Tasks', 'tasks'],
    ['/reports', 'Reports', 'reports'],
    ['/settings/profile', 'My Profile', 'profile-and-session-security'],
    ['/settings/users', 'Users', 'team-access'],
    ['/settings/imports', 'Data imports', 'data-imports'],
    ['/settings/billing', 'Plan & Billing', 'billing-and-export']
  ]

  for (const [path, title, surface] of authenticatedSurfaces) {
    await visitAuthenticatedSurface(page, path, title, surface)
  }
})
