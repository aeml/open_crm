import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { SessionManagement } from './session_management'

afterEach(() => {
  vi.unstubAllGlobals()
})

function response(data, { ok = true, status = 200 } = {}) {
  return { ok, status, json: async () => data }
}

const currentSession = {
  id: 11,
  organization: { id: 1, name: 'Acme Services' },
  createdAt: '2026-07-19T14:00:00Z',
  lastSeenAt: '2026-07-20T14:00:00Z',
  expiresAt: '2026-08-19T14:00:00Z',
  current: true
}

const otherSession = {
  id: 19,
  organization: { id: 1, name: 'Acme Services' },
  createdAt: '2026-07-18T10:00:00Z',
  lastSeenAt: '2026-07-18T11:00:00Z',
  expiresAt: '2026-08-17T10:00:00Z',
  current: false
}

describe('active sign-ins', () => {
  it('identifies the current session and revokes another only after confirmation', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response({ data: { sessions: [currentSession, otherSession] } }))
      .mockResolvedValueOnce(response({}, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)

    render(<SessionManagement />)

    expect(await screen.findByRole('heading', { name: 'Active sign-ins' })).toBeInTheDocument()
    const rows = await screen.findAllByRole('listitem')
    expect(within(rows[0]).getByText('This sign-in')).toBeInTheDocument()
    expect(within(rows[0]).queryByRole('button', { name: 'Sign out' })).not.toBeInTheDocument()
    fireEvent.click(within(rows[1]).getByRole('button', { name: 'Sign out' }))
    expect(fetchMock).toHaveBeenCalledTimes(1)
    fireEvent.click(within(rows[1]).getByRole('button', { name: 'Confirm sign out' }))

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      expect.stringMatching(/\/api\/me\/sessions\/19$/),
      expect.objectContaining({ method: 'DELETE', credentials: 'include' })
    ))
    expect(await screen.findByText('That sign-in has been ended.')).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(1)
  })

  it('revokes every other session while preserving the current row', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response({ data: { sessions: [currentSession, otherSession, { ...otherSession, id: 20 }] } }))
      .mockResolvedValueOnce(response({ data: { revoked: 2 } }))
    vi.stubGlobal('fetch', fetchMock)

    render(<SessionManagement />)
    fireEvent.click(await screen.findByRole('button', { name: 'Sign out all other sessions' }))
    expect(fetchMock).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: 'Confirm sign out all others' }))

    expect(await screen.findByText('2 other sign-ins have been ended.')).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(1)
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringMatching(/\/api\/me\/sessions\/others$/),
      expect.objectContaining({ method: 'DELETE', credentials: 'include' })
    )
  })

  it('shows a recoverable loading error without inventing session data', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response({ error: { message: 'Sign-ins unavailable.' } }, { ok: false, status: 500 }))
      .mockResolvedValueOnce(response({ data: { sessions: [currentSession] } }))
    vi.stubGlobal('fetch', fetchMock)

    render(<SessionManagement />)

    expect(await screen.findByRole('alert')).toHaveTextContent('Sign-ins unavailable.')
    expect(screen.queryByRole('listitem')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('This sign-in')).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})
