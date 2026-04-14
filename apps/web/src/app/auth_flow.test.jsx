import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from './router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('auth flow', () => {
  it('redirects protected routes to login when auth check returns unauthorized', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 401,
        json: async () => ({
          error: {
            code: 'UNAUTHORIZED',
            message: 'Authentication required'
          }
        })
      })
    )

    window.history.pushState({}, '', '/companies')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /sign in to open crm/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(window.location.pathname).toBe('/login')
    })
  })

  it('submits login credentials and lands on dashboard after success', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: false,
        status: 401,
        json: async () => ({
          error: {
            code: 'UNAUTHORIZED',
            message: 'Authentication required'
          }
        })
      })
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

    vi.stubGlobal('fetch', fetchMock)

    window.history.pushState({}, '', '/dashboard')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /sign in to open crm/i })).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/email/i), { target: { value: 'owner@acme.test' } })
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: 'opencrm-demo-password' } })
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringMatching(/\/auth\/login$/),
        expect.objectContaining({
          method: 'POST',
          credentials: 'include'
        })
      )
    })

    expect(await screen.findByText(/see what is live in the pipeline/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(window.location.pathname).toBe('/dashboard')
    })
  })
})
