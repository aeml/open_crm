import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('settings billing route', () => {
  it('shows the active plan, usage, and plan comparison', async () => {
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
            entitlements: {
              plan: { key: 'free', name: 'Free', description: 'Get started', monthlyPriceUsd: 0, features: ['saved_views', 'csv_export'] },
              features: ['saved_views', 'csv_export'],
              seats: { used: 2, limit: 2, unlimited: false, exceeded: false },
              contacts: { used: 600, limit: 500, unlimited: false, exceeded: true },
              deals: { used: 10, limit: 250, unlimited: false, exceeded: false }
            }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            plans: [
              { key: 'free', name: 'Free', description: 'Get started', monthlyPriceUsd: 0, features: ['saved_views', 'csv_export'] },
              { key: 'pro', name: 'Pro', description: 'Scaling teams', monthlyPriceUsd: 49, features: ['automation', 'api_access'] }
            ]
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/billing')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /plan & billing/i })).toBeInTheDocument()
    expect(await screen.findByText(/team seats/i)).toBeInTheDocument()
    expect(screen.getByText('600 / 500')).toBeInTheDocument()
    expect(screen.getByText(/over plan limit/i)).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /pro/i })).toBeInTheDocument()
    expect(screen.getByText('$49/mo')).toBeInTheDocument()
  })
})
