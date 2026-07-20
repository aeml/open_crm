import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

function unauthenticatedResponse() {
  return {
    ok: false,
    status: 401,
    json: async () => ({ error: { code: 'UNAUTHORIZED', message: 'Authentication required' } })
  }
}

describe('password recovery flow', () => {
  it('requests generically and exposes the local fake-provider link', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(unauthenticatedResponse())
      .mockResolvedValueOnce({
        ok: true,
        status: 202,
        json: async () => ({ data: { accepted: true, resetLink: '/reset-password?token=local-reset-token' } })
      })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/forgot-password')

    render(<AppRouter />)
    expect(await screen.findByRole('heading', { name: 'Reset your password' })).toBeInTheDocument()
    expect(screen.getByText(/result is the same whether or not an active account exists/i)).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'owner@example.test' } })
    fireEvent.click(screen.getByRole('button', { name: 'Send reset link' }))

    expect(await screen.findByRole('heading', { name: 'Check your email' })).toBeInTheDocument()
    expect(screen.getByText(/if an active open crm account matches/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Reset password locally' })).toHaveAttribute('href', '/reset-password?token=local-reset-token')
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      expect.stringMatching(/\/auth\/request-password-reset$/),
      expect.objectContaining({ method: 'POST', credentials: 'include', body: JSON.stringify({ email: 'owner@example.test' }) })
    ))
  })

  it('validates confirmation and completes a one-time reset', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(unauthenticatedResponse())
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => ({ data: { status: 'password_reset' } }) })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/reset-password?token=reset-token-123')

    render(<AppRouter />)
    expect(await screen.findByRole('heading', { name: 'Choose a new password' })).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('New password'), { target: { value: 'Replacement-Password-28!' } })
    fireEvent.change(screen.getByLabelText('Confirm new password'), { target: { value: 'Different-Password-29!' } })
    fireEvent.click(screen.getByRole('button', { name: 'Reset password' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Passwords do not match.')
    expect(fetchMock).toHaveBeenCalledTimes(1)

    fireEvent.change(screen.getByLabelText('Confirm new password'), { target: { value: 'Replacement-Password-28!' } })
    fireEvent.click(screen.getByRole('button', { name: 'Reset password' }))
    expect(await screen.findByRole('heading', { name: 'Password reset complete' })).toBeInTheDocument()
    expect(screen.getByText(/old sessions have been signed out/i)).toBeInTheDocument()
    expect(window.location.search).toBe('')
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      expect.stringMatching(/\/auth\/reset-password$/),
      expect.objectContaining({
        method: 'POST',
        credentials: 'include',
        body: JSON.stringify({ token: 'reset-token-123', password: 'Replacement-Password-28!' })
      })
    ))
  })

  it('blocks completion when the reset token is absent', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(unauthenticatedResponse()))
    window.history.pushState({}, '', '/reset-password')

    render(<AppRouter />)
    expect(await screen.findByText(/reset token is missing/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Reset password' })).toBeDisabled()
  })
})
