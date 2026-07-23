import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('settings users route', () => {
  it('shows org users and lets admins create another user', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            user: {
              id: 1,
              email: 'owner@acme.test',
              firstName: 'Demo',
              lastName: 'Owner'
            },
            organization: {
              id: 1,
              name: 'Acme, Inc.',
              slug: 'acme-inc'
            },
            membership: {
              role: 'owner'
            }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { unreadCount: 0 } })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            users: [
              { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' },
              { id: 2, email: 'admin@acme.test', firstName: 'Demo', lastName: 'Admin', role: 'admin' }
            ]
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            user: { id: 3, email: 'ops@acme.test', firstName: 'Ops', lastName: 'Lead', role: 'member', invitationDeliveryStatus: 'sent', setupLink: '/setup-password?token=setup-token-123' }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            user: { id: 3, email: 'ops@acme.test', firstName: 'Ops', lastName: 'Lead', role: 'admin' }
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/users')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /team access/i })).toBeInTheDocument()
    expect(await screen.findByText('owner@acme.test')).toBeInTheDocument()
    expect(screen.getByText('admin@acme.test')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/first name/i), { target: { value: 'Ops' } })
    fireEvent.change(screen.getByLabelText(/last name/i), { target: { value: 'Lead' } })
    fireEvent.change(screen.getByLabelText(/^email$/i), { target: { value: 'ops@acme.test' } })
    fireEvent.change(screen.getByLabelText(/^role$/i), { target: { value: 'member' } })
    fireEvent.click(screen.getByRole('button', { name: /invite user/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringMatching(/\/api\/users$/),
        expect.objectContaining({
          method: 'POST',
          credentials: 'include'
        })
      )
    })

    expect(await screen.findByText('ops@acme.test')).toBeInTheDocument()
    expect(await screen.findByText('/setup-password?token=setup-token-123')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/role for ops@acme.test/i), { target: { value: 'admin' } })

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringMatching(/\/api\/users\/3\/role$/),
        expect.objectContaining({
          method: 'PATCH',
          credentials: 'include'
        })
      )
    })
    expect(await screen.findByText(/now has the admin role.*audit trail and portable workspace export/i)).toBeInTheDocument()
  })

  it('hides create form for non-admin members', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            user: {
              id: 3,
              email: 'member@acme.test',
              firstName: 'Demo',
              lastName: 'Member'
            },
            organization: {
              id: 1,
              name: 'Acme, Inc.',
              slug: 'acme-inc'
            },
            membership: {
              role: 'member'
            }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: false,
        status: 403,
        json: async () => ({
          error: {
            code: 'FORBIDDEN',
            message: 'Admin access required'
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/users')

    render(<AppRouter />)

    expect(await screen.findByText(/admin access required/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /invite user/i })).not.toBeInTheDocument()
  })

  it('deactivates with explicit reassignment, ends sessions, and reactivates safely', async () => {
    const owner = { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner', status: 'active', ownedWork: {} }
    const member = { id: 2, email: 'member@acme.test', firstName: 'Casey', lastName: 'Member', role: 'member', status: 'active', ownedWork: { contacts: 2, tasks: 1 } }
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { user: owner, organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' }, membership: { role: 'owner' } } })
      })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { users: [owner, member] } }) })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            user: { ...member, status: 'disabled', ownedWork: {} },
            reassigned: { contacts: 2, tasks: 1 },
            sessionsInvalidated: 2,
            changed: true
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { user: { ...member, status: 'active', ownedWork: {} }, reassigned: {}, sessionsInvalidated: 0, changed: true } })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/users')
    render(<AppRouter />)

    expect(await screen.findByText('member@acme.test')).toBeInTheDocument()
    expect(screen.getByText(/2 contacts, 1 tasks/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /^deactivate$/i }))
    fireEvent.change(screen.getByLabelText(/reassign work from member@acme.test/i), { target: { value: '1' } })
    fireEvent.click(screen.getByRole('button', { name: /confirm deactivation/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringMatching(/\/api\/users\/2\/status$/),
        expect.objectContaining({ method: 'PATCH', body: JSON.stringify({ status: 'disabled', reassignToUserId: 1 }) })
      )
    })
    expect(await screen.findByText(/3 active work items reassigned; 2 sessions ended/i)).toBeInTheDocument()
    expect(screen.getByText('Disabled')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /^reactivate$/i }))

    expect(await screen.findByText(/can sign in again; mailbox sync remains off/i)).toBeInTheDocument()
    expect(screen.getAllByText('Active')).toHaveLength(2)
  })

  it('shows invitation expiry, resends with token rotation, and confirms revocation', async () => {
    const owner = { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner', status: 'active', ownedWork: {} }
    const expiredInvite = { id: 2, email: 'invitee@acme.test', firstName: 'Jamie', lastName: 'Pilot', role: 'member', status: 'active', ownedWork: {}, invitationStatus: 'expired', invitationExpiresAt: '2026-07-19T12:00:00Z' }
    const pendingInvite = { ...expiredInvite, invitationStatus: 'pending', invitationExpiresAt: '2026-07-27T12:00:00Z', setupLink: '/setup-password?token=new-local-token' }
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { user: owner, organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' }, membership: { role: 'owner' } } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { users: [owner, expiredInvite] } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { user: pendingInvite } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { user: { ...pendingInvite, status: 'disabled', invitationStatus: 'revoked', invitationExpiresAt: null, setupLink: undefined }, reassigned: {}, sessionsInvalidated: 0, changed: true } }) })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/users')
    render(<AppRouter />)

    expect(await screen.findByText(/invitation expired/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /resend invitation/i }))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/users\/2\/invitation\/resend$/), expect.objectContaining({ method: 'POST' }))
    })
    expect(await screen.findByText(/old links are invalid/i)).toBeInTheDocument()
    expect(screen.getByText('/setup-password?token=new-local-token')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /revoke invitation/i }))
    expect(screen.getByText(/one-time setup link will stop working immediately/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /confirm revocation/i }))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/users\/2\/invitation$/), expect.objectContaining({ method: 'DELETE' }))
    })
    expect(await screen.findByText(/all links are invalid/i)).toBeInTheDocument()
    expect(screen.queryByText('/setup-password?token=new-local-token')).not.toBeInTheDocument()
    expect(screen.getByText('Invitation revoked')).toBeInTheDocument()
    expect(screen.getByText('Disabled')).toBeInTheDocument()
  })

  it('shows provider feedback and blocks resend after a spam complaint', async () => {
    const owner = { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner', status: 'active', ownedWork: {} }
    const bouncedInvite = { id: 2, email: 'bounced@acme.test', firstName: 'Bounce', lastName: 'Recipient', role: 'member', status: 'active', ownedWork: {}, invitationStatus: 'pending', invitationExpiresAt: '2026-07-27T12:00:00Z', invitationDeliveryStatus: 'bounced' }
    const complainedInvite = { id: 3, email: 'complaint@acme.test', firstName: 'Spam', lastName: 'Reporter', role: 'member', status: 'active', ownedWork: {}, invitationStatus: 'pending', invitationExpiresAt: '2026-07-27T12:00:00Z', invitationDeliveryStatus: 'complaint' }
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { user: owner, organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' }, membership: { role: 'owner' } } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { users: [owner, bouncedInvite, complainedInvite] } }) })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/users')
    render(<AppRouter />)

    const bouncedRow = (await screen.findByText('bounced@acme.test')).closest('article')
    const complaintRow = screen.getByText('complaint@acme.test').closest('article')
    expect(within(bouncedRow).getByText(/email bounced · resend or revoke/i)).toBeInTheDocument()
    expect(within(bouncedRow).getByRole('button', { name: /resend invitation/i })).toBeInTheDocument()
    expect(within(complaintRow).getByText(/spam complaint · email blocked/i)).toBeInTheDocument()
    expect(within(complaintRow).queryByRole('button', { name: /resend invitation/i })).not.toBeInTheDocument()
    expect(within(complaintRow).getByRole('button', { name: /revoke invitation/i })).toBeInTheDocument()
  })
})
