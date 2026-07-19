import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => vi.unstubAllGlobals())

function jsonResponse(payload, status = 200) {
  return { ok: status < 400, status, json: async () => payload }
}

function sessionResponse() {
  return jsonResponse({ data: { user: { id: 1, email: 'owner@acme.test' }, organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' }, membership: { role: 'owner' } } })
}

describe('settings task automations route', () => {
  it('hides non-executable foundations and creates a bounded stage task rule', async () => {
    const createdRule = {
      id: 8,
      name: 'Proposal follow-up',
      triggerType: 'stage_changed',
      targetEntityType: 'deal',
      triggerConfig: { stageId: 12 },
      conditionLogic: 'all',
      conditions: [],
      actions: [{ type: 'create_task', config: { title: 'Prepare proposal', description: 'Confirm scope.' }, delayMinutes: 2880 }],
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
      if (path.endsWith('/api/workflow-automation-runs')) return jsonResponse({ data: { runs: [{ id: 21, automationId: 5, automationName: 'Qualify new deals', triggerEventKey: 'deal:7:activity:90', status: 'succeeded', actionsTotal: 1, actionsCompleted: 1, createdAt: '2026-07-19T12:00:00Z' }] } })
      if (path.endsWith('/api/workflow-automations') && method === 'POST') return jsonResponse({ data: { automation: createdRule } }, 201)
      if (path.endsWith('/api/workflow-automations')) {
        return jsonResponse({ data: { automations: [
          { id: 5, name: 'Qualify new deals', triggerType: 'record_created', targetEntityType: 'deal', triggerConfig: {}, conditions: [], actions: [{ type: 'create_task', config: { title: 'Qualify deal' }, delayMinutes: 1440 }], isActive: true },
          { id: 6, name: 'Legacy email action', triggerType: 'record_created', targetEntityType: 'contact', triggerConfig: {}, conditions: [], actions: [{ type: 'send_email', config: { subject: 'Welcome', body: 'Hello' } }], isActive: true }
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
    expect(screen.getByText(/1 stored legacy workflow definition is hidden/i)).toBeInTheDocument()
    expect(screen.getByRole('list', { name: 'Task automation runs' })).toHaveTextContent('1/1 tasks created')

    fireEvent.change(screen.getByLabelText('Rule name'), { target: { value: 'Proposal follow-up' } })
    fireEvent.change(screen.getByLabelText('When'), { target: { value: 'stage_changed' } })
    fireEvent.change(screen.getByLabelText(/destination stage/i), { target: { value: '12' } })
    fireEvent.change(screen.getByLabelText('Task title'), { target: { value: 'Prepare proposal' } })
    fireEvent.change(screen.getByLabelText('Task description'), { target: { value: 'Confirm scope.' } })
    fireEvent.change(screen.getByLabelText('Due in days', { exact: false }), { target: { value: '2' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create task rule' }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/workflow-automations') && call[1]?.method === 'POST')
      expect(JSON.parse(createCall[1].body)).toEqual({
        name: 'Proposal follow-up',
        description: 'Creates one assigned follow-up task from a deal event.',
        triggerType: 'stage_changed',
        targetEntityType: 'deal',
        triggerConfig: { stageId: 12 },
        conditionLogic: 'all',
        conditions: [],
        actions: [{ type: 'create_task', config: { title: 'Prepare proposal', description: 'Confirm scope.' }, delayMinutes: 2880 }],
        isActive: true,
        position: 0
      })
    })
    expect(await screen.findByRole('heading', { name: 'Proposal follow-up' })).toBeInTheDocument()
    expect(screen.getByText('When moved to Sales pipeline · Proposal')).toBeInTheDocument()
  })
})
