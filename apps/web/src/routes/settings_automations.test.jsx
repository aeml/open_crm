import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'
import { activeRunRefreshDelay } from '../lib/workflow_automation_polling'

afterEach(() => vi.unstubAllGlobals())

function jsonResponse(payload, status = 200) {
  return { ok: status < 400, status, json: async () => payload }
}

function sessionResponse() {
  return jsonResponse({ data: { user: { id: 1, email: 'owner@acme.test' }, organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' }, membership: { role: 'owner' } } })
}

describe('settings task automations route', () => {
  it('polls due work promptly without hot-polling future scheduled runs', () => {
    const now = Date.parse('2026-07-21T12:00:00Z')
    expect(activeRunRefreshDelay([{ status: 'succeeded' }], now)).toBeNull()
    expect(activeRunRefreshDelay([{ status: 'running' }], now)).toBe(1000)
    expect(activeRunRefreshDelay([{ status: 'queued' }], now)).toBe(1000)
    expect(activeRunRefreshDelay([{ status: 'queued', scheduledAt: '2026-07-21T12:00:02.500Z' }], now)).toBe(2500)
    expect(activeRunRefreshDelay([{ status: 'queued', scheduledAt: '2026-07-22T12:00:00Z' }], now)).toBe(60000)
  })

  it('hides non-executable foundations and creates a bounded stage task playbook', async () => {
    const createdRule = {
      id: 8,
      name: 'Proposal follow-up',
      triggerType: 'stage_changed',
      targetEntityType: 'deal',
      triggerConfig: { stageId: 12, conditionContract: 'deal_snapshot_v1', taskPlanContract: 'deal_task_plan_v1' },
      conditionLogic: 'all',
      conditions: [{ field: 'valueAmount', operator: 'greaterThan', value: '5000' }],
      actions: [
        { type: 'create_task', config: { title: 'Prepare proposal', description: 'Confirm scope.' }, delayMinutes: 2880 },
        { type: 'create_task', config: { title: 'Schedule decision review' }, delayMinutes: 7200 }
      ],
      isActive: true,
      position: 0
    }
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/deal-pipelines')) return jsonResponse({ data: { pipelines: [{ id: 3, name: 'Sales pipeline', stages: [{ id: 11, name: 'Discovery' }, { id: 12, name: 'Proposal' }] }] } })
      if (path.endsWith('/api/lead-capture-forms')) return jsonResponse({ data: { forms: [] } })
      if (path.endsWith('/api/users')) return jsonResponse({ data: { users: [] } })
      if (path.endsWith('/api/workflow-automation-runs')) return jsonResponse({ data: { runs: [{ id: 21, automationId: 5, automationName: 'Qualify new deals', triggerEventKey: 'deal:7:activity:90', status: 'succeeded', actionsTotal: 1, actionsCompleted: 1, createdAt: '2026-07-19T12:00:00Z' }] } })
      if (path.endsWith('/api/workflow-automations') && method === 'POST') return jsonResponse({ data: { automation: createdRule } }, 201)
      if (path.endsWith('/api/workflow-automations')) {
        return jsonResponse({ data: { automations: [
          { id: 5, name: 'Qualify new deals', triggerType: 'record_created', targetEntityType: 'deal', triggerConfig: {}, conditions: [], actions: [{ type: 'create_task', config: { title: 'Qualify deal' }, delayMinutes: 1440 }], isActive: true },
          { id: 6, name: 'Legacy email action', triggerType: 'record_created', targetEntityType: 'contact', triggerConfig: {}, conditions: [], actions: [{ type: 'send_email', config: { subject: 'Welcome', body: 'Hello' } }], isActive: true },
          { id: 7, name: 'Unsupported multi-condition lead rule', triggerType: 'form_submitted', targetEntityType: 'lead_form', triggerConfig: {}, conditionLogic: 'all', conditions: [{ field: 'utmSource', operator: 'equals', value: 'partner' }, { field: 'utmMedium', operator: 'equals', value: 'paid' }], actions: [{ type: 'create_task', config: { title: 'Call partner lead', assignedToUserId: 7 } }], isActive: true },
          { id: 10, name: 'Legacy deal condition', triggerType: 'stage_changed', targetEntityType: 'deal', triggerConfig: { stageId: 12 }, conditionLogic: 'all', conditions: [{ field: 'valueAmount', operator: 'greaterThan', value: '5000' }], actions: [{ type: 'create_task', config: { title: 'Must remain hidden' } }], isActive: true }
        ] } })
      }
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/automations')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /task automation rules/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: 'Qualify new deals' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Legacy email action' })).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Unsupported multi-condition lead rule' })).not.toBeInTheDocument()
    expect(screen.getByText(/3 unsupported stored definitions hidden/i)).toBeInTheDocument()
    expect(screen.getByRole('list', { name: 'Task automation runs' })).toHaveTextContent('1/1 tasks created')

    fireEvent.change(screen.getByLabelText('Rule name'), { target: { value: 'Proposal follow-up' } })
    fireEvent.change(screen.getByLabelText('When'), { target: { value: 'stage_changed' } })
    fireEvent.change(screen.getByLabelText(/destination stage/i), { target: { value: '12' } })
    fireEvent.change(screen.getByLabelText(/optional deal condition/i), { target: { value: 'valueAmount' } })
    fireEvent.change(screen.getByLabelText('Deal condition value'), { target: { value: '5000' } })
    fireEvent.change(screen.getByLabelText('Task title'), { target: { value: 'Prepare proposal' } })
    fireEvent.change(screen.getByLabelText('Task description'), { target: { value: 'Confirm scope.' } })
    fireEvent.change(screen.getByLabelText('Due in days', { exact: false }), { target: { value: '2' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add another task' }))
    fireEvent.change(screen.getByLabelText('Task 2 title'), { target: { value: 'Schedule decision review' } })
    fireEvent.change(screen.getByLabelText('Task 2 due in days', { exact: false }), { target: { value: '5' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create task rule' }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/workflow-automations') && call[1]?.method === 'POST')
      expect(JSON.parse(createCall[1].body)).toEqual({
        name: 'Proposal follow-up',
        description: 'Creates 2 assigned follow-up tasks from a deal event.',
        triggerType: 'stage_changed',
        targetEntityType: 'deal',
        triggerConfig: { stageId: 12, conditionContract: 'deal_snapshot_v1', taskPlanContract: 'deal_task_plan_v1' },
        conditionLogic: 'all',
        conditions: [{ field: 'valueAmount', operator: 'greaterThan', value: '5000' }],
        actions: [
          { type: 'create_task', config: { title: 'Prepare proposal', description: 'Confirm scope.' }, delayMinutes: 2880 },
          { type: 'create_task', config: { title: 'Schedule decision review' }, delayMinutes: 7200 }
        ],
        isActive: true,
        position: 0
      })
    })
    expect(await screen.findByRole('heading', { name: 'Proposal follow-up' })).toBeInTheDocument()
    expect(screen.getByText('When moved to Sales pipeline · Proposal')).toBeInTheDocument()
    expect(screen.getByText(/only if value amount is greater than 5000/i)).toBeInTheDocument()
    expect(screen.getByText(/2-task playbook/i)).toBeInTheDocument()
    expect(screen.getByRole('list', { name: 'Proposal follow-up task plan' })).toHaveTextContent('Schedule decision review')
  })

  it('creates an executable durable lead follow-up rule with retained attribution conditions', async () => {
    const createdRule = {
      id: 9,
      name: 'Partner lead follow-up',
      triggerType: 'form_submitted',
      targetEntityType: 'lead_form',
      triggerConfig: { formId: 31 },
      conditionLogic: 'all',
      conditions: [{ field: 'utmSource', operator: 'equals', value: 'partner' }],
      actions: [{ type: 'create_task', config: { title: 'Call partner lead', assignedToUserId: 7, dueDays: 1 }, delayMinutes: 2880 }],
      isActive: true,
      position: 0
    }
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/deal-pipelines')) return jsonResponse({ data: { pipelines: [] } })
      if (path.endsWith('/api/lead-capture-forms')) return jsonResponse({ data: { forms: [{ id: 31, name: 'Partner inquiry', isActive: true }] } })
      if (path.endsWith('/api/users')) return jsonResponse({ data: { users: [{ id: 7, firstName: 'Riley', lastName: 'Chen', email: 'riley@example.test', status: 'active' }] } })
      if (path.endsWith('/api/workflow-automation-runs')) return jsonResponse({ data: { runs: [] } })
      if (path.endsWith('/api/workflow-automations') && method === 'POST') return jsonResponse({ data: { automation: createdRule } }, 201)
      if (path.endsWith('/api/workflow-automations')) return jsonResponse({ data: { automations: [] } })
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/automations')

    render(<AppRouter />)

    await screen.findByRole('heading', { name: /task automation rules/i })
    await screen.findByText('No executable task rules yet.')
    fireEvent.change(screen.getByLabelText('Rule name'), { target: { value: 'Partner lead follow-up' } })
    fireEvent.change(screen.getByLabelText('When'), { target: { value: 'lead_form_submitted' } })
    fireEvent.change(await screen.findByLabelText('Create task after days', { exact: false }), { target: { value: '2' } })
    fireEvent.change(screen.getByRole('combobox', { name: /^Lead form/ }), { target: { value: '31' } })
    fireEvent.change(screen.getByLabelText('Optional attribution condition'), { target: { value: 'utmSource' } })
    fireEvent.change(screen.getByLabelText('Condition value'), { target: { value: 'partner' } })
    fireEvent.change(screen.getByLabelText('Assign task to'), { target: { value: '7' } })
    fireEvent.change(screen.getByLabelText('Task title'), { target: { value: 'Call partner lead' } })
    fireEvent.change(screen.getByLabelText('Due in days', { exact: false }), { target: { value: '1' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create task rule' }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/workflow-automations') && call[1]?.method === 'POST')
      expect(JSON.parse(createCall[1].body)).toEqual({
        name: 'Partner lead follow-up',
        description: 'Creates one durable assigned follow-up task from an accepted lead form submission.',
        triggerType: 'form_submitted',
        targetEntityType: 'lead_form',
        triggerConfig: { formId: 31 },
        conditionLogic: 'all',
        conditions: [{ field: 'utmSource', operator: 'equals', value: 'partner' }],
        actions: [{ type: 'create_task', config: { title: 'Call partner lead', assignedToUserId: 7, dueDays: 1 }, delayMinutes: 2880 }],
        isActive: true,
        position: 0
      })
    })
    expect(await screen.findByRole('heading', { name: 'Partner lead follow-up' })).toBeInTheDocument()
    expect(screen.getByText('When Partner inquiry is submitted')).toBeInTheDocument()
    expect(screen.getByText(/create after 2 days · due 1 day later/i)).toBeInTheDocument()
    expect(screen.getByText(/assign to Riley Chen/i)).toBeInTheDocument()
  })

  it('refreshes an active durable run until its terminal task evidence is visible', async () => {
    const rule = {
      id: 9,
      name: 'Inbound lead follow-up',
      triggerType: 'form_submitted',
      targetEntityType: 'lead_form',
      triggerConfig: { formId: 31 },
      conditionLogic: 'all',
      conditions: [],
      actions: [{ type: 'create_task', config: { title: 'Call inbound lead', assignedToUserId: 7, dueDays: 0 }, delayMinutes: 0 }],
      isActive: true
    }
    let runReads = 0
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      const path = new URL(String(url), 'http://localhost').pathname
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/deal-pipelines')) return jsonResponse({ data: { pipelines: [] } })
      if (path.endsWith('/api/lead-capture-forms')) return jsonResponse({ data: { forms: [{ id: 31, name: 'Website', isActive: true }] } })
      if (path.endsWith('/api/users')) return jsonResponse({ data: { users: [{ id: 7, firstName: 'Riley', lastName: 'Chen', status: 'active' }] } })
      if (path.endsWith('/api/workflow-automations')) return jsonResponse({ data: { automations: [rule] } })
      if (path.endsWith('/api/workflow-automation-runs')) {
        runReads += 1
        return jsonResponse({ data: { runs: [{ id: 44, automationId: 9, automationName: rule.name, status: runReads === 1 ? 'queued' : 'succeeded', actionsTotal: 1, actionsCompleted: runReads === 1 ? 0 : 1, scheduledAt: '2026-07-21T12:00:00Z', createdAt: '2026-07-21T12:00:00Z' }] } })
      }
      throw new Error(`Unexpected fetch: ${path}`)
    }))
    window.history.pushState({}, '', '/settings/automations')

    render(<AppRouter />)

    expect(await screen.findByText('queued', { exact: true })).toBeInTheDocument()
    expect(await screen.findByText('succeeded', { exact: true }, { timeout: 3000 })).toBeInTheDocument()
    expect(screen.getByText(/1\/1 tasks created/)).toBeInTheDocument()
    expect(runReads).toBeGreaterThanOrEqual(2)
  })
})
