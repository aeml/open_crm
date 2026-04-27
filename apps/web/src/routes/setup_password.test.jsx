import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('setup password route', () => {
  it('submits setup token and new password', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: false,
        status: 401,
        json: async () => ({ error: { code: 'UNAUTHORIZED', message: 'Session required' } })
      })
      .mockResolvedValueOnce({ ok: true, status: 204, json: async () => ({}) })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/setup-password?token=setup-token-123')

    render(<AppRouter />)

    fireEvent.change(await screen.findByLabelText(/new password/i), { target: { value: 'new-password' } })
    fireEvent.click(screen.getByRole('button', { name: /set password/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringMatching(/\/auth\/setup-password$/),
        expect.objectContaining({
          method: 'POST',
          credentials: 'include',
          body: JSON.stringify({ token: 'setup-token-123', password: 'new-password' })
        })
      )
    })
  })
})
