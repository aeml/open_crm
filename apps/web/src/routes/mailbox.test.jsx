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
          { id: 10, toEmail: 'lead@acme.test', subject: 'Following up', status: 'sent', entityType: 'deal', entityId: 22, openCount: 2, clickCount: 1, createdAt: '2026-05-01T12:00:00Z' }
        ] } }) }
      }
      if (path.endsWith('/api/email-messages/10')) {
        return { ok: true, json: async () => ({ data: { message: { id: 10, toEmail: 'lead@acme.test', subject: 'Following up', body: 'Thanks for talking today.', status: 'sent', entityType: 'deal', entityId: 22, openCount: 2, clickCount: 1, createdAt: '2026-05-01T12:00:00Z' } } }) }
      }
      return { ok: true, json: async () => ({ data: { unreadCount: 0 } }) }
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/mailbox')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /mailbox/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /following up/i })).toBeInTheDocument()
    expect(screen.getByText(/to lead@acme.test/i)).toBeInTheDocument()
    expect(screen.getByText(/opened 2 times/i)).toBeInTheDocument()
    expect(screen.getByText(/clicked 1 time/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /open deal #22/i })).toHaveAttribute('href', '/deals/22')
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/me\/email-messages$/), expect.any(Object))
    })
    fireEvent.click(screen.getByRole('button', { name: /view details/i }))
    expect(await screen.findByText(/thanks for talking today/i)).toBeInTheDocument()
  })

  it('shows synced inbound mailbox messages', async () => {
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
          { id: 11, direction: 'inbound', fromEmail: 'customer@acme.test', toEmail: 'rep@acme.test', subject: 'Re: Estimate', status: 'received', receivedAt: '2026-05-02T14:30:00Z', createdAt: '2026-05-02T14:31:00Z' }
        ] } }) }
      }
      if (path.endsWith('/api/email-messages/11')) {
        return { ok: true, json: async () => ({ data: { message: { id: 11, direction: 'inbound', fromEmail: 'customer@acme.test', toEmail: 'rep@acme.test', subject: 'Re: Estimate', body: 'Can we schedule Tuesday?', status: 'received', receivedAt: '2026-05-02T14:30:00Z', createdAt: '2026-05-02T14:31:00Z' } } }) }
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
  })
})
