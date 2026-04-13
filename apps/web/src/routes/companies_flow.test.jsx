import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('companies flow', () => {
  it('loads searchable companies list and opens company detail with linked contacts', async () => {
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
              { id: 5, name: 'Northstar Logistics', domain: 'northstar.example', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' }
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
              { id: 5, name: 'Northstar Logistics', domain: 'northstar.example', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            company: { id: 5, name: 'Northstar Logistics', domain: 'northstar.example', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' },
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
        status: 201,
        json: async () => ({
          data: {
            task: {
              id: 89,
              entityType: 'company',
              entityId: 5,
              entityLabel: 'Northstar Logistics',
              title: 'Send procurement checklist',
              description: 'Share vendor intake requirements.',
              status: 'open',
              dueAt: '2026-04-20T16:30:00Z',
              completedAt: '',
              assignedToUserId: 2,
              assignedToUserName: 'Alex Admin',
              createdByUserId: 1,
              createdByUserName: 'Demo Owner'
            },
            activities: [
              { id: 112, action: 'task.created', summary: 'Task created' }
            ]
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/companies')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /companies/i })).toBeInTheDocument()
    expect(await screen.findByText('northstar.example')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/search companies/i), { target: { value: 'northstar' } })

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/companies\?q=northstar/), expect.any(Object))
    })

    fireEvent.click(screen.getByRole('button', { name: /northstar logistics/i }))

    expect(await screen.findByRole('heading', { name: /northstar logistics/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(window.location.pathname).toBe('/companies/5')
    })
    expect(screen.getByText('morgan@acme.test')).toBeInTheDocument()
    expect(screen.getByText(/company created/i)).toBeInTheDocument()
    expect(await screen.findByText(/met procurement lead and validated timeline/i)).toBeInTheDocument()
    expect(screen.getByText(/collect warehouse onboarding contacts/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/assigned to user id/i)).not.toBeInTheDocument()
    expect(screen.getByLabelText(/^assigned to$/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/notes\?entityType=company&entityId=5$/), expect.any(Object))
    })
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks\?status=open&entityType=company&entityId=5$/), expect.any(Object))
    })

    fireEvent.change(screen.getByLabelText(/task title/i), { target: { value: 'Send procurement checklist' } })
    fireEvent.change(screen.getByLabelText(/task description/i), { target: { value: 'Share vendor intake requirements.' } })
    fireEvent.change(screen.getByLabelText(/^assigned to$/i), { target: { value: '2' } })
    fireEvent.change(screen.getByLabelText(/due at/i), { target: { value: '2026-04-20T16:30' } })
    fireEvent.click(screen.getByRole('button', { name: /^save task$/i }))

    expect(await screen.findByText(/send procurement checklist/i)).toBeInTheDocument()
    expect(screen.getByText(/task created/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks$/), expect.objectContaining({
        method: 'POST',
        body: expect.stringContaining('"entityType":"company"')
      }))
    })
  })

  it('creates, updates, and archives a company', async () => {
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
            company: { id: 6, name: 'Atlas Manufacturing', domain: 'atlas.example', industry: 'Industrial', phone: '555-0200', website: 'https://atlas.example', status: 'prospect' },
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
        status: 201,
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
            company: { id: 6, name: 'Atlas Manufacturing', domain: 'atlas.example', industry: 'Industrial', phone: '555-0200', website: 'https://atlas.example', status: 'customer' },
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
      .mockResolvedValueOnce({ ok: true, status: 204, json: async () => ({}) })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/companies')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /companies/i })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /add company/i }))
    expect(screen.queryByLabelText(/linked contact ids/i)).not.toBeInTheDocument()
    expect(screen.getByLabelText(/^linked contact$/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/company name/i), { target: { value: 'Atlas Manufacturing' } })
    fireEvent.change(screen.getByLabelText(/domain/i), { target: { value: 'atlas.example' } })
    fireEvent.change(screen.getByLabelText(/industry/i), { target: { value: 'Industrial' } })
    fireEvent.change(screen.getByLabelText(/phone/i), { target: { value: '555-0200' } })
    fireEvent.change(screen.getByLabelText(/website/i), { target: { value: 'https://atlas.example' } })
    fireEvent.change(screen.getByLabelText(/^linked contact$/i), { target: { value: '7' } })
    fireEvent.click(screen.getByRole('button', { name: /save company/i }))

    expect(await screen.findByRole('heading', { name: /atlas manufacturing/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(window.location.pathname).toBe('/companies/6')
    })

    fireEvent.change(screen.getByLabelText(/new note/i), { target: { value: 'Procurement asked for revised payment terms.' } })
    fireEvent.click(screen.getByRole('button', { name: /add note/i }))

    expect(await screen.findByText(/procurement asked for revised payment terms/i)).toBeInTheDocument()
    expect(screen.getByText(/note added/i)).toBeInTheDocument()

    expect(screen.queryByLabelText(/linked contact ids/i)).not.toBeInTheDocument()
    expect(screen.getByLabelText(/^linked contact$/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/status/i), { target: { value: 'customer' } })
    fireEvent.change(screen.getByLabelText(/^linked contact$/i), { target: { value: '8' } })
    fireEvent.click(screen.getByRole('button', { name: /update company/i }))

    expect(await screen.findByText(/company updated/i)).toBeInTheDocument()
    expect(screen.getByText('ava@acme.test')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /archive company/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/companies\/6$/), expect.objectContaining({ method: 'DELETE' }))
    })
    await waitFor(() => {
      expect(window.location.pathname).toBe('/companies')
    })
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
              { id: 5, name: 'Northstar Logistics', domain: 'northstar.example', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' }
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
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            company: { id: 5, name: 'Northstar Logistics', domain: 'northstar.example', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' },
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
