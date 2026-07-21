import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('settings email log route', () => {
  it('shows org-wide sent emails for an admin', async () => {
    const fetchMock = vi.fn(async (url) => {
      const path = new URL(String(url), 'http://localhost').pathname
      if (path.endsWith('/auth/me')) {
        return {
          ok: true,
          json: async () => ({
            data: {
              user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
              organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
              membership: { role: 'owner' }
            }
          })
        }
      }
      if (path.endsWith('/api/email-messages')) {
        return {
          ok: true,
          json: async () => ({
            data: {
              messages: [
                { id: 1, toEmail: 'lead@acme.test', subject: 'Following up', status: 'sent', deliveryOutcome: 'bounced', deliveryOutcomeAt: '2026-05-02T12:00:00Z', sentByName: 'Demo Owner', engagementTrackingState: 'active', engagementTrackingExpiresAt: '2026-08-01T12:00:00Z', openCount: 1, clickCount: 3, createdAt: '2026-05-01T12:00:00Z' }
              ]
            }
          })
        }
      }
      if (path.endsWith('/api/email-messages/1')) {
        return {
          ok: true,
          json: async () => ({
            data: {
              message: { id: 1, toEmail: 'lead@acme.test', subject: 'Following up', body: 'Admin-visible full body.', status: 'sent', deliveryOutcome: 'bounced', deliveryOutcomeAt: '2026-05-02T12:00:00Z', sentByName: 'Demo Owner', engagementTrackingState: 'active', engagementTrackingExpiresAt: '2026-08-01T12:00:00Z', openCount: 1, clickCount: 3, createdAt: '2026-05-01T12:00:00Z' }
            }
          })
        }
      }
      return { ok: true, json: async () => ({ data: { unreadCount: 0 } }) }
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/email-log')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /email log/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /following up/i })).toBeInTheDocument()
    expect(screen.getByText(/to lead@acme.test/i)).toBeInTheDocument()
    expect(screen.getByText(/opens 1 · clicks 3/i)).toBeInTheDocument()
    expect(screen.getByText(/· bounced/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /view details/i }))
    expect(await screen.findByText(/admin-visible full body/i)).toBeInTheDocument()
  })
})
