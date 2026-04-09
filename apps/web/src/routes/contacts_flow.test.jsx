import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('contacts flow', () => {
  it('loads searchable contacts list and opens contact detail', async () => {
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
            contact: { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', jobTitle: 'Head of RevOps', status: 'lead' },
            notes: [],
            tasks: [],
            activities: [
              { id: 100, action: 'contact.created', summary: 'Contact created' }
            ]
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/contacts')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /contacts/i })).toBeInTheDocument()
    expect(await screen.findByText('morgan@acme.test')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/search contacts/i), { target: { value: 'morgan' } })

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/contacts\?q=morgan/), expect.any(Object))
    })

    fireEvent.click(screen.getByRole('button', { name: /morgan lee/i }))

    expect(await screen.findByRole('heading', { name: /morgan lee/i })).toBeInTheDocument()
    expect(screen.getByText(/contact created/i)).toBeInTheDocument()
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
            contact: { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0100', jobTitle: 'COO', status: 'lead' },
            notes: [],
            tasks: [],
            activities: []
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            contact: { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@acme.test', phone: '555-0100', jobTitle: 'COO', status: 'customer' },
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
    window.history.pushState({}, '', '/contacts')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /contacts/i })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /add contact/i }))
    fireEvent.change(screen.getByLabelText(/first name/i), { target: { value: 'Ava' } })
    fireEvent.change(screen.getByLabelText(/last name/i), { target: { value: 'Stone' } })
    fireEvent.change(screen.getByLabelText(/^email$/i), { target: { value: 'ava@acme.test' } })
    fireEvent.change(screen.getByLabelText(/phone/i), { target: { value: '555-0100' } })
    fireEvent.change(screen.getByLabelText(/job title/i), { target: { value: 'COO' } })
    fireEvent.click(screen.getByRole('button', { name: /save contact/i }))

    expect(await screen.findByRole('heading', { name: /ava stone/i })).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/status/i), { target: { value: 'customer' } })
    fireEvent.click(screen.getByRole('button', { name: /update contact/i }))

    expect(await screen.findByText(/contact updated/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /archive contact/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/contacts\/8$/), expect.objectContaining({ method: 'DELETE' }))
    })
  })
})
