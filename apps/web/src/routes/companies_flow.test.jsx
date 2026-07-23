import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

function withTouchpointSummary(fetchMock) {
  return vi.fn((url, options = {}) => {
    if (String(url).includes('/api/touchpoints/')) return Promise.resolve({ ok: true, json: async () => ({ data: { staleDays: 30, isStale: false, createdAt: '2026-04-10T09:00:00Z', recent: [], semantics: [] } }) })
    if (String(url).includes('/api/reports/client-health')) return Promise.resolve({ ok: true, json: async () => ({ data: { entityType: 'company', status: 'all', count: 0, totals: { total: 0, healthy: 0, watch: 0, needsAttention: 0 }, records: [], semantics: [] } }) })
    return fetchMock(url, options)
  })
}

describe('companies flow', () => {
  it('loads searchable clients list and opens client detail with linked contacts', async () => {
    const jsonResponse = (payload, init = {}) => ({
      ok: init.ok ?? true,
      status: init.status ?? 200,
      json: async () => payload
    })

    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')

      if (requestURL.pathname.endsWith('/auth/me')) {
        return jsonResponse({
          data: {
            user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' },
            membership: { role: 'owner' }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/companies/5')) {
        return jsonResponse({
          data: {
            company: { id: 5, name: 'Northstar Logistics', clientType: 'organization', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' },
            linkedContacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', relationshipTitle: 'Champion', isPrimary: true }
            ],
            activities: [
              { id: 22, action: 'company.created', summary: 'Company created' }
            ]
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/contacts/7')) {
        return jsonResponse({
          data: {
            contact: { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', jobTitle: 'Head of RevOps', status: 'lead' },
            notes: [],
            activities: [
              { id: 100, action: 'contact.created', summary: 'Contact created' }
            ]
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/companies')) {
        return jsonResponse({
          data: {
            companies: [
              { id: 5, name: 'Northstar Logistics', clientType: 'organization', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/contacts')) {
        return jsonResponse({
          data: {
            contacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', jobTitle: 'Head of RevOps', status: 'lead', isClient: false },
              { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0101', jobTitle: 'COO', status: 'lead', isClient: false }
            ],
            meta: { page: 1, pageSize: 20, total: 2 }
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

      if (requestURL.pathname.endsWith('/api/notes') && requestURL.searchParams.get('entityType') === 'company' && requestURL.searchParams.get('entityId') === '5') {
        return jsonResponse({
          data: {
            notes: [
              {
                id: 41,
                entityType: 'company',
                entityId: 5,
                body: 'Met procurement lead and validated timeline.',
                createdByUserId: 1,
                createdByUserName: 'Demo Owner',
                createdAt: '2026-04-10T11:00:00Z',
                updatedAt: '2026-04-10T11:00:00Z'
              }
            ]
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks') && requestURL.searchParams.get('entityType') === 'company' && requestURL.searchParams.get('entityId') === '5') {
        return jsonResponse({
          data: {
            tasks: [
              {
                id: 88,
                entityType: 'company',
                entityId: 5,
                entityLabel: 'Northstar Logistics',
                title: 'Collect warehouse onboarding contacts',
                description: 'Need ops and procurement owners.',
                status: 'open',
                dueAt: '2026-04-18T15:00:00Z',
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

      if (requestURL.pathname.endsWith('/api/tasks') && requestURL.searchParams.get('entityType') === 'contact' && requestURL.searchParams.get('entityId') === '7') {
        return jsonResponse({
          data: {
            tasks: [
              {
                id: 88,
                entityType: 'contact',
                entityId: 7,
                entityLabel: 'Morgan Lee',
                title: 'Collect warehouse onboarding contacts',
                description: 'Need ops and procurement owners.',
                status: 'open',
                dueAt: '2026-04-18T15:00:00Z',
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

      if (requestURL.pathname.endsWith('/api/deals') && requestURL.searchParams.get('companyId') === '5') {
        return jsonResponse({
          data: {
            deals: [
              { id: 11, name: 'Northstar Expansion', stageId: 3, stageName: 'Proposal', companyId: 5, companyName: 'Northstar Logistics', primaryContactId: 7, primaryContactName: 'Morgan Lee', status: 'open', valueAmount: '48000.00', valueCurrency: 'USD', expectedCloseDate: '2026-04-19', ownerUserId: 1 }
            ],
            meta: { page: 1, pageSize: 20, total: 1, openCount: 1, wonCount: 0, pipelineValue: '48000.00' }
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

      if (requestURL.pathname.endsWith('/api/custom-fields')) {
        return jsonResponse({ data: { definitions: [] } })
      }

      throw new Error(`Unexpected fetch: ${requestURL.pathname}${requestURL.search}`)
    })

    vi.stubGlobal('fetch', withTouchpointSummary(fetchMock))
    window.history.pushState({}, '', '/companies')

    render(<AppRouter />)

    expect(await screen.findByText(/see client ownership, linked people, and live pipeline in one place/i)).toBeInTheDocument()
    expect(await screen.findByText('https://northstar.example')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/search clients/i), { target: { value: 'northstar' } })

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/companies\?q=northstar/), expect.any(Object))
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/contacts\?q=northstar/), expect.any(Object))
    })

    fireEvent.click(screen.getByRole('button', { name: /northstar logistics/i }))

    expect(await screen.findByRole('heading', { name: /northstar logistics/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(window.location.pathname).toBe('/companies/5')
    })
    expect(screen.getByText('morgan@acme.test')).toBeInTheDocument()
    expect(screen.getByText(/company created/i)).toBeInTheDocument()
    expect(screen.getByText(/time unavailable/i)).toBeInTheDocument()
    expect(await screen.findByText(/met procurement lead and validated timeline/i)).toBeInTheDocument()
    expect(screen.getByText(/northstar expansion/i)).toBeInTheDocument()
    expect(screen.getByText('$48,000.00')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /create deal/i })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /morgan lee/i }))
    await waitFor(() => {
      expect(window.location.pathname).toBe('/contacts/7')
    })
    expect(await screen.findByText(/contact created/i)).toBeInTheDocument()
    expect(screen.getByText(/collect warehouse onboarding contacts/i)).toBeInTheDocument()
    expect(screen.getByText(/northstar expansion/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/assigned to user id/i)).not.toBeInTheDocument()
    expect(screen.getByLabelText(/^assigned to$/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks\?status=open&entityType=contact&entityId=7$/), expect.any(Object))
    })
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/deals\?primaryContactId=7$/), expect.any(Object))
    })
  })

  it('searches clients by linked contact name, phone, and address details', async () => {
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
      .mockResolvedValue({
        ok: true,
        json: async () => ({
          data: { companies: [], contacts: [], users: [], meta: { page: 1, pageSize: 20, total: 0 } }
        })
      })

    vi.stubGlobal('fetch', withTouchpointSummary(fetchMock))
    window.history.pushState({}, '', '/companies')

    render(<AppRouter />)

    expect(await screen.findByText(/see client ownership, linked people, and live pipeline in one place/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/search clients/i), { target: { value: 'morgan' } })
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/companies\?q=morgan/), expect.any(Object))
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/contacts\?q=morgan/), expect.any(Object))
    })

    fireEvent.change(screen.getByLabelText(/search clients/i), { target: { value: '555-0200' } })
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/companies\?q=555-0200/), expect.any(Object))
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/contacts\?q=555-0200/), expect.any(Object))
    })

    fireEvent.change(screen.getByLabelText(/search clients/i), { target: { value: 'detroit' } })
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/companies\?q=detroit/), expect.any(Object))
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/contacts\?q=detroit/), expect.any(Object))
    })
  })

  it('creates, updates, and archives a client', async () => {
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
          data: { companies: [], meta: { page: 1, pageSize: 20, total: 0 } }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            contacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', jobTitle: 'Head of RevOps', status: 'lead', isClient: false },
              { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0101', jobTitle: 'COO', status: 'lead', isClient: false }
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
              { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' },
              { id: 2, email: 'alex@acme.test', firstName: 'Alex', lastName: 'Admin', role: 'admin' }
            ]
          }
        })
      })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { definitions: [] } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { definitions: [] } }) })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            company: { id: 6, name: 'Atlas Manufacturing', clientType: 'organization', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', industry: 'Industrial', phone: '555-0200', website: 'https://atlas.example', status: 'prospect' },
            linkedContacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', relationshipTitle: 'Champion', isPrimary: true }
            ],
            activities: [],
            notes: []
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            note: {
              id: 42,
              entityType: 'company',
              entityId: 6,
              body: 'Procurement asked for revised payment terms.',
              createdByUserId: 1,
              createdByUserName: 'Demo Owner',
              createdAt: '2026-04-10T11:30:00Z',
              updatedAt: '2026-04-10T11:30:00Z'
            },
            activity: {
              id: 24,
              action: 'note.created',
              summary: 'Note added',
              createdAt: '2026-04-10T11:30:00Z'
            }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            company: { id: 6, name: 'Atlas Manufacturing', clientType: 'organization', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', industry: 'Industrial', phone: '555-0200', website: 'https://atlas.example', status: 'customer' },
            linkedContacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', relationshipTitle: 'Champion', isPrimary: true },
              { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', relationshipTitle: 'Evaluator', isPrimary: false }
            ],
            activities: [
              { id: 23, action: 'company.updated', summary: 'Company updated' }
            ]
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { exists: false, semantics: [] } })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 204,
        json: async () => ({})
      })

    const fetchWithContactLookup = vi.fn((url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      if (requestURL.pathname.endsWith('/api/contacts') && requestURL.searchParams.get('pageSize') === '20') {
        return Promise.resolve({
          ok: true,
          json: async () => ({
            data: {
              contacts: [
                { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', jobTitle: 'Head of RevOps', status: 'lead', isClient: false },
                { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0101', jobTitle: 'COO', status: 'lead', isClient: false }
              ],
              meta: { page: 1, pageSize: 20, total: 2 }
            }
          })
        })
      }
      return fetchMock(url, options)
    })
    vi.stubGlobal('fetch', withTouchpointSummary(fetchWithContactLookup))
    window.history.pushState({}, '', '/companies')

    render(<AppRouter />)

    expect(await screen.findByText(/see client ownership, linked people, and live pipeline in one place/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /add client/i }))
    const createForm = screen.getByRole('button', { name: /save client/i }).closest('form')
    expect(createForm).not.toBeNull()
    expect(screen.queryByLabelText(/linked contact ids/i)).not.toBeInTheDocument()
    expect(within(createForm).getByLabelText(/linked contact/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/client type/i)).toHaveValue('organization')
    expect(await within(createForm).findByRole('option', { name: /morgan lee/i })).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/client name/i), { target: { value: 'Atlas Manufacturing' } })
    fireEvent.change(screen.getByLabelText(/address line 1/i), { target: { value: '55 Foundry Way' } })
    fireEvent.change(screen.getByLabelText(/city/i), { target: { value: 'Detroit' } })
    fireEvent.change(screen.getByLabelText(/state/i), { target: { value: 'MI' } })
    fireEvent.change(screen.getByLabelText(/postal code/i), { target: { value: '48201' } })
    fireEvent.change(screen.getByLabelText(/country/i), { target: { value: 'US' } })
    fireEvent.change(screen.getByLabelText(/industry/i), { target: { value: 'Industrial' } })
    fireEvent.change(screen.getByLabelText(/phone/i), { target: { value: '555-0200' } })
    fireEvent.change(within(createForm).getAllByLabelText(/website/i)[0], { target: { value: 'https://atlas.example' } })
    fireEvent.change(within(createForm).getByLabelText(/linked contact/i), { target: { value: '7' } })
    fireEvent.click(screen.getByRole('button', { name: /save client/i }))

    expect(await screen.findByRole('heading', { name: /atlas manufacturing/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(window.location.pathname).toBe('/companies/6')
    })

    fireEvent.change(screen.getByLabelText(/new note/i), { target: { value: 'Procurement asked for revised payment terms.' } })
    fireEvent.click(screen.getByRole('button', { name: /add note/i }))

    expect(await screen.findByText(/procurement asked for revised payment terms/i)).toBeInTheDocument()
    expect(screen.getByText(/note added/i)).toBeInTheDocument()

    const detailForm = screen.getByRole('button', { name: /update client/i }).closest('form')
    expect(detailForm).not.toBeNull()
    expect(screen.queryByLabelText(/linked contact ids/i)).not.toBeInTheDocument()
    expect(within(detailForm).queryByLabelText(/linked contact/i)).not.toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/status/i), { target: { value: 'customer' } })
    fireEvent.change(screen.getByLabelText(/address line 1/i), { target: { value: '55 Foundry Way' } })
    fireEvent.change(screen.getByLabelText(/city/i), { target: { value: 'Detroit' } })
    fireEvent.change(screen.getByLabelText(/state/i), { target: { value: 'MI' } })
    fireEvent.change(screen.getByLabelText(/postal code/i), { target: { value: '48201' } })
    fireEvent.change(screen.getByLabelText(/country/i), { target: { value: 'US' } })
    fireEvent.click(screen.getByRole('button', { name: /update client/i }))

    expect(await screen.findByText(/company updated/i)).toBeInTheDocument()
    expect(screen.getByText(/time unavailable/i)).toBeInTheDocument()
    expect(screen.getAllByText('ava@acme.test').length).toBeGreaterThan(0)

    fireEvent.click(screen.getByRole('button', { name: /archive client/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/companies\/6$/), expect.objectContaining({ method: 'DELETE' }))
    })
    await waitFor(() => {
      expect(window.location.pathname).toBe('/companies')
    })

    const createCall = fetchMock.mock.calls.find(([url, options]) => String(url).match(/\/api\/companies$/) && options?.method === 'POST')
    expect(createCall).toBeTruthy()
    expect(JSON.parse(createCall[1].body)).toMatchObject({ clientType: 'organization', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', linkedContactIDs: [7] })

    const updateCall = fetchMock.mock.calls.find(([url, options]) => String(url).match(/\/api\/companies\/6$/) && options?.method === 'PATCH')
    expect(updateCall).toBeTruthy()
    const updatePayload = JSON.parse(updateCall[1].body)
    expect(updatePayload).toMatchObject({ clientType: 'organization', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US' })
    expect(updatePayload).not.toHaveProperty('linkedContactIDs')
  })

  it('adds a new linked person from company detail', async () => {
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

      if (requestURL.pathname.endsWith('/api/companies') && method === 'GET') {
        return jsonResponse({
          data: {
            companies: [
              { id: 6, name: 'Atlas Manufacturing', clientType: 'organization', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', industry: 'Industrial', phone: '555-0200', website: 'https://atlas.example', status: 'prospect' }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/contacts') && method === 'GET') {
        return jsonResponse({
          data: {
            contacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', jobTitle: 'Head of RevOps', status: 'lead', isClient: false },
              { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0101', jobTitle: 'COO', status: 'lead', isClient: false }
            ],
            meta: { page: 1, pageSize: 20, total: 2 }
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

      if (requestURL.pathname.endsWith('/api/companies/6') && method === 'GET') {
        return jsonResponse({
          data: {
            company: { id: 6, name: 'Atlas Manufacturing', clientType: 'organization', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', industry: 'Industrial', phone: '555-0200', website: 'https://atlas.example', status: 'prospect' },
            linkedContacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', relationshipTitle: 'Champion', isPrimary: true }
            ],
            activities: [
              { id: 22, action: 'company.created', summary: 'Company created' }
            ]
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/notes') && requestURL.searchParams.get('entityType') === 'company' && requestURL.searchParams.get('entityId') === '6') {
        return jsonResponse({ data: { notes: [] } })
      }

      if (requestURL.pathname.endsWith('/api/tasks') && requestURL.searchParams.get('entityType') === 'company' && requestURL.searchParams.get('entityId') === '6') {
        return jsonResponse({
          data: {
            tasks: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/deals') && requestURL.searchParams.get('companyId') === '6') {
        return jsonResponse({
          data: {
            deals: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/companies/6/linked-contacts') && method === 'POST') {
        return jsonResponse({
          data: {
            contact: { id: 9, firstName: 'Riley', lastName: 'Chen', email: 'riley@atlas.test', phone: '555-0110', addressLine1: '', addressLine2: '', city: '', state: '', postalCode: '', country: '', jobTitle: 'Procurement Lead', status: 'prospect', isClient: false },
            link: { relationshipTitle: 'Procurement Lead', isPrimary: false },
            activity: { id: 23, action: 'company.contact_linked', summary: 'Contact linked: Riley Chen' }
          }
        }, { status: 201 })
      }

      if (requestURL.pathname.endsWith('/api/custom-fields')) {
        return jsonResponse({ data: { definitions: [] } })
      }

      throw new Error(`Unexpected fetch: ${method} ${requestURL.pathname}${requestURL.search}`)
    })

    vi.stubGlobal('fetch', withTouchpointSummary(fetchMock))
    window.history.pushState({}, '', '/companies/6')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /atlas manufacturing/i })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /add person/i }))

    const createPersonForm = screen.getByRole('button', { name: /save person/i }).closest('form')
    expect(createPersonForm).not.toBeNull()

    fireEvent.change(within(createPersonForm).getByLabelText(/first name/i), { target: { value: 'Riley' } })
    fireEvent.change(within(createPersonForm).getByLabelText(/last name/i), { target: { value: 'Chen' } })
    fireEvent.change(within(createPersonForm).getByLabelText(/^email$/i), { target: { value: 'riley@atlas.test' } })
    fireEvent.change(within(createPersonForm).getByLabelText(/^phone$/i), { target: { value: '555-0110' } })
    fireEvent.change(within(createPersonForm).getByLabelText(/job title/i), { target: { value: 'Procurement Lead' } })
    fireEvent.click(screen.getByRole('button', { name: /save person/i }))

    expect(await screen.findByRole('button', { name: /riley chen/i })).toBeInTheDocument()
    expect(screen.getByText('riley@atlas.test')).toBeInTheDocument()
    expect(screen.getByText('Procurement Lead')).toBeInTheDocument()
    expect(screen.getByText('Contact linked: Riley Chen')).toBeInTheDocument()

    const createCall = fetchMock.mock.calls.find(([requestURL, requestOptions]) => String(requestURL).match(/\/api\/companies\/6\/linked-contacts$/) && requestOptions?.method === 'POST')
    expect(createCall).toBeTruthy()
    expect(JSON.parse(createCall[1].body)).toMatchObject({
      firstName: 'Riley',
      lastName: 'Chen',
      email: 'riley@atlas.test',
      phone: '555-0110',
      jobTitle: 'Procurement Lead',
      status: 'prospect'
    })
    expect(fetchMock.mock.calls.some(([requestURL, requestOptions]) => String(requestURL).match(/\/api\/contacts$/) && requestOptions?.method === 'POST')).toBe(false)
    expect(fetchMock.mock.calls.some(([requestURL, requestOptions]) => String(requestURL).match(/\/api\/companies\/6$/) && requestOptions?.method === 'PATCH')).toBe(false)
  })

  it('creates an individual client with one linked person record', async () => {
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

      if (requestURL.pathname.endsWith('/api/companies') && method === 'GET') {
        return jsonResponse({
          data: { companies: [], meta: { page: 1, pageSize: 20, total: 0 } }
        })
      }

      if (requestURL.pathname.endsWith('/api/contacts') && method === 'GET') {
        return jsonResponse({
          data: {
            contacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'Consultant', status: 'lead', isClient: false },
              { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0101', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'Founder', status: 'lead', isClient: false }
            ],
            meta: { page: 1, pageSize: 20, total: 2 }
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

      if (requestURL.pathname.endsWith('/api/contacts') && method === 'POST') {
        return jsonResponse({
          data: {
            contact: { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0100', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: '', status: 'prospect', isClient: true },
            notes: [],
            activities: []
          }
        }, { status: 201 })
      }

      if (requestURL.pathname.endsWith('/api/contacts/8')) {
        return jsonResponse({
          data: {
            contact: { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0100', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: '', status: 'prospect', isClient: true },
            notes: [],
            activities: []
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks') && requestURL.searchParams.get('entityType') === 'contact' && requestURL.searchParams.get('entityId') === '8') {
        return jsonResponse({
          data: {
            tasks: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/deals') && requestURL.searchParams.get('primaryContactId') === '8') {
        return jsonResponse({
          data: {
            deals: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/custom-fields')) {
        return jsonResponse({ data: { definitions: [] } })
      }

      throw new Error(`Unexpected fetch: ${method} ${requestURL.pathname}${requestURL.search}`)
    })

    vi.stubGlobal('fetch', withTouchpointSummary(fetchMock))
    window.history.pushState({}, '', '/companies')

    render(<AppRouter />)

    expect(await screen.findByText(/see client ownership, linked people, and live pipeline in one place/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /add client/i }))
    const createForm = screen.getByRole('button', { name: /save client/i }).closest('form')
    expect(createForm).not.toBeNull()
    fireEvent.change(screen.getByLabelText(/client type/i), { target: { value: 'individual' } })

    expect(screen.getByText(/individual clients need one linked person record/i)).toBeInTheDocument()
    expect(within(createForm).getByLabelText(/person record/i)).toBeInTheDocument()
    expect(within(createForm).getByLabelText(/full name/i)).toBeInTheDocument()
    expect(within(createForm).getByLabelText(/email/i)).toBeInTheDocument()
    expect(within(createForm).getByLabelText(/phone number/i)).toBeInTheDocument()
    expect(within(createForm).getByLabelText(/address line 1/i)).toBeInTheDocument()
    expect(within(createForm).getByLabelText(/city/i)).toBeInTheDocument()
    expect(within(createForm).getByLabelText(/state/i)).toBeInTheDocument()
    expect(within(createForm).getByLabelText(/postal code/i)).toBeInTheDocument()
    expect(within(createForm).getByLabelText(/country/i)).toBeInTheDocument()
    expect(within(createForm).queryByLabelText(/^domain/i)).not.toBeInTheDocument()
    expect(within(createForm).queryByLabelText(/industry/i)).not.toBeInTheDocument()
    expect(within(createForm).queryByLabelText(/website/i)).not.toBeInTheDocument()

    fireEvent.change(within(createForm).getByLabelText(/person record/i), { target: { value: '7' } })
    expect(within(createForm).getByLabelText(/full name/i)).toHaveValue('Morgan Lee')
    expect(within(createForm).getByLabelText(/phone number/i)).toHaveValue('555-0100')
    fireEvent.change(within(createForm).getByLabelText(/person record/i), { target: { value: '8' } })
    fireEvent.change(within(createForm).getByLabelText(/full name/i), { target: { value: 'Ava Stone' } })
    fireEvent.change(within(createForm).getByLabelText(/email/i), { target: { value: 'ava@acme.test' } })
    fireEvent.change(within(createForm).getByLabelText(/address line 1/i), { target: { value: '55 Foundry Way' } })
    fireEvent.change(within(createForm).getByLabelText(/city/i), { target: { value: 'Detroit' } })
    fireEvent.change(within(createForm).getByLabelText(/state/i), { target: { value: 'MI' } })
    fireEvent.change(within(createForm).getByLabelText(/postal code/i), { target: { value: '48201' } })
    fireEvent.change(within(createForm).getByLabelText(/country/i), { target: { value: 'US' } })
    fireEvent.click(screen.getByRole('button', { name: /save client/i }))

    expect(await screen.findByRole('button', { name: /ava stone/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(window.location.pathname).toBe('/contacts/8')
    })
    expect(await screen.findByRole('heading', { name: /ava stone/i })).toBeInTheDocument()

    const createCall = fetchMock.mock.calls.find(([url, options]) => String(url).match(/\/api\/contacts$/) && options?.method === 'POST')
    expect(createCall).toBeTruthy()
    expect(JSON.parse(createCall[1].body)).toMatchObject({ firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0100', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', status: 'prospect', isClient: true })
  })

  it('opens the matching contact when an individual client hits a duplicate contact conflict', async () => {
    const jsonResponse = (payload, init = {}) => ({
      ok: init.ok ?? true,
      status: init.status ?? 200,
      json: async () => payload
    })

    const duplicatePayload = {
      error: {
        message: 'duplicate contact: Ava Stone (matching email)',
        details: {
          duplicate: { id: 8, entityType: 'contact', label: 'Ava Stone', reason: 'matching email' }
        }
      }
    }

    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = String(url)
      const method = options.method || 'GET'

      if (requestURL.endsWith('/auth/me')) {
        return jsonResponse({
          data: {
            user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' },
            membership: { role: 'owner' }
          }
        })
      }

      if (requestURL.endsWith('/api/companies') && method === 'GET') {
        return jsonResponse({
          data: {
            companies: [],
            meta: { page: 1, pageSize: 20, total: 0 }
          }
        })
      }

      if (requestURL.endsWith('/api/contacts') && method === 'POST') {
        return jsonResponse(duplicatePayload, { ok: false, status: 409 })
      }

      if (requestURL.endsWith('/api/contacts/8')) {
        return jsonResponse({
          data: {
            contact: { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0100', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'Founder', status: 'lead' },
            notes: [],
            activities: [
              { id: 100, action: 'contact.created', summary: 'Contact created' }
            ]
          }
        })
      }

      if (requestURL.includes('/api/tasks?status=open&entityType=contact&entityId=8')) {
        return jsonResponse({
          data: {
            tasks: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0 }
          }
        })
      }

      if (requestURL.includes('/api/deals?primaryContactId=8')) {
        return jsonResponse({
          data: {
            deals: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' }
          }
        })
      }

      if (requestURL.endsWith('/api/contacts')) {
        return jsonResponse({
          data: {
            contacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', jobTitle: 'Consultant', status: 'lead', isClient: false },
              { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0101', jobTitle: 'Founder', status: 'lead', isClient: false }
            ],
            meta: { page: 1, pageSize: 20, total: 2 }
          }
        })
      }

      if (new URL(requestURL, 'http://localhost').pathname.endsWith('/api/users')) {
        return jsonResponse({
          data: {
            users: [
              { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' }
            ],
            meta: { page: 1, pageSize: 100, total: 1 }
          }
        })
      }

      if (requestURL.includes('/api/custom-fields?')) {
        return jsonResponse({ data: { definitions: [] } })
      }

      throw new Error(`Unexpected fetch: ${method} ${requestURL}`)
    })

    vi.stubGlobal('fetch', withTouchpointSummary(fetchMock))
    window.history.pushState({}, '', '/companies')

    render(<AppRouter />)

    expect(await screen.findByText(/see client ownership, linked people, and live pipeline in one place/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /add client/i }))
    const createForm = screen.getByRole('button', { name: /save client/i }).closest('form')
    fireEvent.change(screen.getByLabelText(/client type/i), { target: { value: 'individual' } })
    fireEvent.change(within(createForm).getByLabelText(/full name/i), { target: { value: 'Ava Stone' } })
    fireEvent.change(within(createForm).getByLabelText(/email/i), { target: { value: 'ava@acme.test' } })
    fireEvent.click(screen.getByRole('button', { name: /save client/i }))

    expect(await screen.findByText(/possible duplicate contact: matching email\. review the existing record before saving again\./i)).toBeInTheDocument()
    expect(screen.getByText(/duplicate contact: ava stone/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /open matching contact/i })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /open matching contact/i }))

    await waitFor(() => {
      expect(window.location.pathname).toBe('/contacts/8')
    })
    expect(await screen.findByRole('heading', { name: /ava stone/i })).toBeInTheDocument()
    expect(screen.getByText(/contact created/i)).toBeInTheDocument()
  })

  it('shows a clearer duplicate warning when creating a company client fails with conflict', async () => {
    const jsonResponse = (payload, init = {}) => ({
      ok: init.ok ?? true,
      status: init.status ?? 200,
      json: async () => payload
    })

    const duplicatePayload = {
      error: {
        message: 'duplicate company: Atlas Manufacturing (matching website)',
        details: {
          duplicate: { id: 9, entityType: 'company', label: 'Atlas Manufacturing', reason: 'matching website' }
        }
      }
    }

    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = String(url)
      const method = options.method || 'GET'

      if (requestURL.endsWith('/auth/me')) {
        return jsonResponse({
          data: {
            user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' },
            membership: { role: 'owner' }
          }
        })
      }

      if (requestURL.endsWith('/api/companies') && method === 'GET') {
        return jsonResponse({
          data: {
            companies: [],
            meta: { page: 1, pageSize: 20, total: 0 }
          }
        })
      }

      if (requestURL.endsWith('/api/contacts')) {
        return jsonResponse({
          data: {
            contacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', jobTitle: 'Head of RevOps', status: 'lead', isClient: false }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
          }
        })
      }

      if (new URL(requestURL, 'http://localhost').pathname.endsWith('/api/users')) {
        return jsonResponse({
          data: {
            users: [
              { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' }
            ],
            meta: { page: 1, pageSize: 100, total: 1 }
          }
        })
      }

      if (requestURL.endsWith('/api/companies') && method === 'POST') {
        return jsonResponse(duplicatePayload, { ok: false, status: 409 })
      }

      if (requestURL.endsWith('/api/companies/9')) {
        return jsonResponse({
          data: {
            company: { id: 9, name: 'Atlas Manufacturing', clientType: 'organization', industry: 'Industrial', phone: '555-0200', website: 'https://atlas.example', status: 'prospect' },
            linkedContacts: [],
            activities: []
          }
        })
      }

      if (requestURL.includes('/api/notes?entityType=company&entityId=9')) {
        return jsonResponse({
          data: {
            notes: []
          }
        })
      }

      if (requestURL.includes('/api/tasks?status=open&entityType=company&entityId=9')) {
        return jsonResponse({
          data: {
            tasks: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0 }
          }
        })
      }

      if (requestURL.includes('/api/deals?companyId=9')) {
        return jsonResponse({
          data: {
            deals: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' }
          }
        })
      }

      if (requestURL.includes('/api/custom-fields?')) {
        return jsonResponse({ data: { definitions: [] } })
      }

      throw new Error(`Unexpected fetch: ${method} ${requestURL}`)
    })

    vi.stubGlobal('fetch', withTouchpointSummary(fetchMock))
    window.history.pushState({}, '', '/companies')

    render(<AppRouter />)

    expect(await screen.findByText(/see client ownership, linked people, and live pipeline in one place/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /add client/i }))
    const createForm = screen.getByRole('button', { name: /save client/i }).closest('form')

    fireEvent.change(within(createForm).getByLabelText(/client name/i), { target: { value: 'Atlas Manufacturing' } })
    fireEvent.change(within(createForm).getByLabelText(/phone/i), { target: { value: '555-0200' } })
    fireEvent.change(within(createForm).getAllByLabelText(/website/i)[0], { target: { value: 'https://atlas.example' } })
    fireEvent.click(screen.getByRole('button', { name: /save client/i }))

    expect(await screen.findByText(/possible duplicate company: matching website\. review the existing record before saving again\./i)).toBeInTheDocument()
    expect(screen.getByText(/duplicate company: atlas manufacturing/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /search existing clients for atlas manufacturing/i })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /open matching client/i }))

    await waitFor(() => {
      expect(window.location.pathname).toBe('/companies/9')
    })
    expect(await screen.findByRole('heading', { name: /atlas manufacturing/i })).toBeInTheDocument()
  })

  it('loads a company directly from the detail route', async () => {
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

      if (requestURL.pathname.endsWith('/api/companies') && method === 'GET') {
        return jsonResponse({
          data: {
            companies: [
              { id: 5, name: 'Northstar Logistics', clientType: 'organization', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/contacts') && method === 'GET') {
        return jsonResponse({
          data: {
            contacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', jobTitle: 'Head of RevOps', status: 'lead', isClient: false }
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

      if (requestURL.pathname.endsWith('/api/companies/5')) {
        return jsonResponse({
          data: {
            company: { id: 5, name: 'Northstar Logistics', clientType: 'organization', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' },
            linkedContacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', relationshipTitle: 'Champion', isPrimary: true }
            ],
            activities: [
              { id: 22, action: 'company.created', summary: 'Company created' }
            ]
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/notes') && requestURL.searchParams.get('entityType') === 'company' && requestURL.searchParams.get('entityId') === '5') {
        return jsonResponse({
          data: {
            notes: [
              {
                id: 41,
                entityType: 'company',
                entityId: 5,
                body: 'Met procurement lead and validated timeline.',
                createdByUserId: 1,
                createdByUserName: 'Demo Owner',
                createdAt: '2026-04-10T11:00:00Z',
                updatedAt: '2026-04-10T11:00:00Z'
              }
            ]
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks') && requestURL.searchParams.get('entityType') === 'company' && requestURL.searchParams.get('entityId') === '5') {
        return jsonResponse({
          data: {
            tasks: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/deals') && requestURL.searchParams.get('companyId') === '5') {
        return jsonResponse({
          data: {
            deals: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/custom-fields')) {
        return jsonResponse({ data: { definitions: [] } })
      }

      throw new Error(`Unexpected fetch: ${method} ${requestURL.pathname}${requestURL.search}`)
    })

    vi.stubGlobal('fetch', withTouchpointSummary(fetchMock))
    window.history.pushState({}, '', '/companies/5')

    render(<AppRouter />)

    expect(await screen.findByRole('button', { name: /northstar logistics/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(window.location.pathname).toBe('/companies/5')
    })
    expect(await screen.findByRole('heading', { name: /northstar logistics/i })).toBeInTheDocument()
    expect(screen.getByText(/met procurement lead and validated timeline/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/companies\/5$/), expect.any(Object))
    })
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/deals\?companyId=5$/), expect.any(Object))
    })
  })
})
