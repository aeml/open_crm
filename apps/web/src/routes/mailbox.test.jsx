import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
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
          { id: 10, toEmail: 'lead@acme.test', subject: 'Following up', status: 'sent', entityType: 'deal', entityId: 22, createdAt: '2026-05-01T12:00:00Z' }
        ] } }) }
      }
      return { ok: true, json: async () => ({ data: { unreadCount: 0 } }) }
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/mailbox')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /mailbox/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /following up/i })).toBeInTheDocument()
    expect(screen.getByText(/to lead@acme.test/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /open deal #22/i })).toHaveAttribute('href', '/deals/22')
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/me\/email-messages$/), expect.any(Object))
    })
  })
})
