import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('contacts flow', () => {
  it('loads a contact directly from a person detail route', async () => {
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
            contact: { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'Head of RevOps', status: 'lead' },
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
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: async () => ({
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
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/contacts/7')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /morgan lee/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(window.location.pathname).toBe('/contacts/7')
    })
    expect(screen.getByText(/contact created/i)).toBeInTheDocument()
    expect(screen.getByText(/time unavailable/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/assigned to user id/i)).not.toBeInTheDocument()
    expect(screen.getByLabelText(/^assigned to$/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks\?status=open&entityType=contact&entityId=7$/), expect.any(Object))
    })

    fireEvent.change(screen.getByLabelText(/task title/i), { target: { value: 'Book follow-up demo' } })
    fireEvent.change(screen.getByLabelText(/task description/i), { target: { value: 'Lock demo slot with ops team.' } })
    fireEvent.change(screen.getByLabelText(/^assigned to$/i), { target: { value: '2' } })
    fireEvent.change(screen.getByLabelText(/due at/i), { target: { value: '2026-04-19T14:00' } })
    fireEvent.click(screen.getByRole('button', { name: /^save task$/i }))

    expect(await screen.findByText(/book follow-up demo/i)).toBeInTheDocument()
    expect(screen.getByText(/task created/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks$/), expect.objectContaining({
        method: 'POST',
        body: expect.stringContaining('"entityType":"contact"')
      }))
    })
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
        ok: false,
        status: 409,
        json: async () => ({
          error: { message: 'duplicate contact: Ava Stone (matching email)' }
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

    expect(await screen.findByText(/possible duplicate contact\. review the existing record before saving again\./i)).toBeInTheDocument()
    expect(screen.getByText(/duplicate contact: ava stone/i)).toBeInTheDocument()
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
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            contact: { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', addressLine1: '100 Dock St', city: 'Detroit', state: 'MI', postalCode: '48201', country: 'US', jobTitle: 'Head of RevOps', status: 'lead' },
            notes: [],
            tasks: [],
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
            tasks: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0 }
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/contacts/7')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /morgan lee/i })).toBeInTheDocument()
    expect(screen.getByText(/contact created/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/contacts\/7$/), expect.any(Object))
    })
  })
})
