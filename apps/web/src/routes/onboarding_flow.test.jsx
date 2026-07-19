import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('onboarding flow', () => {
  it('provisions idempotently, requires email verification, then starts the owner session', async () => {
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
            email: 'owner@northstar.test',
            verificationRequired: true,
            verificationLink: '/verify-email?token=verification-token-123',
            created: true
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
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
    expect(screen.getByRole('link', { name: /sign in/i })).toHaveAttribute('href', '/login')
    expect(screen.getByRole('option', { name: /^services \(clients \+ jobs\)$/i })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: /^product sales \(accounts \+ opportunities\)$/i })).toBeInTheDocument()

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

    const bootstrapBody = JSON.parse(fetchMock.mock.calls.find(([url]) => String(url).endsWith('/auth/bootstrap'))[1].body)
    expect(bootstrapBody.idempotencyKey).toMatch(/^workspace-/)
    expect(await screen.findByRole('heading', { name: /check your email/i })).toBeInTheDocument()
    expect(screen.getByText(/trial starts only after verification/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('link', { name: /verify email locally/i }))

    expect(await screen.findByText(/see what is live in the pipeline/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(window.location.pathname).toBe('/dashboard')
    })
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringMatching(/\/auth\/verify-email$/),
      expect.objectContaining({ method: 'POST', credentials: 'include', body: JSON.stringify({ token: 'verification-token-123' }) })
    )
  })
})
