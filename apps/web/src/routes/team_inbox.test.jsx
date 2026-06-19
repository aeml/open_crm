import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('team inbox route', () => {
  it('lists shared inbound messages and updates assignment/status', async () => {
    const jsonResponse = (payload) => ({ ok: true, json: async () => payload })
    const fetchMock = vi.fn(async (url, options = {}) => {
      const path = new URL(String(url), 'http://localhost').pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) {
        return jsonResponse({ data: {
          user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
          organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
          membership: { role: 'member' }
        } })
      }
      if (path.endsWith('/api/shared-inbox/email-messages')) {
        return jsonResponse({ data: { messages: [{ id: 12, direction: 'inbound', visibility: 'shared', fromEmail: 'lead@example.test', toEmail: 'team@acme.test', subject: 'Pricing question', status: 'received', sharedInboxStatus: 'open', receivedAt: '2026-05-03T12:00:00Z', createdAt: '2026-05-03T12:01:00Z' }] } })
      }
      if (path.endsWith('/api/email-messages/12/shared-inbox') && method === 'PATCH') {
        const input = JSON.parse(options.body)
        return jsonResponse({ data: { message: { id: 12, direction: 'inbound', visibility: 'shared', fromEmail: 'lead@example.test', toEmail: 'team@acme.test', subject: 'Pricing question', body: 'Can you send pricing?', status: 'received', sharedInboxStatus: input.status || 'open', sharedInboxAssignedToUserId: input.assignedToUserId || 0, sharedInboxAssignedToUserName: input.assignedToUserId ? 'Demo Owner' : '', receivedAt: '2026-05-03T12:00:00Z', createdAt: '2026-05-03T12:01:00Z' } } })
      }
      if (path.endsWith('/api/email-messages/12')) {
        return jsonResponse({ data: { message: { id: 12, direction: 'inbound', visibility: 'shared', fromEmail: 'lead@example.test', toEmail: 'team@acme.test', subject: 'Pricing question', body: 'Can you send pricing?', status: 'received', sharedInboxStatus: 'open', receivedAt: '2026-05-03T12:00:00Z', createdAt: '2026-05-03T12:01:00Z' } } })
      }
      return jsonResponse({ data: { unreadCount: 0 } })
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/team-inbox')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /team inbox/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /pricing question/i })).toBeInTheDocument()
    expect(screen.getByText(/from lead@example.test/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /assign to me/i }))
    await waitFor(() => {
      const assignCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/email-messages/12/shared-inbox') && JSON.parse(call[1].body).assignedToUserId === 1)
      expect(assignCall).toBeTruthy()
    })
    expect(await screen.findByText(/demo owner/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /^close$/i }))
    await waitFor(() => {
      const closeCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/email-messages/12/shared-inbox') && JSON.parse(call[1].body).status === 'closed')
      expect(closeCall).toBeTruthy()
    })

    fireEvent.click(screen.getByRole('button', { name: /view details/i }))
    expect(await screen.findByText(/can you send pricing/i)).toBeInTheDocument()
  })
})
