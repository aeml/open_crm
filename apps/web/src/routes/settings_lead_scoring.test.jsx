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

describe('settings lead scoring route', () => {
  it('lists users and creates a lead scoring rule', async () => {
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/users')) {
        return jsonResponse({ data: { users: [{ id: 2, email: 'alex@acme.test', firstName: 'Alex', lastName: 'Admin', role: 'admin' }] } })
      }
      if (path.endsWith('/api/lead-scoring-rules') && method === 'POST') {
        return jsonResponse({ data: { rule: { id: 8, name: 'High-intent demo', field: 'utmCampaign', operator: 'contains', value: 'demo', scoreDelta: 30, assignToUserId: 2, assignToUserName: 'Alex Admin', isActive: true, position: 1 } } }, 201)
      }
      if (path.endsWith('/api/lead-scoring-rules')) {
        return jsonResponse({ data: { rules: [{ id: 5, name: 'Website lead', field: 'leadSource', operator: 'equals', value: 'Website form', scoreDelta: 20, isActive: true, position: 0 }] } })
      }
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/lead-scoring')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /lead scoring and routing/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /website lead/i })).toBeInTheDocument()
    expect(screen.getByText(/lead source equals website form/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/^rule name$/i), { target: { value: 'High-intent demo' } })
    fireEvent.change(screen.getByLabelText(/^field$/i), { target: { value: 'utmCampaign' } })
    fireEvent.change(screen.getByLabelText(/^operator$/i), { target: { value: 'contains' } })
    fireEvent.change(screen.getByLabelText(/^rule value$/i), { target: { value: 'demo' } })
    fireEvent.change(screen.getByLabelText(/^score delta$/i), { target: { value: '30' } })
    fireEvent.change(screen.getByLabelText(/assign to/i), { target: { value: '2' } })
    fireEvent.change(screen.getByLabelText(/^order$/i), { target: { value: '1' } })
    fireEvent.click(screen.getByRole('button', { name: /create scoring rule/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/lead-scoring-rules') && call[1]?.method === 'POST'
      )
      expect(createCall).toBeTruthy()
      expect(JSON.parse(createCall[1].body)).toEqual({
        name: 'High-intent demo',
        description: '',
        field: 'utmCampaign',
        operator: 'contains',
        value: 'demo',
        scoreDelta: 30,
        assignToUserId: 2,
        isActive: true,
        position: 1
      })
    })
    expect(await screen.findByRole('heading', { name: /high-intent demo/i })).toBeInTheDocument()
  })
})
