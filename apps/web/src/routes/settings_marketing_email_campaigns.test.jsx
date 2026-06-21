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

describe('settings marketing email campaigns route', () => {
  it('lists audiences and creates a scheduled campaign', async () => {
    const scheduledValue = '2030-05-01T15:30'
    const expectedScheduledAt = new Date(scheduledValue).toISOString()
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/lead-audiences')) {
        return jsonResponse({ data: { audiences: [{ id: 5, name: 'Spring leads', filters: { status: 'lead' }, memberCount: 14, isActive: true }] } })
      }
      if (path.endsWith('/api/marketing-email-campaigns') && method === 'POST') {
        return jsonResponse({ data: { campaign: { id: 8, name: 'Spring demo blast', audienceId: 5, audienceName: 'Spring leads', subject: 'Join the spring demo', body: 'Campaign body', status: 'scheduled', scheduledAt: expectedScheduledAt, analytics: { recipientCount: 14, sentCount: 0, openedCount: 0, clickedCount: 0 } } } }, 201)
      }
      if (path.endsWith('/api/marketing-email-campaigns')) {
        return jsonResponse({ data: { campaigns: [{ id: 3, name: 'Website newsletter', audienceId: 5, audienceName: 'Spring leads', subject: 'Newsletter', status: 'draft', analytics: { recipientCount: 9, sentCount: 0, openedCount: 0, clickedCount: 0 } }] } })
      }
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/marketing-email-campaigns')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /email campaigns/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /website newsletter/i })).toBeInTheDocument()
    expect(screen.getByText(/9 recipients/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/^campaign name$/i), { target: { value: 'Spring demo blast' } })
    fireEvent.change(screen.getByLabelText(/^audience$/i), { target: { value: '5' } })
    fireEvent.change(screen.getByLabelText(/^status$/i), { target: { value: 'scheduled' } })
    fireEvent.change(screen.getByLabelText(/^scheduled time$/i), { target: { value: scheduledValue } })
    fireEvent.change(screen.getByLabelText(/^subject$/i), { target: { value: 'Join the spring demo' } })
    fireEvent.change(screen.getByLabelText(/^preview text$/i), { target: { value: 'Reserve your spot.' } })
    fireEvent.change(screen.getByLabelText(/^body$/i), { target: { value: 'Campaign body' } })
    fireEvent.click(screen.getByRole('button', { name: /create campaign/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/marketing-email-campaigns') && call[1]?.method === 'POST'
      )
      expect(createCall).toBeTruthy()
      expect(JSON.parse(createCall[1].body)).toEqual({
        name: 'Spring demo blast',
        description: '',
        audienceId: 5,
        subject: 'Join the spring demo',
        previewText: 'Reserve your spot.',
        body: 'Campaign body',
        status: 'scheduled',
        scheduledAt: expectedScheduledAt
      })
    })
    expect(await screen.findByRole('heading', { name: /spring demo blast/i })).toBeInTheDocument()
  })
})
