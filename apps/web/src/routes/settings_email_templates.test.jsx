import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

function sessionResponse() {
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

describe('settings email templates route', () => {
  it('lists templates and creates a new one', async () => {
    const jsonResponse = (payload) => ({ ok: true, json: async () => payload })
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) {
        return sessionResponse()
      }
      if (path.endsWith('/api/email-templates/merge-fields')) {
        return jsonResponse({ data: { groups: [{ key: 'contact', label: 'Contact fields', fields: [{ token: '{{first_name}}', label: 'First name' }] }] } })
      }
      if (path.endsWith('/api/email-snippets') && method === 'POST') {
        return jsonResponse({ data: { snippet: { id: 6, name: 'Scheduling CTA', body: 'Would next week work?' } } })
      }
      if (path.endsWith('/api/email-snippets')) {
        return jsonResponse({ data: { snippets: [{ id: 4, name: 'Trial CTA', body: 'Want to try it this week?' }] } })
      }
      if (path.endsWith('/api/email-templates') && method === 'POST') {
        return jsonResponse({ data: { template: { id: 5, name: 'Follow up', subject: 'Checking in', body: 'Hi {{first_name}}' } } })
      }
      if (path.endsWith('/api/email-templates')) {
        return jsonResponse({ data: { templates: [{ id: 3, name: 'Welcome', subject: 'Hi there', body: 'Hello {{first_name}}' }] } })
      }
      return jsonResponse({ data: { unreadCount: 0 } })
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/email-templates')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /email templates/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /welcome/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /trial cta/i })).toBeInTheDocument()
    expect(await screen.findByText('{{first_name}}')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/^name$/i), { target: { value: 'Follow up' } })
    fireEvent.change(screen.getByLabelText(/^subject$/i), { target: { value: 'Checking in' } })
    fireEvent.change(screen.getByLabelText(/^body$/i), { target: { value: 'Hi {{first_name}}' } })
    fireEvent.click(screen.getByRole('button', { name: /create template/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/email-templates') && call[1]?.method === 'POST'
      )
      expect(createCall).toBeTruthy()
      expect(JSON.parse(createCall[1].body)).toEqual({ name: 'Follow up', subject: 'Checking in', body: 'Hi {{first_name}}' })
    })
    expect(await screen.findByRole('heading', { name: /follow up/i })).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/snippet name/i), { target: { value: 'Scheduling CTA' } })
    fireEvent.change(screen.getByLabelText(/snippet body/i), { target: { value: 'Would next week work?' } })
    fireEvent.click(screen.getByRole('button', { name: /create snippet/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/email-snippets') && call[1]?.method === 'POST'
      )
      expect(createCall).toBeTruthy()
      expect(JSON.parse(createCall[1].body)).toEqual({ name: 'Scheduling CTA', body: 'Would next week work?' })
    })
    expect(await screen.findByRole('heading', { name: /scheduling cta/i })).toBeInTheDocument()
  })
})
