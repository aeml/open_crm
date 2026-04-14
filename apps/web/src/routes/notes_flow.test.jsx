import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('notes workflow', () => {
  it('loads contact detail notes and creates a new note from the detail pane', async () => {
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
            notes: [
              {
                id: 17,
                entityType: 'contact',
                entityId: 7,
                body: 'Initial discovery call logged.',
                createdByUserId: 1,
                createdByUserName: 'Demo Owner',
                createdAt: '2026-04-10T09:30:00Z',
                updatedAt: '2026-04-10T09:30:00Z'
              }
            ],
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
      .mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: async () => ({
          data: {
            note: {
              id: 18,
              entityType: 'contact',
              entityId: 7,
              body: 'Sent follow-up recap with pricing ranges.',
              createdByUserId: 1,
              createdByUserName: 'Demo Owner',
              createdAt: '2026-04-10T10:15:00Z',
              updatedAt: '2026-04-10T10:15:00Z'
            },
            activity: {
              id: 101,
              action: 'note.created',
              summary: 'Note added',
              createdAt: '2026-04-10T10:15:00Z'
            }
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/contacts/7')

    render(<AppRouter />)

    expect(await screen.findByText(/initial discovery call logged/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/new note/i), { target: { value: 'Sent follow-up recap with pricing ranges.' } })
    fireEvent.click(screen.getByRole('button', { name: /add note/i }))

    expect(await screen.findByText(/sent follow-up recap with pricing ranges/i)).toBeInTheDocument()
    expect(await screen.findByText(/note added/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/notes$/), expect.objectContaining({ method: 'POST' }))
    })
  })
})
