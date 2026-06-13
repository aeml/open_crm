import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('contact email flow', () => {
  it('opens the composer, applies a template, and sends an email', async () => {
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
        return jsonResponse({
          data: {
            user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' },
            membership: { role: 'owner' }
          }
        })
      }
      if (path.endsWith('/api/contacts/7/email') && method === 'POST') {
        return jsonResponse({ data: { sent: true, to: 'morgan@acme.test' } })
      }
      if (path.endsWith('/api/contacts/7')) {
        return jsonResponse({
          data: {
            contact: { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', status: 'lead' },
            notes: [],
            activities: []
          }
        })
      }
      if (path.endsWith('/api/email-templates')) {
        return jsonResponse({ data: { templates: [{ id: 2, name: 'Intro', subject: 'Hello {{first_name}}', body: 'Hi {{first_name}}!' }] } })
      }
      if (path.endsWith('/api/contacts') && method === 'GET') {
        return jsonResponse({ data: { contacts: [{ id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', status: 'lead' }], meta: { page: 1, pageSize: 20, total: 1 } } })
      }
      return jsonResponse({ data: { tasks: [], deals: [], users: [], unreadCount: 0 } })
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/contacts/7')

    render(<AppRouter />)

    const sendEmailToggle = await screen.findByRole('button', { name: /^send email$/i })
    fireEvent.click(sendEmailToggle)

    // Template dropdown appears after lazy load
    const templateSelect = await screen.findByLabelText(/template/i)
    fireEvent.change(templateSelect, { target: { value: '2' } })

    expect(screen.getByLabelText(/subject/i)).toHaveValue('Hello {{first_name}}')

    const submit = screen.getByRole('button', { name: /^send email$/i })
    fireEvent.click(submit)

    await waitFor(() => {
      const sendCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/contacts/7/email') && (call[1]?.method === 'POST')
      )
      expect(sendCall).toBeTruthy()
      expect(JSON.parse(sendCall[1].body)).toEqual({ subject: 'Hello {{first_name}}', body: 'Hi {{first_name}}!' })
    })
    expect(await screen.findByText(/email sent to morgan@acme.test/i)).toBeInTheDocument()
  })
})
