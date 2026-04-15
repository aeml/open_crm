import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('deals flow', () => {
  it('filters deals with the shared search pattern', async () => {
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
            stages: [
              { id: 1, name: 'Lead', position: 1, isClosed: false, isWon: false },
              { id: 2, name: 'Qualified', position: 2, isClosed: false, isWon: false }
            ]
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            deals: [
              { id: 11, name: 'Northstar Expansion', stageId: 2, stageName: 'Qualified', companyId: 5, companyName: 'Northstar Logistics', primaryContactId: 7, primaryContactName: 'Morgan Lee', status: 'open', valueAmount: '48000.00', valueCurrency: 'USD', expectedCloseDate: '2026-04-19', ownerUserId: 1 },
              { id: 12, name: 'Bluebird Rollout', stageId: 2, stageName: 'Qualified', companyId: 6, companyName: 'Bluebird Health', primaryContactId: 8, primaryContactName: 'Ava Stone', status: 'open', valueAmount: '60000.00', valueCurrency: 'USD', expectedCloseDate: '2026-05-02', ownerUserId: 1 }
            ],
            meta: { page: 1, pageSize: 20, total: 2, openCount: 2, wonCount: 0, pipelineValue: '108000.00' }
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
              { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' },
              { id: 2, email: 'alex@acme.test', firstName: 'Alex', lastName: 'Admin', role: 'admin' }
            ]
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

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/deals')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /deals/i })).toBeInTheDocument()
    expect(await screen.findByText(/northstar expansion/i)).toBeInTheDocument()
    expect(screen.getByText(/showing 2 of 2 deals/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/search deals/i), { target: { value: 'bluebird' } })

    expect(await screen.findByText(/showing 1 of 1 deals/i)).toBeInTheDocument()
    expect(window.location.search).toBe('?q=bluebird')
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/deals\?q=bluebird$/), expect.any(Object))
    })
  })

  it('hydrates deal filters from the query string on first load', async () => {
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
            stages: [
              { id: 1, name: 'Lead', position: 1, isClosed: false, isWon: false },
              { id: 2, name: 'Qualified', position: 2, isClosed: false, isWon: false }
            ]
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            deals: [
              { id: 12, name: 'Bluebird Rollout', stageId: 2, stageName: 'Qualified', companyId: 6, companyName: 'Bluebird Health', primaryContactId: 8, primaryContactName: 'Ava Stone', status: 'open', valueAmount: '60000.00', valueCurrency: 'USD', expectedCloseDate: '2026-05-02', ownerUserId: 2 }
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
              { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' },
              { id: 2, email: 'alex@acme.test', firstName: 'Alex', lastName: 'Admin', role: 'admin' }
            ]
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/deals?q=bluebird&stage=2&owner=2')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /deals/i })).toBeInTheDocument()
    expect(screen.getByLabelText(/search deals/i)).toHaveValue('bluebird')
    expect(screen.getByLabelText(/stage filter/i)).toHaveValue('2')
    expect(screen.getByLabelText(/owner filter/i)).toHaveValue('2')
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/deals\?q=bluebird&stageId=2&ownerUserId=2$/), expect.any(Object))
    })
  })

  it('prefills company and primary contact from the query string when creating a deal', async () => {
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
            stages: [
              { id: 1, name: 'Lead', position: 1, isClosed: false, isWon: false },
              { id: 2, name: 'Qualified', position: 2, isClosed: false, isWon: false }
            ]
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            deals: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            companies: [
              { id: 5, name: 'Northstar Logistics', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' },
              { id: 6, name: 'Bluebird Health', industry: 'Healthcare', phone: '555-0201', website: 'https://bluebird.example', status: 'prospect' }
            ],
            meta: { page: 1, pageSize: 20, total: 2 }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            contacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', jobTitle: 'Head of RevOps', status: 'lead' },
              { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0101', jobTitle: 'COO', status: 'lead' }
            ],
            meta: { page: 1, pageSize: 20, total: 2 }
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
    window.history.pushState({}, '', '/deals?companyId=5&primaryContactId=7')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /deals/i })).toBeInTheDocument()
    expect(screen.getByLabelText(/^company$/i)).toHaveValue('5')
    expect(screen.getByLabelText(/primary contact/i)).toHaveValue('7')
  })

  it('loads stages and deals, creates a deal, and moves it to another stage', async () => {
    let hasCreatedDeal = false
    let currentDeal = { id: 12, name: 'Bluebird Rollout', stageId: 2, stageName: 'Qualified', companyId: 6, companyName: 'Bluebird Health', primaryContactId: 8, primaryContactName: 'Ava Stone', status: 'open', valueAmount: '60000.00', valueCurrency: 'USD', expectedCloseDate: '2026-05-02', ownerUserId: 2 }

    const jsonResponse = (payload, init = {}) => ({
      ok: init.ok ?? true,
      status: init.status ?? 200,
      json: async () => payload
    })

    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const method = options.method || 'GET'

      if (requestURL.pathname.endsWith('/auth/me')) {
        return jsonResponse({
          data: {
            user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
            membership: { role: 'owner' }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/deal-stages')) {
        return jsonResponse({
          data: {
            stages: [
              { id: 1, name: 'Lead', position: 1, isClosed: false, isWon: false },
              { id: 2, name: 'Qualified', position: 2, isClosed: false, isWon: false },
              { id: 3, name: 'Proposal', position: 3, isClosed: false, isWon: false }
            ]
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/deals') && method === 'GET') {
        const stageId = requestURL.searchParams.get('stageId') || ''
        const ownerUserId = requestURL.searchParams.get('ownerUserId') || ''
        const defaultDeals = hasCreatedDeal
          ? [
              { id: 11, name: 'Northstar Expansion', stageId: 3, stageName: 'Proposal', companyId: 5, companyName: 'Northstar Logistics', primaryContactId: 7, primaryContactName: 'Morgan Lee', status: 'open', valueAmount: '48000.00', valueCurrency: 'USD', expectedCloseDate: '2026-04-19', ownerUserId: 1 },
              currentDeal
            ]
          : [
              { id: 11, name: 'Northstar Expansion', stageId: 3, stageName: 'Proposal', companyId: 5, companyName: 'Northstar Logistics', primaryContactId: 7, primaryContactName: 'Morgan Lee', status: 'open', valueAmount: '48000.00', valueCurrency: 'USD', expectedCloseDate: '2026-04-19', ownerUserId: 1 }
            ]

        if (stageId === '2' && ownerUserId === '1') {
          return jsonResponse({
            data: {
              deals: [],
              meta: { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' }
            }
          })
        }

        if (stageId === '2') {
          return jsonResponse({
            data: {
              deals: hasCreatedDeal ? [currentDeal] : [],
              meta: { page: 1, pageSize: 20, total: hasCreatedDeal ? 1 : 0, openCount: hasCreatedDeal ? 1 : 0, wonCount: 0, pipelineValue: hasCreatedDeal ? currentDeal.valueAmount : '0' }
            }
          })
        }

        if (ownerUserId === '1') {
          return jsonResponse({
            data: {
              deals: [
                { id: 11, name: 'Northstar Expansion', stageId: 3, stageName: 'Proposal', companyId: 5, companyName: 'Northstar Logistics', primaryContactId: 7, primaryContactName: 'Morgan Lee', status: 'open', valueAmount: '48000.00', valueCurrency: 'USD', expectedCloseDate: '2026-04-19', ownerUserId: 1 }
              ],
              meta: { page: 1, pageSize: 20, total: 1, openCount: 1, wonCount: 0, pipelineValue: '48000.00' }
            }
          })
        }

        return jsonResponse({
          data: {
            deals: defaultDeals,
            meta: {
              page: 1,
              pageSize: 20,
              total: defaultDeals.length,
              openCount: defaultDeals.length,
              wonCount: 0,
              pipelineValue: hasCreatedDeal ? '108000.00' : '48000.00'
            }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/companies') && method === 'GET') {
        return jsonResponse({
          data: {
            companies: [
              { id: 6, name: 'Bluebird Health', industry: 'Healthcare', phone: '555-0200', website: 'https://bluebird.example', status: 'prospect' }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/contacts') && method === 'GET') {
        return jsonResponse({
          data: {
            contacts: [
              { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@bluebird.example', phone: '555-0300', jobTitle: 'Operations Director', status: 'lead' }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/users')) {
        return jsonResponse({
          data: {
            users: [
              { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' },
              { id: 2, email: 'alex@acme.test', firstName: 'Alex', lastName: 'Admin', role: 'admin' }
            ]
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/deals') && method === 'POST') {
        hasCreatedDeal = true
        return jsonResponse({
          data: {
            deal: currentDeal,
            activities: [],
            notes: [],
            tasks: []
          }
        }, { status: 201 })
      }

      if (requestURL.pathname.endsWith('/api/deals/12') && method === 'PATCH') {
        currentDeal = { id: 12, name: 'Bluebird Expansion', stageId: 2, stageName: 'Qualified', companyId: 6, companyName: 'Bluebird Health', primaryContactId: 8, primaryContactName: 'Ava Stone', status: 'won', valueAmount: '72000.00', valueCurrency: 'USD', expectedCloseDate: '2026-05-14', ownerUserId: 2 }
        return jsonResponse({
          data: {
            deal: currentDeal,
            activities: [
              { id: 98, action: 'deal.updated', summary: 'Deal updated' }
            ]
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks') && method === 'POST') {
        return jsonResponse({
          data: {
            task: {
              id: 92,
              entityType: 'deal',
              entityId: 12,
              entityLabel: 'Bluebird Rollout',
              title: 'Draft rollout kickoff agenda',
              description: 'Include legal and operations handoff.',
              status: 'open',
              dueAt: '2026-04-21T13:00:00Z',
              completedAt: '',
              assignedToUserId: 2,
              assignedToUserName: 'Alex Admin',
              createdByUserId: 1,
              createdByUserName: 'Demo Owner'
            },
            activities: [
              { id: 101, action: 'task.created', summary: 'Task created' }
            ]
          }
        }, { status: 201 })
      }

      if (requestURL.pathname.endsWith('/api/notes') && method === 'POST') {
        return jsonResponse({
          data: {
            note: {
              id: 51,
              entityType: 'deal',
              entityId: 12,
              body: 'Legal requested updated indemnity language.',
              createdByUserId: 1,
              createdByUserName: 'Demo Owner',
              createdAt: '2026-04-10T12:00:00Z',
              updatedAt: '2026-04-10T12:00:00Z'
            },
            activity: {
              id: 100,
              action: 'note.created',
              summary: 'Note added',
              createdAt: '2026-04-10T12:00:00Z'
            }
          }
        }, { status: 201 })
      }

      if (requestURL.pathname.endsWith('/api/notes') && requestURL.searchParams.get('entityType') === 'deal' && requestURL.searchParams.get('entityId') === '12') {
        return jsonResponse({
          data: {
            notes: []
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks') && requestURL.searchParams.get('entityType') === 'deal' && requestURL.searchParams.get('entityId') === '12') {
        return jsonResponse({
          data: {
            tasks: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/deals/12/stage') && method === 'PATCH') {
        currentDeal = { ...currentDeal, stageId: 3, stageName: 'Proposal' }
        return jsonResponse({
          data: {
            deal: currentDeal,
            activities: [
              { id: 99, action: 'deal.stage_changed', summary: 'Deal moved to Proposal' }
            ]
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/deals/12') && method === 'DELETE') {
        return jsonResponse({}, { status: 204 })
      }

      throw new Error(`Unexpected fetch: ${method} ${requestURL.pathname}${requestURL.search}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/deals')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /deals/i })).toBeInTheDocument()
    expect(screen.getByText(/pipeline value/i)).toBeInTheDocument()
    expect(await screen.findByText(/northstar expansion/i)).toBeInTheDocument()
    expect(screen.getAllByText('$48,000.00').length).toBeGreaterThan(0)
    expect(screen.getAllByText(/proposal/i).length).toBeGreaterThan(0)

    expect(screen.queryByLabelText(/company id/i)).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/primary contact id/i)).not.toBeInTheDocument()
    expect(screen.getByLabelText(/^company$/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/primary contact/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/deal name/i), { target: { value: 'Bluebird Rollout' } })
    fireEvent.change(screen.getByLabelText(/^stage$/i), { target: { value: '2' } })
    fireEvent.change(screen.getByLabelText(/^company$/i), { target: { value: '6' } })
    fireEvent.change(screen.getByLabelText(/primary contact/i), { target: { value: '8' } })
    fireEvent.change(screen.getByLabelText(/value amount/i), { target: { value: '60000.00' } })
    fireEvent.change(screen.getByLabelText(/value currency/i), { target: { value: 'USD' } })
    fireEvent.change(screen.getByLabelText(/expected close date/i), { target: { value: '2026-05-02' } })
    fireEvent.change(screen.getByLabelText(/^owner$/i), { target: { value: '2' } })
    fireEvent.click(screen.getByRole('button', { name: /save deal/i }))

    expect((await screen.findAllByText(/bluebird rollout/i)).length).toBeGreaterThan(0)
    await waitFor(() => {
      expect(window.location.pathname).toBe('/deals/12')
    })

    fireEvent.change(screen.getByLabelText(/stage filter/i), { target: { value: '2' } })

    expect(await screen.findByText(/showing 1 of 1 deals/i)).toBeInTheDocument()
    expect(window.location.search).toBe('?stage=2')
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/deals\?stageId=2$/), expect.any(Object))
    })
    expect(screen.getByRole('button', { name: /bluebird rollout/i })).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/owner filter/i), { target: { value: '1' } })

    expect(await screen.findByText(/showing 0 of 0 deals/i)).toBeInTheDocument()
    expect(window.location.search).toBe('?stage=2&owner=1')
    expect(screen.getByText(/no deals match the current filters/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/stage filter/i), { target: { value: 'all' } })

    expect(await screen.findByText(/showing 1 of 1 deals/i)).toBeInTheDocument()
    expect(window.location.search).toBe('?owner=1')
    expect(screen.getByText(/northstar expansion/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/owner filter/i), { target: { value: 'all' } })

    expect(await screen.findByText(/showing 2 of 2 deals/i)).toBeInTheDocument()
    expect(window.location.search).toBe('')
    const detailForm = await screen.findByRole('form', { name: /deal details form/i })

    fireEvent.change(within(detailForm).getByLabelText(/deal name/i), { target: { value: 'Bluebird Expansion' } })
    fireEvent.change(within(detailForm).getByLabelText(/status/i), { target: { value: 'won' } })
    fireEvent.change(within(detailForm).getByLabelText(/value amount/i), { target: { value: '72000.00' } })
    fireEvent.change(within(detailForm).getByLabelText(/expected close date/i), { target: { value: '2026-05-14' } })
    fireEvent.change(within(detailForm).getByLabelText(/^owner$/i), { target: { value: '2' } })
    fireEvent.click(within(detailForm).getByRole('button', { name: /update deal/i }))

    expect(await screen.findByText(/deal updated/i)).toBeInTheDocument()
    expect(screen.getByText(/time unavailable/i)).toBeInTheDocument()
    expect(screen.getAllByText(/bluebird expansion/i).length).toBeGreaterThan(0)
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/deals\/12$/), expect.objectContaining({
        method: 'PATCH',
        body: expect.stringContaining('"status":"won"')
      }))
    })
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/deals\/12$/), expect.objectContaining({
        method: 'PATCH',
        body: expect.stringContaining('"ownerUserId":2')
      }))
    })

    expect(screen.getAllByText(/qualified/i).length).toBeGreaterThan(0)
    expect(screen.queryByLabelText(/assigned to user id/i)).not.toBeInTheDocument()
    expect(screen.getByLabelText(/^assigned to$/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/task title/i), { target: { value: 'Draft rollout kickoff agenda' } })
    fireEvent.change(screen.getByLabelText(/task description/i), { target: { value: 'Include legal and operations handoff.' } })
    fireEvent.change(screen.getByLabelText(/^assigned to$/i), { target: { value: '2' } })
    fireEvent.change(screen.getByLabelText(/due at/i), { target: { value: '2026-04-21T13:00' } })
    fireEvent.click(screen.getByRole('button', { name: /^save task$/i }))

    expect(await screen.findByText(/draft rollout kickoff agenda/i)).toBeInTheDocument()
    expect(screen.getByText(/task created/i)).toBeInTheDocument()
    expect(screen.getAllByText(/time unavailable/i).length).toBeGreaterThan(0)
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks$/), expect.objectContaining({
        method: 'POST',
        body: expect.stringContaining('"entityType":"deal"')
      }))
    })

    fireEvent.change(screen.getByLabelText(/new note/i), { target: { value: 'Legal requested updated indemnity language.' } })
    fireEvent.click(screen.getByRole('button', { name: /add note/i }))

    expect(await screen.findByText(/legal requested updated indemnity language/i)).toBeInTheDocument()
    expect(screen.getByText(/note added/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/move stage/i), { target: { value: '3' } })
    fireEvent.click(screen.getByRole('button', { name: /move to stage/i }))

    expect(await screen.findByText(/deal moved to proposal/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/deals\/12\/stage$/), expect.objectContaining({ method: 'PATCH' }))
    })

    fireEvent.click(screen.getByRole('button', { name: /archive deal/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/deals\/12$/), expect.objectContaining({ method: 'DELETE' }))
    })
    await waitFor(() => {
      expect(window.location.pathname).toBe('/deals')
    })
  })

  it('loads a deal directly from the detail route when it is not present in the pipeline list', async () => {
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
            stages: [
              { id: 1, name: 'Lead', position: 1, isClosed: false, isWon: false },
              { id: 2, name: 'Qualified', position: 2, isClosed: false, isWon: false },
              { id: 3, name: 'Proposal', position: 3, isClosed: false, isWon: false }
            ]
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            deals: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' }
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
            deal: { id: 12, name: 'Bluebird Rollout', stageId: 2, stageName: 'Qualified', companyId: 6, companyName: 'Bluebird Health', primaryContactId: 8, primaryContactName: 'Ava Stone', status: 'open', valueAmount: '60000.00', valueCurrency: 'USD', expectedCloseDate: '2026-05-02', ownerUserId: 1 },
            activities: [
              { id: 99, action: 'deal.created', summary: 'Deal created' }
            ]
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            notes: [
              {
                id: 51,
                entityType: 'deal',
                entityId: 12,
                body: 'Legal requested updated indemnity language.',
                createdByUserId: 1,
                createdByUserName: 'Demo Owner',
                createdAt: '2026-04-10T12:00:00Z',
                updatedAt: '2026-04-10T12:00:00Z'
              }
            ]
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            tasks: [
              {
                id: 92,
                entityType: 'deal',
                entityId: 12,
                entityLabel: 'Bluebird Rollout',
                title: 'Draft rollout kickoff agenda',
                description: 'Include legal and operations handoff.',
                status: 'open',
                dueAt: '2026-04-21T13:00:00Z',
                completedAt: '',
                assignedToUserId: 1,
                assignedToUserName: 'Demo Owner',
                createdByUserId: 1,
                createdByUserName: 'Demo Owner'
              }
            ],
            meta: { page: 1, pageSize: 20, total: 1, openCount: 1, completedCount: 0 }
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/deals/12')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /bluebird rollout/i })).toBeInTheDocument()
    expect(screen.getByText(/legal requested updated indemnity language/i)).toBeInTheDocument()
    expect(screen.getByText(/draft rollout kickoff agenda/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/deals\/12$/), expect.any(Object))
    })
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/notes\?entityType=deal&entityId=12$/), expect.any(Object))
    })
  })

  it('uses jobs language for service businesses on the pipeline page', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'services' },
            membership: { role: 'owner' }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            stages: [
              { id: 1, name: 'Lead', position: 1, isClosed: false, isWon: false },
              { id: 2, name: 'Quote', position: 2, isClosed: false, isWon: false }
            ]
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            deals: [
              { id: 21, name: 'Northstar Boiler Repair', stageId: 2, stageName: 'Quote', companyId: 5, companyName: 'Northstar Logistics', primaryContactId: 7, primaryContactName: 'Morgan Lee', status: 'open', valueAmount: '4800.00', valueCurrency: 'USD', expectedCloseDate: '2026-04-19', ownerUserId: 1 }
            ],
            meta: { page: 1, pageSize: 20, total: 1, openCount: 1, wonCount: 0, pipelineValue: '4800.00' }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { companies: [], meta: { page: 1, pageSize: 20, total: 0 } } })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { contacts: [], meta: { page: 1, pageSize: 20, total: 0 } } })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { users: [{ id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' }] } })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/deals')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /jobs/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /jobs/i })).toBeInTheDocument()
    expect(screen.getByText(/open jobs/i)).toBeInTheDocument()
    expect(screen.getByText(/won jobs/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/search jobs/i)).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /new job/i })).toBeInTheDocument()
    expect(screen.getByLabelText(/job name/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/^client$/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/job value/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/target date/i)).toBeInTheDocument()
    expect(screen.getByText(/showing 1 of 1 jobs/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /save job/i })).toBeInTheDocument()
  })
})
