import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

function jsonResponse(payload) {
  return { ok: true, json: async () => payload }
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

const standardFields = [
  { key: 'firstName', label: 'First name', fieldType: 'text', required: true, mapTo: 'firstName' },
  { key: 'lastName', label: 'Last name', fieldType: 'text', required: true, mapTo: 'lastName' },
  { key: 'email', label: 'Email', fieldType: 'email', required: true, mapTo: 'email' },
  { key: 'phone', label: 'Phone', fieldType: 'tel', required: false, mapTo: 'phone' },
  { key: 'message', label: 'How can we help?', fieldType: 'textarea', required: false, mapTo: '' }
]

describe('settings lead forms route', () => {
  it('lists lead forms and creates a mapped form', async () => {
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) {
        return sessionResponse()
      }
      if (path.endsWith('/api/notifications/unread-count')) {
        return jsonResponse({ data: { unreadCount: 0 } })
      }
      if (path.endsWith('/api/lead-capture-forms') && method === 'POST') {
        return jsonResponse({ data: { form: { id: 8, name: 'Demo request', slug: 'demo-request', publicId: 'lf_created', title: 'Book a demo', description: '', successMessage: 'Thanks!', sourceLabel: 'Demo form', isActive: true, submissionCount: 0, fields: standardFields } } })
      }
      if (path.endsWith('/api/lead-capture-forms')) {
        return jsonResponse({ data: { forms: [{ id: 3, name: 'Website Leads', slug: 'website-leads', publicId: 'lf_existing', title: 'Talk to sales', description: 'Main website form', successMessage: 'Thanks!', sourceLabel: 'Website form', isActive: true, submissionCount: 2, fields: standardFields }] } })
      }
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/lead-forms')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /lead forms/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /website leads/i })).toBeInTheDocument()
    expect(screen.getByDisplayValue(/api\/public\/lead-capture-forms\/lf_existing\/submissions/)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/^name$/i), { target: { value: 'Demo request' } })
    fireEvent.change(screen.getByLabelText(/slug/i), { target: { value: 'demo-request' } })
    fireEvent.change(screen.getByLabelText(/^title$/i), { target: { value: 'Book a demo' } })
    fireEvent.change(screen.getByLabelText(/^success message$/i), { target: { value: 'Thanks!' } })
    fireEvent.change(screen.getByLabelText(/^source label$/i), { target: { value: 'Demo form' } })
    fireEvent.click(screen.getByRole('button', { name: /create lead form/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/lead-capture-forms') && call[1]?.method === 'POST'
      )
      expect(createCall).toBeTruthy()
      const body = JSON.parse(createCall[1].body)
      expect(body.name).toBe('Demo request')
      expect(body.slug).toBe('demo-request')
      expect(body.fields.find((field) => field.key === 'firstName').mapTo).toBe('firstName')
      expect(body.fields.find((field) => field.key === 'message').mapTo).toBe('')
    })
    expect(await screen.findByRole('heading', { name: /demo request/i })).toBeInTheDocument()
  })
})
