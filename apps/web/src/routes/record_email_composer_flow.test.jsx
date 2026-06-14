import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('record email composer flow', () => {
  it('sends company email to a linked contact', async () => {
    const jsonResponse = (payload, init = {}) => ({
      ok: init.ok ?? true,
      status: init.status ?? 200,
      json: async () => payload
    })

    const companyDetail = {
      company: { id: 5, name: 'Northstar Logistics', clientType: 'organization', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' },
      linkedContacts: [
        { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@northstar.test', relationshipTitle: 'Champion', isPrimary: true }
      ],
      activities: []
    }

    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) {
        return jsonResponse({ data: {
          user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
          organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
          membership: { role: 'owner' }
        } })
      }
      if (path.endsWith('/api/companies/5/email') && method === 'POST') {
        return jsonResponse({ data: { sent: true, to: 'morgan@northstar.test' } })
      }
      if (path.endsWith('/api/companies/5')) {
        return jsonResponse({ data: companyDetail })
      }
      if (path.endsWith('/api/companies')) {
        return jsonResponse({ data: { companies: [companyDetail.company], meta: { page: 1, pageSize: 20, total: 1 } } })
      }
      if (path.endsWith('/api/contacts')) {
        return jsonResponse({ data: { contacts: [
          { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@northstar.test', phone: '555-0100', status: 'lead', isClient: false }
        ], meta: { page: 1, pageSize: 20, total: 1 } } })
      }
      if (path.endsWith('/api/users')) {
        return jsonResponse({ data: { users: [{ id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' }] } })
      }
      if (path.endsWith('/api/email-templates')) {
        return jsonResponse({ data: { templates: [{ id: 2, name: 'Intro', subject: 'Hello {{first_name}}', body: 'Hi {{first_name}} from {{company_name}}.' }] } })
      }
      if (path.endsWith('/api/email-messages')) {
        return jsonResponse({ data: { messages: [] } })
      }
      if (path.endsWith('/api/notes')) {
        return jsonResponse({ data: { notes: [] } })
      }
      if (path.endsWith('/api/tasks')) {
        return jsonResponse({ data: { tasks: [] } })
      }
      if (path.endsWith('/api/deals')) {
        return jsonResponse({ data: { deals: [], meta: { page: 1, pageSize: 20, total: 0 } } })
      }
      return jsonResponse({ data: { unreadCount: 0 } })
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/companies/5')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /northstar logistics/i })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))
    const templateSelect = await screen.findByLabelText(/template/i)
    fireEvent.change(templateSelect, { target: { value: '2' } })
    expect(screen.getByLabelText(/subject/i)).toHaveValue('Hello {{first_name}}')

    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))

    await waitFor(() => {
      const sendCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/companies/5/email') && call[1]?.method === 'POST'
      )
      expect(sendCall).toBeTruthy()
      expect(JSON.parse(sendCall[1].body)).toEqual({ subject: 'Hello {{first_name}}', body: 'Hi {{first_name}} from {{company_name}}.', contactId: 7 })
    })
    expect(await screen.findByText(/email sent to morgan@northstar.test/i)).toBeInTheDocument()
  })
})
