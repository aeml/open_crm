import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'
import { AdminMemberEmail } from './admin_member_email'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('admin member email management', () => {
  it('lets an admin set a member mailbox', async () => {
    const fetchMock = vi.fn(async (url, options = {}) => {
      const path = new URL(String(url), 'http://localhost').pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) {
        return { ok: true, json: async () => ({ data: {
          user: { id: 1, email: 'admin@acme.test', firstName: 'Ada', lastName: 'Admin' },
          organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
          membership: { role: 'owner' }
        } }) }
      }
      if (path.endsWith('/api/users') && method === 'GET') {
        return { ok: true, json: async () => ({ data: { users: [
          { id: 1, email: 'admin@acme.test', firstName: 'Ada', lastName: 'Admin', role: 'owner' },
          { id: 9, email: 'rep@acme.test', firstName: 'Rep', lastName: 'Person', role: 'member' }
        ] } }) }
      }
      if (path.endsWith('/api/users/9/email-account') && method === 'GET') {
        return { ok: true, json: async () => ({ data: { account: null, configured: true } }) }
      }
      if (path.endsWith('/api/users/9/email-account') && method === 'PUT') {
        return { ok: true, json: async () => ({ data: { account: { fromEmail: 'rep@acme.test', smtpHost: 'smtp.acme.test', smtpPort: 587, smtpUsername: 'rep', smtpUseTls: true, hasPassword: true } } }) }
      }
      return { ok: true, json: async () => ({ data: { unreadCount: 0 } }) }
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/users')

    render(<AppRouter />)

    const memberSelect = await screen.findByLabelText(/team member/i)
    await screen.findByRole('option', { name: /rep person/i })
    fireEvent.change(memberSelect, { target: { value: '9' } })

    fireEvent.change(await screen.findByLabelText(/from email/i), { target: { value: 'rep@acme.test' } })
    fireEvent.change(screen.getByLabelText(/smtp host/i), { target: { value: 'smtp.acme.test' } })
    fireEvent.change(screen.getByLabelText(/smtp username/i), { target: { value: 'rep' } })
    fireEvent.change(screen.getByLabelText(/smtp password/i), { target: { value: 'app-pass' } })
    fireEvent.click(screen.getByRole('button', { name: /save member email/i }))

    await waitFor(() => {
      const putCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/users/9/email-account') && call[1]?.method === 'PUT'
      )
      expect(putCall).toBeTruthy()
      expect(JSON.parse(putCall[1].body).smtpHost).toBe('smtp.acme.test')
    })
    expect(await screen.findByText(/email connection saved for this member/i)).toBeInTheDocument()
  })

  it('loads the complete active member catalog when the selector is used', async () => {
    const initialUsers = [{ id: 1, firstName: 'First', lastName: 'Member', email: 'first@example.test' }]
    const completeUsers = [
      ...initialUsers,
      { id: 51, firstName: 'Later', lastName: 'Member', email: 'later@example.test' }
    ]
    const loadUsers = vi.fn().mockResolvedValue(completeUsers)

    render(<AdminMemberEmail users={initialUsers} loadUsers={loadUsers} />)

    const selector = screen.getByLabelText('Team member')
    expect(screen.queryByRole('option', { name: /later member/i })).not.toBeInTheDocument()
    fireEvent.focus(selector)

    await waitFor(() => expect(screen.getByRole('option', { name: /later member/i })).toBeInTheDocument())
    expect(loadUsers).toHaveBeenCalledTimes(1)

    fireEvent.blur(selector)
    fireEvent.focus(selector)
    expect(loadUsers).toHaveBeenCalledTimes(1)
  })
})
