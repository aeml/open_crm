import AxeBuilder from '@axe-core/playwright'
import { expect, test } from '@playwright/test'
import { execFileSync } from 'node:child_process'

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
  test.setTimeout(120_000)
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

  await customFieldForm.getByRole('combobox', { name: 'Record type', exact: true }).selectOption('company')
  await customFieldForm.getByRole('textbox', { name: 'Label', exact: true }).fill('Service tier')
  await customFieldForm.getByRole('combobox', { name: 'Type', exact: true }).selectOption('select')
  await customFieldForm.getByRole('textbox', { name: 'Options', exact: false }).fill('Gold, Silver')
  await customFieldForm.getByLabel('Required when a record is created or edited').check()
  await customFieldForm.getByLabel('Show in record lists').check()
  await customFieldForm.getByRole('button', { name: 'Create field' }).click()
  await expect(page.getByText('created with stable key custom:service_tier', { exact: false })).toBeVisible()

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

  seedLeadReviewContinuation(owner.email, runID)

  const leadFollowUpRuleName = `Inbound lead follow-up ${runID}`
  const leadFollowUpTaskTitle = `Review inbound lead ${runID}`
  await page.getByRole('link', { name: 'Automations', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Task automation rules' })).toBeVisible()
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
  await page.getByRole('button', { name: 'Create task rule' }).click()
  await expect(page.getByRole('heading', { name: leadFollowUpRuleName })).toBeVisible()
	await expect(page.getByText('1 of 50 active task actions allocated. Each task in a playbook uses one slot.')).toBeVisible()

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
  await publicLeadPage.getByRole('checkbox', { name: publicLeadConsent, exact: true }).check()
  await publicLeadPage.getByRole('button', { name: 'Request follow-up' }).click()
  await expect(publicLeadPage.getByText('Thanks. We will be in touch soon.', { exact: true })).toBeVisible()
  await publicLeadContext.close()

  await page.getByRole('link', { name: 'Automations', exact: true }).click()
  const leadFollowUpRun = page.getByRole('list', { name: 'Task automation runs' }).getByRole('listitem').filter({ hasText: leadFollowUpRuleName })
  await expect(leadFollowUpRun).toContainText('1/1 tasks created', { timeout: 30000 })
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
  await page.getByLabel('New stage name for Sales pipeline').fill(discoveryStage)
  await page.getByLabel('New stage probability for Sales pipeline', { exact: false }).fill('65')
  await page.getByRole('button', { name: 'Add stage' }).click()
  await expect(page.getByRole('button', { name: `Save ${discoveryStage}` })).toBeVisible()

  const automatedTaskTitle = `Qualify new deal ${runID}`
  const automatedDecisionTaskTitle = `Confirm decision date ${runID}`
  await page.getByRole('link', { name: 'Automations', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Task automation rules' })).toBeVisible()
  await page.getByLabel('Rule name').fill(`New deal qualification ${runID}`)
  await page.getByLabel(/Optional deal condition/i).selectOption('valueAmount')
  await page.getByLabel('Deal condition value').fill('20000')
  await page.getByLabel('Task title').fill(automatedTaskTitle)
  await page.getByLabel('Task description').fill('Confirm fit and agree the next step.')
  await page.getByLabel('Due in days', { exact: false }).fill('1')
  await page.getByRole('button', { name: 'Add another task' }).click()
  await page.getByLabel('Task 2 title').fill(automatedDecisionTaskTitle)
  await page.getByLabel('Task 2 description').fill('Set the next commercial decision checkpoint.')
  await page.getByLabel('Task 2 due in days').fill('3')
  const dealPlaybookAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa'])
    .analyze()
  await test.info().attach('axe-deal-task-playbook', {
    body: JSON.stringify({ url: page.url(), violations: dealPlaybookAccessibility.violations }, null, 2),
    contentType: 'application/json'
  })
  expect(dealPlaybookAccessibility.violations, 'deal task playbook authoring must have no automated WCAG A/AA violations').toEqual([])
  await page.getByRole('button', { name: 'Create task rule' }).click()
  await expect(page.getByRole('heading', { name: `New deal qualification ${runID}` })).toBeVisible()
  await expect(page.getByText('Only if value amount is greater than 20000', { exact: true })).toBeVisible()
  await expect(page.getByText(/2-task playbook/)).toBeVisible()
	await expect(page.getByText('3 of 50 active task actions allocated. Each task in a playbook uses one slot.')).toBeVisible()

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
  await expect(page.getByRole('list', { name: 'Deal tasks list' }).getByText(automatedTaskTitle)).toBeVisible()
  await expect(page.getByRole('list', { name: 'Deal tasks list' }).getByText(automatedDecisionTaskTitle)).toBeVisible()
  await page.getByRole('link', { name: 'Automations', exact: true }).click()
  const dealAutomationRun = page.getByRole('list', { name: 'Task automation runs' }).getByRole('listitem').filter({ hasText: `New deal qualification ${runID}` })
  await expect(dealAutomationRun).toContainText('2/2 tasks created')
  await dealAutomationRun.getByText('Inspect 2 action outcomes').click()
  const dealActionOutcomes = dealAutomationRun.getByRole('list', { name: `New deal qualification ${runID} run actions` })
  await expect(dealActionOutcomes).toContainText(`1. ${automatedTaskTitle}`)
  await expect(dealActionOutcomes).toContainText(`2. ${automatedDecisionTaskTitle}`)
  await expect(dealActionOutcomes.getByText('Action succeeded · 1 attempt')).toHaveCount(2)
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
  await expect(page.getByText('2 due within 24 hours', { exact: true })).toBeVisible()
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
  await expect(salesActivityCard.getByText('Complete event coverage', { exact: false })).toBeVisible()
  const salesTotals = salesActivityCard.getByRole('list', { name: 'Sales activity totals' })
  await expect(salesTotals.getByRole('listitem').filter({ hasText: 'Deals created' })).toContainText('1')
  await expect(salesTotals.getByRole('listitem').filter({ hasText: 'Tasks created' })).toContainText('5')
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
  const dealAssignmentNotification = memberPage.getByRole('listitem').filter({ hasText: `assigned a deal: Website renewal ${runID}` })
  await expect(dealAssignmentNotification).toBeVisible()
  await dealAssignmentNotification.getByRole('button', { name: 'Open record' }).click()
  await expect(memberPage).toHaveURL(new RegExp(`/deals/${configuredDealID}$`))
  await memberPage.goto('/notifications')
  const mentionNotification = memberPage.getByRole('listitem').filter({ hasText: `mentioned you on Website renewal ${runID}` })
  await expect(mentionNotification).toBeVisible()
  await expect(memberPage.getByRole('heading', { name: 'Activity digest' })).toBeVisible()
  await expect(memberPage.getByRole('list', { name: 'Activity digest' }).getByText(`Website renewal ${runID}: Note added`)).toBeVisible()
  await mentionNotification.getByRole('button', { name: 'Open record' }).click()
  await expect(memberPage).toHaveURL(/\/deals\/\d+$/)
  await memberPage.getByRole('button', { name: 'Followers' }).click()
  await expect(memberPage.getByRole('button', { name: 'Following' })).toBeVisible()
  await memberContext.close()

  await page.getByRole('link', { name: 'Reports', exact: true }).click()
  await expect(salesActivityCard.getByRole('list', { name: 'Sales activity totals' }).getByRole('listitem').filter({ hasText: 'Won outcomes' })).toContainText('1')
  const closeReasonReport = salesActivityCard.getByRole('list', { name: 'Win and loss reasons' })
  await expect(closeReasonReport.getByRole('listitem').filter({ hasText: 'Best solution fit' })).toContainText('1')
  await expect(salesActivityCard.getByRole('list', { name: 'Recent deal events' }).getByText('Strong service fit and a clear implementation plan.', { exact: false })).toBeVisible()

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
  await page.getByRole('button', { name: 'Refresh audit' }).click()
  await expect(page.getByRole('list', { name: 'Admin audit events' }).getByText('Downloaded audit event CSV', { exact: true })).toBeVisible()

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
