import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('companies flow', () => {
  it('loads searchable clients list and opens client detail with linked contacts', async () => {
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
              { id: 5, name: 'Northstar Logistics', clientType: 'organization', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', domain: 'northstar.example', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' }
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
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            companies: [
              { id: 5, name: 'Northstar Logistics', clientType: 'organization', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', domain: 'northstar.example', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            contacts: [],
            meta: { page: 1, pageSize: 20, total: 0 }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            company: { id: 5, name: 'Northstar Logistics', clientType: 'organization', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', domain: 'northstar.example', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' },
            linkedContacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', relationshipTitle: 'Champion', isPrimary: true }
            ],
            activities: [
              { id: 22, action: 'company.created', summary: 'Company created' }
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
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
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
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            contacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', jobTitle: 'Head of RevOps', status: 'lead', isClient: false }
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
            contact: { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', jobTitle: 'Head of RevOps', status: 'lead' },
            notes: [],
            activities: [
              { id: 100, action: 'contact.created', summary: 'Contact created' }
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
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/companies')

    render(<AppRouter />)

    expect(await screen.findByText(/see client ownership, linked people, and live pipeline in one place/i)).toBeInTheDocument()
    expect(await screen.findByText(/100 Dock St \| Detroit, MI, 48201 \| US/i)).toBeInTheDocument()

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
    fireEvent.click(screen.getByRole('button', { name: /morgan lee/i }))
    await waitFor(() => {
      expect(window.location.pathname).toBe('/contacts/7')
    })
    expect(await screen.findByText(/contact created/i)).toBeInTheDocument()
    expect(screen.getByText(/collect warehouse onboarding contacts/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/assigned to user id/i)).not.toBeInTheDocument()
    expect(screen.getByLabelText(/^assigned to$/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks\?status=open&entityType=contact&entityId=7$/), expect.any(Object))
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

    vi.stubGlobal('fetch', fetchMock)
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
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            company: { id: 6, name: 'Atlas Manufacturing', clientType: 'organization', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', domain: 'atlas.example', industry: 'Industrial', phone: '555-0200', website: 'https://atlas.example', status: 'prospect' },
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
            company: { id: 6, name: 'Atlas Manufacturing', clientType: 'organization', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', domain: 'atlas.example', industry: 'Industrial', phone: '555-0200', website: 'https://atlas.example', status: 'customer' },
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
        status: 204,
        json: async () => ({})
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/companies')

    render(<AppRouter />)

    expect(await screen.findByText(/see client ownership, linked people, and live pipeline in one place/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /add client/i }))
    const createForm = screen.getByRole('button', { name: /save client/i }).closest('form')
    expect(createForm).not.toBeNull()
    expect(screen.queryByLabelText(/linked contact ids/i)).not.toBeInTheDocument()
    expect(within(createForm).getByLabelText(/linked contact/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/client type/i)).toHaveValue('organization')

    fireEvent.change(screen.getByLabelText(/client name/i), { target: { value: 'Atlas Manufacturing' } })
    fireEvent.change(screen.getByLabelText(/address line 1/i), { target: { value: '55 Foundry Way' } })
    fireEvent.change(screen.getByLabelText(/city/i), { target: { value: 'Detroit' } })
    fireEvent.change(screen.getByLabelText(/state/i), { target: { value: 'MI' } })
    fireEvent.change(screen.getByLabelText(/postal code/i), { target: { value: '48201' } })
    fireEvent.change(screen.getByLabelText(/country/i), { target: { value: 'US' } })
    fireEvent.change(screen.getByLabelText(/domain/i), { target: { value: 'atlas.example' } })
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
    expect(within(detailForm).getByLabelText(/linked contact/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/status/i), { target: { value: 'customer' } })
    fireEvent.change(screen.getByLabelText(/address line 1/i), { target: { value: '55 Foundry Way' } })
    fireEvent.change(screen.getByLabelText(/city/i), { target: { value: 'Detroit' } })
    fireEvent.change(screen.getByLabelText(/state/i), { target: { value: 'MI' } })
    fireEvent.change(screen.getByLabelText(/postal code/i), { target: { value: '48201' } })
    fireEvent.change(screen.getByLabelText(/country/i), { target: { value: 'US' } })
    fireEvent.change(within(detailForm).getByLabelText(/linked contact/i), { target: { value: '8' } })
    fireEvent.click(screen.getByRole('button', { name: /update client/i }))

    expect(await screen.findByText(/company updated/i)).toBeInTheDocument()
    expect(screen.getByText(/time unavailable/i)).toBeInTheDocument()
    expect(screen.getByText('ava@acme.test')).toBeInTheDocument()

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
    expect(JSON.parse(updateCall[1].body)).toMatchObject({ clientType: 'organization', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', linkedContactIDs: [8] })
  })

  it('creates an individual client with one linked person record', async () => {
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
          data: { companies: [], meta: { page: 1, pageSize: 20, total: 0 } }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            contacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'Consultant', status: 'lead', isClient: false },
              { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0101', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'Founder', status: 'lead', isClient: false }
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
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'Consultant', status: 'lead', isClient: false },
              { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0101', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'Founder', status: 'lead', isClient: false }
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
      .mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: async () => ({
          data: {
            contact: { id: 8, firstName: 'Ava', lastName: 'Stone', email: '', phone: '555-0100', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: '', status: 'prospect', isClient: true },
            notes: [],
            activities: []
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            contacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'Consultant', status: 'lead', isClient: false },
              { id: 8, firstName: 'Ava', lastName: 'Stone', email: '', phone: '555-0100', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: '', status: 'prospect', isClient: true }
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
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            contact: { id: 8, firstName: 'Ava', lastName: 'Stone', email: '', phone: '555-0100', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: '', status: 'prospect', isClient: true },
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

    vi.stubGlobal('fetch', fetchMock)
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
    fireEvent.change(within(createForm).getByLabelText(/address line 1/i), { target: { value: '55 Foundry Way' } })
    fireEvent.change(within(createForm).getByLabelText(/city/i), { target: { value: 'Detroit' } })
    fireEvent.change(within(createForm).getByLabelText(/state/i), { target: { value: 'MI' } })
    fireEvent.change(within(createForm).getByLabelText(/postal code/i), { target: { value: '48201' } })
    fireEvent.change(within(createForm).getByLabelText(/country/i), { target: { value: 'US' } })
    fireEvent.click(screen.getByRole('button', { name: /save client/i }))

    expect(await screen.findByRole('heading', { name: /ava stone/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(window.location.pathname).toBe('/contacts/8')
    })

    const createCall = fetchMock.mock.calls.find(([url, options]) => String(url).match(/\/api\/contacts$/) && options?.method === 'POST')
    expect(createCall).toBeTruthy()
    expect(JSON.parse(createCall[1].body)).toMatchObject({ firstName: 'Ava', lastName: 'Stone', phone: '555-0100', addressLine1: '55 Foundry Way', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', status: 'prospect', isClient: true })
  })

  it('loads a company directly from the detail route', async () => {
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
              { id: 5, name: 'Northstar Logistics', clientType: 'organization', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', domain: 'northstar.example', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' }
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
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', jobTitle: 'Head of RevOps', status: 'lead', isClient: false }
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
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', jobTitle: 'Head of RevOps', status: 'lead', isClient: false }
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
            company: { id: 5, name: 'Northstar Logistics', clientType: 'organization', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', domain: 'northstar.example', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' },
            linkedContacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', relationshipTitle: 'Champion', isPrimary: true }
            ],
            activities: [
              { id: 22, action: 'company.created', summary: 'Company created' }
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

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/companies/5')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /northstar logistics/i })).toBeInTheDocument()
    expect(screen.getByText(/met procurement lead and validated timeline/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/companies\/5$/), expect.any(Object))
    })
  })
})
