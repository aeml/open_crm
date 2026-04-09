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
        json: async () => ({
          data: {
            profile: {
              organizationId: 1,
              businessType: 'construction-services',
              displayName: 'Construction Services',
              modules: ['contacts', 'companies', 'deals', 'tasks', 'estimates'],
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
    expect(await screen.findByDisplayValue('construction-services')).toBeInTheDocument()
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
        json: async () => ({
          data: {
            profile: {
              organizationId: 1,
              businessType: 'general',
              displayName: 'General CRM',
              modules: ['contacts', 'companies', 'deals', 'tasks'],
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
              displayName: 'Product Sales',
              modules: ['contacts', 'companies', 'deals', 'tasks', 'catalog'],
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
    fireEvent.click(screen.getByRole('button', { name: /save business profile/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/organization\/profile$/), expect.objectContaining({ method: 'PATCH' }))
    })

    expect(await screen.findByRole('link', { name: /accounts/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /opportunities/i })).toBeInTheDocument()
  })
})
