import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
  window.sessionStorage.clear()
})

function sessionResponse() {
  return {
    ok: true,
    json: async () => ({
      data: {
        user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
        organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
        membership: { role: 'owner' }
      }
    })
  }
}

describe('settings billing route', () => {
  it('shows the active plan, usage, and plan comparison', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(sessionResponse())
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
              subscription: { status: 'trialing', inTrial: true, trialDaysLeft: 7 },
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
    expect(screen.getByText(/7 days left in your trial/i)).toBeInTheDocument()
  })

  it('lets an owner switch plans', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(sessionResponse())
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            entitlements: {
              plan: { key: 'free', name: 'Free', description: 'Get started', monthlyPriceUsd: 0, features: ['saved_views'] },
              features: ['saved_views'],
              seats: { used: 1, limit: 2, unlimited: false, exceeded: false },
              contacts: { used: 10, limit: 500, unlimited: false, exceeded: false },
              deals: { used: 2, limit: 250, unlimited: false, exceeded: false }
            }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            plans: [
              { key: 'free', name: 'Free', description: 'Get started', monthlyPriceUsd: 0, features: ['saved_views'] },
              { key: 'pro', name: 'Pro', description: 'Scaling teams', monthlyPriceUsd: 49, features: ['automation'] }
            ]
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            entitlements: {
              plan: { key: 'pro', name: 'Pro', description: 'Scaling teams', monthlyPriceUsd: 49, features: ['automation'] },
              features: ['automation'],
              seats: { used: 1, limit: 25, unlimited: false, exceeded: false },
              contacts: { used: 10, limit: 50000, unlimited: false, exceeded: false },
              deals: { used: 2, limit: 50000, unlimited: false, exceeded: false }
            }
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/billing')

    render(<AppRouter />)

    const switchButton = await screen.findByRole('button', { name: /switch to this plan/i })
    fireEvent.click(switchButton)

    await waitFor(() => {
      const changeCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/billing/change-plan'))
      expect(changeCall).toBeTruthy()
      expect(JSON.parse(changeCall[1].body)).toEqual({ plan: 'pro' })
    })
    expect(await screen.findByRole('heading', { name: /pro · current plan/i })).toBeInTheDocument()
  })

  it('uses hosted checkout without activating a Stripe plan in the browser', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(sessionResponse())
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            entitlements: {
              plan: { key: 'free', name: 'Free', description: 'Get started', monthlyPriceUsd: 0, features: ['saved_views'] },
              features: ['saved_views'],
              subscription: { status: 'trialing', provider: 'stripe', checkoutAvailablePlans: ['pro'], portalAvailable: false },
              seats: { used: 1, limit: 2, unlimited: false, exceeded: false },
              contacts: { used: 10, limit: 500, unlimited: false, exceeded: false },
              deals: { used: 2, limit: 250, unlimited: false, exceeded: false }
            }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            plans: [
              { key: 'free', name: 'Free', description: 'Get started', monthlyPriceUsd: 0, features: ['saved_views'] },
              { key: 'pro', name: 'Pro', description: 'Scaling teams', monthlyPriceUsd: 49, features: ['automation'] }
            ]
          }
        })
      })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { id: 'cs_test', url: '' } }) })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/billing')
    render(<AppRouter />)

    fireEvent.click(await screen.findByRole('button', { name: /continue to secure checkout/i }))
    await waitFor(() => {
      const checkoutCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/billing/checkout-session'))
      expect(checkoutCall).toBeTruthy()
      const body = JSON.parse(checkoutCall[1].body)
      expect(body.plan).toBe('pro')
      expect(body.idempotencyKey).toMatch(/^checkout-/)
    })
    expect(screen.queryByRole('button', { name: /switch to this plan/i })).not.toBeInTheDocument()
  })

  it('opens the hosted portal for an established Stripe customer', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(sessionResponse())
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            entitlements: {
              plan: { key: 'pro', name: 'Pro', description: 'Scaling teams', monthlyPriceUsd: 49, features: ['automation'] },
              features: ['automation'],
              subscription: { status: 'active', provider: 'stripe', customerEstablished: true, portalAvailable: true, checkoutAvailablePlans: [] },
              seats: { used: 1, limit: 25, unlimited: false, exceeded: false },
              contacts: { used: 10, limit: 50000, unlimited: false, exceeded: false },
              deals: { used: 2, limit: 50000, unlimited: false, exceeded: false }
            }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { plans: [{ key: 'pro', name: 'Pro', description: 'Scaling teams', monthlyPriceUsd: 49, features: ['automation'] }] } })
      })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { id: 'bps_test', url: '' } }) })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/billing')
    render(<AppRouter />)

    fireEvent.click(await screen.findByRole('button', { name: /manage payment method, invoices, or cancellation/i }))
    await waitFor(() => {
      expect(fetchMock.mock.calls.some((call) => String(call[0]).endsWith('/api/billing/portal-session'))).toBe(true)
    })
  })
})
