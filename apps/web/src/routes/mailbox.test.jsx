import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('mailbox route', () => {
  it('shows the current user sent CRM emails', async () => {
    const fetchMock = vi.fn(async (url) => {
      const path = new URL(String(url), 'http://localhost').pathname
      if (path.endsWith('/auth/me')) {
        return { ok: true, json: async () => ({ data: {
          user: { id: 1, email: 'rep@acme.test', firstName: 'Rep', lastName: 'Person' },
          organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
          membership: { role: 'member' }
        } }) }
      }
      if (path.endsWith('/api/me/email-messages')) {
        return { ok: true, json: async () => ({ data: { messages: [
          { id: 10, toEmail: 'lead@acme.test', subject: 'Following up', status: 'sent', entityType: 'deal', entityId: 22, engagementTrackingState: 'active', engagementTrackingExpiresAt: '2026-08-01T12:00:00Z', openCount: 2, clickCount: 1, createdAt: '2026-05-01T12:00:00Z' }
        ] } }) }
      }
      if (path.endsWith('/api/email-threads/10')) {
        return { ok: true, json: async () => ({ data: { messages: [{ id: 10, direction: 'outbound', toEmail: 'lead@acme.test', subject: 'Following up', body: 'Thanks for talking today.', status: 'sent', entityType: 'deal', entityId: 22, engagementTrackingState: 'active', engagementTrackingExpiresAt: '2026-08-01T12:00:00Z', openCount: 2, clickCount: 1, createdAt: '2026-05-01T12:00:00Z' }], replies: [] } }) }
      }
      return { ok: true, json: async () => ({ data: { unreadCount: 0 } }) }
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/mailbox')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /mailbox/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /following up/i })).toBeInTheDocument()
    expect(screen.getByText(/to lead@acme.test/i)).toBeInTheDocument()
    expect(screen.getByText(/opens 2 · clicks 1/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /open deal #22/i })).toHaveAttribute('href', '/deals/22')
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/me\/email-messages$/), expect.any(Object))
    })
    fireEvent.click(screen.getByRole('button', { name: /view details/i }))
    expect(await screen.findByText(/thanks for talking today/i)).toBeInTheDocument()
  })

  it('shows synced inbound mailbox messages and shares them with the team', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValueOnce(false).mockReturnValue(true)
    let currentMessage = { id: 11, direction: 'inbound', visibility: 'private', fromEmail: 'customer@acme.test', toEmail: 'rep@acme.test', subject: 'Re: Estimate', status: 'received', sharedInboxStatus: 'open', sharedInboxUpdatedAt: '2026-05-02T14:31:00.123456Z', receivedAt: '2026-05-02T14:30:00Z', createdAt: '2026-05-02T14:31:00Z' }
    const fetchMock = vi.fn(async (url, options = {}) => {
      const path = new URL(String(url), 'http://localhost').pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) {
        return { ok: true, json: async () => ({ data: {
          user: { id: 1, email: 'rep@acme.test', firstName: 'Rep', lastName: 'Person' },
          organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
          membership: { role: 'member' }
        } }) }
      }
      if (path.endsWith('/api/me/email-messages')) {
        return { ok: true, json: async () => ({ data: { messages: [currentMessage] } }) }
      }
      if (path.endsWith('/api/email-threads/11')) {
        return { ok: true, json: async () => ({ data: { messages: [{ ...currentMessage, body: 'Can we schedule Tuesday?' }], replies: [] } }) }
      }
      if (path.endsWith('/api/email-messages/11/shared-inbox') && method === 'PATCH') {
        const input = JSON.parse(options.body)
        expect(input.expectedUpdatedAt).toBe(currentMessage.sharedInboxUpdatedAt)
        currentMessage = {
          ...currentMessage,
          visibility: input.visibility,
          sharedInboxStatus: 'open',
          sharedInboxAssignedToUserId: input.visibility === 'shared' ? 1 : 0,
          sharedInboxUpdatedAt: input.visibility === 'shared' ? '2026-05-02T14:31:01.123456Z' : '2026-05-02T14:31:02.123456Z'
        }
        return { ok: true, json: async () => ({ data: { message: { ...currentMessage, body: 'Can we schedule Tuesday?' } } }) }
      }
      return { ok: true, json: async () => ({ data: { unreadCount: 0 } }) }
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/mailbox')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /re: estimate/i })).toBeInTheDocument()
    expect(screen.getByText(/from customer@acme.test/i)).toBeInTheDocument()
    expect(screen.getByText(/received/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /view details/i }))
    expect(await screen.findByText(/can we schedule tuesday/i)).toBeInTheDocument()
    expect(screen.getAllByText(/from customer@acme.test/i).length).toBeGreaterThan(0)
    fireEvent.click(screen.getByRole('button', { name: /share with team/i }))
    expect(confirmSpy).toHaveBeenCalledWith(expect.stringMatching(/complete message with everyone in this workspace/i))
    expect(fetchMock.mock.calls.filter((call) => String(call[0]).endsWith('/api/email-messages/11/shared-inbox'))).toHaveLength(0)
    fireEvent.click(screen.getByRole('button', { name: /share with team/i }))
    await waitFor(() => {
      const shareCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/email-messages/11/shared-inbox'))
      expect(shareCall).toBeTruthy()
      expect(JSON.parse(shareCall[1].body)).toEqual({ visibility: 'shared', expectedUpdatedAt: '2026-05-02T14:31:00.123456Z', status: 'open', assignedToUserId: 1 })
    })
    fireEvent.click(await screen.findByRole('button', { name: /make private/i }))
    expect(confirmSpy).toHaveBeenLastCalledWith(expect.stringMatching(/access ends now/i))
    await waitFor(() => {
      const privacyCalls = fetchMock.mock.calls.filter((call) => String(call[0]).endsWith('/api/email-messages/11/shared-inbox'))
      expect(privacyCalls).toHaveLength(2)
      expect(JSON.parse(privacyCalls[1][1].body)).toEqual({ visibility: 'private', expectedUpdatedAt: '2026-05-02T14:31:01.123456Z' })
    })
    expect(await screen.findByRole('button', { name: /share with team/i })).toBeInTheDocument()
  })
})
