import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

function sessionResponse(role) {
  return { ok: true, json: async () => ({ data: { user: { id: 1 }, organization: { id: 1, name: 'Acme' }, membership: { role } } }) }
}

const unreadResponse = { ok: true, json: async () => ({ data: { unreadCount: 0 } }) }

describe('archived records settings route', () => {
  it('lets a member search and restore a recoverable record', async () => {
    const archivedRecord = { entityType: 'task', entityId: 19, label: 'Call Ava', ownerName: 'Mina Park', archivedAt: '2026-07-19T12:00:00Z' }
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(sessionResponse('member'))
      .mockResolvedValueOnce(unreadResponse)
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { records: [archivedRecord] } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { record: archivedRecord } }) })
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('confirm', vi.fn(() => true))
    window.history.pushState({}, '', '/settings/archived-records')
    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /archived records/i })).toBeInTheDocument()
    expect(await screen.findByText('Call Ava')).toBeInTheDocument()
    expect(screen.getByText(/owner Mina Park/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Restore' }))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/data-operations\/archive\/task\/19\/restore$/), expect.objectContaining({ method: 'POST' }))
    })
    expect(await screen.findByText(/Call Ava was restored/i)).toBeInTheDocument()
    expect(screen.queryByText(/owner Mina Park/i)).not.toBeInTheDocument()
  })

  it('shows permanent merge history and prevents viewers from restoring', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(sessionResponse('viewer'))
      .mockResolvedValueOnce(unreadResponse)
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { records: [
        { entityType: 'contact', entityId: 7, label: 'Merged Source', archivedAt: '2026-07-19T12:00:00Z', restoreBlockedReason: 'This record was consumed by a duplicate merge and is retained as permanent history.' },
        { entityType: 'company', entityId: 8, label: 'Archived Client', archivedAt: '2026-07-18T12:00:00Z' }
      ] } }) })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/archived-records')
    render(<AppRouter />)

    expect(await screen.findByText(/consumed by a duplicate merge/i)).toBeInTheDocument()
    expect(screen.getByText(/view-only role/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Restore' })).not.toBeInTheDocument()
  })
})
