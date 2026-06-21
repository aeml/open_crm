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
  sourceLabel: 'Website form',
  isActive: true,
  submissionCount: 0,
  fields: [
    { key: 'firstName', label: 'First name', fieldType: 'text', required: true, mapTo: 'firstName' },
    { key: 'lastName', label: 'Last name', fieldType: 'text', required: true, mapTo: 'lastName' },
    { key: 'email', label: 'Email', fieldType: 'email', required: true, mapTo: 'email' }
  ]
}

describe('settings landing pages route', () => {
  it('lists landing pages and creates a hosted page tied to a lead form', async () => {
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/lead-capture-forms')) return jsonResponse({ data: { forms: [leadForm] } })
      if (path.endsWith('/api/lead-landing-pages') && method === 'POST') {
        return jsonResponse({ data: { page: { id: 7, name: 'Demo Page', slug: 'demo-request', publicId: 'lp_created', title: 'Book a demo', subtitle: 'See it live', body: 'Talk to our team.', ctaLabel: 'Request demo', theme: 'blue', leadCaptureFormId: 3, leadCaptureFormName: 'Website Leads', leadCaptureFormPublicId: 'lf_existing', isActive: true } } })
      }
      if (path.endsWith('/api/lead-landing-pages')) {
        return jsonResponse({ data: { pages: [{ id: 4, name: 'Website Demo', slug: 'website-demo', publicId: 'lp_existing', title: 'See Open CRM', subtitle: 'A focused demo page.', body: 'Learn more.', ctaLabel: 'Book demo', theme: 'light', leadCaptureFormId: 3, leadCaptureFormName: 'Website Leads', leadCaptureFormPublicId: 'lf_existing', isActive: true }] } })
      }
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/landing-pages')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /landing pages/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /website demo/i })).toBeInTheDocument()
    expect(screen.getByText(/\/lp\/website-demo/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/^lead form$/i), { target: { value: '3' } })
    fireEvent.change(screen.getByLabelText(/^name$/i), { target: { value: 'Demo Page' } })
    fireEvent.change(screen.getByLabelText(/slug/i), { target: { value: 'demo-request' } })
    fireEvent.change(screen.getByLabelText(/^title$/i), { target: { value: 'Book a demo' } })
    fireEvent.change(screen.getByLabelText(/^subtitle$/i), { target: { value: 'See it live' } })
    fireEvent.change(screen.getByLabelText(/^body$/i), { target: { value: 'Talk to our team.' } })
    fireEvent.change(screen.getByLabelText(/^cta label$/i), { target: { value: 'Request demo' } })
    fireEvent.change(screen.getByLabelText(/^theme$/i), { target: { value: 'blue' } })
    fireEvent.click(screen.getByRole('button', { name: /create landing page/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/lead-landing-pages') && call[1]?.method === 'POST'
      )
      expect(createCall).toBeTruthy()
      expect(JSON.parse(createCall[1].body)).toEqual({
        name: 'Demo Page',
        slug: 'demo-request',
        title: 'Book a demo',
        subtitle: 'See it live',
        body: 'Talk to our team.',
        ctaLabel: 'Request demo',
        theme: 'blue',
        leadCaptureFormId: 3,
        isActive: true
      })
    })
    expect(await screen.findByRole('heading', { name: /demo page/i })).toBeInTheDocument()
  })
})
