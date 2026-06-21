import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('business profile flow', () => {
  it('loads business profile settings and adapts labels for construction services', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'construction-services' },
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
            profile: {
              organizationId: 1,
              businessType: 'construction-services',
              baseCurrency: 'USD',
              displayName: 'Construction Services',
              modules: ['contacts', 'companies', 'deals', 'tasks', 'estimates'],
              exchangeRates: [],
              labels: {
                companies: 'Clients',
                deals: 'Jobs',
                tasks: 'Site Tasks',
                businessProfile: 'Business Profile'
              }
            }
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/business-profile')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /business profile/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByRole('option', { name: /^construction services \(clients \+ jobs\)$/i }).selected).toBe(true)
    })
    expect(screen.getByRole('link', { name: /clients/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /jobs/i })).toBeInTheDocument()
  })

  it('lets an owner update the business type and refreshes adaptive labels in the shell', async () => {
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
            profile: {
              organizationId: 1,
              businessType: 'general',
              baseCurrency: 'USD',
              displayName: 'General CRM',
              modules: ['contacts', 'companies', 'deals', 'tasks'],
              exchangeRates: [],
              labels: {
                companies: 'Companies',
                deals: 'Deals',
                tasks: 'Tasks',
                businessProfile: 'Business Profile'
              }
            }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            profile: {
              organizationId: 1,
              businessType: 'product-sales',
              baseCurrency: 'EUR',
              displayName: 'Product Sales',
              modules: ['contacts', 'companies', 'deals', 'tasks', 'catalog'],
              exchangeRates: [],
              labels: {
                companies: 'Accounts',
                deals: 'Opportunities',
                tasks: 'Follow-ups',
                businessProfile: 'Business Profile'
              }
            }
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/business-profile')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /business profile/i })).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/business type/i), { target: { value: 'product-sales' } })
    fireEvent.change(screen.getByLabelText(/base currency/i), { target: { value: 'EUR' } })
    fireEvent.click(screen.getByRole('button', { name: /save business profile/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/organization\/profile$/), expect.objectContaining({ method: 'PATCH' }))
    })

    expect(await screen.findByRole('link', { name: /accounts/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /opportunities/i })).toBeInTheDocument()
  })

  it('lets an owner save a manual exchange rate', async () => {
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const method = options.method || 'GET'

      if (requestURL.pathname.endsWith('/auth/me')) {
        return { ok: true, json: async () => ({ data: { user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' }, organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' }, membership: { role: 'owner' } } }) }
      }
      if (requestURL.pathname.endsWith('/api/notifications/unread-count')) {
        return { ok: true, json: async () => ({ data: { unreadCount: 0 } }) }
      }
      if (requestURL.pathname.endsWith('/api/organization/profile') && method === 'GET') {
        return { ok: true, json: async () => ({ data: { profile: { organizationId: 1, businessType: 'general', baseCurrency: 'USD', displayName: 'General CRM', modules: ['contacts', 'companies', 'deals', 'tasks'], exchangeRates: [], labels: { companies: 'Companies', deals: 'Deals', tasks: 'Tasks' } } } }) }
      }
      if (requestURL.pathname.endsWith('/api/organization/exchange-rates/EUR') && method === 'PUT') {
        return { ok: true, json: async () => ({ data: { profile: { organizationId: 1, businessType: 'general', baseCurrency: 'USD', displayName: 'General CRM', modules: ['contacts', 'companies', 'deals', 'tasks'], exchangeRates: [{ id: 7, baseCurrency: 'USD', quoteCurrency: 'EUR', rateToBase: '1.08000000', effectiveDate: '2026-06-20', source: 'manual' }], labels: { companies: 'Companies', deals: 'Deals', tasks: 'Tasks' } } } }) }
      }

      throw new Error(`Unexpected fetch: ${method} ${requestURL.pathname}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/business-profile')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /currency rates/i })).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/quote currency/i), { target: { value: 'EUR' } })
    fireEvent.change(screen.getByLabelText(/rate to usd/i), { target: { value: '1.08' } })
    fireEvent.change(screen.getByLabelText(/effective date/i), { target: { value: '2026-06-20' } })
    fireEvent.click(screen.getByRole('button', { name: /save exchange rate/i }))

    expect(await screen.findByText(/eur to usd/i)).toBeInTheDocument()
    expect(screen.getByText('1.08000000')).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/organization\/exchange-rates\/EUR$/), expect.objectContaining({
        method: 'PUT',
        body: expect.stringContaining('"rateToBase":"1.08"')
      }))
    })
  })
})
