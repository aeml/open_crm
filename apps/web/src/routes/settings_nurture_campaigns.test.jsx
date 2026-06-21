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

describe('settings nurture campaigns route', () => {
  it('lists audiences and sequences and creates a nurture campaign', async () => {
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/lead-audiences')) {
        return jsonResponse({ data: { audiences: [{ id: 5, name: 'Demo leads', filters: { status: 'lead' }, memberCount: 14, isActive: true }] } })
      }
      if (path.endsWith('/api/email-sequences')) {
        return jsonResponse({ data: { sequences: [{ id: 7, name: 'Welcome nurture', status: 'active', steps: [{ id: 1, stepOrder: 1, delayDays: 0, subject: 'Welcome', body: 'Hi' }] }] } })
      }
      if (path.endsWith('/api/lead-nurture-campaigns') && method === 'POST') {
        return jsonResponse({ data: { campaign: { id: 8, name: 'Demo request nurture', audienceId: 5, audienceName: 'Demo leads', sequenceId: 7, sequenceName: 'Welcome nurture', sequenceStatus: 'active', status: 'active', eligibleCount: 14, enrolledCount: 0 } } }, 201)
      }
      if (path.endsWith('/api/lead-nurture-campaigns')) {
        return jsonResponse({ data: { campaigns: [{ id: 3, name: 'Website nurture', audienceId: 5, audienceName: 'Demo leads', sequenceId: 7, sequenceName: 'Welcome nurture', sequenceStatus: 'active', status: 'draft', eligibleCount: 9, enrolledCount: 0 }] } })
      }
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/nurture-campaigns')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /nurture campaigns/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /website nurture/i })).toBeInTheDocument()
    expect(screen.getByText(/9 eligible/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/^campaign name$/i), { target: { value: 'Demo request nurture' } })
    fireEvent.change(screen.getByLabelText(/^audience$/i), { target: { value: '5' } })
    fireEvent.change(screen.getByLabelText(/^email sequence$/i), { target: { value: '7' } })
    fireEvent.change(screen.getByLabelText(/^status$/i), { target: { value: 'active' } })
    fireEvent.click(screen.getByRole('button', { name: /create nurture campaign/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/lead-nurture-campaigns') && call[1]?.method === 'POST'
      )
      expect(createCall).toBeTruthy()
      expect(JSON.parse(createCall[1].body)).toEqual({
        name: 'Demo request nurture',
        description: '',
        audienceId: 5,
        sequenceId: 7,
        status: 'active'
      })
    })
    expect(await screen.findByRole('heading', { name: /demo request nurture/i })).toBeInTheDocument()
  })
})
