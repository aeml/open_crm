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

const leadForm = {
  id: 3,
  name: 'Website Leads',
  slug: 'website-leads',
  publicId: 'lf_existing',
  title: 'Talk to sales',
  description: 'Tell us about your project.',
  successMessage: 'Thanks!',
  sourceLabel: 'Website widget',
  isActive: true,
  submissionCount: 0,
  fields: [
    { key: 'firstName', label: 'First name', fieldType: 'text', required: true, mapTo: 'firstName' },
    { key: 'lastName', label: 'Last name', fieldType: 'text', required: true, mapTo: 'lastName' },
    { key: 'email', label: 'Email', fieldType: 'email', required: true, mapTo: 'email' }
  ]
}

describe('settings lead widgets route', () => {
  it('lists widgets and creates an embeddable website widget', async () => {
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/lead-capture-forms')) return jsonResponse({ data: { forms: [leadForm] } })
      if (path.endsWith('/api/lead-chat-widgets') && method === 'POST') {
        return jsonResponse({ data: { widget: { id: 7, name: 'Demo Widget', publicId: 'cw_created', title: 'Need help?', welcomeMessage: 'Tell us what you need.', promptLabel: 'Chat with sales', ctaLabel: 'Send message', theme: 'blue', position: 'bottom-left', leadCaptureFormId: 3, leadCaptureFormName: 'Website Leads', leadCaptureFormPublicId: 'lf_existing', isActive: true } } }, 201)
      }
      if (path.endsWith('/api/lead-chat-widgets')) {
        return jsonResponse({ data: { widgets: [{ id: 4, name: 'Website Chat', publicId: 'cw_existing', title: 'Need help?', welcomeMessage: 'Tell us about your project.', promptLabel: 'Chat with us', ctaLabel: 'Send', theme: 'light', position: 'bottom-right', leadCaptureFormId: 3, leadCaptureFormName: 'Website Leads', leadCaptureFormPublicId: 'lf_existing', isActive: true }] } })
      }
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/lead-widgets')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /website widgets/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /website chat/i })).toBeInTheDocument()
    expect(screen.getAllByText(/\/widget\/cw_existing/i).length).toBeGreaterThan(0)

    fireEvent.change(screen.getByLabelText(/^lead form$/i), { target: { value: '3' } })
    fireEvent.change(screen.getByLabelText(/^name$/i), { target: { value: 'Demo Widget' } })
    fireEvent.change(screen.getByLabelText(/^title$/i), { target: { value: 'Need help?' } })
    fireEvent.change(screen.getByLabelText(/^welcome message$/i), { target: { value: 'Tell us what you need.' } })
    fireEvent.change(screen.getByLabelText(/^prompt label$/i), { target: { value: 'Chat with sales' } })
    fireEvent.change(screen.getByLabelText(/^cta label$/i), { target: { value: 'Send message' } })
    fireEvent.change(screen.getByLabelText(/^theme$/i), { target: { value: 'blue' } })
    fireEvent.change(screen.getByLabelText(/^position$/i), { target: { value: 'bottom-left' } })
    fireEvent.click(screen.getByRole('button', { name: /create website widget/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/lead-chat-widgets') && call[1]?.method === 'POST'
      )
      expect(createCall).toBeTruthy()
      expect(JSON.parse(createCall[1].body)).toEqual({
        name: 'Demo Widget',
        title: 'Need help?',
        welcomeMessage: 'Tell us what you need.',
        promptLabel: 'Chat with sales',
        ctaLabel: 'Send message',
        theme: 'blue',
        position: 'bottom-left',
        leadCaptureFormId: 3,
        isActive: true
      })
    })
    expect(await screen.findByRole('heading', { name: /demo widget/i })).toBeInTheDocument()
  })
})
