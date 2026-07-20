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

function usageResponse(periodBasis = 'calendar_month') {
  return {
    ok: true,
    json: async () => ({ data: { usage: {
      snapshotId: 7,
      periodStart: '2026-07-01T00:00:00Z',
      periodEnd: '2026-08-01T00:00:00Z',
      periodBasis,
      observedAt: '2026-07-20T02:00:00Z',
      sourceTableCount: 42,
      metrics: [
        { key: 'seats', label: 'Active team seats', used: 2, unit: 'seats', scope: 'current', source: 'active organization memberships' },
        { key: 'outbound_messages', label: 'Sent outbound email', used: 12, unit: 'messages', scope: 'period', source: 'outbound email messages recorded as sent' },
        { key: 'storage_bytes', label: 'Tenant database row storage', used: 1536, unit: 'bytes', scope: 'current', source: 'PostgreSQL row bytes across tenant-scoped base tables' }
      ]
    } } })
  }
}

describe('settings billing route', () => {
  it('shows unrestricted local usage without hosted lifecycle or plan controls in self-hosted mode', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(sessionResponse())
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { entitlements: {
          plan: { key: 'free', name: 'Free', monthlyPriceUsd: 0, features: ['saved_views'] },
          features: ['saved_views'],
          subscription: { managed: false, status: 'canceled', provider: 'fake' },
          seats: { used: 3, limit: -1, unlimited: true, exceeded: false },
          contacts: { used: 620, limit: -1, unlimited: true, exceeded: false },
          deals: { used: 280, limit: -1, unlimited: true, exceeded: false }
        } } })
      })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { plans: [{ key: 'free', name: 'Free', monthlyPriceUsd: 0, features: ['saved_views'] }] } }) })
      .mockResolvedValueOnce(usageResponse())
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { exports: [] } }) })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/billing')
    render(<AppRouter />)

    expect(await screen.findByText(/self-hosted mode does not enforce hosted trials/i)).toBeInTheDocument()
    expect(screen.getByText(/self-hosted · unmanaged/i)).toBeInTheDocument()
    expect(screen.getByText('620 / Unlimited')).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: /compare plans/i })).not.toBeInTheDocument()
    expect(screen.queryByText(/subscription is canceled/i)).not.toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /measured usage/i })).toBeInTheDocument()
    expect(screen.getByText(/UTC calendar month/i)).toBeInTheDocument()
  })

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
      .mockResolvedValueOnce(usageResponse('provider_subscription'))
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { exports: [] } }) })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/billing')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /plan & billing/i })).toBeInTheDocument()
    expect((await screen.findAllByText(/team seats/i)).length).toBeGreaterThan(0)
    expect(screen.getByText('600 / 500')).toBeInTheDocument()
    expect(screen.getByText(/over plan limit/i)).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /pro/i })).toBeInTheDocument()
    expect(screen.getByText('$49/mo')).toBeInTheDocument()
    expect(screen.getByText(/7 days left in your trial/i)).toBeInTheDocument()
    expect(screen.getByText('12 messages')).toBeInTheDocument()
    expect(screen.getByText('1.50 KiB')).toBeInTheDocument()
    expect(screen.getByText(/not enforced until hosted plan policy is approved/i)).toBeInTheDocument()
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
      .mockResolvedValueOnce(usageResponse())
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { exports: [] } }) })
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
      .mockResolvedValueOnce(usageResponse('provider_subscription'))
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { exports: [] } }) })
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
      .mockResolvedValueOnce(usageResponse('provider_subscription'))
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { exports: [] } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { id: 'bps_test', url: '' } }) })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/billing')
    render(<AppRouter />)

    fireEvent.click(await screen.findByRole('button', { name: /manage payment method, invoices, or cancellation/i }))
    await waitFor(() => {
      expect(fetchMock.mock.calls.some((call) => String(call[0]).endsWith('/api/billing/portal-session'))).toBe(true)
    })
  })

  it('keeps portable offboarding export available in hosted read-only mode', async () => {
    const checksum = 'a'.repeat(64)
    const readyExport = {
      id: 12,
      status: 'ready',
      byteSize: 524288,
      contentSha256: checksum,
      datasetCounts: { contacts: 2, companies: 1 },
      createdAt: '2026-07-20T00:00:00Z',
      expiresAt: '2026-07-27T00:00:00Z'
    }
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            user: { id: 1, email: 'owner@acme.test' },
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' },
            membership: { role: 'owner' },
            workspaceAccess: { state: 'read_only' }
          }
        })
      })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            entitlements: {
              plan: { key: 'pro', name: 'Pro', monthlyPriceUsd: 49, features: [] },
              features: [],
              subscription: { status: 'canceled', suspended: true },
              seats: { used: 1, limit: 25 }, contacts: { used: 2, limit: 50000 }, deals: { used: 1, limit: 50000 }
            }
          }
        })
      })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { plans: [{ key: 'pro', name: 'Pro', monthlyPriceUsd: 49, features: [] }] } }) })
      .mockResolvedValueOnce(usageResponse('provider_subscription'))
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { exports: [readyExport] } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { export: { id: 13, status: 'pending', datasetCounts: {}, createdAt: '2026-07-20T00:05:00Z' } } }) })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/billing')
    render(<AppRouter />)

    const download = await screen.findByRole('link', { name: /download zip/i })
    expect(download).toHaveAttribute('href', expect.stringMatching(/\/api\/workspace-exports\/12\/download$/))
    expect(screen.getByText(new RegExp(checksum))).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /create workspace export/i }))
    await waitFor(() => {
      const requestCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/workspace-exports') && call[1]?.method === 'POST')
      expect(requestCall).toBeTruthy()
      expect(requestCall[1].headers['Idempotency-Key']).toMatch(/^workspace-export-/)
    })
    expect(await screen.findByRole('heading', { name: /workspace export #13 · pending/i })).toBeInTheDocument()
  })
})
