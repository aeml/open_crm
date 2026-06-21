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

describe('settings lead audiences route', () => {
  it('lists, previews, and creates dynamic lead audiences', async () => {
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/lead-audiences/preview') && method === 'POST') {
        return jsonResponse({ data: { filters: { status: 'lead', leadSource: 'Website form', utmCampaign: 'spring-demo', utmSource: 'google', hasEmail: 'true' }, memberCount: 14 } })
      }
      if (path.endsWith('/api/lead-audiences') && method === 'POST') {
        return jsonResponse({ data: { audience: { id: 8, name: 'Spring demo leads', description: 'Campaign leads', filters: { status: 'lead', leadSource: 'Website form', utmCampaign: 'spring-demo', utmSource: 'google', hasEmail: 'true' }, memberCount: 14, isActive: true } } }, 201)
      }
      if (path.endsWith('/api/lead-audiences')) {
        return jsonResponse({ data: { audiences: [{ id: 5, name: 'Website leads', description: 'All website leads', filters: { status: 'lead', leadSource: 'Website form' }, memberCount: 9, isActive: true }] } })
      }
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/lead-audiences')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /lead audiences/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /website leads/i })).toBeInTheDocument()
    expect(screen.getByText(/9 members/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/^name$/i), { target: { value: 'Spring demo leads' } })
    fireEvent.change(screen.getByLabelText(/^description$/i), { target: { value: 'Campaign leads' } })
    fireEvent.change(screen.getByLabelText(/^lead source$/i), { target: { value: 'Website form' } })
    fireEvent.change(screen.getByLabelText(/^utm campaign$/i), { target: { value: 'spring-demo' } })
    fireEvent.change(screen.getByLabelText(/^utm source$/i), { target: { value: 'google' } })
    fireEvent.change(screen.getByLabelText(/^email availability$/i), { target: { value: 'true' } })
    fireEvent.click(screen.getByRole('button', { name: /preview count/i }))

    expect(await screen.findByText(/14 matching contacts/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /create audience/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/lead-audiences') && call[1]?.method === 'POST'
      )
      expect(createCall).toBeTruthy()
      expect(JSON.parse(createCall[1].body)).toEqual({
        name: 'Spring demo leads',
        description: 'Campaign leads',
        filters: {
          status: 'lead',
          leadSource: 'Website form',
          utmCampaign: 'spring-demo',
          utmSource: 'google',
          hasEmail: 'true'
        },
        isActive: true
      })
    })
    expect(await screen.findByRole('heading', { name: /spring demo leads/i })).toBeInTheDocument()
  })
})
