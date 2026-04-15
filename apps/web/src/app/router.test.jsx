import { describe, expect, it, vi, afterEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
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

  it('adapts dashboard metric copy for service businesses', async () => {
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
              businessType: 'services'
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
            pipelineValue: '12400.00',
            openDealsCount: 4,
            wonDealsCount: 2,
            openTasksCount: 6,
            dueTodayCount: 3,
            newContactsCount: 2,
            recentActivities: [
              {
                id: 91,
                action: 'deal.stage_changed',
                summary: 'Job moved to Quote',
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

    expect(await screen.findByText(/open jobs value/i)).toBeInTheDocument()
    expect(screen.getAllByText(/open jobs/i).length).toBeGreaterThan(0)
    expect(screen.getByText(/won jobs/i)).toBeInTheDocument()
    expect(screen.getByText(/jobs, contacts, clients, and service tasks/i)).toBeInTheDocument()
  })

  it('links dashboard task actions into filtered task views', async () => {
    const jsonResponse = (payload, init = {}) => ({
      ok: init.ok ?? true,
      status: init.status ?? 200,
      json: async () => payload
    })

    const fetchMock = vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')

      if (requestURL.pathname.endsWith('/auth/me')) {
        return jsonResponse({
          data: {
            user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
            membership: { role: 'owner' }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/dashboard/summary')) {
        return jsonResponse({
          data: {
            pipelineValue: '48000.00',
            openDealsCount: 3,
            wonDealsCount: 1,
            openTasksCount: 8,
            dueTodayCount: 2,
            newContactsCount: 5,
            recentActivities: []
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks') && requestURL.searchParams.get('status') === 'open') {
        return jsonResponse({
          data: {
            tasks: [
              {
                id: 51,
                entityType: 'company',
                entityId: 6,
                entityLabel: 'Bluebird Health',
                title: 'Verify site access window',
                description: 'Need lockbox confirmation.',
                status: 'open',
                dueAt: '2099-04-18T15:00:00Z',
                completedAt: '',
                assignedToUserId: 0,
                assignedToUserName: '',
                createdByUserId: 1,
                createdByUserName: 'Demo Owner'
              }
            ],
            meta: { page: 1, pageSize: 20, total: 1, openCount: 1, completedCount: 0 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/deals')) {
        return jsonResponse({ data: { deals: [], meta: { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' } } })
      }

      if (requestURL.pathname.endsWith('/api/companies')) {
        return jsonResponse({ data: { companies: [], meta: { page: 1, pageSize: 20, total: 0 } } })
      }

      if (requestURL.pathname.endsWith('/api/contacts')) {
        return jsonResponse({ data: { contacts: [], meta: { page: 1, pageSize: 20, total: 0 } } })
      }

      if (requestURL.pathname.endsWith('/api/users')) {
        return jsonResponse({ data: { users: [{ id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' }] } })
      }

      throw new Error(`Unexpected fetch: ${requestURL.pathname}${requestURL.search}`)
    })

    vi.stubGlobal('fetch', fetchMock)

    window.history.pushState({}, '', '/dashboard')

    render(<AppRouter />)

    expect(await screen.findByText('$48,000.00')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /review due today/i }))

    expect(await screen.findByRole('heading', { name: /^tasks due today$/i })).toBeInTheDocument()
    expect(window.location.pathname).toBe('/tasks')
    expect(window.location.search).toBe('?due=dueToday')
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks\?status=open$/), expect.any(Object))
    })
  })

  it('loads task detail routes directly', async () => {
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
            tasks: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0 }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            deals: [
              { id: 12, name: 'Bluebird Rollout', stageId: 2, stageName: 'Qualified', companyId: 6, companyName: 'Bluebird Health', primaryContactId: 8, primaryContactName: 'Ava Stone', status: 'open', valueAmount: '60000.00', valueCurrency: 'USD', expectedCloseDate: '2026-05-02', ownerUserId: 1 }
            ],
            meta: { page: 1, pageSize: 20, total: 1, openCount: 1, wonCount: 0, pipelineValue: '60000.00' }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            companies: [
              { id: 6, name: 'Bluebird Health', industry: 'Healthcare', phone: '555-0200', website: 'https://bluebird.example', status: 'prospect' }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            contacts: [
              { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@bluebird.example', phone: '555-0300', jobTitle: 'Operations Director', status: 'lead' }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            users: [
              { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' }
            ]
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            task: {
              id: 77,
              entityType: 'deal',
              entityId: 12,
              entityLabel: 'Bluebird Rollout',
              title: 'Prepare rollout checklist',
              description: 'Lock owners before kickoff.',
              status: 'open',
              dueAt: '2026-04-16T09:00:00Z',
              completedAt: '',
              assignedToUserId: 1,
              assignedToUserName: 'Demo Owner',
              createdByUserId: 1,
              createdByUserName: 'Demo Owner'
            },
            activities: [
              {
                id: 201,
                action: 'task.created',
                summary: 'Task created'
              }
            ]
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)

    window.history.pushState({}, '', '/tasks/77')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /prepare rollout checklist/i })).toBeInTheDocument()
    expect(window.location.pathname).toBe('/tasks/77')
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks\/77$/), expect.any(Object))
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

  it('redirects the old contacts workspace route to clients', async () => {
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
            companies: [
              { id: 5, name: 'Northstar Logistics', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            contacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', jobTitle: 'Head of RevOps', status: 'lead' }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            users: [
              { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' }
            ]
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)

    window.history.pushState({}, '', '/contacts')

    render(<AppRouter />)

    expect(await screen.findByText(/see client ownership, linked people, and live pipeline in one place/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(window.location.pathname).toBe('/companies')
    })
  })

  it('logs out from the app header and returns to login', async () => {
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
            recentActivities: []
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 204,
        json: async () => ({})
      })

    vi.stubGlobal('fetch', fetchMock)

    window.history.pushState({}, '', '/dashboard')

    render(<AppRouter />)

    expect(await screen.findByText('$48,000.00')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /log out/i }))

    expect(await screen.findByRole('heading', { name: /sign in to open crm/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(window.location.pathname).toBe('/login')
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/auth\/logout$/), expect.objectContaining({ method: 'POST' }))
    })
  })
})
