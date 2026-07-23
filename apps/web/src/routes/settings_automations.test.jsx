import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { AppRouter } from '../app/router'
import { activeRunRefreshDelay } from '../lib/workflow_automation_polling'

afterEach(() => vi.unstubAllGlobals())

function jsonResponse(payload, status = 200) {
  return { ok: status < 400, status, json: async () => payload }
}

function sessionResponse() {
  return jsonResponse({ data: { user: { id: 1, email: 'owner@acme.test' }, organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' }, membership: { role: 'owner' } } })
}

function workflowPage(automations, { page = 1, pageSize = 50, total = automations.length } = {}) {
  return jsonResponse({ data: { automations, meta: {
    page,
    pageSize,
    total,
    activeActionCount: automations.reduce((sum, automation) => sum + (automation.isActive ? (automation.actions || []).length : 0), 0)
  } } })
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
    let storedDefinitions = [
      { id: 5, name: 'Qualify new deals', triggerType: 'record_created', targetEntityType: 'deal', triggerConfig: {}, conditions: [], actions: [{ type: 'create_task', config: { title: 'Qualify deal' }, delayMinutes: 1440 }], isActive: true },
      { id: 6, name: 'Legacy email action', triggerType: 'record_created', targetEntityType: 'contact', triggerConfig: {}, conditionLogic: 'all', conditions: [], actions: [{ type: 'send_email', config: { subject: 'Welcome', body: 'Hello' } }], isActive: true },
      { id: 7, name: 'Unsupported multi-condition lead rule', triggerType: 'form_submitted', targetEntityType: 'lead_form', triggerConfig: {}, conditionLogic: 'all', conditions: [{ field: 'utmSource', operator: 'equals', value: 'partner' }, { field: 'utmMedium', operator: 'equals', value: 'paid' }], actions: [{ type: 'create_task', config: { title: 'Call partner lead', assignedToUserId: 7 } }], isActive: true },
      { id: 10, name: 'Legacy deal condition', triggerType: 'stage_changed', targetEntityType: 'deal', triggerConfig: { stageId: 12 }, conditionLogic: 'all', conditions: [{ field: 'valueAmount', operator: 'greaterThan', value: '5000' }], actions: [{ type: 'create_task', config: { title: 'Must remain hidden' } }], isActive: true }
    ]
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/deal-pipelines')) return jsonResponse({ data: { pipelines: [{ id: 3, name: 'Sales pipeline', stages: [{ id: 11, name: 'Discovery' }, { id: 12, name: 'Proposal' }] }] } })
      if (path.endsWith('/api/lead-capture-forms')) return jsonResponse({ data: { forms: [] } })
      if (path.endsWith('/api/users')) return jsonResponse({ data: { users: [] } })
      if (path.endsWith('/api/workflow-approvals')) return jsonResponse({ data: { approvals: [] } })
      if (path.endsWith('/api/workflow-automation-runs')) return jsonResponse({ data: { runs: [{ id: 21, automationId: 5, automationName: 'Qualify new deals', triggerEventKey: 'deal:7:activity:90', status: 'succeeded', actionsTotal: 1, actionsCompleted: 1, createdAt: '2026-07-19T12:00:00Z', actions: [{ id: 31, position: 1, type: 'create_task', label: 'Qualify deal', status: 'succeeded', attempts: 1, scheduledAt: '2026-07-19T12:00:00Z', completedAt: '2026-07-19T12:00:01Z', taskId: 88, taskDueAt: '2026-07-20T12:00:00Z', lastError: '' }] }] } })
      if (path.endsWith('/api/workflow-automations') && method === 'POST') {
        storedDefinitions = [createdRule, ...storedDefinitions]
        return jsonResponse({ data: { automation: createdRule } }, 201)
      }
      if (path.endsWith('/api/workflow-automations/6') && method === 'PATCH') {
        const updated = { ...storedDefinitions.find((automation) => automation.id === 6), isActive: false, position: 0 }
        storedDefinitions = storedDefinitions.map((automation) => automation.id === 6 ? updated : automation)
        return jsonResponse({ data: { automation: updated } })
      }
      if (path.endsWith('/api/workflow-automations')) return workflowPage(storedDefinitions)
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/automations')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /workflow automation rules/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: 'Qualify new deals' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Legacy email action' })).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Unsupported multi-condition lead rule' })).not.toBeInTheDocument()
    expect(screen.getByText(/showing 4 of 4 stored definitions/i)).toBeInTheDocument()
    expect(screen.getByText(/3 unsupported loaded definitions hidden/i)).toBeInTheDocument()
    expect(screen.getByText(/4 of 50 active workflow actions allocated/i)).toBeInTheDocument()
    const recoveryList = screen.getByRole('list', { name: 'Active unsupported workflow definitions' })
    const legacyRecovery = within(recoveryList).getByText('Legacy email action').closest('article')
    fireEvent.click(within(legacyRecovery).getByRole('button', { name: 'Deactivate stored definition' }))
    await waitFor(() => expect(screen.getByText(/Legacy email action deactivated/i)).toBeInTheDocument())
    const deactivateCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/workflow-automations/6') && call[1]?.method === 'PATCH')
    expect(JSON.parse(deactivateCall[1].body)).toEqual({ deactivateOnly: true })
    expect(screen.getByText(/3 of 50 active workflow actions allocated/i)).toBeInTheDocument()
    const runList = screen.getByRole('list', { name: 'Workflow automation runs' })
    expect(runList).toHaveTextContent('1/1 actions completed')
    fireEvent.click(within(runList).getByText('Inspect 1 action outcome'))
    const actionList = screen.getByRole('list', { name: 'Qualify new deals run actions' })
    expect(actionList).toHaveTextContent('1. Qualify deal')
    expect(actionList).toHaveTextContent('Action succeeded · 1 attempt')
    expect(within(actionList).getByRole('link', { name: 'Open created task' })).toHaveAttribute('href', '/tasks/88')

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
    fireEvent.click(screen.getByRole('button', { name: 'Create workflow rule' }))

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

  it('authors and inspects exact deal primary-contact sequence enrollment', async () => {
    const sequence = {
      id: 31,
      name: 'Proposal cadence',
      status: 'active',
      revision: 2,
      approvedRevision: 2,
      approvedAt: '2026-07-23T12:00:00Z'
    }
    const storedRule = {
      id: 41,
      name: 'Start proposal cadence',
      triggerType: 'stage_changed',
      targetEntityType: 'deal',
      triggerConfig: { stageId: 12, actionPlanContract: 'deal_add_to_sequence_v1' },
      conditionLogic: 'all',
      conditions: [],
      actions: [{ type: 'add_to_sequence', config: { sequenceId: 31 } }],
      isActive: true,
      position: 0
    }
    const updatedRule = {
      ...storedRule,
      name: 'Start reviewed proposal cadence'
    }
    let storedDefinitions = [storedRule]
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/deal-pipelines')) return jsonResponse({ data: { pipelines: [{ id: 3, name: 'Sales pipeline', stages: [{ id: 12, name: 'Proposal' }] }] } })
      if (path.endsWith('/api/lead-capture-forms')) return jsonResponse({ data: { forms: [] } })
      if (path.endsWith('/api/users')) return jsonResponse({ data: { users: [] } })
      if (path.endsWith('/api/email-sequences')) return jsonResponse({ data: { sequences: [sequence, { ...sequence, id: 32, name: 'Unreviewed cadence', approvedRevision: 1 }], meta: { page: 1, pageSize: 50, total: 2 } } })
      if (path.endsWith('/api/workflow-approvals')) return jsonResponse({ data: { approvals: [] } })
      if (path.endsWith('/api/workflow-automation-runs')) return jsonResponse({ data: { runs: [{
        id: 51,
        automationId: 41,
        automationName: 'Start proposal cadence',
        triggerEventKey: 'deal:7:activity:91',
        status: 'succeeded',
        actionsTotal: 1,
        actionsCompleted: 1,
        createdAt: '2026-07-23T12:05:00Z',
        actions: [{
          id: 61,
          position: 1,
          type: 'add_to_sequence',
          label: 'Enroll primary contact in email sequence',
          status: 'succeeded',
          attempts: 1,
          scheduledAt: '2026-07-23T12:05:00Z',
          completedAt: '2026-07-23T12:05:00Z',
          sequenceId: 31,
          sequenceName: 'Proposal cadence',
          sequenceEnrollmentId: 71,
          sequenceContactId: 81,
          sequenceContactName: 'Pilot Buyer',
          sequenceEnrollmentCreated: true,
          lastError: ''
        }]
      }] } })
      if (path.endsWith('/api/workflow-automations/41') && method === 'PATCH') {
		const payload = JSON.parse(options.body)
		if (payload.deactivateOnly) {
			const deactivated = { ...storedDefinitions[0], isActive: false }
			storedDefinitions = [deactivated]
			return jsonResponse({ data: { automation: deactivated } })
		}
        storedDefinitions = [updatedRule]
        return jsonResponse({ data: { automation: updatedRule } })
      }
      if (path.endsWith('/api/workflow-automations')) return workflowPage(storedDefinitions)
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/automations')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: 'Start proposal cadence' })).toBeInTheDocument()
    expect(screen.getByText(/Enroll the primary contact in Proposal cadence using the current deal owner as sender/i)).toBeInTheDocument()
    expect(screen.queryByText('Unreviewed cadence')).not.toBeInTheDocument()
    const runList = screen.getByRole('list', { name: 'Workflow automation runs' })
    fireEvent.click(within(runList).getByText('Inspect 1 action outcome'))
    const actionList = screen.getByRole('list', { name: 'Start proposal cadence run actions' })
    expect(actionList).toHaveTextContent('Enrolled Pilot Buyer in Proposal cadence; the first delivery is queued durably.')
    expect(within(actionList).getByRole('link', { name: 'Open enrolled contact' })).toHaveAttribute('href', '/contacts/81')

    const storedRuleArticle = screen.getByRole('heading', { name: 'Start proposal cadence' }).closest('article')
    fireEvent.click(within(storedRuleArticle).getByRole('button', { name: 'Edit' }))
    expect(screen.getByLabelText('Outcome')).toHaveValue('add_to_sequence')
    expect(screen.queryByLabelText('Task title')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Assign deal owner to')).not.toBeInTheDocument()
    expect(screen.getByLabelText(/^Approved email sequence/)).toHaveValue('31')
    fireEvent.change(screen.getByLabelText('Rule name'), { target: { value: 'Start reviewed proposal cadence' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save workflow rule' }))

    await waitFor(() => {
      const updateCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/workflow-automations/41') && call[1]?.method === 'PATCH')
      expect(JSON.parse(updateCall[1].body)).toEqual({
        name: 'Start reviewed proposal cadence',
        description: 'Enrolls the current primary contact in one approved sequence using the active deal owner as sender.',
        triggerType: 'stage_changed',
        targetEntityType: 'deal',
        triggerConfig: { stageId: 12, actionPlanContract: 'deal_add_to_sequence_v1' },
        conditionLogic: 'all',
        conditions: [],
        actions: [{ type: 'add_to_sequence', config: { sequenceId: 31 } }],
        isActive: true,
        position: 0
      })
    })
    expect(await screen.findByRole('heading', { name: 'Start reviewed proposal cadence' })).toBeInTheDocument()
	const updatedRuleArticle = screen.getByRole('heading', { name: 'Start reviewed proposal cadence' }).closest('article')
	fireEvent.click(within(updatedRuleArticle).getByRole('button', { name: 'Deactivate rule' }))
	await waitFor(() => expect(screen.getByText(/Start reviewed proposal cadence deactivated/i)).toBeInTheDocument())
	const deactivationCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/workflow-automations/41') && call[1]?.method === 'PATCH' && JSON.parse(call[1].body).deactivateOnly)
	expect(JSON.parse(deactivationCall[1].body)).toEqual({ deactivateOnly: true })
  })

  it('authors and inspects an exact transactional expected-close-date update', async () => {
    const storedRule = {
      id: 42,
      name: 'Set proposal decision date',
      triggerType: 'stage_changed',
      targetEntityType: 'deal',
      triggerConfig: { stageId: 12, actionPlanContract: 'deal_set_expected_close_v1' },
      conditionLogic: 'all',
      conditions: [],
      actions: [{ type: 'update_field', config: { field: 'expectedCloseDate', value: 30 } }],
      isActive: true,
      position: 0
    }
    let storedDefinitions = [storedRule]
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/deal-pipelines')) return jsonResponse({ data: { pipelines: [{ id: 3, name: 'Sales pipeline', stages: [{ id: 12, name: 'Proposal' }] }] } })
      if (path.endsWith('/api/lead-capture-forms')) return jsonResponse({ data: { forms: [] } })
      if (path.endsWith('/api/users')) return jsonResponse({ data: { users: [] } })
      if (path.endsWith('/api/email-sequences')) return jsonResponse({ data: { sequences: [], meta: { page: 1, pageSize: 50, total: 0 } } })
      if (path.endsWith('/api/workflow-approvals')) return jsonResponse({ data: { approvals: [] } })
      if (path.endsWith('/api/workflow-automation-runs')) return jsonResponse({ data: { runs: [{
        id: 52,
        automationId: 42,
        automationName: 'Set proposal decision date',
        triggerEventKey: 'deal:7:activity:92',
        status: 'succeeded',
        actionsTotal: 1,
        actionsCompleted: 1,
        createdAt: '2026-07-23T12:05:00Z',
        actions: [{
          id: 62,
          position: 1,
          type: 'update_field',
          label: 'Set expected close date',
          status: 'succeeded',
          attempts: 1,
          scheduledAt: '2026-07-23T12:05:00Z',
          completedAt: '2026-07-23T12:05:00Z',
          updatedField: 'expectedCloseDate',
          previousValue: '2026-07-31',
          currentValue: '2026-08-22',
          fieldValueChanged: true,
          lastError: ''
        }]
      }] } })
      if (path.endsWith('/api/workflow-automations/42') && method === 'PATCH') {
        const payload = JSON.parse(options.body)
        const updated = { ...storedRule, ...payload }
        storedDefinitions = [updated]
        return jsonResponse({ data: { automation: updated } })
      }
      if (path.endsWith('/api/workflow-automations')) return workflowPage(storedDefinitions)
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/automations')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: 'Set proposal decision date' })).toBeInTheDocument()
    expect(screen.getByText('One transactional expected-close update · exact no-op evidence is retained.')).toBeInTheDocument()
    expect(screen.getByText('Set expected close to 30 days from the triggering event.')).toBeInTheDocument()
    const runList = screen.getByRole('list', { name: 'Workflow automation runs' })
    fireEvent.click(within(runList).getByText('Inspect 1 action outcome'))
    const actionList = screen.getByRole('list', { name: 'Set proposal decision date run actions' })
    expect(actionList).toHaveTextContent('Expected close changed from 2026-07-31 to 2026-08-22.')

    const ruleArticle = screen.getByRole('heading', { name: 'Set proposal decision date' }).closest('article')
    fireEvent.click(within(ruleArticle).getByRole('button', { name: 'Edit' }))
    expect(screen.getByLabelText('Outcome')).toHaveValue('set_expected_close')
    expect(screen.queryByLabelText('Task title')).not.toBeInTheDocument()
    expect(screen.getByLabelText(/^Expected close in days/)).toHaveValue(30)
    fireEvent.change(screen.getByLabelText(/^Expected close in days/), { target: { value: '45' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save workflow rule' }))

    await waitFor(() => {
      const updateCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/workflow-automations/42') && call[1]?.method === 'PATCH')
      expect(JSON.parse(updateCall[1].body)).toEqual({
        name: 'Set proposal decision date',
        description: 'Sets the expected close date to a reviewed whole-day offset from the triggering deal event.',
        triggerType: 'stage_changed',
        targetEntityType: 'deal',
        triggerConfig: { stageId: 12, actionPlanContract: 'deal_set_expected_close_v1' },
        conditionLogic: 'all',
        conditions: [],
        actions: [{ type: 'update_field', config: { field: 'expectedCloseDate', value: 45 } }],
        isActive: true,
        position: 0
      })
    })
    expect(await screen.findByText('Set expected close to 45 days from the triggering event.')).toBeInTheDocument()
  })

  it('creates an executable durable lead follow-up rule with retained attribution conditions', async () => {
    const createdRule = {
      id: 9,
      name: 'Partner lead follow-up',
      triggerType: 'form_submitted',
      targetEntityType: 'lead_form',
    triggerConfig: { taskContract: 'lead_follow_up_task_v1', formId: 31 },
      conditionLogic: 'all',
      conditions: [{ field: 'utmSource', operator: 'equals', value: 'partner' }],
      actions: [{ type: 'create_task', config: { title: 'Call partner lead', assignedToUserId: 7, dueDays: 1 }, delayMinutes: 2880 }],
      isActive: true,
      position: 0
    }
    let storedDefinitions = []
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/deal-pipelines')) return jsonResponse({ data: { pipelines: [] } })
      if (path.endsWith('/api/lead-capture-forms')) return jsonResponse({ data: { forms: [{ id: 31, name: 'Partner inquiry', isActive: true }] } })
      if (path.endsWith('/api/users')) return jsonResponse({ data: { users: [{ id: 7, firstName: 'Riley', lastName: 'Chen', email: 'riley@example.test', status: 'active' }] } })
      if (path.endsWith('/api/workflow-approvals')) return jsonResponse({ data: { approvals: [] } })
      if (path.endsWith('/api/workflow-automation-runs')) return jsonResponse({ data: { runs: [] } })
      if (path.endsWith('/api/workflow-automations') && method === 'POST') {
        storedDefinitions = [createdRule]
        return jsonResponse({ data: { automation: createdRule } }, 201)
      }
      if (path.endsWith('/api/workflow-automations')) return workflowPage(storedDefinitions)
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/automations')

    render(<AppRouter />)

    await screen.findByRole('heading', { name: /workflow automation rules/i })
    await screen.findByText('No executable workflow rules yet.')
    fireEvent.change(screen.getByLabelText('Rule name'), { target: { value: 'Partner lead follow-up' } })
    fireEvent.change(screen.getByLabelText('When'), { target: { value: 'lead_form_submitted' } })
    fireEvent.change(await screen.findByLabelText('Create task after days', { exact: false }), { target: { value: '2' } })
    fireEvent.change(screen.getByRole('combobox', { name: /^Lead form/ }), { target: { value: '31' } })
    fireEvent.change(screen.getByLabelText('Optional attribution condition'), { target: { value: 'utmSource' } })
    fireEvent.change(screen.getByLabelText('Condition value'), { target: { value: 'partner' } })
    fireEvent.change(screen.getByLabelText('Assign task to'), { target: { value: '7' } })
    fireEvent.change(screen.getByLabelText('Task title'), { target: { value: 'Call partner lead' } })
    fireEvent.change(screen.getByLabelText('Due in days', { exact: false }), { target: { value: '1' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create workflow rule' }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/workflow-automations') && call[1]?.method === 'POST')
      expect(JSON.parse(createCall[1].body)).toEqual({
        name: 'Partner lead follow-up',
        description: 'Creates one durable assigned follow-up task from an accepted lead form submission.',
        triggerType: 'form_submitted',
        targetEntityType: 'lead_form',
    triggerConfig: { taskContract: 'lead_follow_up_task_v1', formId: 31 },
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

  it('authors and decides a retained approval-gated deal task plan', async () => {
    let approvals = [{
      id: 77,
      runId: 71,
      automationId: 17,
      automationName: 'Approved proposal playbook',
      dealId: 27,
      dealName: 'Acme renewal',
      actionPosition: 1,
      name: 'Proposal readiness',
      approverRole: 'owner',
      message: 'Confirm scope before creating tasks.',
      status: 'pending',
      pendingTaskCount: 2,
      requestedByUserId: 9,
      requestedByUserName: 'Ari Requester',
      requestedAt: '2026-07-22T20:00:00Z'
    }]
    let storedDefinitions = []
    let runLoads = 0
    const createdRule = {
      id: 18,
      name: 'Approved renewal tasks',
      triggerType: 'record_created',
      targetEntityType: 'deal',
      triggerConfig: { taskPlanContract: 'deal_approval_task_plan_v1' },
      conditionLogic: 'all',
      conditions: [],
      actions: [
        { type: 'request_approval', config: { approvalName: 'Renewal readiness', approverRole: 'record_owner', message: 'Verify renewal scope.' } },
        { type: 'create_task', config: { title: 'Prepare renewal' }, delayMinutes: 1440 }
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
      if (path.endsWith('/api/deal-pipelines')) return jsonResponse({ data: { pipelines: [] } })
      if (path.endsWith('/api/lead-capture-forms')) return jsonResponse({ data: { forms: [] } })
      if (path.endsWith('/api/users')) return jsonResponse({ data: { users: [] } })
      if (path.endsWith('/api/workflow-automation-runs')) {
        runLoads += 1
        if (runLoads > 1) throw new Error('Run history unavailable')
        return jsonResponse({ data: { runs: [] } })
      }
      if (path.endsWith('/api/workflow-approvals/77/decision') && method === 'POST') {
        approvals = []
        return jsonResponse({ data: { approval: { id: 77, status: 'approved' } } })
      }
      if (path.endsWith('/api/workflow-approvals')) return jsonResponse({ data: { approvals } })
      if (path.endsWith('/api/workflow-automations') && method === 'POST') {
        storedDefinitions = [createdRule]
        return jsonResponse({ data: { automation: createdRule } }, 201)
      }
      if (path.endsWith('/api/workflow-automations')) return workflowPage(storedDefinitions)
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/automations')

    render(<AppRouter />)

    const approvalList = await screen.findByRole('list', { name: 'Pending workflow approvals' })
    expect(approvalList).toHaveTextContent('Proposal readiness')
    expect(approvalList).toHaveTextContent('2 tasks are created')
    fireEvent.click(within(approvalList).getByRole('button', { name: 'Approve and create tasks' }))
    expect(await screen.findByText('Workflow approved and its captured tasks were created.')).toBeInTheDocument()
    expect(screen.getByText('Decision saved. Reload to refresh run history.')).toBeInTheDocument()
    const decisionCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/workflow-approvals/77/decision'))
    expect(decisionCall[1].method).toBe('POST')
    expect(decisionCall[1].headers['Idempotency-Key']).toMatch(/^workflow-approval-/)
    expect(JSON.parse(decisionCall[1].body)).toEqual({ decision: 'approved', note: '' })
    expect(approvalList).toHaveTextContent('No workflow approvals need your decision.')

    fireEvent.change(screen.getByLabelText('Rule name'), { target: { value: 'Approved renewal tasks' } })
    fireEvent.click(screen.getByLabelText(/require a decision before creating any tasks/i))
    fireEvent.change(screen.getByLabelText('Approval name'), { target: { value: 'Renewal readiness' } })
    fireEvent.change(screen.getByLabelText('Who can approve'), { target: { value: 'record_owner' } })
    fireEvent.change(screen.getByLabelText('Reviewer guidance'), { target: { value: 'Verify renewal scope.' } })
    fireEvent.change(screen.getByLabelText('Task title'), { target: { value: 'Prepare renewal' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create workflow rule' }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/workflow-automations') && call[1]?.method === 'POST')
      expect(JSON.parse(createCall[1].body)).toEqual({
        name: 'Approved renewal tasks',
        description: 'Requests a human decision before creating 1 follow-up task from a deal event.',
        triggerType: 'record_created',
        targetEntityType: 'deal',
        triggerConfig: { taskPlanContract: 'deal_approval_task_plan_v1' },
        conditionLogic: 'all',
        conditions: [],
        actions: [
          { type: 'request_approval', config: { approvalName: 'Renewal readiness', approverRole: 'record_owner', message: 'Verify renewal scope.' } },
          { type: 'create_task', config: { title: 'Prepare renewal' }, delayMinutes: 1440 }
        ],
        isActive: true,
        position: 0
      })
    })
    expect(await screen.findByRole('heading', { name: 'Approved renewal tasks' })).toBeInTheDocument()
    expect(screen.getByText(/waits for a retained human decision/i)).toBeInTheDocument()
  })

  it('authors a bounded teammate notification and inspects delivery plus causal loop evidence', async () => {
    const createdRule = {
      id: 28,
      name: 'Notify proposal team',
      triggerType: 'record_created',
      targetEntityType: 'deal',
      triggerConfig: { taskPlanContract: 'deal_task_notify_plan_v1' },
      conditionLogic: 'all',
      conditions: [],
      actions: [
        { type: 'create_task', config: { title: 'Prepare proposal' }, delayMinutes: 1440 },
        { type: 'notify', config: { recipientRole: 'admin', message: 'Proposal preparation has started.' } }
      ],
      isActive: true,
      position: 0
    }
    const existingRule = { ...createdRule, id: 27, name: 'Existing notification rule' }
    let storedDefinitions = [existingRule]
    const runs = [
      {
        id: 81, automationId: 27, automationName: 'Existing notification rule', triggerEventKey: 'deal:4:root',
        status: 'succeeded', causalDepth: 0, actionsTotal: 2, actionsCompleted: 2,
        createdAt: '2026-07-22T21:00:00Z', triggerPayload: {}, actions: [
          { id: 91, position: 1, type: 'create_task', label: 'Prepare proposal', status: 'succeeded', attempts: 1, scheduledAt: '2026-07-22T21:00:00Z', taskId: 101, lastError: '' },
          { id: 92, position: 2, type: 'notify', label: 'Notify admin', status: 'succeeded', attempts: 1, scheduledAt: '2026-07-22T21:00:00Z', notificationCount: 2, lastError: '' }
        ]
      },
      {
        id: 82, automationId: 27, automationName: 'Existing notification rule', triggerEventKey: 'deal:4:nested',
        causationRunId: 81, causationActionPosition: 2, causalDepth: 1, status: 'skipped',
        actionsTotal: 2, actionsCompleted: 0, createdAt: '2026-07-22T21:01:00Z',
        triggerPayload: { skipReason: 'Automation re-entry prevented.' }, actions: [
          { id: 93, position: 1, type: 'create_task', label: 'Prepare proposal', status: 'skipped', attempts: 0, scheduledAt: '2026-07-22T21:01:00Z', lastError: 'Automation re-entry prevented.' },
          { id: 94, position: 2, type: 'notify', label: 'Notify admin', status: 'skipped', attempts: 0, scheduledAt: '2026-07-22T21:01:00Z', lastError: 'Automation re-entry prevented.' }
        ]
      }
    ]
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/deal-pipelines')) return jsonResponse({ data: { pipelines: [] } })
      if (path.endsWith('/api/lead-capture-forms')) return jsonResponse({ data: { forms: [] } })
      if (path.endsWith('/api/users')) return jsonResponse({ data: { users: [] } })
      if (path.endsWith('/api/workflow-approvals')) return jsonResponse({ data: { approvals: [] } })
      if (path.endsWith('/api/workflow-automation-runs')) return jsonResponse({ data: { runs } })
      if (path.endsWith('/api/workflow-automations') && method === 'POST') {
        storedDefinitions = [createdRule]
        return jsonResponse({ data: { automation: createdRule } }, 201)
      }
      if (path.endsWith('/api/workflow-automations')) return workflowPage(storedDefinitions)
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/automations')

    render(<AppRouter />)

    const runList = await screen.findByRole('list', { name: 'Workflow automation runs' })
    expect(runList).toHaveTextContent('Nested depth 1 · caused by run #81, action 2.')
    expect(runList).toHaveTextContent('Loop guard: Automation re-entry prevented.')
    expect(runList).toHaveTextContent('Delivered to 2 eligible teammates.')

    fireEvent.change(screen.getByLabelText('Rule name'), { target: { value: 'Notify proposal team' } })
    fireEvent.change(screen.getByLabelText('Task title'), { target: { value: 'Prepare proposal' } })
    fireEvent.click(screen.getByLabelText(/notify eligible teammates after every task commits/i))
    fireEvent.change(screen.getByLabelText('Notify'), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText('Notification message'), { target: { value: 'Proposal preparation has started.' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create workflow rule' }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/workflow-automations') && call[1]?.method === 'POST')
      expect(JSON.parse(createCall[1].body)).toEqual({
        name: 'Notify proposal team',
        description: 'Creates 1 follow-up task and then notifies eligible teammates in the same deal transaction.',
        triggerType: 'record_created',
        targetEntityType: 'deal',
        triggerConfig: { taskPlanContract: 'deal_task_notify_plan_v1' },
        conditionLogic: 'all',
        conditions: [],
        actions: [
          { type: 'create_task', config: { title: 'Prepare proposal' }, delayMinutes: 1440 },
          { type: 'notify', config: { recipientRole: 'admin', message: 'Proposal preparation has started.' } }
        ],
        isActive: true,
        position: 0
      })
    })
    expect(await screen.findByRole('heading', { name: 'Notify proposal team' })).toBeInTheDocument()
    expect(screen.getByText(/then notifies eligible teammates in the same transaction/i)).toBeInTheDocument()
    expect(screen.getByText(/notification: workspace owners and admins/i)).toBeInTheDocument()
  })

  it('authors an exact deal-owner assignment and inspects changed, no-op, and causal-limit evidence', async () => {
    const existingRule = {
      id: 37,
      name: 'Route changed deals',
      triggerType: 'record_updated',
      targetEntityType: 'deal',
      triggerConfig: { event: 'owner_changed', actionPlanContract: 'deal_assign_owner_v1' },
      conditionLogic: 'all',
      conditions: [],
      actions: [{ type: 'assign_owner', config: { userId: 8 } }],
      isActive: true,
      position: 0
    }
    const createdRule = {
      ...existingRule,
      id: 38,
      name: 'Assign created deals',
      triggerType: 'record_created',
      conditions: [{ field: 'status', operator: 'equals', value: 'open' }],
      triggerConfig: { actionPlanContract: 'deal_assign_owner_v1', conditionContract: 'deal_snapshot_v1' }
    }
    let storedDefinitions = [existingRule]
    const runs = [
      {
        id: 101, automationId: 37, automationName: existingRule.name, triggerEventKey: 'deal:4:owner-root',
        status: 'succeeded', causalDepth: 0, actionsTotal: 1, actionsCompleted: 1,
        createdAt: '2026-07-22T21:00:00Z', triggerPayload: {}, actions: [
          { id: 111, position: 1, type: 'assign_owner', label: 'Assign deal owner', status: 'succeeded', attempts: 1, scheduledAt: '2026-07-22T21:00:00Z', assignedUserId: 8, assignedUserName: 'Riley Chen', assignmentChanged: true, lastError: '' }
        ]
      },
      {
        id: 102, automationId: 37, automationName: existingRule.name, triggerEventKey: 'deal:4:owner-noop',
        status: 'succeeded', causalDepth: 0, actionsTotal: 1, actionsCompleted: 1,
        createdAt: '2026-07-22T21:01:00Z', triggerPayload: {}, actions: [
          { id: 112, position: 1, type: 'assign_owner', label: 'Assign deal owner', status: 'succeeded', attempts: 1, scheduledAt: '2026-07-22T21:01:00Z', assignedUserId: 8, assignedUserName: 'Riley Chen', assignmentChanged: false, lastError: '' }
        ]
      },
      {
        id: 103, automationId: 37, automationName: existingRule.name, triggerEventKey: 'deal:4:owner-limit',
        causationRunId: 101, causationActionPosition: 1, causalDepth: 1, status: 'skipped',
        actionsTotal: 1, actionsCompleted: 0, createdAt: '2026-07-22T21:02:00Z',
        triggerPayload: { skipReason: 'Workflow causal run limit reached.' }, actions: [
          { id: 113, position: 1, type: 'assign_owner', label: 'Assign deal owner', status: 'skipped', attempts: 0, scheduledAt: '2026-07-22T21:02:00Z', lastError: 'Workflow causal run limit reached.' }
        ]
      }
    ]
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/deal-pipelines')) return jsonResponse({ data: { pipelines: [] } })
      if (path.endsWith('/api/lead-capture-forms')) return jsonResponse({ data: { forms: [] } })
      if (path.endsWith('/api/users')) return jsonResponse({ data: { users: [
        { id: 7, firstName: 'Ari', lastName: 'Owner', email: 'ari@example.test', status: 'active' },
        { id: 8, firstName: 'Riley', lastName: 'Chen', email: 'riley@example.test', status: 'active' }
      ] } })
      if (path.endsWith('/api/workflow-approvals')) return jsonResponse({ data: { approvals: [] } })
      if (path.endsWith('/api/workflow-automation-runs')) return jsonResponse({ data: { runs } })
      if (path.endsWith('/api/workflow-automations') && method === 'POST') {
        storedDefinitions = [createdRule, ...storedDefinitions]
        return jsonResponse({ data: { automation: createdRule } }, 201)
      }
      if (path.endsWith('/api/workflow-automations')) return workflowPage(storedDefinitions)
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/automations')

    render(<AppRouter />)

    expect(await screen.findByText('After every direct deal owner change')).toBeInTheDocument()
    expect(screen.getByText('Assign deal to Riley Chen.')).toBeInTheDocument()
    const runList = screen.getByRole('list', { name: 'Workflow automation runs' })
    const changedRun = within(runList).getByText('deal:4:owner-root').closest('article')
    fireEvent.click(within(changedRun).getByText('Inspect 1 action outcome'))
    expect(within(changedRun).getByText('Assigned to Riley Chen.')).toBeInTheDocument()
    const noOpRun = within(runList).getByText('deal:4:owner-noop').closest('article')
    fireEvent.click(within(noOpRun).getByText('Inspect 1 action outcome'))
    expect(within(noOpRun).getByText('Already assigned to Riley Chen; no record change was needed.')).toBeInTheDocument()
    expect(runList).toHaveTextContent('Loop guard: Workflow causal run limit reached.')

    fireEvent.change(screen.getByLabelText('Rule name'), { target: { value: 'Assign created deals' } })
    fireEvent.change(screen.getByLabelText('Outcome'), { target: { value: 'assign_owner' } })
		fireEvent.change(screen.getByLabelText('When'), { target: { value: 'owner_changed' } })
		fireEvent.change(screen.getByLabelText('Outcome'), { target: { value: 'tasks' } })
		expect(screen.getByLabelText('When')).toHaveValue('created')
		fireEvent.change(screen.getByLabelText('Outcome'), { target: { value: 'assign_owner' } })
    fireEvent.change(screen.getByLabelText('Optional deal condition'), { target: { value: 'status' } })
    fireEvent.change(screen.getByLabelText('Deal condition value'), { target: { value: 'open' } })
    fireEvent.change(screen.getByLabelText('Assign deal owner to'), { target: { value: '8' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create workflow rule' }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/workflow-automations') && call[1]?.method === 'POST')
      expect(JSON.parse(createCall[1].body)).toEqual({
        name: 'Assign created deals',
        description: 'Assigns the deal to one active teammate and emits one causally bounded owner-change event.',
        triggerType: 'record_created',
        targetEntityType: 'deal',
        triggerConfig: { conditionContract: 'deal_snapshot_v1', actionPlanContract: 'deal_assign_owner_v1' },
        conditionLogic: 'all',
        conditions: [{ field: 'status', operator: 'equals', value: 'open' }],
        actions: [{ type: 'assign_owner', config: { userId: 8 } }],
        isActive: true,
        position: 0
      })
    })
    expect(await screen.findByRole('heading', { name: 'Assign created deals' })).toBeInTheDocument()
    expect(screen.getAllByText('Assign deal to Riley Chen.')).toHaveLength(2)
  })

  it('loads stored definition row 51 with exact continuation metadata', async () => {
    const definition = (id, name) => ({
      id,
      name,
      triggerType: 'record_created',
      targetEntityType: 'deal',
      triggerConfig: { taskPlanContract: 'deal_task_plan_v1' },
      conditionLogic: 'all',
      conditions: [],
      actions: [{ type: 'create_task', config: { title: `Task ${id}` } }],
      isActive: false,
      position: 0
    })
    const firstPage = Array.from({ length: 50 }, (_, index) => definition(index + 1, `Stored rule ${index + 1}`))
    const row51 = definition(51, 'Stored continuation rule')
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/deal-pipelines')) return jsonResponse({ data: { pipelines: [] } })
      if (path.endsWith('/api/lead-capture-forms')) return jsonResponse({ data: { forms: [] } })
      if (path.endsWith('/api/users')) return jsonResponse({ data: { users: [] } })
      if (path.endsWith('/api/workflow-approvals')) return jsonResponse({ data: { approvals: [] } })
      if (path.endsWith('/api/workflow-automation-runs')) return jsonResponse({ data: { runs: [] } })
      if (path.endsWith('/api/workflow-automations')) {
        const page = Number(requestURL.searchParams.get('page'))
        return workflowPage(page === 2 ? [row51] : firstPage, { page, total: 51 })
      }
      throw new Error(`Unexpected fetch: ${options.method || 'GET'} ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/automations')

    render(<AppRouter />)

    expect(await screen.findByText('Showing 50 of 51 stored definitions.')).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: row51.name })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Load more stored definitions' }))
    expect(await screen.findByRole('heading', { name: row51.name })).toBeInTheDocument()
    expect(screen.getByText('Showing 51 of 51 stored definitions.')).toBeInTheDocument()
    const continuationCall = fetchMock.mock.calls.find((call) => new URL(String(call[0]), 'http://localhost').searchParams.get('page') === '2')
    expect(new URL(String(continuationCall[0]), 'http://localhost').searchParams.get('pageSize')).toBe('50')
  })

  it('reconciles a dead durable operation and guides an admin to audited replay', async () => {
    const rule = {
      id: 9,
      name: 'Inbound lead follow-up',
      triggerType: 'form_submitted',
      targetEntityType: 'lead_form',
      triggerConfig: { formId: 31, taskContract: 'lead_follow_up_task_v1' },
      conditionLogic: 'all',
      conditions: [],
      actions: [{ type: 'create_task', config: { title: 'Call inbound lead', assignedToUserId: 7, dueDays: 0 }, delayMinutes: 0 }],
      isActive: true
    }
    const failedRun = {
      id: 44,
      automationId: 9,
      automationName: rule.name,
      triggerEventKey: 'lead-form-submission:72',
      status: 'failed',
      actionsTotal: 1,
      actionsCompleted: 0,
      retryCount: 4,
      lastError: 'database remained unavailable',
      scheduledAt: '2026-07-21T12:00:00Z',
      completedAt: '2026-07-21T12:08:00Z',
      createdAt: '2026-07-21T12:00:00Z',
      operation: { id: 81, status: 'dead', attempts: 5, maxAttempts: 5, lastError: 'database remained unavailable', runAt: '2026-07-21T12:04:00Z', updatedAt: '2026-07-21T12:08:00Z' },
      actions: [{ id: 91, position: 1, type: 'create_task', label: 'Call inbound lead', status: 'failed', attempts: 5, scheduledAt: '2026-07-21T12:00:00Z', completedAt: '2026-07-21T12:08:00Z', lastError: 'database remained unavailable' }]
    }
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/deal-pipelines')) return jsonResponse({ data: { pipelines: [] } })
      if (path.endsWith('/api/lead-capture-forms')) return jsonResponse({ data: { forms: [{ id: 31, name: 'Website', isActive: true }] } })
      if (path.endsWith('/api/users')) return jsonResponse({ data: { users: [{ id: 7, firstName: 'Riley', lastName: 'Chen', status: 'active' }] } })
      if (path.endsWith('/api/workflow-automations')) return workflowPage([rule])
      if (path.endsWith('/api/workflow-approvals')) return jsonResponse({ data: { approvals: [] } })
      if (path.endsWith('/api/workflow-automation-runs')) return jsonResponse({ data: { runs: [failedRun] } })
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/automations')

    render(<AppRouter />)

    expect(await screen.findByText('database remained unavailable')).toBeInTheDocument()
    expect(screen.getByText('Durable attempt 5 of 5 · dead')).toBeInTheDocument()
    fireEvent.click(screen.getByText('Inspect 1 action outcome'))
    expect(screen.getByText('Action failed · 5 attempts')).toBeInTheDocument()
    expect(screen.getByText('Action issue: database remained unavailable')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Review and replay in Operations' })).toHaveAttribute('href', '/settings/operations')
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
      if (path.endsWith('/api/workflow-approvals')) return jsonResponse({ data: { approvals: [] } })
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
    expect(screen.getByText(/1\/1 actions completed/)).toBeInTheDocument()
    expect(runReads).toBeGreaterThanOrEqual(2)
  })
})
