import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

function jsonResponse(payload, status = 200) {
  return { ok: status < 400, status, json: async () => payload }
}

describe('public landing page route', () => {
  it('renders a hosted page and submits the embedded lead form', async () => {
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) return jsonResponse({ error: { message: 'Authentication required' } }, 401)
      if (path.endsWith('/api/public/landing-pages/demo-request')) {
        return jsonResponse({
          data: {
            page: { id: 7, name: 'Demo Page', slug: 'demo-request', publicId: 'lp_public', title: 'Book a demo', subtitle: 'See it live', body: 'Talk to our team about your pipeline.', ctaLabel: 'Request demo', theme: 'blue', leadCaptureFormId: 3, leadCaptureFormName: 'Website Leads', leadCaptureFormPublicId: 'lf_public', isActive: true },
            form: {
              id: 3,
              name: 'Website Leads',
              publicId: 'lf_public',
              title: 'Talk to sales',
              description: 'We will follow up shortly.',
              successMessage: 'Thanks. We will be in touch soon.',
              sourceLabel: 'Website form',
              consentText: 'I agree to receive a reply about this demo request.',
              isActive: true,
              fields: [
                { key: 'firstName', label: 'First name', fieldType: 'text', required: true, mapTo: 'firstName' },
                { key: 'lastName', label: 'Last name', fieldType: 'text', required: true, mapTo: 'lastName' },
                { key: 'email', label: 'Email', fieldType: 'email', required: true, mapTo: 'email' },
                { key: 'message', label: 'Message', fieldType: 'textarea', required: false, mapTo: '' }
              ]
            }
          }
        })
      }
      if (path.endsWith('/api/public/lead-capture-forms/lf_public/challenge') && method === 'POST') {
        return jsonResponse({ data: { challenge: { token: 'lead-challenge-token', consentText: 'I agree to receive a reply about this demo request.', notBefore: '2020-01-01T00:00:00Z', expiresAt: '2030-01-01T00:00:00Z' } } }, 201)
      }
      if (path.endsWith('/api/public/lead-capture-forms/lf_public/submissions') && method === 'POST') {
        return jsonResponse({ data: { successMessage: 'Thanks. We will be in touch soon.' } }, 201)
      }
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/lp/demo-request?utm_source=google&utm_medium=cpc&utm_campaign=spring-demo&utm_term=crm&utm_content=headline')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /book a demo/i })).toBeInTheDocument()
    expect(screen.getByText(/talk to our team about your pipeline/i)).toBeInTheDocument()
    const submitButton = screen.getByRole('button', { name: /request demo/i })
    await waitFor(() => expect(submitButton).toBeEnabled())

    fireEvent.change(screen.getByLabelText(/^first name$/i), { target: { value: 'Ada' } })
    fireEvent.change(screen.getByLabelText(/^last name$/i), { target: { value: 'Lovelace' } })
    fireEvent.change(screen.getByLabelText(/^email$/i), { target: { value: 'ada@example.com' } })
    fireEvent.change(screen.getByLabelText(/^message$/i), { target: { value: 'I want a walkthrough.' } })
    fireEvent.click(screen.getByRole('checkbox', { name: /receive a reply about this demo request/i }))
    fireEvent.click(submitButton)

    expect(await screen.findByRole('status')).toHaveTextContent(/thanks/i)
    await waitFor(() => {
      const submitCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/public/lead-capture-forms/lf_public/submissions') && call[1]?.method === 'POST'
      )
      expect(submitCall).toBeTruthy()
      expect(JSON.parse(submitCall[1].body)).toEqual({
        values: {
          firstName: 'Ada',
          lastName: 'Lovelace',
          email: 'ada@example.com',
          message: 'I want a walkthrough.'
        },
        sourceUrl: 'http://localhost:3000/lp/demo-request?utm_source=google&utm_medium=cpc&utm_campaign=spring-demo&utm_term=crm&utm_content=headline',
        attribution: {
          leadSource: 'Website form',
          utmSource: 'google',
          utmMedium: 'cpc',
          utmCampaign: 'spring-demo',
          utmTerm: 'crm',
          utmContent: 'headline'
        },
        challengeToken: 'lead-challenge-token',
        consentGranted: true
      })
    })
  })
})
