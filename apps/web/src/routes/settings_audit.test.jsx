import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('settings audit route', () => {
  it('shows audit events and lets admins filter them', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
            membership: { role: 'owner' }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { unreadCount: 0 } })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            policy: { mode: 'workspace_lifetime', summary: 'Audit events are immutable and retained for the workspace lifetime.', appendOnly: true, portableExportIncluded: true, standaloneExportMaxRows: 10000, deletionBoundary: 'Audit history is removed only with the workspace.' },
            events: [
              { id: 5, eventType: 'user.invited', entityType: 'user', entityId: 9, summary: 'Invited ops@acme.test as member', actorName: 'Demo Owner', createdAt: '2026-05-01T12:00:00Z', metadata: { email: 'ops@acme.test' } },
              { id: 4, eventType: 'organization.profile_updated', entityType: 'organization', entityId: 1, summary: 'Changed business profile to services', actorName: 'Demo Owner', createdAt: '2026-05-01T11:00:00Z', metadata: { businessType: 'services' } }
            ]
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            policy: { mode: 'workspace_lifetime', summary: 'Audit events are immutable and retained for the workspace lifetime.', appendOnly: true, portableExportIncluded: true, standaloneExportMaxRows: 10000, deletionBoundary: 'Audit history is removed only with the workspace.' },
            events: [
              { id: 5, eventType: 'user.invited', entityType: 'user', entityId: 9, summary: 'Invited ops@acme.test as member', actorName: 'Demo Owner', createdAt: '2026-05-01T12:00:00Z', metadata: { email: 'ops@acme.test' } }
            ]
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/audit')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /admin audit trail/i })).toBeInTheDocument()
    expect(await screen.findByText(/invited ops@acme.test as member/i)).toBeInTheDocument()
    expect(screen.getByText(/changed business profile to services/i)).toBeInTheDocument()
    expect(screen.getByText(/immutable and retained for the workspace lifetime/i)).toBeInTheDocument()
    expect(screen.getByText(/CSVs over 10,000 rows are refused/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /export filtered csv/i })).toHaveAttribute('href', expect.stringMatching(/\/api\/audit-events\/export\.csv$/))

    fireEvent.change(screen.getByLabelText(/audit event filter/i), { target: { value: 'user.invited' } })

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/audit-events\?eventType=user\.invited&limit=50$/), expect.any(Object))
    })
    expect(screen.getByRole('link', { name: /export filtered csv/i })).toHaveAttribute('href', expect.stringMatching(/\/api\/audit-events\/export\.csv\?eventType=user\.invited$/))
  })
})
