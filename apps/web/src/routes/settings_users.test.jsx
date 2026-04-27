import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
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
            user: { id: 3, email: 'ops@acme.test', firstName: 'Ops', lastName: 'Lead', role: 'member', setupLink: '/setup-password?token=setup-token-123' }
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
    fireEvent.change(screen.getByLabelText(/role/i), { target: { value: 'member' } })
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
})
