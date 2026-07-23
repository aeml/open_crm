import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

function jsonResponse(payload, status = 200) {
  return { ok: status < 400, status, json: async () => payload }
}

describe('public lead widget route', () => {
  it('renders a chat widget and submits the embedded lead form', async () => {
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) return jsonResponse({ error: { message: 'Authentication required' } }, 401)
      if (path.endsWith('/api/public/lead-chat-widgets/cw_public')) {
        return jsonResponse({
          data: {
            widget: { id: 7, name: 'Website Chat', publicId: 'cw_public', title: 'Need help?', welcomeMessage: 'Tell us what you need.', promptLabel: 'Chat with us', ctaLabel: 'Send', theme: 'blue', position: 'bottom-right', leadCaptureFormId: 3, leadCaptureFormName: 'Website Leads', leadCaptureFormPublicId: 'lf_public', isActive: true },
            form: {
              id: 3,
              name: 'Website Leads',
              publicId: 'lf_public',
			  revision: 2,
              title: 'Talk to sales',
              description: 'We will follow up shortly.',
              successMessage: 'Thanks. We will be in touch soon.',
              sourceLabel: 'Website widget',
              consentText: 'I agree to receive a reply about this request.',
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
        return jsonResponse({ data: { challenge: { token: 'widget-challenge-token', formRevision: 2, consentText: 'I agree to receive a reply about this request.', notBefore: '2020-01-01T00:00:00Z', expiresAt: '2030-01-01T00:00:00Z' } } }, 201)
      }
      if (path.endsWith('/api/public/lead-capture-forms/lf_public/submissions') && method === 'POST') {
        return jsonResponse({ data: { successMessage: 'Thanks. We will be in touch soon.' } }, 201)
      }
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/widget/cw_public?utm_source=linkedin&utm_campaign=widget-demo')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /need help/i })).toBeInTheDocument()
    expect(screen.getByText(/tell us what you need/i)).toBeInTheDocument()
    const submitButton = screen.getByRole('button', { name: /^send$/i })
    await waitFor(() => expect(submitButton).toBeEnabled())

    fireEvent.change(screen.getByLabelText(/^first name$/i), { target: { value: 'Ada' } })
    fireEvent.change(screen.getByLabelText(/^last name$/i), { target: { value: 'Lovelace' } })
    fireEvent.change(screen.getByLabelText(/^email$/i), { target: { value: 'ada@example.com' } })
    fireEvent.change(screen.getByLabelText(/^message$/i), { target: { value: 'Can we talk?' } })
    fireEvent.click(screen.getByRole('checkbox', { name: /receive a reply about this request/i }))
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
          message: 'Can we talk?'
        },
        sourceUrl: 'http://localhost:3000/widget/cw_public?utm_source=linkedin&utm_campaign=widget-demo',
        attribution: {
          leadSource: 'Website widget',
          utmSource: 'linkedin',
          utmMedium: '',
          utmCampaign: 'widget-demo',
          utmTerm: '',
          utmContent: ''
        },
        challengeToken: 'widget-challenge-token',
        consentGranted: true
      })
    })
  })
})
