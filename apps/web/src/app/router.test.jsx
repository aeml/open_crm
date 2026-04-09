import { describe, expect, it, vi, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from './router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AppRouter', () => {
  it('renders dashboard summary metrics and recent activity when authenticated', async () => {
    const fetchMock = vi
      .fn()
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
              slug: 'acme-inc',
              businessType: 'general'
            },
            membership: {
              role: 'owner'
            }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            pipelineValue: '48000.00',
            openDealsCount: 3,
            wonDealsCount: 1,
            openTasksCount: 8,
            dueTodayCount: 2,
            newContactsCount: 5,
            recentActivities: [
              {
                id: 91,
                action: 'deal.stage_changed',
                summary: 'Deal moved to Negotiation',
                entityType: 'deal',
                entityId: 12,
                actorName: 'Alex Admin',
                createdAt: '2026-04-10T12:00:00Z'
              }
            ]
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)

    window.history.pushState({}, '', '/dashboard')

    render(<AppRouter />)

    expect(await screen.findByText('$48,000.00')).toBeInTheDocument()
    expect(await screen.findByText(/deal moved to negotiation/i)).toBeInTheDocument()
    expect(screen.getByText('5 this week')).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/dashboard\/summary$/), expect.any(Object))
    })
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
