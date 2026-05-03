import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('contacts flow', () => {
  it('loads a contact directly from a person detail route', async () => {
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
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' },
            membership: { role: 'owner' }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/contacts/7')) {
        return jsonResponse({
          data: {
            contact: { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'Head of RevOps', status: 'lead' },
            notes: [],
            activities: [
              { id: 101, action: 'note.created', summary: 'Note added', createdAt: '2026-04-11T09:30:00Z' },
              { id: 100, action: 'contact.created', summary: 'Contact created', createdAt: '2026-04-10T12:00:00Z' }
            ]
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/contacts') && method === 'GET') {
        return jsonResponse({
          data: {
            contacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'Head of RevOps', status: 'lead' }
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

      if (requestURL.pathname.endsWith('/api/tasks') && requestURL.searchParams.get('entityType') === 'contact' && requestURL.searchParams.get('entityId') === '7') {
        return jsonResponse({
          data: {
            tasks: [
              {
                id: 72,
                entityType: 'contact',
                entityId: 7,
                entityLabel: 'Morgan Lee',
                title: 'Confirm rollout owner',
                description: 'Get final stakeholder list.',
                status: 'open',
                dueAt: '2026-04-18T12:00:00Z',
                completedAt: '',
                assignedToUserId: 2,
                assignedToUserName: 'Alex Admin',
                createdByUserId: 1,
                createdByUserName: 'Demo Owner'
              }
            ],
            meta: { page: 1, pageSize: 20, total: 1, openCount: 1, completedCount: 0 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/deals') && requestURL.searchParams.get('primaryContactId') === '7') {
        return jsonResponse({
          data: {
            deals: [
              { id: 11, name: 'Northstar Expansion', stageId: 3, stageName: 'Proposal', companyId: 5, companyName: 'Northstar Logistics', primaryContactId: 7, primaryContactName: 'Morgan Lee', status: 'open', valueAmount: '48000.00', valueCurrency: 'USD', expectedCloseDate: '2026-04-19', ownerUserId: 1 }
            ],
            meta: { page: 1, pageSize: 20, total: 1, openCount: 1, wonCount: 0, pipelineValue: '48000.00' }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks') && method === 'POST') {
        return jsonResponse({
          data: {
            task: {
              id: 73,
              entityType: 'contact',
              entityId: 7,
              entityLabel: 'Morgan Lee',
              title: 'Book follow-up demo',
              description: 'Lock demo slot with ops team.',
              status: 'open',
              dueAt: '2026-04-19T14:00:00Z',
              completedAt: '',
              assignedToUserId: 2,
              assignedToUserName: 'Alex Admin',
              createdByUserId: 1,
              createdByUserName: 'Demo Owner'
            },
            activities: [
              { id: 111, action: 'task.created', summary: 'Task created' }
            ]
          }
        }, { status: 201 })
      }

      throw new Error(`Unexpected fetch: ${method} ${requestURL.pathname}${requestURL.search}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/contacts/7')

    render(<AppRouter />)

    expect(await screen.findByRole('button', { name: /morgan lee/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(window.location.pathname).toBe('/contacts/7')
    })
    expect(await screen.findByRole('heading', { name: /morgan lee/i })).toBeInTheDocument()
    expect(screen.getAllByText(/contact created/i).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/note added/i).length).toBeGreaterThan(0)
    expect(screen.getByText(/apr 10, 2026/i)).toBeInTheDocument()
    expect(screen.getByText(/apr 11, 2026/i)).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText(/activity type filter/i), { target: { value: 'note.created' } })
    expect(screen.getByText(/^note added$/i, { selector: '.activity-summary' })).toBeInTheDocument()
    expect(screen.queryByText(/^contact created$/i, { selector: '.activity-summary' })).not.toBeInTheDocument()
    fireEvent.change(screen.getByLabelText(/activity type filter/i), { target: { value: 'all' } })
    expect(screen.getByText(/northstar expansion/i)).toBeInTheDocument()
    expect(screen.getByText('$48,000.00')).toBeInTheDocument()
    expect(screen.queryByLabelText(/assigned to user id/i)).not.toBeInTheDocument()
    expect(screen.getByLabelText(/^assigned to$/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks\?status=open&entityType=contact&entityId=7$/), expect.any(Object))
    })
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/deals\?primaryContactId=7$/), expect.any(Object))
    })
    expect(screen.getByRole('button', { name: /create deal/i })).toBeInTheDocument()

    const taskCard = screen.getByRole('button', { name: /^save task$/i }).closest('.card')
    fireEvent.change(within(taskCard).getByLabelText(/task title/i), { target: { value: 'Book follow-up demo' } })
    fireEvent.change(within(taskCard).getByLabelText(/task description/i), { target: { value: 'Lock demo slot with ops team.' } })
    fireEvent.change(within(taskCard).getByLabelText(/^assigned to$/i), { target: { value: '2' } })
    fireEvent.change(within(taskCard).getByLabelText(/due at/i), { target: { value: '2026-04-19T14:00' } })
    fireEvent.click(within(taskCard).getByRole('button', { name: /^save task$/i }))

    expect(await screen.findByText(/book follow-up demo/i)).toBeInTheDocument()
    expect(screen.getByText(/^task created$/i, { selector: '.activity-summary' })).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks$/), expect.objectContaining({
        method: 'POST',
        body: expect.stringContaining('"entityType":"contact"')
      }))
    })

    fireEvent.click(screen.getByRole('button', { name: /open in tasks/i }))

    await waitFor(() => {
      expect(window.location.pathname).toBe('/tasks')
    })
    expect(window.location.search).toBe('?entityType=contact&entityId=7')
  })

  it('shows a clearer duplicate warning when updating a contact fails with conflict', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' },
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
            contacts: [
              { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0100', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'COO', status: 'lead' }
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
            contact: { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0100', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'COO', status: 'lead' },
            notes: [],
            activities: []
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
            deals: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: false,
        status: 409,
        json: async () => ({
          error: {
            message: 'duplicate contact: Ava Stone (matching email)',
            details: {
              duplicate: { id: 8, entityType: 'contact', label: 'Ava Stone', reason: 'matching email' }
            }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            contacts: [
              { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0100', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'COO', status: 'lead' }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/contacts/8')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /ava stone/i })).toBeInTheDocument()

    const detailForm = screen.getByRole('button', { name: /update contact/i }).closest('form')
    fireEvent.change(within(detailForm).getByLabelText(/email/i), { target: { value: 'ava+dupe@acme.test' } })
    fireEvent.click(screen.getByRole('button', { name: /update contact/i }))

    expect(await screen.findByText(/possible duplicate contact: matching email\. review the existing record before saving again\./i)).toBeInTheDocument()
    expect(screen.getByText(/duplicate contact: ava stone/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /open matching contact/i }))

    await waitFor(() => {
      expect(window.location.pathname).toBe('/contacts/8')
    })

    fireEvent.click(screen.getByRole('button', { name: /search existing contacts for ava stone/i }))

    await waitFor(() => {
      expect(screen.getByLabelText(/search contacts/i)).toHaveValue('Ava Stone')
    })
    expect(await screen.findByRole('button', { name: /ava stone/i })).toBeInTheDocument()
  })

  it('redirects the old contacts workspace route to clients', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' },
            membership: { role: 'owner' }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            companies: [
              { id: 5, name: 'Northstar Logistics', clientType: 'organization', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' }
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
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'Head of RevOps', status: 'lead' }
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

  it('creates, updates, and archives a contact', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' },
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
          data: { contacts: [], meta: { page: 1, pageSize: 20, total: 0 } }
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
            contact: { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0100', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'COO', status: 'lead' },
            notes: [],
            activities: []
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
            tasks: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0 }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            contact: { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0100', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'COO', status: 'customer' },
            notes: [],
            tasks: [],
            activities: [
              { id: 101, action: 'contact.updated', summary: 'Contact updated' }
            ]
          }
        })
      })
      .mockResolvedValueOnce({ ok: true, status: 204, json: async () => ({}) })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/contacts/8')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /ava stone/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(window.location.pathname).toBe('/contacts/8')
    })

    fireEvent.change(screen.getByLabelText(/address line 1/i), { target: { value: '55 Foundry Way' } })
    fireEvent.change(screen.getByLabelText(/city/i), { target: { value: 'Detroit' } })
    fireEvent.change(screen.getByLabelText(/state/i), { target: { value: 'MI' } })
    fireEvent.change(screen.getByLabelText(/postal code/i), { target: { value: '48201' } })
    fireEvent.change(screen.getByLabelText(/country/i), { target: { value: 'US' } })

    fireEvent.change(screen.getByLabelText(/status/i), { target: { value: 'customer' } })
    fireEvent.click(screen.getByRole('button', { name: /update contact/i }))

    expect(await screen.findByText(/contact updated/i)).toBeInTheDocument()
    expect(screen.getByText(/time unavailable/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /archive contact/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/contacts\/8$/), expect.objectContaining({ method: 'DELETE' }))
    })
    await waitFor(() => {
      expect(window.location.pathname).toBe('/companies')
    })

    const updateCall = fetchMock.mock.calls.find(([url, options]) => String(url).match(/\/api\/contacts\/8$/) && options?.method === 'PATCH')
    expect(updateCall).toBeTruthy()
    expect(JSON.parse(updateCall[1].body)).toMatchObject({ addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', status: 'customer' })
  })

  it('loads a contact directly from the detail route', async () => {
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
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' },
            membership: { role: 'owner' }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/contacts/7')) {
        return jsonResponse({
          data: {
            contact: { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'Head of RevOps', status: 'lead' },
            notes: [],
            activities: [
              { id: 100, action: 'contact.created', summary: 'Contact created' }
            ]
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/contacts') && method === 'GET') {
        return jsonResponse({
          data: {
            contacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'Head of RevOps', status: 'lead' }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/users')) {
        return jsonResponse({
          data: {
            users: [
              { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' }
            ]
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks') && requestURL.searchParams.get('entityType') === 'contact' && requestURL.searchParams.get('entityId') === '7') {
        return jsonResponse({
          data: {
            tasks: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/deals') && requestURL.searchParams.get('primaryContactId') === '7') {
        return jsonResponse({
          data: {
            deals: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' }
          }
        })
      }

      throw new Error(`Unexpected fetch: ${method} ${requestURL.pathname}${requestURL.search}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/contacts/7')

    render(<AppRouter />)

    expect(await screen.findByRole('button', { name: /morgan lee/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(window.location.pathname).toBe('/contacts/7')
    })
    expect(await screen.findByRole('heading', { name: /morgan lee/i })).toBeInTheDocument()
    expect(screen.getByText(/contact created/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/contacts\/7$/), expect.any(Object))
    })
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/deals\?primaryContactId=7$/), expect.any(Object))
    })
  })
})
