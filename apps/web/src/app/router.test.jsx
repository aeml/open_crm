import { describe, expect, it, vi, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from './router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AppRouter', () => {
  it('renders dashboard content at the default route shell when authenticated', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
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
    )

    window.history.pushState({}, '', '/dashboard')

    render(<AppRouter />)

    expect(await screen.findByText(/keep the next best action obvious/i)).toBeInTheDocument()
  })

  it('redirects protected routes to login when unauthenticated', async () => {
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

    window.history.pushState({}, '', '/contacts')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /sign in to open crm/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(window.location.pathname).toBe('/login')
    })
  })
})
