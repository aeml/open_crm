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
        user: { id: 1, email: 'rep@acme.test', firstName: 'Demo', lastName: 'Owner' },
        organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
        membership: { role: 'owner' }
      }
    })
  }
}

describe('settings email account route', () => {
  it('connects a user mailbox via SMTP', async () => {
    const fetchMock = vi.fn(async (url, options = {}) => {
      const path = new URL(String(url), 'http://localhost').pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/me/email-account') && method === 'GET') {
        return { ok: true, json: async () => ({ data: { account: null, configured: true } }) }
      }
      if (path.endsWith('/api/me/email-account') && method === 'PUT') {
        return { ok: true, json: async () => ({ data: { account: { fromEmail: 'rep@acme.test', smtpHost: 'smtp.acme.test', smtpPort: 587, smtpUsername: 'rep', smtpUseTls: true, hasPassword: true } } }) }
      }
      return { ok: true, json: async () => ({ data: { unreadCount: 0 } }) }
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/email-account')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /my email connection/i })).toBeInTheDocument()

    fireEvent.change(await screen.findByLabelText(/from email/i), { target: { value: 'rep@acme.test' } })
    fireEvent.change(screen.getByLabelText(/smtp host/i), { target: { value: 'smtp.acme.test' } })
    fireEvent.change(screen.getByLabelText(/smtp username/i), { target: { value: 'rep' } })
    fireEvent.change(screen.getByLabelText(/^smtp password$/i), { target: { value: 'app-pass' } })
    fireEvent.click(screen.getByLabelText(/enable mailbox sync metadata/i))
    fireEvent.change(screen.getByLabelText(/imap host/i), { target: { value: 'imap.acme.test' } })
    fireEvent.change(screen.getByLabelText(/imap username/i), { target: { value: 'rep' } })
    fireEvent.change(screen.getByLabelText(/^imap password$/i), { target: { value: 'imap-pass' } })
    fireEvent.click(screen.getByRole('button', { name: /save connection/i }))

    await waitFor(() => {
      const saveCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/me/email-account') && call[1]?.method === 'PUT'
      )
      expect(saveCall).toBeTruthy()
      const payload = JSON.parse(saveCall[1].body)
      expect(payload.fromEmail).toBe('rep@acme.test')
      expect(payload.smtpHost).toBe('smtp.acme.test')
      expect(payload.smtpPort).toBe(587)
      expect(payload.smtpPassword).toBe('app-pass')
      expect(payload.syncEnabled).toBe(true)
      expect(payload.provider).toBe('imap')
      expect(payload.imapHost).toBe('imap.acme.test')
      expect(payload.imapPort).toBe(993)
      expect(payload.imapPassword).toBe('imap-pass')
    })
    expect(await screen.findByText(/email account saved/i)).toBeInTheDocument()
  })

  it('shows a notice when encryption is not configured on the server', async () => {
    const fetchMock = vi.fn(async (url) => {
      const path = new URL(String(url), 'http://localhost').pathname
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/me/email-account')) {
        return { ok: true, json: async () => ({ data: { account: null, configured: false } }) }
      }
      return { ok: true, json: async () => ({ data: { unreadCount: 0 } }) }
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/email-account')

    render(<AppRouter />)

    expect(await screen.findByText(/not enabled on this server/i)).toBeInTheDocument()
  })

  it('shows OAuth provider readiness and callback status', async () => {
    const account = {
      fromEmail: 'rep@acme.test',
      smtpHost: 'smtp.acme.test',
      smtpPort: 587,
      smtpUsername: 'rep',
      smtpUseTls: true,
      hasPassword: true,
      provider: 'google',
      authMethod: 'oauth',
      syncEnabled: true,
      syncStatus: 'pending',
      oauthConnected: true
    }
    const fetchMock = vi.fn(async (url, options = {}) => {
      const path = new URL(String(url), 'http://localhost').pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/me/email-account') && method === 'GET') {
        return { ok: true, json: async () => ({ data: { account, configured: true } }) }
      }
      if (path.endsWith('/api/me/email-sync/status')) {
        return {
          ok: true,
          json: async () => ({
            data: {
              account,
              configured: true,
              connected: true,
              oauthProviders: [
                { provider: 'google', label: 'Google Workspace / Gmail', configured: true, status: 'ready', scopes: [] },
                { provider: 'microsoft', label: 'Microsoft 365 / Outlook', configured: false, status: 'missing_client_credentials', scopes: [] }
              ]
            }
          })
        }
      }
      return { ok: true, json: async () => ({ data: { unreadCount: 0 } }) }
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/email-account?emailSync=oauth_connected')

    render(<AppRouter />)

    expect(await screen.findByText(/mailbox oauth connected/i)).toBeInTheDocument()
    expect(screen.getByText('Google Workspace / Gmail')).toBeInTheDocument()
    expect(screen.getByText(/connected for mailbox sync/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /connect google/i })).toBeEnabled()
    expect(screen.getByRole('button', { name: /connect microsoft/i })).toBeDisabled()
  })

  it('checks mailbox sync readiness for a saved account', async () => {
    const account = {
      fromEmail: 'rep@acme.test',
      smtpHost: 'smtp.acme.test',
      smtpPort: 587,
      smtpUsername: 'rep',
      smtpUseTls: true,
      hasPassword: true,
      provider: 'imap',
      authMethod: 'password',
      syncEnabled: true,
      syncStatus: 'pending',
      imapHost: 'imap.acme.test',
      imapPort: 993,
      imapUsername: 'rep',
      imapUseTls: true,
      hasImapPassword: true
    }
    const fetchMock = vi.fn(async (url, options = {}) => {
      const path = new URL(String(url), 'http://localhost').pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/me/email-account') && method === 'GET') {
        return { ok: true, json: async () => ({ data: { account, configured: true } }) }
      }
      if (path.endsWith('/api/me/email-sync/status')) {
        return { ok: true, json: async () => ({ data: { account, configured: true, connected: true, oauthProviders: [] } }) }
      }
      if (path.endsWith('/api/me/email-sync/check') && method === 'POST') {
        return { ok: true, json: async () => ({ data: { status: 'ready', account: { ...account, syncStatus: 'ready' } } }) }
      }
      return { ok: true, json: async () => ({ data: { unreadCount: 0 } }) }
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/email-account')

    render(<AppRouter />)

    fireEvent.click(await screen.findByRole('button', { name: /check sync readiness/i }))

    await waitFor(() => {
      expect(fetchMock.mock.calls.some((call) => String(call[0]).endsWith('/api/me/email-sync/check') && call[1]?.method === 'POST')).toBe(true)
    })
    expect(await screen.findByText(/mailbox sync settings are ready/i)).toBeInTheDocument()
  })
})
