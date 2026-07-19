import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('unverified login recovery', () => {
  it('offers an enumeration-safe verification resend after a correct unverified login', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: false, status: 401, json: async () => ({ error: { code: 'UNAUTHORIZED', message: 'Authentication required' } }) })
      .mockResolvedValueOnce({ ok: false, status: 403, json: async () => ({ error: { code: 'EMAIL_VERIFICATION_REQUIRED', message: 'Verify your email before signing in' } }) })
      .mockResolvedValueOnce({ ok: true, status: 202, json: async () => ({ data: { accepted: true } }) })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/login')

    render(<AppRouter />)
    expect(await screen.findByRole('heading', { name: /sign in to open crm/i })).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'owner@northstar.test' } })
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'correct-unverified-password' } })
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }))

    expect(await screen.findByText('Verify your email before signing in')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Resend verification email' }))
    expect(await screen.findByText(/if this address is awaiting verification/i)).toBeInTheDocument()
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      expect.stringMatching(/\/auth\/resend-verification$/),
      expect.objectContaining({ method: 'POST', body: JSON.stringify({ email: 'owner@northstar.test' }) })
    ))
  })
})
