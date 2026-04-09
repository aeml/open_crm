import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('onboarding flow', () => {
  it('creates a workspace, signs in the owner, and lands on the dashboard', async () => {
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
        status: 201,
        json: async () => ({
          data: {
            user: {
              id: 10,
              email: 'owner@northstar.test',
              firstName: 'Morgan',
              lastName: 'Lee'
            },
            organization: {
              id: 42,
              name: 'Northstar Logistics',
              slug: 'northstar-logistics',
              businessType: 'product-sales'
            },
            membership: {
              role: 'owner'
            }
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)

    window.history.pushState({}, '', '/bootstrap')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /create your workspace/i })).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/company name/i), { target: { value: 'Northstar Logistics' } })
    fireEvent.change(screen.getByLabelText(/business type/i), { target: { value: 'product-sales' } })
    fireEvent.change(screen.getByLabelText(/first name/i), { target: { value: 'Morgan' } })
    fireEvent.change(screen.getByLabelText(/last name/i), { target: { value: 'Lee' } })
    fireEvent.change(screen.getByLabelText(/^email$/i), { target: { value: 'owner@northstar.test' } })
    fireEvent.change(screen.getByLabelText(/^password$/i), { target: { value: 'super-secret-password' } })
    fireEvent.click(screen.getByRole('button', { name: /create workspace/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringMatching(/\/auth\/bootstrap$/),
        expect.objectContaining({
          method: 'POST',
          credentials: 'include'
        })
      )
    })

    expect(await screen.findByText(/keep the next best action obvious/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(window.location.pathname).toBe('/dashboard')
    })
  })
})
