import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('contact sequence enrollment flow', () => {
  it('enrolls a contact in an email sequence from contact detail', async () => {
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
      if (path.endsWith('/api/contacts/7')) {
        return jsonResponse({
          data: {
            contact: { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', status: 'lead' },
            notes: [],
            activities: []
          }
        })
      }
      if (path.endsWith('/api/email-sequences')) {
        return jsonResponse({ data: { sequences: [{ id: 4, name: 'Trial nurture', status: 'active', revision: 1, approvedRevision: 1, approvedAt: '2026-06-14T12:00:00Z', steps: [{ id: 11, stepOrder: 1, delayDays: 0, subject: 'Welcome', body: 'Hi' }] }] } })
      }
      if (path.endsWith('/api/email-sequence-enrollments') && method === 'POST') {
        return jsonResponse({ data: { enrollment: { id: 9, sequenceId: 4, sequenceName: 'Trial nurture', contactId: 7, status: 'active', currentStepOrder: 1, nextSendAt: '2026-06-15T12:00:00Z' } } })
      }
      if (path.endsWith('/api/email-sequence-enrollments')) {
        return jsonResponse({ data: { enrollments: [] } })
      }
      if (path.endsWith('/api/contacts') && method === 'GET') {
        return jsonResponse({ data: { contacts: [{ id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', status: 'lead' }], meta: { page: 1, pageSize: 20, total: 1 } } })
      }
      return jsonResponse({ data: { tasks: [], deals: [], users: [], unreadCount: 0 } })
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/contacts/7')

    render(<AppRouter />)

    const sequencesToggle = await screen.findByRole('button', { name: /manage sequences/i })
    fireEvent.click(sequencesToggle)

    const sequenceSelect = await screen.findByLabelText(/^sequence$/i, {}, { timeout: 5000 })
    fireEvent.change(sequenceSelect, { target: { value: '4' } })
    fireEvent.click(screen.getByRole('button', { name: /enroll contact/i }))

    await waitFor(() => {
      const enrollCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/email-sequence-enrollments') && call[1]?.method === 'POST'
      )
      expect(enrollCall).toBeTruthy()
      expect(JSON.parse(enrollCall[1].body)).toEqual({ contactId: 7, sequenceId: 4 })
    })
    expect(await screen.findByText(/enrolled in trial nurture/i)).toBeInTheDocument()
  })
})
