import AxeBuilder from '@axe-core/playwright'
import { expect, test } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { createServer } from 'node:http'

const apiURL = process.env.OPEN_CRM_E2E_API_URL || 'http://127.0.0.1:8081'
const smtpCaptureURL = process.env.OPEN_CRM_E2E_SMTP_CAPTURE_URL || 'http://127.0.0.1:2526'
const smtpHost = process.env.OPEN_CRM_E2E_SMTP_HOST || 'localhost'
const smtpPort = process.env.OPEN_CRM_E2E_SMTP_PORT || '2525'
const databaseURL = process.env.OPEN_CRM_E2E_DATABASE_URL

// This journey plus the accessibility scan intentionally consume the exact
// three-request public signup budget. Retrying would exceed that production
// policy and hide the original failure behind a secondary rate-limit error.
test.describe.configure({ retries: 0 })

function uniqueRunID() {
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function datetimeLocalDaysFromNow(days) {
  const value = new Date(Date.now() + days * 24 * 60 * 60 * 1000)
  value.setSeconds(0, 0)
  return value.toISOString().slice(0, 16)
}

function utcDateDaysFromNow(days) {
  const value = new Date(Date.now() + days * 24 * 60 * 60 * 1000)
  return value.toISOString().slice(0, 10)
}

function seedSharedInboxContinuation(ownerEmail, runID) {
  execFileSync('go', ['run', './cmd/e2e_seed_shared_inbox', ownerEmail, runID], {
    cwd: '../api',
    env: {
      ...process.env,
      DATABASE_URL: databaseURL,
      GO_ENV: 'test'
    },
    stdio: ['ignore', 'pipe', 'pipe']
  })
}

function seedLeadReviewContinuation(ownerEmail, runID) {
  execFileSync('go', ['run', './cmd/e2e_seed_lead_reviews', ownerEmail, runID], {
    cwd: '../api',
    env: {
      ...process.env,
      DATABASE_URL: databaseURL,
      GO_ENV: 'test'
    },
    stdio: ['ignore', 'pipe', 'pipe']
  })
}

function seedLeadFormContinuation(ownerEmail, runID) {
  execFileSync('go', ['run', './cmd/e2e_seed_lead_forms', ownerEmail, runID], {
    cwd: '../api',
    env: {
      ...process.env,
      DATABASE_URL: databaseURL,
      GO_ENV: 'test'
    },
    stdio: ['ignore', 'pipe', 'pipe']
  })
}

function seedLeadSurfaceContinuation(ownerEmail, runID) {
  execFileSync('go', ['run', './cmd/e2e_seed_lead_surfaces', ownerEmail, runID], {
    cwd: '../api',
    env: {
      ...process.env,
      DATABASE_URL: databaseURL,
      GO_ENV: 'test'
    },
    stdio: ['ignore', 'pipe', 'pipe']
  })
}

function seedProductCatalogContinuation(ownerEmail, runID) {
  execFileSync('go', ['run', './cmd/e2e_seed_product_catalog', ownerEmail, runID], {
    cwd: '../api',
    env: {
      ...process.env,
      DATABASE_URL: databaseURL,
      GO_ENV: 'test'
    },
    stdio: ['ignore', 'pipe', 'pipe']
  })
}

function seedWorkflowDefinitionContinuation(ownerEmail, runID) {
  execFileSync('go', ['run', './cmd/e2e_seed_workflow_definitions', ownerEmail, runID], {
    cwd: '../api',
    env: {
      ...process.env,
      DATABASE_URL: databaseURL,
      GO_ENV: 'test'
    },
    stdio: ['ignore', 'pipe', 'pipe']
  })
}

function seedReportDefinitionContinuation(ownerEmail, runID) {
  execFileSync('go', ['run', './cmd/e2e_seed_report_definitions', ownerEmail, runID], {
    cwd: '../api',
    env: {
      ...process.env,
      DATABASE_URL: databaseURL,
      GO_ENV: 'test'
    },
    stdio: ['ignore', 'pipe', 'pipe']
  })
}

function makeReportScheduleDue(ownerEmail) {
  execFileSync('go', ['run', './cmd/e2e_due_report_schedule', ownerEmail], {
    cwd: '../api',
    env: {
      ...process.env,
      DATABASE_URL: databaseURL,
      GO_ENV: 'test'
    },
    stdio: ['ignore', 'pipe', 'pipe']
  })
}

function seedQuoteTemplateContinuation(ownerEmail, runID) {
  execFileSync('go', ['run', './cmd/e2e_seed_quote_templates', ownerEmail, runID], {
    cwd: '../api',
    env: {
      ...process.env,
      DATABASE_URL: databaseURL,
      GO_ENV: 'test'
    },
    stdio: ['ignore', 'pipe', 'pipe']
  })
}

function seedEmailSequenceContinuation(ownerEmail, runID) {
  execFileSync('go', ['run', './cmd/e2e_seed_email_sequences', ownerEmail, runID], {
    cwd: '../api',
    env: {
      ...process.env,
      DATABASE_URL: databaseURL,
      GO_ENV: 'test'
    },
    stdio: ['ignore', 'pipe', 'pipe']
  })
}

function seedEmailDefinitionContinuation(ownerEmail, runID) {
  execFileSync('go', ['run', './cmd/e2e_seed_email_definitions', ownerEmail, runID], {
    cwd: '../api',
    env: {
      ...process.env,
      DATABASE_URL: databaseURL,
      GO_ENV: 'test'
    },
    stdio: ['ignore', 'pipe', 'pipe']
  })
}

function seedSavedViewContinuation(ownerEmail, runID) {
  execFileSync('go', ['run', './cmd/e2e_seed_saved_views', ownerEmail, runID], {
    cwd: '../api',
    env: {
      ...process.env,
      DATABASE_URL: databaseURL,
      GO_ENV: 'test'
    },
    stdio: ['ignore', 'pipe', 'pipe']
  })
}

function seedUserCatalogContinuation(ownerEmail, runID) {
  execFileSync('go', ['run', './cmd/e2e_seed_user_catalog', ownerEmail, runID], {
    cwd: '../api',
    env: {
      ...process.env,
      DATABASE_URL: databaseURL,
      GO_ENV: 'test'
    },
    stdio: ['ignore', 'pipe', 'pipe']
  })
}

async function bootstrapWorkspace(page, runID, prefix = 'Pilot') {
  const email = `${prefix.toLowerCase()}-owner-${runID}@example.test`
  const password = 'Correct-Horse-Battery-27!'
  const organizationName = `${prefix} Workspace ${runID}`

  await page.goto('/bootstrap')
  await expect(page.getByRole('heading', { name: 'Create your workspace' })).toBeVisible()
  await page.getByLabel('Company name').fill(organizationName)
  await page.getByLabel('Business type').selectOption('general')
  await page.getByLabel('First name').fill(prefix)
  await page.getByLabel('Last name').fill('Owner')
  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: 'Create workspace' }).click()

  await expect(page.getByRole('heading', { name: 'Check your email' })).toBeVisible()
  await expect(page.getByText('your 14-day trial starts only after verification', { exact: false })).toBeVisible()
  await page.getByRole('link', { name: 'Verify email locally' }).click()
  await expect(page).toHaveURL(/\/dashboard$/)
  await expect(page.getByText(organizationName, { exact: true })).toBeVisible()
  return { email, password, organizationName }
}

test('pilot lead-to-client journey persists data and isolates tenants', async ({ browser, page }) => {
  // This intentionally broad journey exercises the complete pilot contract and
  // can exceed the default wall-clock budget on a shared CI runner. Action and
  // assertion timeouts remain bounded by the global 10-second limits.
  test.setTimeout(150_000)
  const runID = uniqueRunID()
  const resetSMTP = await page.request.delete(`${smtpCaptureURL}/messages`)
  expect(resetSMTP.status()).toBe(200)
  const owner = await bootstrapWorkspace(page, runID)
  const invitedEmail = `jamie-${runID}@example.test`
  const invitedPassword = 'Jamie-Pilot-Secure-29!'

  await page.getByRole('link', { name: 'My Email', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'My email connection' })).toBeVisible()
  await page.getByLabel('From email').fill(owner.email)
  await page.getByLabel('From name').fill('Pilot Owner')
  await page.getByLabel('SMTP host').fill(smtpHost)
  await page.getByLabel('SMTP port').fill(smtpPort)
  await page.getByLabel('SMTP username').fill(owner.email)
  await page.getByLabel('SMTP password').fill(`smtp-sandbox-${runID}`)
  await page.getByRole('checkbox', { name: 'Use TLS / STARTTLS' }).uncheck()
  await page.getByRole('button', { name: 'Save connection' }).click()
  await expect(page.getByText('Email account saved. Emails you send to contacts will come from your address.')).toBeVisible()

  const quoteCatalogName = `Discovery and implementation ${runID}`
  const quoteCatalogSKU = `PILOT-${runID}`.toUpperCase()
  seedProductCatalogContinuation(owner.email, runID)
  await page.getByRole('link', { name: 'Product Catalog', exact: true }).click()
  await expect(page.getByRole('heading', { name: `Browser catalog ${runID} #001`, exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: `Browser catalog ${runID} #051`, exact: true })).toHaveCount(0)
  await expect(page.getByText('Showing 50 of 51 catalog items', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: 'Next page' }).click()
  await expect(page.getByRole('heading', { name: `Browser catalog ${runID} #051`, exact: true })).toBeVisible()
  await page.getByLabel('Search product catalog').fill(`Browser catalog ${runID} #051`)
  await page.getByRole('button', { name: 'Apply search' }).click()
  await expect(page.getByText('Showing 1 of 1 catalog items', { exact: false })).toBeVisible()
  await page.getByLabel('Search product catalog').fill('')
  await page.getByRole('button', { name: 'Apply search' }).click()
  const catalogForm = page.locator('form').filter({ has: page.getByRole('button', { name: 'Create catalog item' }) })
  await catalogForm.getByLabel('Name').fill(quoteCatalogName)
  await catalogForm.getByLabel('SKU').fill(quoteCatalogSKU)
  await catalogForm.getByLabel('Type').selectOption('service')
  await catalogForm.getByLabel('Unit price').fill('25000')
  await catalogForm.getByLabel('Currency').fill('USD')
  await catalogForm.getByLabel('Unit', { exact: true }).fill('project')
  await catalogForm.getByLabel('Description').fill('Reusable pilot implementation package')
  await catalogForm.getByRole('button', { name: 'Create catalog item' }).click()
  await expect(page.getByText('Catalog item created.', { exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: quoteCatalogName, exact: true })).toBeVisible()
  const productCatalogAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-product-catalog-continuation', {
    body: JSON.stringify({ url: page.url(), violations: productCatalogAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(productCatalogAccessibility.violations).toEqual([])

  seedSharedInboxContinuation(owner.email, runID)
  await page.getByRole('link', { name: 'Team Inbox', exact: true }).click()
  await expect(page.getByRole('heading', { name: `Browser inbox ${runID} #1`, exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: `Browser inbox ${runID} #51`, exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Load older messages' }).click()
  await expect(page.getByRole('heading', { name: `Browser inbox ${runID} #51`, exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Load older messages' })).toHaveCount(0)
  const sharedInboxContinuationAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-shared-inbox-continuation', {
    body: JSON.stringify({ url: page.url(), violations: sharedInboxContinuationAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(sharedInboxContinuationAccessibility.violations).toEqual([])

  await page.getByRole('link', { name: 'My Profile', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Preferences' })).toBeVisible()
  const taskReminderPreference = page.getByRole('checkbox', { name: 'Notify me when an assigned task is due soon or overdue', exact: true })
  await expect(taskReminderPreference).toBeChecked()
  await taskReminderPreference.uncheck()
  await page.getByRole('button', { name: 'Save preferences' }).click()
  await expect(page.getByText('Preferences saved.', { exact: true })).toBeVisible()
  await taskReminderPreference.check()
  await page.getByRole('button', { name: 'Save preferences' }).click()
  await expect(page.getByText('Preferences saved.', { exact: true })).toBeVisible()

  await page.getByRole('link', { name: 'Users', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Team access' })).toBeVisible()
  const inviteForm = page.locator('form').filter({ has: page.getByRole('button', { name: 'Invite user' }) })
  await inviteForm.getByLabel('First name').fill('Jamie')
  await inviteForm.getByLabel('Last name').fill('Pilot')
  await inviteForm.getByLabel('Email').fill(invitedEmail)
  await inviteForm.getByLabel('Role').selectOption('member')
  await inviteForm.getByRole('button', { name: 'Invite user' }).click()
  await expect(page.getByText(invitedEmail, { exact: true })).toBeVisible()
  await expect(page.getByText('Setup link created:', { exact: false })).toBeVisible()
  const firstSetupPath = await page.locator('.inline-note code').textContent()
  expect(firstSetupPath).toMatch(/^\/setup-password\?token=/)
  const memberRow = page.getByRole('listitem').filter({ hasText: invitedEmail })
  await memberRow.getByRole('button', { name: 'Resend invitation', exact: true }).click()
  await expect(page.getByText('old links are invalid', { exact: false })).toBeVisible()
  const resentSetupPath = await page.locator('.inline-note code').textContent()
  expect(resentSetupPath).toMatch(/^\/setup-password\?token=/)
  expect(resentSetupPath).not.toBe(firstSetupPath)
  const oldSetupAttempt = await page.request.post(`${apiURL}/auth/setup-password`, {
    data: { token: new URL(firstSetupPath, page.url()).searchParams.get('token'), password: invitedPassword }
  })
  expect(oldSetupAttempt.status()).toBe(400)

  await memberRow.getByRole('button', { name: 'Revoke invitation', exact: true }).click()
  await memberRow.getByRole('button', { name: 'Confirm revocation', exact: true }).click()
  await expect(memberRow.getByText('Invitation revoked', { exact: true })).toBeVisible()
  await expect(memberRow.getByText('Disabled', { exact: true })).toBeVisible()
  const revokedSetupAttempt = await page.request.post(`${apiURL}/auth/setup-password`, {
    data: { token: new URL(resentSetupPath, page.url()).searchParams.get('token'), password: invitedPassword }
  })
  expect(revokedSetupAttempt.status()).toBe(400)

  await memberRow.getByRole('button', { name: 'Reactivate', exact: true }).click()
  await expect(memberRow.getByText('Active', { exact: true })).toBeVisible()
  await memberRow.getByRole('button', { name: 'Resend invitation', exact: true }).click()
  const setupPath = await page.locator('.inline-note code').textContent()
  expect(setupPath).toMatch(/^\/setup-password\?token=/)
  expect(setupPath).not.toBe(resentSetupPath)

  const memberContext = await browser.newContext()
  const memberPage = await memberContext.newPage()
  await memberPage.goto(setupPath)
  await expect(memberPage.getByRole('heading', { name: 'Choose your password' })).toBeVisible()
  await memberPage.getByLabel('New password').fill(invitedPassword)
  await memberPage.getByRole('button', { name: 'Set password' }).click()
  await expect(memberPage).toHaveURL(/\/login$/)
  await memberPage.getByLabel('Email').fill(invitedEmail)
  await memberPage.getByLabel('Password').fill(invitedPassword)
  await memberPage.getByRole('button', { name: 'Sign in' }).click()
  await expect(memberPage).toHaveURL(/\/dashboard$/)
  await memberRow.getByLabel(`Role for ${invitedEmail}`).selectOption('admin')
  await expect(memberRow.getByLabel(`Role for ${invitedEmail}`)).toHaveValue('admin')
  await expect(page.getByText('audit trail and portable workspace export', { exact: false })).toBeVisible()
  await memberPage.reload()
  await expect(memberPage.getByRole('link', { name: 'Quote Templates', exact: true })).toBeVisible()

  await page.reload()
  await expect(page.getByText(invitedEmail, { exact: true })).toBeVisible()
  await memberRow.getByRole('button', { name: 'Deactivate', exact: true }).click()
  await memberRow.getByRole('button', { name: 'Confirm deactivation' }).click()
  await expect(memberRow.getByText('Disabled', { exact: true })).toBeVisible()
  const invalidatedSession = await memberContext.request.get(`${apiURL}/auth/me`)
  expect(invalidatedSession.status()).toBe(401)
  await memberPage.goto('/dashboard')
  await expect(memberPage).toHaveURL(/\/login$/)

  await memberRow.getByRole('button', { name: 'Reactivate', exact: true }).click()
  await expect(memberRow.getByText('Active', { exact: true })).toBeVisible()
  await memberPage.getByLabel('Email').fill(invitedEmail)
  await memberPage.getByLabel('Password').fill(invitedPassword)
  await memberPage.getByRole('button', { name: 'Sign in' }).click()
  await expect(memberPage).toHaveURL(/\/dashboard$/)

  await memberPage.getByRole('link', { name: 'My Email', exact: true }).click()
  await expect(memberPage.getByRole('heading', { name: 'My email connection' })).toBeVisible()
  await memberPage.getByLabel('From email').fill(invitedEmail)
  await memberPage.getByLabel('From name').fill('Jamie Pilot')
  await memberPage.getByLabel('SMTP host').fill(smtpHost)
  await memberPage.getByLabel('SMTP port').fill(smtpPort)
  await memberPage.getByLabel('SMTP username').fill(invitedEmail)
  await memberPage.getByLabel('SMTP password').fill(`smtp-sandbox-jamie-${runID}`)
  await memberPage.getByRole('checkbox', { name: 'Use TLS / STARTTLS' }).uncheck()
  await memberPage.getByRole('button', { name: 'Save connection' }).click()
  await expect(memberPage.getByText('Email account saved. Emails you send to contacts will come from your address.')).toBeVisible()

  seedUserCatalogContinuation(owner.email, runID)
  await page.reload()
  const retainedMemberEmail = `browser-team-${runID}-49@example.test`
  await expect(page.getByText('Showing 50 of 51 team members', { exact: false })).toBeVisible()
  await expect(page.getByText(retainedMemberEmail, { exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Next page' }).click()
  await expect(page.getByText(retainedMemberEmail, { exact: true })).toBeVisible()
  await expect(page.getByText('Showing 1 of 51 team members', { exact: false })).toBeVisible()
  await page.getByLabel('Access status').selectOption('disabled')
  await expect(page.getByText('Showing 1 of 1 team members', { exact: false })).toBeVisible()
  await page.getByLabel('Search team access').fill('Retained %_')
  await page.getByRole('button', { name: 'Search team' }).click()
  await expect(page.getByText('Showing 1 of 1 team members matching “Retained %_”', { exact: false })).toBeVisible()
  const userCatalogAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-user-catalog-continuation', {
    body: JSON.stringify({ url: page.url(), violations: userCatalogAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(userCatalogAccessibility.violations).toEqual([])

  await page.getByRole('link', { name: 'Custom Fields', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Custom fields' })).toBeVisible()
  const customFieldForm = page.locator('form').filter({ has: page.getByRole('button', { name: 'Create field' }) })
  await customFieldForm.getByRole('textbox', { name: 'Label', exact: true }).fill('Relationship segment')
  await customFieldForm.getByRole('combobox', { name: 'Type', exact: true }).selectOption('select')
  await customFieldForm.getByRole('textbox', { name: 'Options', exact: false }).fill('Customer, Partner')
  await customFieldForm.getByLabel('Required when a record is created or edited').check()
  await customFieldForm.getByLabel('Show in record lists').check()
  await customFieldForm.getByRole('button', { name: 'Create field' }).click()
  await expect(page.getByText('created with stable key custom:relationship_segment', { exact: false })).toBeVisible()
  const relationshipFieldRow = page.locator('article').filter({ hasText: 'custom:relationship_segment' })
  await expect(relationshipFieldRow.getByText('revision 1', { exact: false })).toBeVisible()
  await relationshipFieldRow.getByLabel('Position for Relationship segment').fill('1')
  await relationshipFieldRow.getByRole('button', { name: 'Save changes' }).click()
  await expect(page.getByText('Relationship segment updated.', { exact: true })).toBeVisible()
  await expect(relationshipFieldRow.getByText('revision 2', { exact: false })).toBeVisible()

  await customFieldForm.getByRole('combobox', { name: 'Record type', exact: true }).selectOption('company')
  await customFieldForm.getByRole('textbox', { name: 'Label', exact: true }).fill('Service tier')
  await customFieldForm.getByRole('combobox', { name: 'Type', exact: true }).selectOption('select')
  await customFieldForm.getByRole('textbox', { name: 'Options', exact: false }).fill('Gold, Silver')
  await customFieldForm.getByLabel('Required when a record is created or edited').check()
  await customFieldForm.getByLabel('Show in record lists').check()
  await customFieldForm.getByRole('button', { name: 'Create field' }).click()
  await expect(page.getByText('created with stable key custom:service_tier', { exact: false })).toBeVisible()
  await expect(page.getByText('1 of 25 active fields used for this record type.', { exact: true })).toBeVisible()
  await customFieldForm.getByRole('textbox', { name: 'Label', exact: true }).fill('Temporary migration note')
  await customFieldForm.getByRole('combobox', { name: 'Type', exact: true }).selectOption('text')
  await customFieldForm.getByRole('button', { name: 'Create field' }).click()
  const temporaryFieldRow = page.locator('article').filter({ hasText: 'custom:temporary_migration_note' })
  await expect(temporaryFieldRow.getByText('revision 1', { exact: false })).toBeVisible()
  page.once('dialog', (dialog) => dialog.accept())
  await temporaryFieldRow.getByRole('button', { name: 'Archive field' }).click()
  await expect(page.getByText('Temporary migration note archived. Existing record values were retained.', { exact: true })).toBeVisible()
  await expect(page.getByText('1 of 25 active fields used for this record type.', { exact: true })).toBeVisible()

  const recordEmailTemplateName = `Relationship follow-up ${runID}`
  const recordEmailTemplateSubject = `Pilot relationship {{first_name}} ${runID}`
  const recordEmailTemplateBody = 'Hello {{first_name}}, your relationship segment is {{contact.custom.relationship_segment}}.'
  seedEmailDefinitionContinuation(owner.email, runID)
  await page.getByRole('link', { name: 'Email Templates', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Email templates', exact: true })).toBeVisible()
  await expect(page.getByText('{{contact.custom.relationship_segment}}', { exact: true })).toBeVisible()
  const seededTemplateName = `Browser email template ${runID} #051`
  const seededSnippetName = `Browser email snippet ${runID} #051`
  await expect(page.getByRole('heading', { name: `Browser email template ${runID} #001`, exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: seededTemplateName, exact: true })).toHaveCount(0)
  await expect(page.getByText('Showing 50 of 51 email templates', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: 'Next template page' }).click()
  await expect(page.getByRole('heading', { name: seededTemplateName, exact: true })).toBeVisible()
  await page.getByLabel('Search email templates').fill(seededTemplateName)
  await page.getByRole('button', { name: 'Apply template search' }).click()
  await expect(page.getByText('Showing 1 of 1 email templates', { exact: false })).toBeVisible()
  await expect(page.getByRole('heading', { name: `Browser email snippet ${runID} #001`, exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: seededSnippetName, exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Next snippet page' }).click()
  await expect(page.getByRole('heading', { name: seededSnippetName, exact: true })).toBeVisible()
  await page.getByLabel('Search email snippets').fill(seededSnippetName)
  await page.getByRole('button', { name: 'Apply snippet search' }).click()
  await expect(page.getByText('Showing 1 of 1 email snippets', { exact: false })).toBeVisible()
  const emailDefinitionAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-email-definition-continuation', {
    body: JSON.stringify({ url: page.url(), violations: emailDefinitionAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(emailDefinitionAccessibility.violations).toEqual([])
  const emailTemplateForm = page.locator('form').filter({ has: page.getByRole('button', { name: 'Create template' }) })
  await emailTemplateForm.getByLabel('Name').fill(recordEmailTemplateName)
  await emailTemplateForm.getByLabel('Subject').fill(recordEmailTemplateSubject)
  await emailTemplateForm.getByLabel('Body').fill(recordEmailTemplateBody)
  await emailTemplateForm.getByRole('button', { name: 'Create template' }).click()
  await expect(page.getByText('Email template created.', { exact: true })).toBeVisible()
  await expect(page.getByRole('list', { name: 'Email templates' }).getByText(recordEmailTemplateName, { exact: true })).toBeVisible()

  const publicLeadEmail = `website-lead-${runID}@example.test`
  const publicLeadConsent = `I agree that ${owner.organizationName} may contact me about this request.`
  await page.getByRole('link', { name: 'Lead Forms', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Lead Forms' })).toBeVisible()
  const leadFormBuilder = page.locator('form').filter({ has: page.getByRole('button', { name: 'Create lead form' }) })
  await leadFormBuilder.getByLabel('Name', { exact: true }).fill(`Pilot website form ${runID}`)
  await leadFormBuilder.getByLabel(/^Slug/).fill(`pilot-website-${runID}`)
  await leadFormBuilder.getByLabel('Title', { exact: true }).fill('Talk to the pilot team')
  await leadFormBuilder.getByLabel('Description', { exact: true }).fill('Tell us what outcome you need and an owner will follow up.')
  await leadFormBuilder.getByLabel('Consent statement').fill(publicLeadConsent)
  await leadFormBuilder.getByRole('button', { name: 'Create lead form' }).click()
  await expect(page.getByText('Lead form created.', { exact: true })).toBeVisible()
  await expect(page.getByRole('listitem').filter({ hasText: `Pilot website form ${runID}` })).toBeVisible()

	seedLeadFormContinuation(owner.email, runID)
	await page.reload()
	const oldestSeededLeadForm = `Browser lead form ${runID} #051`
	await expect(page.getByRole('heading', { name: `Browser lead form ${runID} #001`, exact: true })).toBeVisible()
	await expect(page.getByRole('heading', { name: oldestSeededLeadForm, exact: true })).toHaveCount(0)
	await expect(page.getByText('Showing 50 of 52 lead forms.', { exact: true })).toBeVisible()
	await page.getByRole('button', { name: 'Next form page' }).click()
	await expect(page.getByRole('heading', { name: oldestSeededLeadForm, exact: true })).toBeVisible()
	await expect(page.getByText('Showing 2 of 52 lead forms.', { exact: true })).toBeVisible()
	const leadFormCatalogAccessibility = await new AxeBuilder({ page })
		.withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
		.analyze()
	await test.info().attach('axe-lead-form-catalog-continuation', {
		body: JSON.stringify({ url: page.url(), violations: leadFormCatalogAccessibility.violations }, null, 2),
		contentType: 'application/json'
	})
	expect(leadFormCatalogAccessibility.violations).toEqual([])

  seedLeadReviewContinuation(owner.email, runID)

  const leadFollowUpRuleName = `Inbound lead follow-up ${runID}`
  const leadFollowUpTaskTitle = `Review inbound lead ${runID}`
  await page.getByRole('link', { name: 'Automations', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Workflow automation rules' })).toBeVisible()
  await page.getByLabel('Rule name').fill(leadFollowUpRuleName)
  await page.getByLabel('When').selectOption('lead_form_submitted')
  await page.getByRole('combobox', { name: /^Lead form/ }).selectOption({ label: `Pilot website form ${runID}` })
  await page.getByLabel('Optional attribution condition').selectOption('utmSource')
  await page.getByLabel('Condition value').fill('pilot-browser')
  await page.getByLabel('Assign task to').selectOption({ label: 'Pilot Owner' })
  await page.getByLabel('Task title').fill(leadFollowUpTaskTitle)
  await page.getByLabel('Task description').fill('Review the retained public request and make first contact.')
  await page.getByLabel('Create task after days').fill('0')
  await page.getByLabel('Due in days', { exact: false }).fill('1')
  await page.getByRole('button', { name: 'Create workflow rule' }).click()
  await expect(page.getByRole('heading', { name: leadFollowUpRuleName })).toBeVisible()
	await expect(page.getByText('1 of 50 active workflow actions allocated. Each task, approval gate, teammate notification, owner assignment, and sequence enrollment uses one slot.')).toBeVisible()

	seedWorkflowDefinitionContinuation(owner.email, runID)
	await page.reload()
	const oldestSeededWorkflow = `Browser workflow ${runID} #001`
	await expect(page.getByText('Showing 50 of 52 stored definitions.')).toBeVisible()
	await expect(page.getByRole('heading', { name: oldestSeededWorkflow })).toHaveCount(0)
	await page.getByRole('button', { name: 'Load more stored definitions' }).click()
	await expect(page.getByRole('heading', { name: oldestSeededWorkflow })).toBeVisible()
	await expect(page.getByText('Showing 52 of 52 stored definitions.')).toBeVisible()
	await expect(page.getByRole('button', { name: 'Load more stored definitions' })).toHaveCount(0)

  await page.getByRole('link', { name: 'Landing Pages', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Landing Pages' })).toBeVisible()
  const landingPageBuilder = page.locator('form').filter({ has: page.getByRole('button', { name: 'Create landing page' }) })
  await landingPageBuilder.getByLabel('Lead form').selectOption({ label: `Pilot website form ${runID}` })
  await landingPageBuilder.getByLabel('Name', { exact: true }).fill(`Pilot request page ${runID}`)
  await landingPageBuilder.getByLabel(/^Slug/).fill(`pilot-request-${runID}`)
  await landingPageBuilder.getByLabel('Title', { exact: true }).fill(`Plan a pilot ${runID}`)
  await landingPageBuilder.getByLabel('Subtitle', { exact: true }).fill('A browser-tested public lead path.')
  await landingPageBuilder.getByLabel('Body', { exact: true }).fill('Share your goals and the revenue team will respond.')
  await landingPageBuilder.getByLabel('CTA label').fill('Request follow-up')
  await landingPageBuilder.getByRole('button', { name: 'Create landing page' }).click()
  await expect(page.getByText('Landing page created.', { exact: true })).toBeVisible()
  const landingPageRow = page.getByRole('listitem').filter({ hasText: `Pilot request page ${runID}` })
  const publicLeadURL = await landingPageRow.getByRole('link').getAttribute('href')
  expect(publicLeadURL).toBeTruthy()

  await page.getByRole('link', { name: 'Website Widgets', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Website Widgets' })).toBeVisible()
  const widgetBuilder = page.locator('form').filter({ has: page.getByRole('button', { name: 'Create website widget' }) })
  await widgetBuilder.getByLabel('Lead form').selectOption({ label: `Pilot website form ${runID}` })
  await widgetBuilder.getByLabel('Name', { exact: true }).fill(`Pilot website widget ${runID}`)
  await widgetBuilder.getByLabel('Title', { exact: true }).fill(`Ask the pilot team ${runID}`)
  await widgetBuilder.getByLabel('Welcome message').fill('Tell us what outcome you need and we will follow up.')
  await widgetBuilder.getByLabel('Prompt label').fill('Talk to the pilot team')
  await widgetBuilder.getByLabel('CTA label').fill('Send request')
  await widgetBuilder.getByLabel('Position').selectOption('inline')
  await widgetBuilder.getByRole('button', { name: 'Create website widget' }).click()
  await expect(page.getByText('Website widget created.', { exact: true })).toBeVisible()
  const widgetRow = page.getByRole('listitem').filter({ hasText: `Pilot website widget ${runID}` })
  const publicWidgetURL = await widgetRow.getByRole('link').getAttribute('href')
  expect(publicWidgetURL).toBeTruthy()

  seedLeadSurfaceContinuation(owner.email, runID)
  await page.reload()
  const oldestSeededWidget = `Browser website widget ${runID} #051`
  await expect(page.getByRole('heading', { name: `Browser website widget ${runID} #001`, exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: oldestSeededWidget, exact: true })).toHaveCount(0)
  await expect(page.getByText('Showing 50 of 52 website widgets.', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Next widget page' }).click()
  await expect(page.getByRole('heading', { name: oldestSeededWidget, exact: true })).toBeVisible()
  await expect(page.getByText('Showing 2 of 52 website widgets.', { exact: true })).toBeVisible()
  const widgetCatalogAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-website-widget-catalog-continuation', {
    body: JSON.stringify({ url: page.url(), violations: widgetCatalogAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(widgetCatalogAccessibility.violations).toEqual([])

  const publicWidgetContext = await browser.newContext()
  const publicWidgetPage = await publicWidgetContext.newPage()
  await publicWidgetPage.goto(publicWidgetURL)
  await expect(publicWidgetPage.getByRole('heading', { name: `Ask the pilot team ${runID}` })).toBeVisible()
  await expect(publicWidgetPage.getByRole('button', { name: 'Send request' })).toBeEnabled()
  const publicWidgetAccessibility = await new AxeBuilder({ page: publicWidgetPage })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-public-website-widget', {
    body: JSON.stringify({ url: publicWidgetPage.url(), violations: publicWidgetAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(publicWidgetAccessibility.violations).toEqual([])
  await publicWidgetContext.close()

  await page.getByRole('link', { name: 'Landing Pages', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Landing Pages' })).toBeVisible()
  const oldestSeededLandingPage = `Browser landing page ${runID} #051`
  await expect(page.getByRole('heading', { name: `Browser landing page ${runID} #001`, exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: oldestSeededLandingPage, exact: true })).toHaveCount(0)
  await expect(page.getByText('Showing 50 of 52 landing pages.', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Next landing page' }).click()
  await expect(page.getByRole('heading', { name: oldestSeededLandingPage, exact: true })).toBeVisible()
  await expect(page.getByText('Showing 2 of 52 landing pages.', { exact: true })).toBeVisible()
  const landingPageCatalogAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-landing-page-catalog-continuation', {
    body: JSON.stringify({ url: page.url(), violations: landingPageCatalogAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(landingPageCatalogAccessibility.violations).toEqual([])

  const publicLeadContext = await browser.newContext()
  const publicLeadPage = await publicLeadContext.newPage()
  await publicLeadPage.goto(`${publicLeadURL}?utm_source=pilot-browser&utm_medium=e2e&utm_campaign=lead-capture`)
  await expect(publicLeadPage.getByRole('heading', { name: `Plan a pilot ${runID}` })).toBeVisible()
  await expect(publicLeadPage.getByRole('button', { name: 'Request follow-up' })).toBeEnabled()
  const publicLeadAccessibility = await new AxeBuilder({ page: publicLeadPage })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-public-lead-capture', {
    body: JSON.stringify({ url: publicLeadPage.url(), violations: publicLeadAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(publicLeadAccessibility.violations, 'public lead capture must have no automated WCAG A/AA violations').toEqual([])
  await publicLeadPage.getByLabel('First name').fill('Taylor')
  await publicLeadPage.getByLabel('Last name').fill(`Inbound ${runID}`)
  await publicLeadPage.getByLabel('Email').fill(publicLeadEmail)
  await publicLeadPage.getByLabel('How can we help?').fill(`Need a trustworthy CRM pilot ${runID}`)
	await publicLeadPage.getByLabel('Relationship segment').selectOption('Partner')
  await publicLeadPage.getByRole('checkbox', { name: publicLeadConsent, exact: true }).check()
  await publicLeadPage.getByRole('button', { name: 'Request follow-up' }).click()
  await expect(publicLeadPage.getByText('Thanks. We will be in touch soon.', { exact: true })).toBeVisible()
  await publicLeadContext.close()

  await page.getByRole('link', { name: 'Automations', exact: true }).click()
  const leadFollowUpRun = page.getByRole('list', { name: 'Workflow automation runs' }).getByRole('listitem').filter({ hasText: leadFollowUpRuleName })
  await expect(leadFollowUpRun).toContainText('1/1 actions completed', { timeout: 30000 })
  await expect(leadFollowUpRun.getByText('succeeded', { exact: true })).toBeVisible()
  await leadFollowUpRun.getByText('Inspect 1 action outcome').click()
  const leadActionOutcomes = leadFollowUpRun.getByRole('list', { name: `${leadFollowUpRuleName} run actions` })
  await expect(leadActionOutcomes).toContainText(`1. ${leadFollowUpTaskTitle}`)
  await expect(leadActionOutcomes).toContainText('Action succeeded · 1 attempt')
  await expect(leadActionOutcomes.getByRole('link', { name: 'Open created task' })).toHaveAttribute('href', /\/tasks\/\d+/)

	await page.getByRole('link', { name: 'Lead Forms', exact: true }).click()
	const capturedSubmission = page.getByRole('list', { name: 'Lead submissions awaiting review' }).getByRole('listitem').filter({ hasText: publicLeadEmail })
	await expect(capturedSubmission).toBeVisible()
	const oldestSeededReview = `browser-review-51-${runID}@example.test`
	await expect(page.getByText(oldestSeededReview, { exact: true })).toHaveCount(0)
	await page.getByRole('button', { name: 'Load older submissions' }).click()
	await expect(page.getByText(oldestSeededReview, { exact: true })).toBeVisible()
	await expect(page.getByRole('button', { name: 'Load older submissions' })).toHaveCount(0)
	const leadReviewAccessibility = await new AxeBuilder({ page })
		.withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
		.analyze()
	await test.info().attach('axe-lead-submission-review', {
		body: JSON.stringify({ url: page.url(), violations: leadReviewAccessibility.violations }, null, 2),
		contentType: 'application/json'
	})
	expect(leadReviewAccessibility.violations, 'lead submission review must have no automated WCAG A/AA violations').toEqual([])
	await capturedSubmission.getByLabel(/^Review note for Taylor Inbound/).fill('Browser-tested spam recovery')
	page.once('dialog', (dialog) => dialog.accept())
	await capturedSubmission.getByRole('button', { name: 'Mark spam' }).click()
	await expect(page.getByText(/Lead quarantined\. 0 queued follow-ups cancelled\. 1 completed follow-up remains as history\./)).toBeVisible()

	await page.getByRole('link', { name: 'Contacts', exact: true }).click()
	await page.getByLabel('Search contacts').fill(publicLeadEmail)
	await expect(page.getByRole('listitem').filter({ hasText: publicLeadEmail })).toHaveCount(0)

	await page.getByRole('link', { name: 'Lead Forms', exact: true }).click()
	await page.getByLabel('Review status').selectOption('spam')
	const quarantinedSubmission = page.getByRole('list', { name: 'Lead submissions awaiting review' }).getByRole('listitem').filter({ hasText: publicLeadEmail })
	await expect(quarantinedSubmission).toBeVisible()
	await quarantinedSubmission.getByRole('button', { name: 'Recover as legitimate' }).click()
	await expect(page.getByText(/Lead restored as legitimate\. 0 follow-ups rescheduled\./)).toBeVisible()

  const externalLeadEmail = `external-embed-${runID}@example.test`
  const pilotFormRow = page.getByRole('list', { name: 'Lead forms' }).getByRole('listitem').filter({ hasText: `Pilot website form ${runID}` })
  const externalEmbedCode = await pilotFormRow.getByLabel(`Embed code for Pilot website form ${runID}`).inputValue()
  expect(externalEmbedCode).toContain("credentials: 'omit'")
  const externalLeadContext = await browser.newContext()
  const externalLeadPage = await externalLeadContext.newPage()
  const externalLeadRequestEvidence = []
  externalLeadPage.on('request', (request) => {
    if (!request.url().startsWith(`${apiURL}/api/public/lead-capture-forms/`)) return
    externalLeadRequestEvidence.push(request.allHeaders().then((headers) => ({ url: request.url(), origin: headers.origin || '', cookie: headers.cookie || '' })))
  })
  const externalLeadServer = createServer((_request, response) => {
    response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
    response.end(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Customer contact page</title></head><body><main><h1>Contact the pilot team</h1>${externalEmbedCode}</main></body></html>`)
  })
  await new Promise((resolve, reject) => {
    externalLeadServer.once('error', reject)
    externalLeadServer.listen(0, '127.0.0.1', resolve)
  })
  const externalLeadAddress = externalLeadServer.address()
  expect(externalLeadAddress).toEqual(expect.objectContaining({ port: expect.any(Number) }))
  const externalLeadOrigin = `http://127.0.0.1:${externalLeadAddress.port}`
  const externalSourceURL = `${externalLeadOrigin}/contact?utm_source=external-browser&utm_medium=e2e&utm_campaign=embedded-form`
  await externalLeadPage.goto(externalSourceURL)
  await expect(externalLeadPage.getByRole('button', { name: 'Submit' })).toBeEnabled()
  const externalLeadAccessibility = await new AxeBuilder({ page: externalLeadPage })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-external-lead-form-embed', {
    body: JSON.stringify({ url: externalLeadPage.url(), violations: externalLeadAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(externalLeadAccessibility.violations, 'external lead form embed must have no automated WCAG A/AA violations').toEqual([])
  await externalLeadPage.getByLabel('First name').fill('Morgan')
  await externalLeadPage.getByLabel('Last name').fill(`External ${runID}`)
  await externalLeadPage.getByLabel('Email').fill(externalLeadEmail)
  await externalLeadPage.getByLabel('How can we help?').fill(`Embedded website request ${runID}`)
  await externalLeadPage.getByLabel('Relationship segment').selectOption('Partner')
  await externalLeadPage.getByRole('checkbox', { name: publicLeadConsent, exact: true }).check()
  await externalLeadPage.getByRole('button', { name: 'Submit' }).click()
  await expect(externalLeadPage.getByText('Thanks. We will be in touch soon.', { exact: true })).toBeVisible()
  await expect(externalLeadPage).toHaveURL(externalSourceURL)
  const externalLeadRequests = await Promise.all(externalLeadRequestEvidence)
  const externalSubmissionRequest = externalLeadRequests.find((request) => request.url.endsWith('/submissions'))
  expect(externalSubmissionRequest).toEqual(expect.objectContaining({ origin: externalLeadOrigin, cookie: '' }))
  await externalLeadContext.close()
  await new Promise((resolve, reject) => externalLeadServer.close((error) => error ? reject(error) : resolve()))

  await page.getByLabel('Review status').selectOption('unreviewed')
  const externalSubmission = page.getByRole('list', { name: 'Lead submissions awaiting review' }).getByRole('listitem').filter({ hasText: externalLeadEmail })
  await expect(externalSubmission).toBeVisible()
  await expect(externalSubmission).toContainText('Source: Lead capture form · external-browser · embedded-form')
  const externalContactHref = await externalSubmission.getByRole('link', { name: 'Open contact' }).getAttribute('href')
  expect(externalContactHref).toMatch(/^\/contacts\/\d+$/)
  await externalSubmission.getByRole('button', { name: 'Mark legitimate' }).click()
  await expect(page.getByText(/Lead restored as legitimate\. 0 follow-ups rescheduled\./)).toBeVisible()
  await page.goto(externalContactHref)
  const externalLeadAttribution = page.getByRole('list', { name: 'Lead attribution' })
  await expect(externalLeadAttribution).toContainText('Lead capture form')
  await expect(externalLeadAttribution).toContainText('embedded-form')
  await expect(externalLeadAttribution).toContainText('external-browser / e2e')
  await expect(externalLeadAttribution).toContainText(externalSourceURL)

  await page.getByRole('link', { name: 'Contacts', exact: true }).click()
  seedSavedViewContinuation(owner.email, runID)
  const savedViewsPanel = page.getByRole('region', { name: 'Saved view management' })
  await savedViewsPanel.getByRole('button', { name: 'Load views', exact: true }).click()
  const retainedSavedViewName = `Browser saved view ${runID} #051`
  await expect(savedViewsPanel.getByRole('option', { name: retainedSavedViewName, exact: true })).toBeAttached()
  await expect(savedViewsPanel.getByText('51 of 100 saved views used for this record type.', { exact: true })).toBeVisible()
  await savedViewsPanel.getByLabel('Saved views').selectOption({ label: retainedSavedViewName })
  await savedViewsPanel.getByRole('button', { name: 'Apply', exact: true }).click()
  await expect(savedViewsPanel.getByText(`Applied ${retainedSavedViewName}.`, { exact: true })).toBeVisible()
  const managedSavedViewName = `Pilot contact view ${runID}`
  await savedViewsPanel.getByLabel('Save current filters as').fill(managedSavedViewName)
  await savedViewsPanel.getByRole('button', { name: 'Save view', exact: true }).click()
  await expect(savedViewsPanel.getByText(`Saved ${managedSavedViewName}.`, { exact: true })).toBeVisible()
  await savedViewsPanel.getByRole('button', { name: 'Make default', exact: true }).click()
  await expect(savedViewsPanel.getByText(`${managedSavedViewName} is now the default.`, { exact: true })).toBeVisible()
  await savedViewsPanel.getByRole('button', { name: 'Update', exact: true }).click()
  await expect(savedViewsPanel.getByText(`Updated ${managedSavedViewName}.`, { exact: true })).toBeVisible()
  await savedViewsPanel.getByRole('button', { name: 'Delete', exact: true }).click()
  await expect(savedViewsPanel.getByText(`Deleted ${managedSavedViewName}.`, { exact: true })).toBeVisible()
  const savedViewAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-saved-view-continuation', {
    body: JSON.stringify({ url: page.url(), violations: savedViewAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(savedViewAccessibility.violations).toEqual([])
  await page.getByLabel('Search contacts').fill(publicLeadEmail)
  const capturedLeadRow = page.getByRole('listitem').filter({ hasText: publicLeadEmail })
  await expect(capturedLeadRow).toBeVisible()
  await expect(capturedLeadRow.getByText('Unassigned', { exact: true })).toBeVisible()
	await expect(capturedLeadRow).toContainText('Relationship segment: Partner')
  await capturedLeadRow.getByRole('checkbox', { name: `Select Taylor Inbound ${runID}` }).check()
  await page.getByLabel('New owner').selectOption({ label: 'Pilot Owner' })
  await page.getByRole('button', { name: 'Apply to 1 selected' }).click()
  await expect(page.getByText('Assign to Pilot Owner: 1 of 1 changed.', { exact: true })).toBeVisible()
  await expect(capturedLeadRow.getByText('Pilot Owner', { exact: true })).toBeVisible()

  await page.getByRole('link', { name: 'Data Imports', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Import CRM data' })).toBeVisible()
  const importedClientName = `Imported Client ${runID}`
  await page.getByLabel('CSV file').setInputFiles({
    name: 'pilot-contacts.csv',
    mimeType: 'text/csv',
    buffer: Buffer.from(`First Name,Last Name,Email Address,Status,Client,Relationship Segment\nImported,Client ${runID},imported-${runID}@example.test,customer,true,Customer\n`)
  })
  await page.getByRole('button', { name: 'Preview and map' }).click()
  await expect(page.getByText('1 valid', { exact: false })).toBeVisible()
  await expect(page.getByLabel('First name column')).toHaveValue('First Name')
  await expect(page.getByLabel('Relationship segment (custom) column')).toHaveValue('Relationship Segment')
  await page.getByRole('button', { name: 'Import valid rows' }).click()
  await expect(page.getByText(/Import queued: 0 \/ 1 processed/)).toBeVisible()
  const importRow = page.getByRole('listitem').filter({ hasText: 'pilot-contacts.csv · completed' })
  await expect(importRow).toBeVisible()
  page.once('dialog', (dialog) => dialog.accept())
  await importRow.getByRole('button', { name: 'Roll back import' }).click()
  await expect(page.getByText('Rollback finished: 1 archived, 0 changed records left active.')).toBeVisible()
  await expect(page.getByRole('listitem').filter({ hasText: 'pilot-contacts.csv · rolled back' })).toBeVisible()

  await page.getByRole('link', { name: 'Clients', exact: true }).click()
  await expect(page.getByText(importedClientName, { exact: true })).toHaveCount(0)

  await page.getByRole('link', { name: 'Clients', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Clients' })).toBeVisible()
  await page.getByRole('button', { name: 'Add client' }).click()
  await page.getByLabel('Client name').fill(`Northstar Advisory ${runID}`)
  await page.getByLabel('Industry').fill('Consulting')
  await page.getByLabel('Website').fill(`https://northstar-${runID}.example.test`)
  await page.getByLabel('Service tier (required)', { exact: false }).selectOption('Gold')
  await page.getByRole('button', { name: 'Save client' }).click()
  await expect(page).toHaveURL(/\/companies\/\d+$/)
  await expect(page.getByRole('heading', { name: `Northstar Advisory ${runID}` })).toBeVisible()

  await page.getByRole('button', { name: 'Add person' }).click()
  const personForm = page.locator('form').filter({ has: page.getByRole('button', { name: 'Save person' }) })
  await personForm.getByLabel('First name').fill('Avery')
  await personForm.getByLabel('Last name').fill('Buyer')
  await personForm.getByLabel('Email').fill(`avery-${runID}@example.test`)
  await personForm.getByLabel('Job title').fill('Operations Director')
  await personForm.getByLabel('Relationship segment (required)', { exact: false }).selectOption('Customer')
  await personForm.getByRole('button', { name: 'Save person' }).click()
  const linkedPeople = page.getByRole('list', { name: 'Linked contacts list' })
  await expect(linkedPeople.getByText('Avery Buyer')).toBeVisible()

  await page.getByRole('button', { name: 'Link existing contact' }).click()
  const linkContactForm = page.locator('form').filter({ has: page.getByRole('button', { name: 'Link contact' }) })
  await expect(linkContactForm.getByRole('textbox', { name: 'Search workspace contacts', exact: true })).toHaveCount(1)
  await expect(linkContactForm.getByRole('combobox', { name: 'Existing contact', exact: true })).toHaveCount(1)
  await linkContactForm.getByRole('textbox', { name: 'Search workspace contacts', exact: true }).fill(publicLeadEmail)
  await linkContactForm.getByRole('button', { name: 'Search', exact: true }).click()
  await linkContactForm.getByRole('combobox', { name: 'Existing contact', exact: true }).selectOption({ label: `Taylor Inbound ${runID}` })
  await linkContactForm.getByLabel('Relationship title').fill('Inbound requester')
  await linkContactForm.getByRole('button', { name: 'Link contact' }).click()
  const linkedLead = linkedPeople.getByRole('listitem').filter({ hasText: `Taylor Inbound ${runID}` })
  await expect(linkedLead).toContainText('Inbound requester')

  const linkedPeopleSearch = page.getByRole('search', { name: 'Search linked people' })
  await linkedPeopleSearch.getByLabel('Search linked people').fill(publicLeadEmail)
  await linkedPeopleSearch.getByRole('button', { name: 'Search', exact: true }).click()
  await expect(linkedPeople).toContainText(`Taylor Inbound ${runID}`)
  await expect(linkedPeople).not.toContainText('Avery Buyer')
  await linkedPeopleSearch.getByRole('button', { name: 'Clear' }).click()
  await expect(linkedPeople).toContainText('Avery Buyer')
  const linkedPeopleAccessibility = await new AxeBuilder({ page })
    .include('.company-people-card')
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-company-linked-people', {
    body: JSON.stringify({ url: page.url(), violations: linkedPeopleAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(linkedPeopleAccessibility.violations, 'company linked-person management must have no automated WCAG A/AA violations').toEqual([])

  const refreshedLinkedLead = linkedPeople.getByRole('listitem').filter({ hasText: `Taylor Inbound ${runID}` })
  await refreshedLinkedLead.getByRole('button', { name: 'Make primary' }).click()
  await expect(linkedPeople.getByRole('listitem').filter({ hasText: `Taylor Inbound ${runID}` })).toContainText('Primary')
  page.once('dialog', (dialog) => dialog.accept())
  await linkedPeople.getByRole('listitem').filter({ hasText: `Taylor Inbound ${runID}` }).getByRole('button', { name: 'Unlink' }).click()
  await expect(linkedPeople).not.toContainText(`Taylor Inbound ${runID}`)
  await expect(linkedPeople.getByRole('listitem').filter({ hasText: 'Avery Buyer' })).toContainText('Primary')

  await page.getByRole('button', { name: 'Avery Buyer', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Evaluate score', exact: true })).toHaveCount(0)
  const contactFollowUp = page.locator('.touchpoint-summary-card')
  await expect(contactFollowUp.getByText(/No qualifying touch yet/)).toBeVisible()
  const directEmailSubject = `Pilot introduction ${runID}`
  let directEmailRequestCount = 0
  let directEmailIdempotencyKey = ''
  page.on('request', (request) => {
    if (request.method() !== 'POST' || !/\/api\/contacts\/\d+\/email$/.test(new URL(request.url()).pathname)) return
    directEmailRequestCount += 1
    directEmailIdempotencyKey = request.headers()['idempotency-key'] || ''
  })
  await page.getByRole('button', { name: 'Send email', exact: true }).click()
  await page.getByLabel('Subject').fill('Unknown {{not_real}}')
  await page.getByLabel('Body').fill('This preview must fail visibly.')
  await page.getByRole('button', { name: 'Preview merged email', exact: true }).click()
  await expect(page.getByText(/Unknown merge fields: \{\{not_real\}\}/)).toBeVisible()
  await expect(page.getByRole('button', { name: 'Send test to me', exact: true })).toBeDisabled()

  await page.getByLabel('Template').selectOption({ label: recordEmailTemplateName })
  await page.getByRole('button', { name: 'Preview merged email', exact: true }).click()
  const mergedEmailPreview = page.getByRole('region', { name: 'Merged email preview' })
  await expect(mergedEmailPreview).toContainText(`Customer recipient: avery-${runID}@example.test`)
  await expect(mergedEmailPreview).toContainText(`Subject: Pilot relationship Avery ${runID}`)
  await expect(mergedEmailPreview).toContainText('Hello Avery, your relationship segment is Customer.')
  await expect(mergedEmailPreview).toContainText('All merge fields resolved for this record.')
  const recordEmailPreviewAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-record-email-template-preview', {
    body: JSON.stringify({ url: page.url(), violations: recordEmailPreviewAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(recordEmailPreviewAccessibility.violations, 'record email template preview must have no automated WCAG A/AA violations').toEqual([])
  await page.getByRole('button', { name: 'Send test to me', exact: true }).click()
  await expect(page.getByText(`Test email sent only to ${owner.email}. The CRM recipient was not emailed.`, { exact: true })).toBeVisible()
  await expect.poll(async () => {
    const response = await page.request.get(`${smtpCaptureURL}/messages`)
    const payload = await response.json()
    return payload.messages.filter((message) => message.data.includes(`Subject: [TEST] Pilot relationship Avery ${runID}`)).length
  }).toBe(1)
  const templateTestMessagesResponse = await page.request.get(`${smtpCaptureURL}/messages`)
  const templateTestMessage = (await templateTestMessagesResponse.json()).messages.find((message) => message.data.includes(`Subject: [TEST] Pilot relationship Avery ${runID}`))
  expect(templateTestMessage.envelopeTo).toContain(owner.email)
  expect(templateTestMessage.envelopeTo).not.toContain(`avery-${runID}@example.test`)
  expect(templateTestMessage.data).toContain('CRM recipient did not receive it')
  expect(templateTestMessage.data).toContain('Hello Avery, your relationship segment is Customer.')

  const trackingConsent = page.getByRole('checkbox', { name: /track opens\/links/i })
  await expect(trackingConsent).not.toBeChecked()
  await trackingConsent.check()
  await expect(trackingConsent).toBeChecked()
  await page.getByLabel('Subject').fill(directEmailSubject)
  await page.getByLabel('Body').fill(`Hello Avery, this is the durable pilot follow-up ${runID}.`)
  await page.getByRole('button', { name: 'Send email', exact: true }).click()
  await expect(page.getByText(`Email sent to avery-${runID}@example.test.`, { exact: true })).toBeVisible()
  await expect(page.getByRole('list', { name: 'contact email history' }).getByText(directEmailSubject, { exact: true })).toBeVisible()
  await expect(page.getByRole('list', { name: 'contact email history' }).getByText(`[TEST] Pilot relationship Avery ${runID}`, { exact: true })).toHaveCount(0)
  expect(directEmailRequestCount).toBe(1)
  expect(directEmailIdempotencyKey).toMatch(/^record-email-/)
  await expect.poll(async () => {
    const response = await page.request.get(`${smtpCaptureURL}/messages`)
    const payload = await response.json()
    return payload.messages.filter((message) => message.data.includes(`Subject: ${directEmailSubject}`)).length
  }).toBe(1)
  const recordEmailAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-record-email-delivery', {
    body: JSON.stringify({ url: page.url(), violations: recordEmailAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(recordEmailAccessibility.violations).toEqual([])
  await page.reload()
  await page.getByRole('button', { name: 'Send email', exact: true }).click()
  await expect(page.getByRole('list', { name: 'contact email history' }).getByText(directEmailSubject, { exact: true })).toBeVisible()
  expect(directEmailRequestCount).toBe(1)

  const uncertainEmailSubject = `Pilot uncertain delivery ${runID}`
  const armSMTPDisconnect = await page.request.post(`${smtpCaptureURL}/disconnect-after-accept-once`)
  expect(armSMTPDisconnect.status()).toBe(200)
  await page.getByLabel('Subject').fill(uncertainEmailSubject)
  await page.getByLabel('Body').fill(`This provider acceptance loses its acknowledgement ${runID}.`)
  await page.getByRole('button', { name: 'Send email', exact: true }).click()
  await expect(page.getByText(/provider outcome is uncertain/i).first()).toBeVisible()
  const unresolvedRecordEmail = page.getByRole('list', { name: 'Unresolved record email deliveries' }).getByRole('listitem').filter({ hasText: uncertainEmailSubject })
  await expect(unresolvedRecordEmail).toContainText(/Outcome uncertain/i)
  await expect(unresolvedRecordEmail.getByRole('button', { name: 'Confirm sent' })).toBeVisible()
  expect(directEmailRequestCount).toBe(2)
  await expect.poll(async () => {
    const response = await page.request.get(`${smtpCaptureURL}/messages`)
    const payload = await response.json()
    return payload.messages.filter((message) => message.data.includes(`Subject: ${uncertainEmailSubject}`)).length
  }).toBe(1)
  const recordEmailRecoveryAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-record-email-recovery', {
    body: JSON.stringify({ url: page.url(), violations: recordEmailRecoveryAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(recordEmailRecoveryAccessibility.violations).toEqual([])

  await page.reload()
  await page.getByRole('button', { name: 'Send email', exact: true }).click()
  const reloadedUncertainEmail = page.getByRole('list', { name: 'Unresolved record email deliveries' }).getByRole('listitem').filter({ hasText: uncertainEmailSubject })
  await expect(reloadedUncertainEmail).toContainText(/Outcome uncertain/i)
  page.once('dialog', (dialog) => dialog.accept())
  await reloadedUncertainEmail.getByRole('button', { name: 'Confirm sent' }).click()
  await expect(page.getByText(`Email to avery-${runID}@example.test confirmed sent.`, { exact: true })).toBeVisible()
  await expect(page.getByRole('list', { name: 'contact email history' }).getByText(uncertainEmailSubject, { exact: true })).toBeVisible()
  expect(directEmailRequestCount).toBe(2)
  const recoveredSMTPMessages = await page.request.get(`${smtpCaptureURL}/messages`)
  expect((await recoveredSMTPMessages.json()).messages.filter((message) => message.data.includes(`Subject: ${uncertainEmailSubject}`))).toHaveLength(1)
  await page.getByRole('button', { name: 'Close', exact: true }).click()

  const sequenceName = `Pilot cadence ${runID}`
  const sequenceSubject = `Pilot sequence follow-up ${runID}`
  seedEmailSequenceContinuation(owner.email, runID)
  await page.getByRole('link', { name: 'Email Sequences', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Email sequences', exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: `Retained browser sequence ${runID} #001`, exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: `Retained browser sequence ${runID} #051`, exact: true })).toHaveCount(0)
  await expect(page.getByText('Showing 50 of 51 email sequences', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: 'Next page' }).click()
  await expect(page.getByRole('heading', { name: `Retained browser sequence ${runID} #051`, exact: true })).toBeVisible()
  await page.getByLabel('Search email sequences').fill(`Retained browser sequence ${runID} #051`)
  await page.getByRole('button', { name: 'Apply search' }).click()
  await expect(page.getByText('Showing 1 of 1 email sequences', { exact: false })).toBeVisible()
  await page.getByLabel('Search email sequences').fill('')
  await page.getByRole('button', { name: 'Apply search' }).click()
  await page.getByLabel('Sequence name').fill(sequenceName)
  await page.getByLabel('Description').fill('Approved one-step pilot cadence through the durable worker')
  await page.getByLabel('Step 1 delay days').fill('0')
  await page.getByLabel('Step 1 subject').fill(sequenceSubject)
  await page.getByLabel('Step 1 body').fill(`Hello {{first_name}}, this is the approved pilot sequence ${runID}.`)
  await page.getByRole('button', { name: 'Create sequence', exact: true }).click()
  const sequenceRow = page.getByRole('list', { name: 'Email sequences' }).getByRole('listitem').filter({ hasText: sequenceName })
  await expect(sequenceRow).toContainText('draft · revision 1 · approval required · steps 1')
  await sequenceRow.getByRole('button', { name: 'Approve & activate', exact: true }).click()
  await expect(sequenceRow).toContainText('active · revision 1 · approved · steps 1')

  await page.getByRole('link', { name: 'Contacts', exact: true }).click()
  await page.getByRole('button', { name: 'Avery Buyer', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Avery Buyer', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Manage sequences', exact: true }).click()
  const enrollmentSequenceSelect = page.getByRole('combobox', { name: 'Sequence', exact: true })
  await expect(enrollmentSequenceSelect.locator('option:checked')).toHaveText(sequenceName)
  expect((await enrollmentSequenceSelect.locator('option').allTextContents()).join('\n')).not.toContain('Retained browser sequence')
  await page.getByRole('button', { name: 'Enroll contact', exact: true }).click()
  await expect(page.getByText(`Enrolled in ${sequenceName}.`, { exact: true })).toBeVisible()
  await expect.poll(async () => {
    const response = await page.request.get(`${smtpCaptureURL}/messages`)
    const payload = await response.json()
    return payload.messages.filter((message) => message.data.includes(`Subject: ${sequenceSubject}`)).length
  }).toBe(1)
  const sequenceMessagesResponse = await page.request.get(`${smtpCaptureURL}/messages`)
  const sequenceMessage = (await sequenceMessagesResponse.json()).messages.find((message) => message.data.includes(`Subject: ${sequenceSubject}`))
  expect(sequenceMessage.envelopeTo).toContain(`avery-${runID}@example.test`)
  expect(sequenceMessage.data).toContain('Message-ID: <')
  expect(sequenceMessage.data).toContain(`Hello Avery, this is the approved pilot sequence ${runID}.`)
  expect(sequenceMessage.data).toContain('/api/email-unsubscribe/')
  expect(sequenceMessage.data).toContain('multipart/alternative')

  await page.getByRole('link', { name: 'Email Sequences', exact: true }).click()
  const completedSequenceRow = page.getByRole('list', { name: 'Email sequences' }).getByRole('listitem').filter({ hasText: sequenceName })
  await expect(completedSequenceRow).toContainText('1 enrolled · 1 accepted · 0 bounced · 0 complaints · 0 replied · 1 finished · 0 suppressed · 0 review')
  await completedSequenceRow.getByRole('button', { name: 'View enrollments', exact: true }).click()
  const sequenceEnrollmentHistory = completedSequenceRow.getByRole('list', { name: `${sequenceName} enrollments`, exact: true })
  await expect(sequenceEnrollmentHistory.getByRole('link', { name: 'Avery Buyer', exact: true })).toBeVisible()
  await expect(sequenceEnrollmentHistory).toContainText('Finished · 1 accepted')
  await expect(sequenceEnrollmentHistory).toContainText(`avery-${runID}@example.test`)
  await expect(completedSequenceRow.getByRole('button', { name: 'Load older enrollments', exact: true })).toHaveCount(0)
  const emailSequenceAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-email-sequence-delivery', {
    body: JSON.stringify({ url: page.url(), violations: emailSequenceAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(emailSequenceAccessibility.violations).toEqual([])

  await page.getByRole('link', { name: 'Contacts', exact: true }).click()
  await page.getByRole('button', { name: 'Avery Buyer', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Avery Buyer', exact: true })).toBeVisible()
  const contactNoteForm = page.locator('form').filter({ has: page.getByRole('button', { name: 'Add note' }) })
  await contactNoteForm.getByLabel('New note').fill(`Pilot follow-up ${runID}`)
  await contactNoteForm.getByRole('button', { name: 'Add note' }).click()
  await expect(contactFollowUp.getByText(/Last touch:/)).toBeVisible()
  await expect(contactFollowUp.getByText('Note added', { exact: true }).first()).toBeVisible()
  await page.getByRole('link', { name: 'Clients', exact: true }).click()
  await page.getByRole('button', { name: `Northstar Advisory ${runID}`, exact: true }).click()
  await expect(page.getByRole('heading', { name: `Northstar Advisory ${runID}` })).toBeVisible()
  const companyFollowUp = page.locator('.touchpoint-summary-card')
  await expect(companyFollowUp.getByText(/Last touch:/)).toBeVisible()
  await expect(companyFollowUp.getByRole('link', { name: 'Avery Buyer' }).first()).toBeVisible()

  const accountHandoffNote = `Kickoff context ${runID}`
  const accountHandoffTask = `Confirm kickoff owner ${runID}`
  const clientNoteForm = page.locator('form').filter({ has: page.getByRole('button', { name: 'Add note' }) })
  await clientNoteForm.getByLabel('New note').fill(accountHandoffNote)
  await clientNoteForm.getByRole('button', { name: 'Add note' }).click()
  const clientTaskForm = page.locator('form').filter({ has: page.getByRole('button', { name: 'Save task' }) })
  await clientTaskForm.getByLabel('Task title').fill(accountHandoffTask)
  await clientTaskForm.getByLabel('Due at').fill('2020-01-01T09:00')
  await clientTaskForm.getByRole('button', { name: 'Save task' }).click()
  await expect(page.getByRole('list', { name: 'Client tasks list' }).getByText(accountHandoffTask)).toBeVisible()

  const duplicateEmail = `avery-duplicate-${runID}@example.test`
  await page.getByRole('button', { name: 'Add person' }).click()
  const duplicatePersonForm = page.locator('form').filter({ has: page.getByRole('button', { name: 'Save person' }) })
  await duplicatePersonForm.getByLabel('First name').fill('Avery')
  await duplicatePersonForm.getByLabel('Last name').fill('Buyer')
  await duplicatePersonForm.getByLabel('Email').fill(duplicateEmail)
  await duplicatePersonForm.getByLabel('Job title').fill('Regional Buyer')
  await duplicatePersonForm.getByLabel('Relationship segment (required)', { exact: false }).selectOption('Partner')
  await duplicatePersonForm.getByRole('button', { name: 'Save person' }).click()
  await expect(page.getByRole('list', { name: 'Linked contacts list' }).getByText(duplicateEmail, { exact: true })).toBeVisible()

  await page.getByRole('link', { name: 'Data Quality', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Duplicate review' })).toBeVisible()
  const duplicatePair = page.getByRole('listitem').filter({ hasText: duplicateEmail })
  await expect(duplicatePair).toBeAttached()
  await duplicatePair.getByRole('button', { name: `Keep Avery Buyer (avery-${runID}@example.test)` }).click()
  await page.getByLabel('Regional Buyer').check()
  await page.getByLabel('Partner').check()
  page.once('dialog', (dialog) => dialog.accept())
  await page.getByRole('button', { name: 'Merge and archive Avery Buyer' }).click()
  await expect(page.getByText('Merge complete.', { exact: false })).toBeVisible()
  await expect(page.getByText('No likely duplicate contacts found.')).toBeVisible()

  await page.getByRole('link', { name: 'Clients', exact: true }).click()
  const northstarRow = page.getByRole('listitem').filter({ hasText: `Northstar Advisory ${runID}` })
  await expect(northstarRow).toContainText('Service tier: Gold')

  await page.getByLabel(`Select Northstar Advisory ${runID}`).check()
  const clientBulkActions = page.getByLabel('Bulk actions for company records')
  await clientBulkActions.getByRole('combobox', { name: 'Bulk change', exact: true }).selectOption('set_status')
  await clientBulkActions.getByRole('combobox', { name: 'New status', exact: true }).selectOption('customer')
  await clientBulkActions.getByRole('button', { name: 'Apply to 1 selected' }).click()
  await expect(clientBulkActions.getByText('Set status to customer: 1 of 1 changed.')).toBeVisible()
  await clientBulkActions.getByText('Recent bulk changes').click()
  page.once('dialog', (dialog) => dialog.accept())
  const undoBulkChange = clientBulkActions.getByRole('button', { name: 'Undo', exact: true })
  await undoBulkChange.click()
  await expect(clientBulkActions.getByText('Undo complete: 1 restored.')).toBeVisible()

  const discoveryStage = `Discovery ${runID}`
  const reviewedDiscoveryStage = `Discovery reviewed ${runID}`
  await page.getByRole('link', { name: 'Pipelines', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Pipeline configuration' })).toBeVisible()
  await expect(page.getByText('Create up to 10 pipelines and manage up to 20 stable stages in each.', { exact: false })).toBeVisible()
  await page.getByLabel('New stage name for Sales pipeline').fill(discoveryStage)
  await page.getByLabel('New stage probability for Sales pipeline', { exact: false }).fill('65')
  await page.getByRole('button', { name: 'Add stage' }).click()
  await expect(page.getByRole('button', { name: `Save ${discoveryStage}` })).toBeVisible()

  const dealApprovalRuleName = `Approved new deal qualification ${runID}`
  const dealApprovalName = `Qualification review ${runID}`
  const automatedTaskTitle = `Qualify new deal ${runID}`
  const automatedDecisionTaskTitle = `Confirm decision date ${runID}`
  const dealNotificationRuleName = `Notify owners about new deals ${runID}`
  const notificationTaskTitle = `Prepare new-deal briefing ${runID}`
  const dealNotificationMessage = `New-deal briefing is ready for Website renewal ${runID}.`
  const dealAssignmentRuleName = `Restore Jamie as deal owner ${runID}`
  const dealSequenceRuleName = `Enroll new deal contacts in ${sequenceName}`
  await page.getByRole('link', { name: 'Automations', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Workflow automation rules' })).toBeVisible()
  await page.getByLabel('Rule name').fill(dealApprovalRuleName)
  await page.getByLabel(/Optional deal condition/i).selectOption('valueAmount')
  await page.getByLabel('Deal condition value').fill('20000')
  await page.getByLabel('Task title').fill(automatedTaskTitle)
  await page.getByLabel('Task description').fill('Confirm fit and agree the next step.')
  await page.getByLabel('Due in days', { exact: false }).fill('1')
  await page.getByRole('button', { name: 'Add another task' }).click()
  await page.getByLabel('Task 2 title').fill(automatedDecisionTaskTitle)
  await page.getByLabel('Task 2 description').fill('Set the next commercial decision checkpoint.')
  await page.getByLabel('Task 2 due in days').fill('3')
  await page.getByLabel(/Require a decision before creating any tasks/i).check()
  await page.getByLabel('Approval name').fill(dealApprovalName)
  await page.getByLabel('Who can approve').selectOption('owner')
  await page.getByLabel('Reviewer guidance').fill('Confirm this high-value opportunity is ready for the captured qualification tasks.')
  const dealPlaybookAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-deal-task-playbook', {
    body: JSON.stringify({ url: page.url(), violations: dealPlaybookAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(dealPlaybookAccessibility.violations, 'deal task playbook authoring must have no automated WCAG A/AA violations').toEqual([])
  await page.getByRole('button', { name: 'Create workflow rule' }).click()
  await expect(page.getByRole('heading', { name: dealApprovalRuleName })).toBeVisible()
  await expect(page.getByText('Only if value amount is greater than 20000', { exact: true })).toBeVisible()
  await expect(page.getByText(/2-task playbook/)).toBeVisible()
	await expect(page.getByText('4 of 50 active workflow actions allocated. Each task, approval gate, teammate notification, owner assignment, and sequence enrollment uses one slot.')).toBeVisible()

  await page.getByLabel('Rule name').fill(dealNotificationRuleName)
  await page.getByLabel('Task title').fill(notificationTaskTitle)
  await page.getByLabel('Task description').fill('Prepare the retained context before an owner reviews the new opportunity.')
  await page.getByLabel('Due in days', { exact: false }).fill('1')
  await page.getByLabel(/Notify eligible teammates after every task commits/i).check()
  await page.getByRole('combobox', { name: 'Notify', exact: true }).selectOption('owner')
  await page.getByLabel('Notification message').fill(dealNotificationMessage)
  const notificationAuthoringAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  expect(notificationAuthoringAccessibility.violations, 'workflow notification authoring must have no automated WCAG A/AA violations').toEqual([])
  await page.getByRole('button', { name: 'Create workflow rule' }).click()
  const notificationDefinition = page.getByRole('list', { name: 'Workflow automation rules' }).getByRole('listitem').filter({ hasText: dealNotificationRuleName })
  await expect(notificationDefinition).toContainText('1-task playbook · then notifies eligible teammates in the same transaction.')
  await expect(notificationDefinition).toContainText(`Notification: Workspace owners · “${dealNotificationMessage}”`)
	await expect(page.getByText('6 of 50 active workflow actions allocated. Each task, approval gate, teammate notification, owner assignment, and sequence enrollment uses one slot.')).toBeVisible()

  await page.getByLabel('Rule name').fill(dealAssignmentRuleName)
  await page.getByLabel('Outcome').selectOption('assign_owner')
  await page.getByLabel('When').selectOption('owner_changed')
  await page.getByLabel('Assign deal owner to').selectOption({ label: 'Jamie Pilot' })
  const ownerAssignmentAuthoringAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  expect(ownerAssignmentAuthoringAccessibility.violations, 'deal owner assignment authoring must have no automated WCAG A/AA violations').toEqual([])
  await page.getByRole('button', { name: 'Create workflow rule' }).click()
  const assignmentDefinition = page.getByRole('list', { name: 'Workflow automation rules' }).getByRole('listitem').filter({ hasText: dealAssignmentRuleName })
  await expect(assignmentDefinition).toContainText('After every direct deal owner change')
  await expect(assignmentDefinition).toContainText('One transactional owner assignment · nested owner-change rules are causally bounded.')
  await expect(assignmentDefinition).toContainText('Assign deal to Jamie Pilot.')
	await expect(page.getByText('7 of 50 active workflow actions allocated. Each task, approval gate, teammate notification, owner assignment, and sequence enrollment uses one slot.')).toBeVisible()

  await page.getByLabel('Rule name').fill(dealSequenceRuleName)
  await page.getByLabel('Outcome').selectOption('add_to_sequence')
  await page.getByRole('combobox', { name: /^Approved email sequence/ }).selectOption({ label: sequenceName })
  const sequenceWorkflowAuthoringAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  expect(sequenceWorkflowAuthoringAccessibility.violations, 'sequence workflow authoring must have no automated WCAG A/AA violations').toEqual([])
  await page.getByRole('button', { name: 'Create workflow rule' }).click()
  const sequenceWorkflowDefinition = page.getByRole('list', { name: 'Workflow automation rules' }).getByRole('listitem').filter({ hasText: dealSequenceRuleName })
  await expect(sequenceWorkflowDefinition).toContainText('When a deal is created')
  await expect(sequenceWorkflowDefinition).toContainText('One transactional primary-contact enrollment · delivery remains durable and recoverable.')
  await expect(sequenceWorkflowDefinition).toContainText(`Enroll the primary contact in ${sequenceName} using the current deal owner as sender.`)
	await expect(page.getByText('8 of 50 active workflow actions allocated. Each task, approval gate, teammate notification, owner assignment, and sequence enrollment uses one slot.')).toBeVisible()

  const quoteTemplateName = `Pilot services terms ${runID}`
  const quoteTemplateTerms = 'Net 30. Scope changes require written approval under the retained pilot services terms.'
  seedQuoteTemplateContinuation(owner.email, runID)
  await page.getByRole('link', { name: 'Quote Templates', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Quote preparation policy' })).toBeVisible()
  await expect(page.getByRole('heading', { name: `Browser quote terms ${runID} #001`, exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: `Browser quote terms ${runID} #051`, exact: true })).toHaveCount(0)
  await expect(page.getByText('Showing 50 of 51 quote templates', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: 'Next page' }).click()
  await expect(page.getByRole('heading', { name: `Browser quote terms ${runID} #051`, exact: true })).toBeVisible()
  await page.getByLabel('Search quote templates').fill(`Browser quote terms ${runID} #051`)
  await page.getByRole('button', { name: 'Apply search' }).click()
  await expect(page.getByText('Showing 1 of 1 quote templates', { exact: false })).toBeVisible()
  await page.getByLabel('Search quote templates').fill('')
  await page.getByRole('button', { name: 'Apply search' }).click()
  await expect(page.getByText('Showing 50 of 51 quote templates', { exact: false })).toBeVisible()
  const quoteTemplateForm = page.getByRole('form', { name: 'Create quote template' })
  await quoteTemplateForm.getByLabel('Template name').fill(quoteTemplateName)
  await quoteTemplateForm.getByLabel('Default validity days').fill('45')
  await quoteTemplateForm.getByLabel('Quote terms').fill(quoteTemplateTerms)
  await quoteTemplateForm.getByLabel('Delivery subject').fill('Finalized quote {{quote_number}}')
  await quoteTemplateForm.getByLabel('Delivery message').fill('Hi {{recipient_name}},\n\nPlease review and electronically sign {{quote_number}} for {{deal_name}}.')
  await quoteTemplateForm.getByRole('checkbox', { name: 'Request electronic signature by default' }).check()
  await quoteTemplateForm.getByRole('checkbox', { name: 'Require independent approval for this template' }).check()
  await quoteTemplateForm.getByRole('button', { name: 'Create template' }).click()
  await expect(page.getByRole('heading', { name: quoteTemplateName })).toBeVisible()
  await page.getByRole('button', { name: 'Require approval for every quote' }).click()
  await expect(page.getByText('Independent approval required workspace-wide', { exact: true })).toBeVisible()
  const quoteTemplateAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-quote-template-approval-settings', {
    body: JSON.stringify({ url: page.url(), violations: quoteTemplateAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(quoteTemplateAccessibility.violations, 'quote template and approval settings must have no automated WCAG A/AA violations').toEqual([])

  await page.getByRole('link', { name: 'Deals', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'New deal' })).toBeVisible()
  const dealForm = page.locator('form').filter({ has: page.getByRole('button', { name: 'Save deal' }) })
  await dealForm.getByLabel('Deal name').fill(`Website renewal ${runID}`)
  await dealForm.getByLabel('Stage').selectOption({ label: `Sales pipeline: ${discoveryStage}` })
  await dealForm.getByLabel('Company').selectOption({ label: `Northstar Advisory ${runID}` })
  await dealForm.getByLabel('Primary contact').selectOption({ label: 'Avery Buyer' })
  await dealForm.getByLabel('Value amount').fill('25000')
  await dealForm.getByLabel('Owner').selectOption({ label: 'Jamie Pilot' })
  await dealForm.getByRole('button', { name: 'Save deal' }).click()
  await expect(page).toHaveURL(/\/deals\/\d+/)
  const configuredDealID = Number(page.url().match(/\/deals\/(\d+)/)?.[1])
  expect(configuredDealID).toBeGreaterThan(0)
  await expect(page.getByRole('heading', { name: `Website renewal ${runID}` })).toBeVisible()
  await expect(page.getByRole('list', { name: 'Deal tasks list' }).getByText(notificationTaskTitle)).toBeVisible()
  await expect(page.getByRole('list', { name: 'Deal tasks list' }).getByText(automatedTaskTitle)).toHaveCount(0)
  await expect(page.getByRole('list', { name: 'Deal tasks list' }).getByText(automatedDecisionTaskTitle)).toHaveCount(0)
  await expect.poll(async () => {
    const response = await page.request.get(`${smtpCaptureURL}/messages`)
    const payload = await response.json()
    return payload.messages.filter((message) => message.data.includes(`Subject: ${sequenceSubject}`)).length
  }).toBe(2)
  const workflowSequenceMessagesResponse = await page.request.get(`${smtpCaptureURL}/messages`)
  const workflowSequenceMessages = (await workflowSequenceMessagesResponse.json()).messages.filter((message) => message.data.includes(`Subject: ${sequenceSubject}`))
  expect(workflowSequenceMessages).toHaveLength(2)
  expect(workflowSequenceMessages.some((message) => message.envelopeFrom.includes(invitedEmail))).toBe(true)
  expect(workflowSequenceMessages.every((message) => message.envelopeTo.includes(`avery-${runID}@example.test`))).toBe(true)
  const dealDetailsForm = page.getByRole('form', { name: 'Deal details form' })
  await dealDetailsForm.getByLabel('Owner').selectOption({ label: 'Pilot Owner' })
  await dealDetailsForm.getByRole('button', { name: 'Update deal' }).click()
  await expect(dealDetailsForm.getByLabel('Owner').locator('option:checked')).toHaveText('Jamie Pilot')
  await page.getByRole('link', { name: 'Automations', exact: true }).click()
  const assignmentRuns = page.getByRole('list', { name: 'Workflow automation runs' }).getByRole('listitem').filter({ hasText: dealAssignmentRuleName })
  await expect(assignmentRuns).toHaveCount(2)
  const assignmentRootRun = assignmentRuns.filter({ hasText: 'Root event · no workflow action caused this run.' })
  await expect(assignmentRootRun).toContainText('1/1 actions completed')
  await expect(assignmentRootRun.getByText('succeeded', { exact: true })).toBeVisible()
  await assignmentRootRun.getByText('Inspect 1 action outcome').click()
  const assignmentActionOutcomes = assignmentRootRun.getByRole('list', { name: `${dealAssignmentRuleName} run actions` })
  await expect(assignmentActionOutcomes).toContainText('1. Assign deal owner')
  await expect(assignmentActionOutcomes).toContainText('Assigned to Jamie Pilot.')
  await expect(assignmentActionOutcomes).toContainText('Action succeeded · 1 attempt')
  const assignmentNestedRun = assignmentRuns.filter({ hasText: 'Nested depth 1' })
  await expect(assignmentNestedRun).toContainText('0/1 actions completed')
  await expect(assignmentNestedRun.getByText('skipped', { exact: true })).toBeVisible()
  await expect(assignmentNestedRun).toContainText('Loop guard: Automation re-entry prevented.')
  const sequenceAutomationRun = page.getByRole('list', { name: 'Workflow automation runs' }).getByRole('listitem').filter({ hasText: dealSequenceRuleName })
  await expect(sequenceAutomationRun).toHaveCount(1)
  await expect(sequenceAutomationRun).toContainText('1/1 actions completed')
  await expect(sequenceAutomationRun.getByText('succeeded', { exact: true })).toBeVisible()
  await expect(sequenceAutomationRun).toContainText('Root event · no workflow action caused this run.')
  await sequenceAutomationRun.getByText('Inspect 1 action outcome').click()
  const sequenceActionOutcomes = sequenceAutomationRun.getByRole('list', { name: `${dealSequenceRuleName} run actions` })
  await expect(sequenceActionOutcomes).toContainText('1. Enroll primary contact in email sequence')
  await expect(sequenceActionOutcomes).toContainText(`Enrolled Avery Buyer in ${sequenceName}; the first delivery is queued durably.`)
  await expect(sequenceActionOutcomes).toContainText('Action succeeded · 1 attempt')
  await expect(sequenceActionOutcomes.getByRole('link', { name: 'Open enrolled contact' })).toHaveAttribute('href', /\/contacts\/\d+$/)
  const sequenceWorkflowRunAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  expect(sequenceWorkflowRunAccessibility.violations, 'sequence workflow outcomes must have no automated WCAG A/AA violations').toEqual([])
  const ownerAssignmentRunAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  expect(ownerAssignmentRunAccessibility.violations, 'deal owner assignment outcomes must have no automated WCAG A/AA violations').toEqual([])
  const notificationAutomationRun = page.getByRole('list', { name: 'Workflow automation runs' }).getByRole('listitem').filter({ hasText: dealNotificationRuleName })
  await expect(notificationAutomationRun).toContainText('2/2 actions completed')
  await expect(notificationAutomationRun.getByText('succeeded', { exact: true })).toBeVisible()
  await expect(notificationAutomationRun).toContainText('Root event · no workflow action caused this run.')
  await notificationAutomationRun.getByText('Inspect 2 action outcomes').click()
  const notificationActionOutcomes = notificationAutomationRun.getByRole('list', { name: `${dealNotificationRuleName} run actions` })
  await expect(notificationActionOutcomes).toContainText(`1. ${notificationTaskTitle}`)
  await expect(notificationActionOutcomes).toContainText('2. Notify owner')
  await expect(notificationActionOutcomes).toContainText('Delivered to 1 eligible teammate.')
  await expect(notificationActionOutcomes.getByText('Action succeeded · 1 attempt')).toHaveCount(2)
  const notificationRunAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  expect(notificationRunAccessibility.violations, 'workflow notification outcomes must have no automated WCAG A/AA violations').toEqual([])
  await page.getByRole('link', { name: 'Email Sequences', exact: true }).click()
  const workflowCompletedSequenceRow = page.getByRole('list', { name: 'Email sequences' }).getByRole('listitem').filter({ hasText: sequenceName })
  await expect(workflowCompletedSequenceRow).toContainText('2 enrolled · 2 accepted · 0 bounced · 0 complaints · 0 replied · 2 finished · 0 suppressed · 0 review')
  await workflowCompletedSequenceRow.getByRole('button', { name: 'View enrollments', exact: true }).click()
  const workflowSequenceHistory = workflowCompletedSequenceRow.getByRole('list', { name: `${sequenceName} enrollments`, exact: true })
  await expect(workflowSequenceHistory.getByRole('link', { name: 'Avery Buyer', exact: true })).toHaveCount(2)
  await expect(workflowSequenceHistory).toContainText('by Pilot Owner')
  await expect(workflowSequenceHistory).toContainText('by Jamie Pilot')
  await expect(workflowSequenceHistory.getByText('Finished · 1 accepted', { exact: true })).toHaveCount(2)
  await page.getByRole('link', { name: 'Automations', exact: true }).click()
  await page.goto('/notifications')
  const workflowNotification = page.getByRole('listitem').filter({ hasText: dealNotificationMessage })
  await expect(workflowNotification).toBeVisible()
  await workflowNotification.getByRole('button', { name: 'Open record' }).click()
  await expect(page).toHaveURL(new RegExp(`/deals/${configuredDealID}$`))
  await page.getByRole('link', { name: 'Automations', exact: true }).click()
  const pendingWorkflowApproval = page.getByRole('list', { name: 'Pending workflow approvals' }).getByRole('listitem').filter({ hasText: dealApprovalName })
  await expect(pendingWorkflowApproval).toContainText('2 tasks are created')
  const dealAutomationRun = page.getByRole('list', { name: 'Workflow automation runs' }).getByRole('listitem').filter({ hasText: dealApprovalRuleName })
  await expect(dealAutomationRun).toContainText('0/3 actions completed')
  await expect(dealAutomationRun.getByText('waiting_approval', { exact: true })).toBeVisible()
  await expect(dealAutomationRun).toContainText('Paused safely until an eligible teammate decides the retained approval.')
  const pendingApprovalAccessibility = await new AxeBuilder({ page })
    .include('[aria-label="Pending workflow approvals"]')
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  expect(pendingApprovalAccessibility.violations, 'pending workflow approval controls must have no automated WCAG A/AA violations').toEqual([])
  await pendingWorkflowApproval.getByRole('button', { name: 'Approve and create tasks' }).click()
  await expect(page.getByText('Workflow approved and its captured tasks were created.', { exact: true })).toBeVisible()
  await expect(pendingWorkflowApproval).toHaveCount(0)
  await expect(dealAutomationRun).toContainText('3/3 actions completed')
  await expect(dealAutomationRun.getByText('succeeded', { exact: true })).toBeVisible()
  await dealAutomationRun.getByText('Inspect 3 action outcomes').click()
  const dealActionOutcomes = dealAutomationRun.getByRole('list', { name: `${dealApprovalRuleName} run actions` })
  await expect(dealActionOutcomes).toContainText(`1. ${dealApprovalName}`)
  await expect(dealActionOutcomes).toContainText(`2. ${automatedTaskTitle}`)
  await expect(dealActionOutcomes).toContainText(`3. ${automatedDecisionTaskTitle}`)
  await expect(dealActionOutcomes).toContainText('Approval approved · owner')
  await expect(dealActionOutcomes.getByText('Action succeeded · 1 attempt')).toHaveCount(3)
  await expect(dealActionOutcomes.getByRole('link', { name: 'Open created task' })).toHaveCount(2)
  const workflowRunAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-workflow-action-outcomes', {
    body: JSON.stringify({ url: page.url(), violations: workflowRunAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(workflowRunAccessibility.violations, 'expanded workflow action outcomes must have no automated WCAG A/AA violations').toEqual([])
  await page.goto(`/deals/${configuredDealID}`)
  await expect(page.getByRole('heading', { name: `Website renewal ${runID}` })).toBeVisible()
  await expect(page.getByRole('list', { name: 'Deal tasks list' }).getByText(automatedTaskTitle)).toBeVisible()
  await expect(page.getByRole('list', { name: 'Deal tasks list' }).getByText(automatedDecisionTaskTitle)).toBeVisible()
  await expect(page.getByLabel('Catalog item').getByRole('option', { name: `Browser catalog ${runID} #001`, exact: false })).toHaveCount(0)
  await page.getByLabel('Catalog item').selectOption({ label: `${quoteCatalogName} (${quoteCatalogSKU})` })
  await expect(page.getByLabel('Line item name')).toHaveValue(quoteCatalogName)
  await expect(page.getByLabel('Line item unit price')).toHaveValue('25000.00')
  await page.getByRole('button', { name: 'Add line item' }).click()
  const saveLineItems = page.getByRole('button', { name: 'Save line items' })
  await saveLineItems.click()
  await expect(saveLineItems).toBeEnabled()
  await expect(page.getByRole('list', { name: 'Deal line items' }).getByText(quoteCatalogName)).toBeVisible()
  await expect(page.getByText('$25,000.00').first()).toBeVisible()
  await page.getByLabel('Quote template').selectOption({ label: `${quoteTemplateName} (revision 1)` })
  await page.getByLabel('Recipient email').fill(`avery-${runID}@example.test`)
  const retainedQuoteTerms = page.getByRole('textbox', { name: 'Terms', exact: true })
  await expect(retainedQuoteTerms).toHaveValue(quoteTemplateTerms)
  await expect(retainedQuoteTerms).toHaveAttribute('readonly')
  await expect(page.getByRole('checkbox', { name: 'Require independent owner/admin approval before delivery' })).toBeChecked()
  await page.getByRole('button', { name: 'Finalize quote' }).click()
  const finalizedQuoteLink = page.getByRole('list', { name: 'Finalized deal quotes' }).getByRole('link', { name: new RegExp(`Download Q-${configuredDealID}-V1`) })
  await expect(finalizedQuoteLink).toBeVisible()
  const finalizedQuoteURL = await finalizedQuoteLink.getAttribute('href')
  const finalizedQuoteID = Number(finalizedQuoteURL?.match(/\/quotes\/(\d+)\/pdf$/)?.[1])
  expect(finalizedQuoteID).toBeGreaterThan(0)
  const finalizedQuoteResponse = await page.context().request.get(finalizedQuoteURL)
  expect(finalizedQuoteResponse.status()).toBe(200)
  expect(finalizedQuoteResponse.headers()['content-type']).toContain('application/pdf')
  expect(finalizedQuoteResponse.headers()['x-open-crm-content-sha256']).toMatch(/^[a-f0-9]{64}$/)
	const finalizedQuoteBody = (await finalizedQuoteResponse.body()).toString()
	expect(finalizedQuoteBody).toContain('Immutable finalized quote.')
	expect(finalizedQuoteBody).toContain(quoteTemplateTerms)
	expect(finalizedQuoteBody).toContain('Currency disclosure')
	expect(finalizedQuoteBody).toContain('Quote currency matches workspace base currency USD; no conversion was applied.')
  const finalizedQuoteRow = page.getByRole('list', { name: 'Finalized deal quotes' }).getByRole('listitem').filter({ hasText: `Q-${configuredDealID}-V1` })
	await expect(finalizedQuoteRow.getByText('Base currency USD; no conversion applied.', { exact: true })).toBeVisible()
  await expect(finalizedQuoteRow.getByText(`${quoteTemplateName} · retained revision 1`, { exact: true })).toBeVisible()
  await expect(finalizedQuoteRow.getByText('Approval: Pending independent review', { exact: true })).toBeVisible()
  await expect(finalizedQuoteRow.getByText('Delivery blocked until independent approval.', { exact: true })).toBeVisible()
  await expect(finalizedQuoteRow.getByText('Deliver by email', { exact: true })).toHaveCount(0)
  const blockedDelivery = await page.context().request.post(`${apiURL}/api/deals/${configuredDealID}/quotes/${finalizedQuoteID}/deliveries`, {
    data: { subject: `Finalized quote Q-${configuredDealID}-V1`, messageBody: 'Must not send before approval.', requestSignature: true },
    headers: { 'Idempotency-Key': `browser-pending-delivery-${runID}` }
  })
  expect(blockedDelivery.status()).toBe(409)
  expect((await blockedDelivery.json()).error.code).toBe('QUOTE_APPROVAL_REQUIRED')

  await memberPage.goto('/settings/quote-templates')
  await expect(memberPage.getByRole('heading', { name: 'Pending quote approvals' })).toBeVisible()
  const pendingApprovalRow = memberPage.getByRole('list', { name: 'Pending quote approvals' }).getByRole('listitem').filter({ hasText: `Q-${configuredDealID}-V1` })
  await expect(pendingApprovalRow).toContainText(`Website renewal ${runID}`)
  await pendingApprovalRow.getByLabel(`Decision note for Q-${configuredDealID}-V1`).fill('Exact PDF digest and retained services terms reviewed.')
  await pendingApprovalRow.getByRole('button', { name: 'Approve exact PDF' }).click()
  await expect(memberPage.getByText('No quotes are waiting for approval.', { exact: true })).toBeVisible()

  await page.reload()
  const approvedQuoteRow = page.getByRole('list', { name: 'Finalized deal quotes' }).getByRole('listitem').filter({ hasText: `Q-${configuredDealID}-V1` })
  await expect(approvedQuoteRow.getByText('Approval: Approved', { exact: true })).toBeVisible()
  await expect(approvedQuoteRow.getByText(/Exact PDF digest and retained services terms reviewed/)).toBeVisible()
  await approvedQuoteRow.getByText('Deliver by email', { exact: true }).click()
  await expect(approvedQuoteRow.getByRole('checkbox', { name: 'Request signature from Avery Buyer' })).toBeChecked()
  await approvedQuoteRow.getByRole('button', { name: 'Send for signature' }).click()
  await expect(approvedQuoteRow.getByText('Sent', { exact: true })).toBeVisible()
  await expect(approvedQuoteRow.getByText('Link accesses: 0', { exact: true })).toBeVisible()
  await expect(approvedQuoteRow.getByText('PDF downloads: 0', { exact: true })).toBeVisible()
  await expect.poll(async () => {
    const response = await page.request.get(`${smtpCaptureURL}/messages`)
    const payload = await response.json()
    return payload.messages.filter((message) => message.data.includes(`Subject: Finalized quote Q-${configuredDealID}-V1`)).length
  }).toBe(1)
  const smtpMessagesResponse = await page.request.get(`${smtpCaptureURL}/messages`)
  const smtpMessages = (await smtpMessagesResponse.json()).messages
  const quoteMessage = smtpMessages.find((message) => message.data.includes(`Subject: Finalized quote Q-${configuredDealID}-V1`))
  expect(quoteMessage.envelopeTo).toContain(`avery-${runID}@example.test`)
  expect(quoteMessage.data).toContain('Review and electronically sign the finalized quote:')
  expect(quoteMessage.data).toMatch(/Message-ID: <[^>]+>/)
  const publicQuoteURL = quoteMessage.data.match(/https?:\/\/[^\s]+\/quote\?token=[A-Za-z0-9_-]+/)?.[0]
  expect(publicQuoteURL).toBeTruthy()
  const publicQuoteToken = new URL(publicQuoteURL).searchParams.get('token')

  const customerContext = await browser.newContext()
  const customerPage = await customerContext.newPage()
  await customerPage.goto(publicQuoteURL)
  await expect(customerPage.getByRole('heading', { name: `Q-${configuredDealID}-V1` })).toBeVisible()
	await expect(customerPage.getByText('Base currency USD; no conversion applied.', { exact: true })).toBeVisible()
  await expect(customerPage.getByText('Receipt is not acceptance.', { exact: false })).toBeVisible()
  await expect(customerPage.getByRole('heading', { name: 'Electronic signature' })).toBeVisible()
  const publicQuoteAccessibility = await new AxeBuilder({ page: customerPage })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-public-finalized-quote', {
    body: JSON.stringify({ url: customerPage.url(), violations: publicQuoteAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(publicQuoteAccessibility.violations, 'public finalized quote must have no automated WCAG A/AA violations').toEqual([])
  const publicQuotePDF = await customerContext.request.get(`${apiURL}/api/public/quotes/${encodeURIComponent(publicQuoteToken)}/pdf`)
  expect(publicQuotePDF.status()).toBe(200)
  expect(publicQuotePDF.headers()['content-type']).toContain('application/pdf')
  expect(publicQuotePDF.headers()['x-open-crm-content-sha256']).toBe(finalizedQuoteResponse.headers()['x-open-crm-content-sha256'])
  await customerPage.getByRole('button', { name: 'Confirm receipt' }).click()
  await expect(customerPage.getByText('Receipt confirmed', { exact: false })).toBeVisible()
  await customerPage.getByLabel('Type the recipient name exactly: Avery Buyer').fill('Avery Buyer')
  await customerPage.getByRole('checkbox', { name: 'I agree to use an electronic signature', exact: false }).check()
  await customerPage.getByRole('button', { name: 'Sign quote' }).click()
  await expect(customerPage.getByText('Signed by Avery Buyer', { exact: true })).toBeVisible()
  const customerCertificate = await customerContext.request.get(`${apiURL}/api/public/quotes/${encodeURIComponent(publicQuoteToken)}/signature-certificate`)
  expect(customerCertificate.status()).toBe(200)
  expect(customerCertificate.headers()['content-type']).toContain('application/pdf')
  expect(customerCertificate.headers()['x-open-crm-content-sha256']).toMatch(/^[a-f0-9]{64}$/)
  const customerCertificateBody = await customerCertificate.body()
  expect(customerCertificateBody.toString()).toContain('Typed signature: Avery Buyer')
  expect(customerCertificateBody.toString()).toContain(finalizedQuoteResponse.headers()['x-open-crm-content-sha256'])
  await customerContext.close()

  const invalidPublicQuote = await page.request.get(`${apiURL}/api/public/quotes/not-a-valid-token`)
  expect(invalidPublicQuote.status()).toBe(404)
  await page.reload()
  await expect(page.getByRole('heading', { name: `Website renewal ${runID}` })).toBeVisible()
  const deliveredQuoteRow = page.getByRole('list', { name: 'Finalized deal quotes' }).getByRole('listitem').filter({ hasText: `Q-${configuredDealID}-V1` })
  await expect(deliveredQuoteRow.getByText(/Link accesses: [1-9]\d*/)).toBeVisible()
  await expect(deliveredQuoteRow.getByText('PDF downloads: 1', { exact: true })).toBeVisible()
  await expect(deliveredQuoteRow.getByText(/^Receipt confirmed \d/)).toBeVisible()
  await expect(deliveredQuoteRow.getByText('Signature: Signed', { exact: false })).toBeVisible()
  const signatureRow = page.getByRole('list', { name: 'Deal quote signature requests' }).getByRole('listitem').filter({ hasText: `Q-${configuredDealID}-V1 · Avery Buyer` })
  await expect(signatureRow.getByText(`avery-${runID}@example.test · Signed`, { exact: true })).toBeVisible()
  const staffCertificateLink = signatureRow.getByRole('link', { name: 'Download certificate' })
  const staffCertificate = await page.context().request.get(await staffCertificateLink.getAttribute('href'))
  expect(staffCertificate.status()).toBe(200)
  expect(staffCertificate.headers()['x-open-crm-content-sha256']).toBe(customerCertificate.headers()['x-open-crm-content-sha256'])
  expect(await staffCertificate.body()).toEqual(customerCertificateBody)
  const quoteResponse = await page.context().request.get(`${apiURL}/api/deals/${configuredDealID}/quote.pdf`)
  expect(quoteResponse.status()).toBe(200)
  expect(quoteResponse.headers()['content-type']).toContain('application/pdf')
  expect((await quoteResponse.body()).toString()).toContain('Discovery and implementation')

  await page.getByRole('link', { name: 'Pipelines', exact: true }).click()
  await page.getByLabel(`Stage name for ${discoveryStage}`).fill(reviewedDiscoveryStage)
  await page.getByRole('button', { name: `Save ${discoveryStage}` }).click()
  await expect(page.getByText(`${reviewedDiscoveryStage} updated without changing its stage ID.`)).toBeVisible()
  await page.getByRole('link', { name: 'Deals', exact: true }).click()
  const configuredDeal = page.getByRole('listitem').filter({ hasText: `Website renewal ${runID}` })
  await expect(configuredDeal).toContainText(`Sales pipeline · ${reviewedDiscoveryStage}`)
  await page.getByRole('link', { name: 'Dashboard', exact: true }).click()
  const configuredStageForecast = page.getByRole('list', { name: 'Forecast stage assumptions' }).getByRole('listitem').filter({ hasText: `Sales pipeline · ${reviewedDiscoveryStage} · 65%` })
  await expect(configuredStageForecast).toContainText('$16,250.00 weighted')
  await expect(page.getByText('3 due within 24 hours', { exact: true })).toBeVisible()
  await page.getByRole('link', { name: 'Deals', exact: true }).click()
  await configuredDeal.getByRole('button', { name: `Website renewal ${runID}` }).click()
  await expect(page.getByRole('heading', { name: `Website renewal ${runID}` })).toBeVisible()

  const taskForm = page.locator('form').filter({ has: page.getByRole('button', { name: 'Save task' }) })
  await taskForm.getByLabel('Task title').fill(`Prepare proposal ${runID}`)
  await taskForm.getByLabel('Task description').fill('Confirm scope and pricing with the client.')
  const [createdDealTaskResponse] = await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/tasks'),
    taskForm.getByRole('button', { name: 'Save task' }).click()
  ])
  expect(createdDealTaskResponse.status()).toBe(201)
  await expect(page.getByRole('list', { name: 'Deal tasks list' }).getByText(`Prepare proposal ${runID}`)).toBeVisible()

  seedReportDefinitionContinuation(owner.email, runID)
  await page.getByRole('link', { name: 'Reports', exact: true }).click()
  const oldestSeededReport = `Browser report ${runID} #001`
  await expect(page.getByText('Showing 50 of 51 stored definitions.')).toBeVisible()
  await expect(page.getByRole('heading', { name: oldestSeededReport })).toHaveCount(0)
  await page.getByRole('button', { name: 'Load more stored report definitions' }).click()
  await expect(page.getByRole('heading', { name: oldestSeededReport })).toBeVisible()
  await expect(page.getByText('Showing 51 of 51 stored definitions.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Load more stored report definitions' })).toHaveCount(0)
  const salesActivityCard = page.locator('.sales-activity-card')
  await expect(salesActivityCard.getByRole('heading', { name: 'Sales activity' })).toBeVisible()
  await expect(salesActivityCard.getByText('Event history: complete', { exact: false })).toBeVisible()
  const salesTotals = salesActivityCard.getByRole('list', { name: 'Sales activity totals' })
  await expect(salesTotals.getByRole('listitem').filter({ hasText: 'Deals created' })).toContainText('1')
  await expect(salesTotals.getByRole('listitem').filter({ hasText: 'Won revenue (USD)' })).toContainText('0.00 USD')
  await expect(salesTotals.getByRole('listitem').filter({ hasText: 'Tasks created' })).toContainText('6')
  await expect(salesActivityCard.getByRole('list', { name: 'Stage movement report' }).getByText(`Sales pipeline / ${discoveryStage}`)).toBeVisible()
  await expect(salesActivityCard.getByRole('list', { name: 'Recent deal events' }).getByText(`Created in Sales pipeline / ${discoveryStage}`)).toBeVisible()
  await expect(salesActivityCard.getByRole('link', { name: `Website renewal ${runID}` })).toBeVisible()
  const followUpReport = page.locator('.follow-up-report-card')
  await expect(followUpReport.getByText(/No records need follow-up/)).toBeVisible()
  await expect(followUpReport.getByText(/How the follow-up queue is calculated/)).toBeVisible()
  const incompleteDealReport = page.getByRole('listitem').filter({ hasText: 'Incomplete open deals' })
  await expect(incompleteDealReport).toContainText('Missing expected close date')

  const savedReportName = `Captured leads ${runID}`
  const savedReportForm = page.locator('form').filter({ has: page.getByRole('button', { name: 'Create report definition' }) })
  await savedReportForm.getByLabel('Name', { exact: true }).fill(savedReportName)
  await savedReportForm.getByRole('button', { name: 'Add filter' }).click()
  await savedReportForm.getByLabel('Filter field 1').selectOption('email')
  await savedReportForm.getByLabel('Filter value 1').fill(publicLeadEmail)
  const [savedReportResponse] = await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/report-definitions'),
    savedReportForm.getByRole('button', { name: 'Create report definition' }).click()
  ])
  expect(savedReportResponse.status()).toBe(201)
  const savedReportID = (await savedReportResponse.json()).data.definition.id
  const savedReport = page.getByRole('listitem').filter({ has: page.getByRole('heading', { name: savedReportName, exact: true }) })
  await expect(savedReport.getByText('Executable table', { exact: true })).toBeVisible()
  await savedReport.getByRole('button', { name: 'Run report' }).click()
  const savedReportResults = savedReport.getByRole('region', { name: `${savedReportName} results` })
  await expect(savedReportResults.getByRole('columnheader', { name: 'Email' })).toBeVisible()
  await expect(savedReportResults.getByText(publicLeadEmail, { exact: true })).toBeVisible()
  const savedReportExportLink = savedReport.getByRole('link', { name: 'Download CSV' })
  await expect(savedReportExportLink).toHaveAttribute('href', new RegExp(`/api/report-definitions/${savedReportID}/export\\.csv$`))
  const savedReportExport = await page.context().request.get(await savedReportExportLink.getAttribute('href'))
  expect(savedReportExport.status()).toBe(200)
  expect(savedReportExport.headers()['content-type']).toContain('text/csv')
  expect(savedReportExport.headers()['content-disposition']).toMatch(new RegExp(`^attachment; filename="saved-report-${savedReportID}-\\d{8}\\.csv"$`))
  expect(savedReportExport.headers()['cache-control']).toContain('no-store')
  expect(savedReportExport.headers()['x-content-type-options']).toBe('nosniff')
  const savedReportCSV = await savedReportExport.body()
  expect([...savedReportCSV.subarray(0, 3)]).toEqual([0xef, 0xbb, 0xbf])
  expect(savedReportCSV.toString('utf8')).toContain(publicLeadEmail)
  const savedReportAccessibility = await new AxeBuilder({ page })
    .include('.custom-report-results')
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-saved-table-report', {
    body: JSON.stringify({ url: page.url(), violations: savedReportAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(savedReportAccessibility.violations, 'saved table report must have no automated WCAG A/AA violations').toEqual([])

  const scheduledDelivery = page.locator('.scheduled-report-delivery')
  await scheduledDelivery.getByLabel('Saved report').selectOption({ label: savedReportName })
  const ownerRecipient = scheduledDelivery.getByLabel(owner.email, { exact: false })
  if (!(await ownerRecipient.isChecked())) await ownerRecipient.check()
  await scheduledDelivery.getByLabel('Cadence').selectOption('daily')
  await scheduledDelivery.getByLabel('Hour (UTC)').selectOption('9')
  const [scheduleResponse] = await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'PUT' && new URL(response.url()).pathname === `/api/report-definitions/${savedReportID}/schedule`),
    scheduledDelivery.getByRole('button', { name: 'Save and enable schedule' }).click()
  ])
  expect(scheduleResponse.status()).toBe(200)
  expect((await scheduleResponse.json()).data.schedule.revision).toBe(1)
  makeReportScheduleDue(owner.email)
  let scheduledRecipientDeliveryID = 0
  await expect.poll(async () => {
    const response = await page.context().request.get(`${apiURL}/api/report-schedules`)
    if (response.status() !== 200) return false
    const body = await response.json()
    const run = body.data.deliveryRuns.find((candidate) => candidate.reportDefinitionId === savedReportID)
    const accepted = run?.recipients.find((delivery) => delivery.status === 'accepted')
    scheduledRecipientDeliveryID = accepted?.id || 0
    return run?.status === 'succeeded' && run.rowCount === 1 && scheduledRecipientDeliveryID > 0
  }, { timeout: 15_000 }).toBe(true)
  await scheduledDelivery.getByRole('button', { name: 'Refresh delivery evidence' }).click()
  const scheduledHistory = scheduledDelivery.getByRole('list', { name: 'Scheduled report delivery history' })
  await expect(scheduledHistory.getByText('Accepted by provider', { exact: false })).toBeVisible()
  await expect(scheduledHistory.getByText(/1 rows/)).toBeVisible()
  const scheduledDeliveryAccessibility = await new AxeBuilder({ page })
    .include('.scheduled-report-delivery')
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-scheduled-report-delivery', {
    body: JSON.stringify({ url: page.url(), violations: scheduledDeliveryAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(scheduledDeliveryAccessibility.violations, 'scheduled report delivery must have no automated WCAG A/AA violations').toEqual([])

  const barReportName = `Contacts by status ${runID}`
  await savedReportForm.getByLabel('Name', { exact: true }).fill(barReportName)
  await savedReportForm.getByLabel('Visualization').selectOption('bar')
  await savedReportForm.getByLabel('Category (group by)').selectOption('status')
  const [barReportResponse] = await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/report-definitions'),
    savedReportForm.getByRole('button', { name: 'Create report definition' }).click()
  ])
  expect(barReportResponse.status()).toBe(201)
  const barReportID = (await barReportResponse.json()).data.definition.id
  const barReport = page.getByRole('listitem').filter({ has: page.getByRole('heading', { name: barReportName, exact: true }) })
  await expect(barReport.getByText('Executable bar', { exact: true })).toBeVisible()
  await barReport.getByRole('button', { name: 'Run report' }).click()
  await expect(barReport.getByRole('img', { name: new RegExp(`${barReportName} grouped bar chart`) })).toBeVisible()
  const barReportData = barReport.getByRole('region', { name: `${barReportName} chart data` })
  await expect(barReportData.getByRole('columnheader', { name: 'Status' })).toBeVisible()
  await expect(barReportData.getByRole('columnheader', { name: 'Record count' })).toBeVisible()
  await expect(barReportData.getByText('lead', { exact: true })).toBeVisible()
  const barReportExport = await page.context().request.get(await barReport.getByRole('link', { name: 'Download CSV' }).getAttribute('href'))
  expect(barReportExport.status()).toBe(200)
  expect((await barReportExport.body()).toString('utf8')).toContain('Status,Record count')
  const barReportAccessibility = await new AxeBuilder({ page })
    .include('.custom-report-results')
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-saved-report-table-and-bar', {
    body: JSON.stringify({ url: page.url(), violations: barReportAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(barReportAccessibility.violations, 'saved table and grouped bar reports must have no automated WCAG A/AA violations').toEqual([])

  const dashboardSettings = page.locator('.report-dashboard-settings')
  await dashboardSettings.getByLabel('Add grouped-bar report').selectOption({ label: barReportName })
  await dashboardSettings.getByRole('button', { name: 'Add to dashboard' }).click()
  await dashboardSettings.getByLabel(`Width for ${barReportName}`).selectOption('full')
  const [dashboardUpdateResponse] = await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'PUT' && new URL(response.url()).pathname === '/api/report-dashboard'),
    dashboardSettings.getByRole('button', { name: 'Save dashboard' }).click()
  ])
  expect(dashboardUpdateResponse.status()).toBe(200)
  await expect(dashboardSettings.getByText('Shared dashboard updated. Everyone in this workspace will see the same snapshot on Dashboard.')).toBeVisible()

  await page.getByRole('link', { name: 'Dashboard', exact: true }).click()
  const sharedReportDashboard = page.getByRole('region', { name: 'Report dashboard' })
  await expect(sharedReportDashboard.getByText(/All charts use one workspace snapshot generated/)).toBeVisible()
  await expect(sharedReportDashboard.getByRole('heading', { name: barReportName, exact: true })).toBeVisible()
  await expect(sharedReportDashboard.getByRole('img', { name: new RegExp(`${barReportName} grouped bar chart`) })).toBeVisible()
  const dashboardBarData = sharedReportDashboard.getByRole('region', { name: `${barReportName} chart data` })
  await expect(dashboardBarData.getByRole('columnheader', { name: 'Status' })).toBeVisible()
  await expect(dashboardBarData.getByRole('columnheader', { name: 'Record count' })).toBeVisible()
  await expect(dashboardBarData.getByText('lead', { exact: true })).toBeVisible()
  const dashboardAccessibility = await new AxeBuilder({ page })
    .include('.report-dashboard-grid')
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-shared-report-dashboard', {
    body: JSON.stringify({ url: page.url(), violations: dashboardAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(dashboardAccessibility.violations, 'shared report dashboard must have no automated WCAG A/AA violations').toEqual([])
  await sharedReportDashboard.getByRole('button', { name: 'Manage reports' }).click()
  await expect(page).toHaveURL(/\/reports$/)

  await incompleteDealReport.getByRole('link', { name: `Website renewal ${runID}` }).click()
  await expect(page).toHaveURL(/\/deals\/\d+/)

  const noteForm = page.locator('form').filter({ has: page.getByRole('button', { name: 'Add note' }) })
  await noteForm.getByLabel('New note').fill('Please review the proposal and confirm the next step.')
  await noteForm.getByRole('button', { name: `@${invitedEmail}` }).click()
  await noteForm.getByRole('button', { name: 'Add note' }).click()
  await expect(page.getByRole('list', { name: 'Deal notes list' }).getByText(`@${invitedEmail}`, { exact: false })).toBeVisible()

  const conversionSignatureRow = page.getByRole('list', { name: 'Deal quote signature requests' }).getByRole('listitem').filter({ hasText: `Q-${configuredDealID}-V1 · Avery Buyer` })
  await conversionSignatureRow.getByText('Convert signed quote to won', { exact: true }).click()
  const signedQuoteConversion = conversionSignatureRow.getByRole('form', { name: `Convert Q-${configuredDealID}-V1 to won` })
  await signedQuoteConversion.getByLabel(`Won stage for Q-${configuredDealID}-V1`).selectOption({ label: 'Closed Won' })
  await signedQuoteConversion.getByLabel('Won reason').selectOption('solution_fit')
  await signedQuoteConversion.getByLabel('Close notes').fill('Strong service fit and a clear implementation plan.')
  const signedConversionAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-signed-quote-conversion', {
    body: JSON.stringify({ url: page.url(), violations: signedConversionAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(signedConversionAccessibility.violations, 'signed quote conversion must have no automated WCAG A/AA violations').toEqual([])
  await signedQuoteConversion.getByRole('button', { name: 'Convert signed quote and hand off client' }).click()
  await expect(conversionSignatureRow.getByText('Converted to Closed Won', { exact: true })).toBeVisible()
  await expect(conversionSignatureRow).toContainText('Best solution fit · Strong service fit and a clear implementation plan.')
  await expect(conversionSignatureRow).toContainText('Later stage changes do not erase this retained conversion evidence.')
  const closeReview = page.getByLabel('Deal close review')
  await expect(closeReview.getByRole('heading', { name: 'Won outcome' })).toBeVisible()
  await expect(closeReview).toContainText('Best solution fit')
  await expect(closeReview).toContainText('Strong service fit and a clear implementation plan.')
  await closeReview.getByRole('link', { name: 'Open customer account' }).click()
  await expect(page).toHaveURL(/\/companies\/\d+$/)
  const accountSummary = page.getByLabel('Client account summary')
  await expect(accountSummary.getByRole('heading', { name: 'Account summary' })).toBeVisible()
  await expect(accountSummary.getByRole('list', { name: 'Won account deals' }).getByText(`Website renewal ${runID}`)).toBeVisible()
  await expect(accountSummary.getByRole('list', { name: 'Open account tasks' }).getByText(accountHandoffTask)).toBeVisible()
  await expect(accountSummary.getByRole('list', { name: 'Recent account notes' }).getByText(accountHandoffNote)).toBeVisible()
  await expect(accountSummary.getByRole('list', { name: 'Key account contacts' }).getByText('Avery Buyer')).toBeVisible()
  const accountForm = page.locator('form').filter({ has: page.getByRole('button', { name: 'Update client' }) })
  await expect(accountForm.getByLabel('Status')).toHaveValue('customer')
  const accountHealth = page.locator('.touchpoint-summary-card')
  await expect(accountHealth.getByText('Needs attention', { exact: true })).toBeVisible()
  await expect(accountHealth.getByText('Overdue: 1')).toBeVisible()

  const renewalTaskTitle = `Client renewal: Northstar Advisory ${runID}`
  const reviewSchedule = page.locator('.client-review-card')
  await expect(reviewSchedule.getByRole('heading', { name: 'Client review schedule' })).toBeVisible()
  await reviewSchedule.getByLabel('Follow-up type').selectOption('renewal')
  await reviewSchedule.getByLabel('Next due time').fill(datetimeLocalDaysFromNow(20))
  await reviewSchedule.getByLabel('Cadence').selectOption('3')
  await reviewSchedule.getByLabel('Assignee').selectOption({ label: 'Pilot Owner' })
  await reviewSchedule.getByRole('button', { name: 'Schedule task' }).click()
  await expect(reviewSchedule.getByText('Client renewal task scheduled.', { exact: true })).toBeVisible()
  const currentRenewal = reviewSchedule.getByRole('listitem')
  const firstRenewalTaskPath = await currentRenewal.getByRole('link', { name: 'Open task' }).getAttribute('href')
  expect(firstRenewalTaskPath).toMatch(/^\/tasks\/\d+$/)

  await page.getByRole('link', { name: 'Dashboard', exact: true }).click()
  const clientObligations = page.locator('.card').filter({ has: page.getByRole('heading', { name: 'Reviews and renewals' }) })
  await expect(clientObligations.getByText('1 due within 30 days', { exact: true })).toBeVisible()
  const renewalObligation = clientObligations.getByRole('listitem').filter({ hasText: `Northstar Advisory ${runID}` })
  await expect(renewalObligation).toContainText('Client renewal')
  await renewalObligation.getByRole('link', { name: 'Open task' }).click()
  await expect(page).toHaveURL(new RegExp(`${firstRenewalTaskPath}$`))
  const renewalTaskForm = page.locator('form').filter({ has: page.getByRole('button', { name: 'Update task' }) })
  await expect(renewalTaskForm.getByLabel('Task title')).toHaveValue(renewalTaskTitle)
  await renewalTaskForm.getByLabel('Status').selectOption('completed')
  await renewalTaskForm.getByRole('button', { name: 'Update task' }).click()
  await expect(renewalTaskForm.getByLabel('Status')).toHaveValue('completed')

  await page.getByRole('link', { name: 'Dashboard', exact: true }).click()
  await expect(clientObligations.getByText('1 later', { exact: true })).toBeVisible()
  const nextRenewal = clientObligations.getByRole('listitem').filter({ hasText: `Northstar Advisory ${runID}` })
  const nextRenewalTaskPath = await nextRenewal.getByRole('link', { name: 'Open task' }).getAttribute('href')
  expect(nextRenewalTaskPath).toMatch(/^\/tasks\/\d+$/)
  expect(nextRenewalTaskPath).not.toBe(firstRenewalTaskPath)
  await nextRenewal.getByRole('link', { name: `Northstar Advisory ${runID}` }).click()
  await expect(page).toHaveURL(/\/companies\/\d+$/)
  await expect(reviewSchedule.getByRole('listitem').getByText('Every 3 months', { exact: false })).toBeVisible()

  await page.getByRole('link', { name: 'Clients', exact: true }).click()
  const clientHealth = page.getByLabel('Client health')
  await expect(clientHealth.getByText('Needs attention: 1')).toBeVisible()
  const healthRecord = clientHealth.getByRole('listitem').filter({ hasText: `Northstar Advisory ${runID}` })
  await expect(healthRecord).toContainText('1 overdue open task')
  await healthRecord.getByRole('button', { name: `Northstar Advisory ${runID}` }).click()
  await expect(page).toHaveURL(/\/companies\/\d+$/)

  await memberPage.goto('/notifications')
  await memberPage.getByLabel('Show').selectOption('assignments')
  const dealAssignmentNotifications = memberPage.getByRole('listitem').filter({ hasText: `assigned a deal: Website renewal ${runID}` })
  await expect(dealAssignmentNotifications).toHaveCount(2)
  await expect(dealAssignmentNotifications.first()).toBeVisible()
  await dealAssignmentNotifications.first().getByRole('button', { name: 'Open record' }).click()
  await expect(memberPage).toHaveURL(new RegExp(`/deals/${configuredDealID}$`))
  await memberPage.goto('/notifications')
  const mentionNotification = memberPage.getByRole('listitem').filter({ hasText: `mentioned you on Website renewal ${runID}` })
  await expect(mentionNotification).toBeVisible()
  await expect(memberPage.getByText(/\d+ unread notifications?\./)).toBeVisible()
  await expect(memberPage.getByRole('heading', { name: 'Activity digest' })).toBeVisible()
  await expect(memberPage.getByRole('list', { name: 'Activity digest' }).getByText(`Website renewal ${runID}: Note added`)).toBeVisible()
  const notificationCenterAccessibility = await new AxeBuilder({ page: memberPage })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-notification-center', {
    body: JSON.stringify({ url: memberPage.url(), violations: notificationCenterAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(notificationCenterAccessibility.violations, 'notification center must have no automated WCAG A/AA violations').toEqual([])
  await memberPage.getByRole('button', { name: 'Mark all read' }).click()
  await expect(memberPage.getByText(/0 unread notifications\./)).toBeVisible()
  await mentionNotification.getByRole('button', { name: 'Open record' }).click()
  await expect(memberPage).toHaveURL(/\/deals\/\d+$/)
  await memberPage.getByRole('button', { name: 'Followers' }).click()
  await expect(memberPage.getByRole('button', { name: 'Following' })).toBeVisible()
  await memberContext.close()

  await page.getByRole('link', { name: 'Reports', exact: true }).click()
  await expect(salesActivityCard.getByRole('list', { name: 'Sales activity totals' }).getByRole('listitem').filter({ hasText: 'Won outcomes' })).toContainText('1')
  await expect(salesActivityCard.getByRole('list', { name: 'Sales activity totals' }).getByRole('listitem').filter({ hasText: 'Won revenue (USD)' })).toContainText('25,000.00 USD')
  await expect(salesActivityCard.getByText('Revenue inputs: 1 backed, 0 missing value/currency, 0 missing event-time FX.', { exact: true })).toBeVisible()
  const closeReasonReport = salesActivityCard.getByRole('list', { name: 'Win and loss reasons' })
  await expect(closeReasonReport.getByRole('listitem').filter({ hasText: 'Best solution fit' })).toContainText('1')
  await expect(salesActivityCard.getByRole('list', { name: 'Recent deal events' }).getByText('Strong service fit and a clear implementation plan.', { exact: false })).toBeVisible()
  const salesActivityAccessibility = await new AxeBuilder({ page })
    .include('.sales-activity-card')
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-sales-activity-revenue', {
    body: JSON.stringify({ url: page.url(), violations: salesActivityAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(salesActivityAccessibility.violations, 'sales activity and revenue report must have no automated WCAG A/AA violations').toEqual([])

  const pipelineFunnel = page.locator('.pipeline-funnel-report-card')
  await expect(pipelineFunnel.getByRole('heading', { name: 'Pipeline conversion and velocity' })).toBeVisible()
  const pipelineFunnelSelect = pipelineFunnel.getByRole('combobox', { name: 'Pipeline', exact: true })
  const pipelineFunnelEntryStage = pipelineFunnel.getByRole('combobox', { name: 'Entry stage', exact: true })
  await pipelineFunnelSelect.selectOption({ label: 'Sales pipeline' })
  await pipelineFunnelEntryStage.selectOption({ label: reviewedDiscoveryStage })
  const funnelPipelineID = Number(await pipelineFunnelSelect.inputValue())
  const funnelEntryStageID = Number(await pipelineFunnelEntryStage.inputValue())
  expect(funnelPipelineID).toBeGreaterThan(0)
  expect(funnelEntryStageID).toBeGreaterThan(0)
  await pipelineFunnel.getByRole('button', { name: 'Run pipeline report' }).click()
  const funnelTotals = pipelineFunnel.getByRole('list', { name: 'Pipeline cohort totals' })
  await expect(funnelTotals.getByRole('listitem').filter({ hasText: 'Cohort deals' })).toContainText('1')
  await expect(funnelTotals.getByRole('listitem').filter({ hasText: 'Won as of date' })).toContainText('1')
  await expect(funnelTotals.getByRole('listitem').filter({ hasText: 'Closed win rate' })).toContainText('100.0%')
  const funnelTable = pipelineFunnel.getByRole('table', { name: /Exact stage reach and elapsed-time metrics/ })
  const discoveryFunnelRow = funnelTable.getByRole('row', { name: new RegExp(reviewedDiscoveryStage) })
  await expect(discoveryFunnelRow.getByText('1 · 100.0%', { exact: true })).toHaveCount(2)
  const wonFunnelRow = funnelTable.getByRole('row', { name: /Closed Won/ })
  await expect(wonFunnelRow).toContainText('1 · 100.0%')
  await pipelineFunnel.getByText('How this cohort and velocity report is calculated').click()
  await expect(pipelineFunnel.getByText(/Cohorts observed for less time can show lower conversion/)).toBeVisible()
  const pipelineFunnelAccessibility = await new AxeBuilder({ page })
    .include('.pipeline-funnel-report-card')
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-pipeline-cohort-velocity', {
    body: JSON.stringify({ url: page.url(), violations: pipelineFunnelAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(pipelineFunnelAccessibility.violations, 'pipeline cohort and velocity report must have no automated WCAG A/AA violations').toEqual([])

  const clientActivity = page.locator('.client-activity-report-card')
  await expect(clientActivity.getByRole('heading', { name: 'Client activity' })).toBeVisible()
  const clientActivityTotals = clientActivity.getByRole('list', { name: 'Client activity totals' })
  await expect(clientActivityTotals.getByRole('listitem').filter({ hasText: 'Clients' })).toContainText('1')
  await expect(clientActivityTotals.getByRole('listitem').filter({ hasText: 'With activity' })).toContainText('1')
  await expect(clientActivityTotals.getByRole('listitem').filter({ hasText: 'Notes added' })).toContainText('2')
  await expect(clientActivityTotals.getByRole('listitem').filter({ hasText: 'Tasks completed' })).toContainText('1')
  const clientActivityTable = clientActivity.getByRole('table', { name: /Client activity from/ })
  const northstarActivity = clientActivityTable.getByRole('row', { name: new RegExp(`^Northstar Advisory ${runID}`) })
  await expect(northstarActivity).toBeVisible()
  await expect(northstarActivity.getByRole('link', { name: `Completed task: ${renewalTaskTitle}` })).toHaveAttribute('href', /^\/companies\/\d+$/)
  await clientActivity.getByText('How client activity is calculated').click()
  await expect(clientActivity.getByText(/This report does not infer historical health changes/)).toBeVisible()
  const clientActivityAccessibility = await new AxeBuilder({ page })
    .include('.client-activity-report-card')
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-client-period-activity', {
    body: JSON.stringify({ url: page.url(), violations: clientActivityAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(clientActivityAccessibility.violations, 'client-period activity must have no automated WCAG A/AA violations').toEqual([])

  await page.getByRole('link', { name: 'Tasks', exact: true }).click()
  await expect(page.getByText(/Overdue \d+ · Due soon [1-9]\d*/)).toBeVisible()
  await page.getByLabel('Task view').selectOption('dueSoon')
  await expect(page.getByRole('heading', { name: 'Tasks due within 24 hours' })).toBeVisible()
  await expect(page.getByRole('button', { name: automatedTaskTitle, exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: leadFollowUpTaskTitle, exact: true })).toBeVisible()
  await page.getByLabel('Task view').selectOption('all')
  await page.getByRole('button', { name: `Complete Prepare proposal ${runID}` }).click()
  await expect(page.getByRole('button', { name: `Complete Prepare proposal ${runID}` })).toHaveCount(0)
  await page.getByRole('button', { name: 'Show completed' }).click()
  await expect(page.getByRole('button', { name: `Reopen Prepare proposal ${runID}` })).toBeVisible()
  await page.getByRole('button', { name: `Prepare proposal ${runID}`, exact: true }).click()
  await expect(page).toHaveURL(/\/tasks\/\d+/)
  await page.getByRole('button', { name: 'Archive task' }).click()
  await expect(page).toHaveURL(/\/tasks(?:\?|$)/)
  await page.getByRole('link', { name: 'Archived Records', exact: true }).click()
  const archivedTask = page.getByRole('listitem').filter({ hasText: `Prepare proposal ${runID}` })
  await expect(archivedTask).toBeVisible()
  page.once('dialog', (dialog) => dialog.accept())
  await archivedTask.getByRole('button', { name: 'Restore', exact: true }).click()
  await expect(page.getByText(`Prepare proposal ${runID} was restored.`, { exact: false })).toBeVisible()
  await page.getByRole('link', { name: 'Open restored record' }).click()
  await expect(page).toHaveURL(/\/tasks\/\d+/)
  await expect(page.getByRole('heading', { name: `Prepare proposal ${runID}` })).toBeVisible()

  const exportExpectations = [
    { path: 'contacts', includes: [`avery-${runID}@example.test`, 'custom:relationship_segment', 'Partner'], excludes: [`avery-duplicate-${runID}@example.test`, `imported-${runID}@example.test`] },
    { path: 'companies', includes: [`Northstar Advisory ${runID}`, 'custom:service_tier', 'Gold'], excludes: [importedClientName] },
    { path: 'deals', includes: [`Website renewal ${runID}`, 'close_reason_label', 'Best solution fit', 'Strong service fit and a clear implementation plan.'], excludes: [] },
    { path: 'tasks', includes: [`Prepare proposal ${runID}`, automatedTaskTitle, automatedDecisionTaskTitle], excludes: [] }
  ]
  for (const exportExpectation of exportExpectations) {
    const exportResponse = await page.context().request.get(`${apiURL}/api/export/${exportExpectation.path}`)
    expect(exportResponse.status()).toBe(200)
    expect(exportResponse.headers()['content-type']).toContain('text/csv')
    const csv = await exportResponse.text()
    for (const expectedValue of exportExpectation.includes) expect(csv).toContain(expectedValue)
    for (const excludedValue of exportExpectation.excludes) expect(csv).not.toContain(excludedValue)
  }

  const durableContactSearch = `avery-${runID}@example.test`
  await page.goto(`/contacts?q=${encodeURIComponent(durableContactSearch)}`)
  await expect(page.getByRole('button', { name: 'Avery Buyer', exact: true })).toBeVisible()
  await page.getByRole('link', { name: 'Queue large CSV' }).click()
  await expect(page).toHaveURL(/\/settings\/operations\?crmExport=/)
  await expect(page.getByText(`Search: ${durableContactSearch}.`, { exact: false })).toBeVisible()
  await page.getByRole('button', { name: 'Queue CSV' }).click()
  const durableExport = page.getByRole('list', { name: 'Filtered CRM export history' }).getByRole('listitem').first()
  await expect(durableExport).toContainText('contacts export #')
  await expect(durableExport).toContainText('· ready', { timeout: 30000 })
  const durableDownloadURL = await durableExport.getByRole('link', { name: 'Download CSV' }).getAttribute('href')
  const durableResponse = await page.context().request.get(durableDownloadURL)
  expect(durableResponse.status()).toBe(200)
  expect(durableResponse.headers()['content-type']).toContain('text/csv')
  expect(durableResponse.headers()['cache-control']).toContain('no-store')
  expect(durableResponse.headers()['x-content-sha256']).toMatch(/^[a-f0-9]{64}$/)
  expect(await durableResponse.text()).toContain(`avery-${runID}@example.test`)
  const durableExportAccessibility = await new AxeBuilder({ page }).include('.crm-export-card').withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa']).analyze()
  expect(durableExportAccessibility.violations, 'durable CRM export controls must have no automated WCAG A/AA violations').toEqual([])

  await page.goto('/settings/audit')
  await expect(page.getByRole('heading', { name: 'Admin audit trail' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Retention and export' })).toBeVisible()
  await expect(page.getByText('Audit events are immutable and retained for the workspace lifetime.', { exact: true })).toBeVisible()
  await expect(page.getByText('Append-only history is in the workspace export', { exact: false })).toBeVisible()
  const auditExport = await page.context().request.get(`${apiURL}/api/audit-events/export.csv?eventType=user.invited`)
  expect(auditExport.status()).toBe(200)
  expect(auditExport.headers()['content-type']).toContain('text/csv')
  expect(auditExport.headers()['content-disposition']).toMatch(/^attachment; filename="audit-events-\d{8}\.csv"$/)
  expect(auditExport.headers()['cache-control']).toContain('no-store')
  const auditCSV = await auditExport.text()
  expect(auditCSV).toContain('event_type')
  expect(auditCSV).toContain('user.invited')
  const roleAuditExport = await page.context().request.get(`${apiURL}/api/audit-events/export.csv?eventType=user.role_changed`)
  expect(roleAuditExport.status()).toBe(200)
  const roleAuditCSV = await roleAuditExport.text()
  expect(roleAuditCSV).toContain('user.role_changed')
  expect(roleAuditCSV).toContain(invitedEmail)
  expect(roleAuditCSV).toContain('previousRole')
  expect(roleAuditCSV).toContain('member')
  expect(roleAuditCSV).toContain('admin')
  await page.getByRole('button', { name: 'Refresh audit' }).click()
  const auditExportEvents = page.getByRole('list', { name: 'Admin audit events' }).getByText('Downloaded audit event CSV', { exact: true })
  await expect(auditExportEvents).toHaveCount(2)
  await expect(auditExportEvents.first()).toBeVisible()

  await page.goto('/settings/billing')
  await expect(page.getByRole('heading', { name: 'Portable workspace export' })).toBeVisible()
  await page.getByRole('button', { name: 'Create workspace export' }).click()
  const portableExport = page.getByRole('list', { name: 'Workspace export history' }).getByRole('listitem').first()
  await expect(portableExport).toContainText('· ready', { timeout: 30000 })
  await expect(portableExport).toContainText('SHA-256:')
  const portableDownloadURL = await portableExport.getByRole('link', { name: 'Download ZIP' }).getAttribute('href')
  expect(portableDownloadURL).toMatch(/\/api\/workspace-exports\/\d+\/download$/)
  const portableResponse = await page.context().request.get(portableDownloadURL)
  expect(portableResponse.status()).toBe(200)
  expect(portableResponse.headers()['content-type']).toContain('application/zip')
  expect(portableResponse.headers()['cache-control']).toContain('no-store')
  expect(portableResponse.headers()['x-content-sha256']).toMatch(/^[a-f0-9]{64}$/)
  const portableArchive = await portableResponse.body()
  expect(portableArchive.subarray(0, 2).toString()).toBe('PK')

  await page.getByRole('button', { name: 'Log out' }).click()
  await expect(page).toHaveURL(/\/login$/)
  await page.getByLabel('Email').fill(owner.email)
  await page.getByLabel('Password').fill(owner.password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page).toHaveURL(/\/dashboard$/)
  await expect(page.getByText(owner.organizationName, { exact: true })).toBeVisible()

  const ownerDeviceContext = await browser.newContext()
  const ownerDevicePage = await ownerDeviceContext.newPage()
  await ownerDevicePage.goto('/login')
  await ownerDevicePage.getByLabel('Email').fill(owner.email)
  await ownerDevicePage.getByLabel('Password').fill(owner.password)
  await ownerDevicePage.getByRole('button', { name: 'Sign in' }).click()
  await expect(ownerDevicePage).toHaveURL(/\/dashboard$/)

  await page.goto('/settings/profile')
  const activeSignIns = page.getByRole('list', { name: 'Active sign-ins' })
  await expect(activeSignIns.getByRole('listitem')).toHaveCount(2)
  await activeSignIns.getByRole('button', { name: 'Sign out', exact: true }).click()
  await activeSignIns.getByRole('button', { name: 'Confirm sign out' }).click()
  await expect(page.getByText('That sign-in has been ended.', { exact: true })).toBeVisible()
  await expect(activeSignIns.getByRole('listitem')).toHaveCount(1)
  const manuallyRevokedOwnerDevice = await ownerDeviceContext.request.get(`${apiURL}/auth/me`)
  expect(manuallyRevokedOwnerDevice.status()).toBe(401)

  await ownerDevicePage.goto('/login')
  await ownerDevicePage.getByLabel('Email').fill(owner.email)
  await ownerDevicePage.getByLabel('Password').fill(owner.password)
  await ownerDevicePage.getByRole('button', { name: 'Sign in' }).click()
  await expect(ownerDevicePage).toHaveURL(/\/dashboard$/)

  const resetPassword = 'Owner-Recovered-Password-31!'
  await page.goto('/forgot-password')
  await expect(page.getByRole('heading', { name: 'Reset your password' })).toBeVisible()
  await page.getByLabel('Email').fill(owner.email)
  await page.getByRole('button', { name: 'Send reset link' }).click()
  await expect(page.getByRole('heading', { name: 'Check your email' })).toBeVisible()
  await page.getByRole('link', { name: 'Reset password locally' }).click()
  await expect(page.getByRole('heading', { name: 'Choose a new password' })).toBeVisible()
  await page.getByLabel('New password', { exact: true }).fill(resetPassword)
  await page.getByLabel('Confirm new password').fill(resetPassword)
  await page.getByRole('button', { name: 'Reset password' }).click()
  await expect(page.getByRole('heading', { name: 'Password reset complete' })).toBeVisible()
  const invalidatedOwnerDevice = await ownerDeviceContext.request.get(`${apiURL}/auth/me`)
  expect(invalidatedOwnerDevice.status()).toBe(401)
  const rejectedOldPassword = await ownerDeviceContext.request.post(`${apiURL}/auth/login`, {
    data: { email: owner.email, password: owner.password }
  })
  expect(rejectedOldPassword.status()).toBe(401)
  await ownerDeviceContext.close()

  await page.getByRole('link', { name: 'Sign in' }).click()
  await page.getByLabel('Email').fill(owner.email)
  await page.getByLabel('Password').fill(resetPassword)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page).toHaveURL(/\/dashboard$/)
  await expect(page.getByText(owner.organizationName, { exact: true })).toBeVisible()

  const otherContext = await browser.newContext()
  try {
    const otherPage = await otherContext.newPage()
    await bootstrapWorkspace(otherPage, runID, 'Other')
    const createResponse = await otherContext.request.post(`${apiURL}/api/contacts`, {
      data: {
        firstName: 'Tenant',
        lastName: 'Secret',
        email: `tenant-secret-${runID}@example.test`,
        status: 'lead'
      }
    })
    expect(createResponse.status()).toBe(201)
    const created = await createResponse.json()
    const otherContactID = created.data.contact.id

    const ownTenantResponse = await otherContext.request.get(`${apiURL}/api/contacts/${otherContactID}`)
    expect(ownTenantResponse.status()).toBe(200)

    const crossTenantResponse = await page.context().request.get(`${apiURL}/api/contacts/${otherContactID}`)
    expect(crossTenantResponse.status()).toBe(404)
    const crossTenantFollowers = await page.context().request.get(`${apiURL}/api/record-followers?entityType=contact&entityId=${otherContactID}`)
    expect(crossTenantFollowers.status()).toBe(404)
    const crossTenantTouchpoints = await page.context().request.get(`${apiURL}/api/touchpoints/contact/${otherContactID}`)
    expect(crossTenantTouchpoints.status()).toBe(404)
    const crossTenantQuote = await otherContext.request.get(`${apiURL}/api/deals/${configuredDealID}/quote.pdf`)
    expect(crossTenantQuote.status()).toBe(404)
    const crossTenantFinalizedQuote = await otherContext.request.get(`${apiURL}/api/deals/${configuredDealID}/quotes/${finalizedQuoteID}/pdf`)
    expect(crossTenantFinalizedQuote.status()).toBe(404)
    const crossTenantClose = await otherContext.request.patch(`${apiURL}/api/deals/${configuredDealID}/stage`, {
      data: { stageId: 1, closeReasonCode: 'solution_fit', closeNotes: 'Must remain hidden.' }
    })
    expect(crossTenantClose.status()).toBe(404)
    const crossTenantPortableExport = await otherContext.request.get(portableDownloadURL)
    expect(crossTenantPortableExport.status()).toBe(404)
    const crossTenantSavedReport = await otherContext.request.get(`${apiURL}/api/report-definitions/${savedReportID}/results`)
    expect(crossTenantSavedReport.status()).toBe(404)
    const crossTenantSavedReportExport = await otherContext.request.get(`${apiURL}/api/report-definitions/${savedReportID}/export.csv`)
    expect(crossTenantSavedReportExport.status()).toBe(404)
    const crossTenantBarReport = await otherContext.request.get(`${apiURL}/api/report-definitions/${barReportID}/results`)
    expect(crossTenantBarReport.status()).toBe(404)
    const crossTenantBarReportExport = await otherContext.request.get(`${apiURL}/api/report-definitions/${barReportID}/export.csv`)
    expect(crossTenantBarReportExport.status()).toBe(404)
    const otherSchedulesResponse = await otherContext.request.get(`${apiURL}/api/report-schedules`)
    expect(otherSchedulesResponse.status()).toBe(200)
    const otherSchedules = await otherSchedulesResponse.json()
    expect(otherSchedules.data.schedules).toEqual([])
    expect(otherSchedules.data.deliveryRuns).toEqual([])
    const crossTenantSchedule = await otherContext.request.put(`${apiURL}/api/report-definitions/${savedReportID}/schedule`, {
      data: { revision: 0, cadence: 'daily', weekdayUtc: null, hourUtc: 9, recipientUserIds: [1], isActive: true }
    })
    expect(crossTenantSchedule.status()).toBe(404)
    const crossTenantScheduleRecovery = await otherContext.request.post(`${apiURL}/api/report-recipient-deliveries/${scheduledRecipientDeliveryID}/resolve`, {
      data: { resolution: 'confirmed_sent', confirmDuplicateRisk: false }
    })
    expect(crossTenantScheduleRecovery.status()).toBe(404)
    const otherDashboardResponse = await otherContext.request.get(`${apiURL}/api/report-dashboard`)
    expect(otherDashboardResponse.status()).toBe(200)
    const otherDashboard = await otherDashboardResponse.json()
    expect(otherDashboard.data.dashboard.widgets).toEqual([])
    const crossTenantDashboardUpdate = await otherContext.request.put(`${apiURL}/api/report-dashboard`, {
      data: { revision: otherDashboard.data.dashboard.revision, widgets: [{ reportDefinitionId: barReportID, width: 'half' }] }
    })
    expect(crossTenantDashboardUpdate.status()).toBe(400)
    const crossTenantDashboardResults = await otherContext.request.get(`${apiURL}/api/report-dashboard/results`)
    expect(crossTenantDashboardResults.status()).toBe(200)
    const crossTenantDashboardResultsBody = await crossTenantDashboardResults.json()
    expect(crossTenantDashboardResultsBody.data.widgets).toEqual([])
    expect(JSON.stringify(crossTenantDashboardResultsBody)).not.toContain(barReportName)
    const crossTenantClientActivity = await otherContext.request.get(`${apiURL}/api/reports/client-activity?entityType=company&from=${utcDateDaysFromNow(-29)}&to=${utcDateDaysFromNow(0)}`)
    expect(crossTenantClientActivity.status()).toBe(200)
    const crossTenantClientActivityBody = await crossTenantClientActivity.json()
    expect(crossTenantClientActivityBody.data.count).toBe(0)
    expect(crossTenantClientActivityBody.data.totals.totalClients).toBe(0)
    expect(JSON.stringify(crossTenantClientActivityBody)).not.toContain(`Northstar Advisory ${runID}`)
    const crossTenantFunnel = await otherContext.request.get(`${apiURL}/api/reports/pipeline-funnel?pipelineId=${funnelPipelineID}&entryStageId=${funnelEntryStageID}&from=${utcDateDaysFromNow(-29)}&to=${utcDateDaysFromNow(0)}&asOf=${utcDateDaysFromNow(0)}`)
    expect(crossTenantFunnel.status()).toBe(400)
  } finally {
    await otherContext.close()
  }
})
