import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

function jsonResponse(payload, status = 200) {
  return { ok: status < 400, status, json: async () => payload }
}

function sessionResponse() {
  return jsonResponse({
    data: {
      user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
      organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
      membership: { role: 'owner' }
    }
  })
}

describe('settings automations route', () => {
  it('lists workflow triggers and creates an automation trigger definition', async () => {
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/workflow-automations') && method === 'POST') {
        return jsonResponse({ data: { automation: { id: 8, name: 'Website form follow-up', description: 'Start after public form capture.', triggerType: 'form_submitted', targetEntityType: 'lead_form', triggerConfig: { formPublicId: 'lf_public' }, conditionLogic: 'all', conditions: [{ field: 'leadSource', operator: 'equals', value: 'Website form' }], actions: [{ type: 'create_task', config: { title: 'Call website lead' }, delayMinutes: 30 }], isActive: true, position: 1 } } }, 201)
      }
      if (path.endsWith('/api/workflow-automations')) {
        return jsonResponse({ data: { automations: [{ id: 5, name: 'New contact welcome', description: 'Start from new contacts.', triggerType: 'record_created', targetEntityType: 'contact', triggerConfig: {}, conditionLogic: 'all', conditions: [{ field: 'status', operator: 'equals', value: 'lead' }], actions: [{ type: 'send_email', config: { subject: 'Welcome', body: 'Thanks for reaching out.' }, scheduledAt: '2030-05-01T15:30:00Z' }], isActive: false, position: 0 }] } })
      }
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/automations')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /workflow automations/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /new contact welcome/i })).toBeInTheDocument()
    expect(screen.getAllByText(/record created/i).length).toBeGreaterThan(0)
    expect(screen.getByRole('heading', { name: /visual workflow builder/i })).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/^automation name$/i), { target: { value: 'Website form follow-up' } })
    fireEvent.change(screen.getByLabelText(/^description$/i), { target: { value: 'Start after public form capture.' } })
    fireEvent.change(screen.getByLabelText(/^trigger type$/i), { target: { value: 'form_submitted' } })
    fireEvent.change(screen.getByLabelText(/trigger config json/i), { target: { value: '{"formPublicId":"lf_public"}' } })
    fireEvent.change(screen.getByLabelText(/^condition logic$/i), { target: { value: 'all' } })
    fireEvent.change(screen.getByLabelText(/^condition field$/i), { target: { value: 'leadSource' } })
    fireEvent.change(screen.getByLabelText(/condition value/i), { target: { value: 'Website form' } })
    fireEvent.click(screen.getByRole('button', { name: /add condition/i }))
    fireEvent.change(screen.getByLabelText(/^task title$/i), { target: { value: 'Call website lead' } })
    fireEvent.change(screen.getByLabelText(/delay minutes/i), { target: { value: '30' } })
    fireEvent.click(screen.getByRole('button', { name: /add action/i }))
    fireEvent.change(screen.getByLabelText(/^order$/i), { target: { value: '1' } })
    fireEvent.click(screen.getByLabelText(/active trigger definition/i))
    fireEvent.click(screen.getByRole('button', { name: /create automation trigger/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/workflow-automations') && call[1]?.method === 'POST'
      )
      expect(createCall).toBeTruthy()
      expect(JSON.parse(createCall[1].body)).toEqual({
        name: 'Website form follow-up',
        description: 'Start after public form capture.',
        triggerType: 'form_submitted',
        targetEntityType: 'lead_form',
        triggerConfig: { formPublicId: 'lf_public' },
        conditionLogic: 'all',
        conditions: [{ field: 'leadSource', operator: 'equals', value: 'Website form' }],
        actions: [{ type: 'create_task', config: { title: 'Call website lead' }, delayMinutes: 30 }],
        isActive: true,
        position: 1
      })
    })
    expect(await screen.findByRole('heading', { name: /website form follow-up/i })).toBeInTheDocument()
  })
})
