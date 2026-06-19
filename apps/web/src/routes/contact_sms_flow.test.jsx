import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('contact sms flow', () => {
  it('sends templated SMS, logs inbound replies, and records opt-outs', async () => {
    const jsonResponse = (payload, init = {}) => ({
      ok: init.ok ?? true,
      status: init.status ?? 200,
      json: async () => payload
    })

    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const method = options.method || 'GET'
      const path = requestURL.pathname

      if (path.endsWith('/auth/me')) {
        return jsonResponse({ data: {
          user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
          organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' },
          membership: { role: 'owner' }
        } })
      }
      if (path.endsWith('/api/contacts/7')) {
        return jsonResponse({ data: {
          contact: { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '+15551234567', status: 'lead' },
          notes: [],
          activities: []
        } })
      }
      if (path.endsWith('/api/contacts') && method === 'GET') {
        return jsonResponse({ data: { contacts: [{ id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '+15551234567', status: 'lead' }], meta: { page: 1, pageSize: 20, total: 1 } } })
      }
      if (path.endsWith('/api/sms-messages') && method === 'GET') {
        return jsonResponse({ data: { messages: [] } })
      }
      if (path.endsWith('/api/contacts/7/sms') && method === 'POST') {
        return jsonResponse({ data: { message: { id: 50, entityType: 'contact', entityId: 7, direction: 'outbound', phoneNumber: '+15551234567', body: 'Hi Morgan, thanks for your time today. Reply STOP to opt out.', status: 'sent', templateName: 'Follow-up', createdByUserName: 'Demo Owner', sentAt: '2026-06-19T21:00:00Z', createdAt: '2026-06-19T21:00:00Z', updatedAt: '2026-06-19T21:00:00Z' } } }, { status: 201 })
      }
      if (path.endsWith('/api/sms-messages/log') && method === 'POST') {
        return jsonResponse({ data: { message: { id: 51, entityType: 'contact', entityId: 7, direction: 'inbound', phoneNumber: '+15551234567', body: 'STOP', status: 'received', createdByUserName: 'Demo Owner', receivedAt: '2026-06-19T21:05:00Z', createdAt: '2026-06-19T21:05:00Z', updatedAt: '2026-06-19T21:05:00Z' } } }, { status: 201 })
      }
      if (path.endsWith('/api/sms/opt-outs') && method === 'POST') {
        return jsonResponse({ data: { suppression: { id: 9, phoneNumber: '+15551234567', reason: 'manual', source: 'contact_detail', entityType: 'contact', entityId: 7, createdByUserId: 1 } } }, { status: 201 })
      }
      return jsonResponse({ data: { tasks: [], deals: [], users: [], unreadCount: 0 } })
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/contacts/7')

    render(<AppRouter />)

    await screen.findByRole('heading', { name: /^sms$/i })
    fireEvent.change(screen.getByLabelText(/sms template/i), { target: { value: 'Follow-up' } })
    fireEvent.click(screen.getByRole('button', { name: /send text/i }))

    await waitFor(() => {
      const sendCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/contacts/7/sms'))
      expect(sendCall).toBeTruthy()
      expect(JSON.parse(sendCall[1].body)).toEqual({ body: 'Hi {{first_name}}, thanks for your time today. Reply STOP to opt out.', templateName: 'Follow-up' })
    })
    expect(await screen.findByText(/sms sent/i)).toBeInTheDocument()
    expect(screen.getByText(/Hi Morgan, thanks for your time today/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /log inbound sms/i }))
    fireEvent.change(screen.getByLabelText(/inbound sms body/i), { target: { value: 'STOP' } })
    fireEvent.click(screen.getByRole('button', { name: /save inbound sms/i }))

    await waitFor(() => {
      const inboundCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/sms-messages/log'))
      expect(inboundCall).toBeTruthy()
      expect(JSON.parse(inboundCall[1].body)).toEqual({ entityType: 'contact', entityId: 7, phoneNumber: '+15551234567', body: 'STOP' })
    })
    expect(await screen.findByText(/stop-style replies opt the number out automatically/i)).toBeInTheDocument()
    expect(screen.getByText(/^STOP$/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /mark sms opt-out/i }))

    await waitFor(() => {
      const optOutCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/sms/opt-outs'))
      expect(optOutCall).toBeTruthy()
      expect(JSON.parse(optOutCall[1].body)).toEqual({ phoneNumber: '+15551234567', reason: 'manual', source: 'contact_detail', entityType: 'contact', entityId: 7 })
    })
    expect(await screen.findByText(/sms opt-out recorded/i)).toBeInTheDocument()
  })
})
