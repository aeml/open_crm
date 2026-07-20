import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AppRouter } from '../app/router'

function deferred() {
  let resolve
  const promise = new Promise((next) => { resolve = next })
  return { promise, resolve }
}

function response(data, status = 200) {
  return { ok: status >= 200 && status < 300, status, json: async () => ({ data }) }
}

function contact(id, firstName, suffix = 'listed') {
  return {
    id,
    firstName,
    lastName: suffix,
    email: `${firstName.toLowerCase()}@acme.test`,
    phone: '',
    jobTitle: '',
    status: 'lead'
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('contact record scoping', () => {
  it('rejects late A -> B -> A loads and keeps an old save off the active contact', async () => {
    const firstAlphaLoad = deferred()
    const alphaUpdate = deferred()
    let alphaReads = 0
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const method = options.method || 'GET'

      if (requestURL.pathname.endsWith('/auth/me')) {
        return response({
          user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
          organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' },
          membership: { role: 'owner' }
        })
      }
      if (requestURL.pathname.endsWith('/api/notifications/unread-count')) return response({ unreadCount: 0 })
      if (requestURL.pathname.endsWith('/api/contacts') && method === 'GET') {
        return response({ contacts: [contact(11, 'Alpha'), contact(12, 'Beta')], meta: { page: 1, pageSize: 20, total: 2 } })
      }
      if (requestURL.pathname.endsWith('/api/users')) {
        return response({ users: [{ id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' }] })
      }
      if (requestURL.pathname.endsWith('/api/custom-fields')) return response({ definitions: [] })
      if (requestURL.pathname.includes('/api/touchpoints/')) return response({ staleDays: 30, isStale: false, recent: [], semantics: [] })
      if (requestURL.pathname.endsWith('/api/tasks')) return response({ tasks: [] })
      if (requestURL.pathname.endsWith('/api/deals')) return response({ deals: [], meta: { page: 1, pageSize: 20, total: 0 } })
      if (requestURL.pathname.endsWith('/api/contacts/11') && method === 'GET') {
        alphaReads += 1
        if (alphaReads === 1) return firstAlphaLoad.promise
        return response({
          contact: contact(11, 'Alpha', 'current'),
          notes: [{ id: 111, entityType: 'contact', entityId: 11, body: 'Alpha current note' }],
          activities: [{ id: 112, summary: 'Alpha current activity' }]
        })
      }
      if (requestURL.pathname.endsWith('/api/contacts/12') && method === 'GET') {
        return response({ contact: contact(12, 'Beta', 'current'), notes: [], activities: [{ id: 121, summary: 'Beta current activity' }] })
      }
      if (requestURL.pathname.endsWith('/api/contacts/11') && method === 'PATCH') return alphaUpdate.promise

      throw new Error(`Unexpected fetch: ${method} ${requestURL.pathname}${requestURL.search}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/contacts/11')
    render(<AppRouter />)

    await waitFor(() => expect(alphaReads).toBe(1))
    fireEvent.click(await screen.findByRole('button', { name: 'Beta listed' }))
    expect(await screen.findByRole('heading', { name: 'Beta current' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Alpha listed' }))
    expect(await screen.findByRole('heading', { name: 'Alpha current' })).toBeInTheDocument()

    await act(async () => {
      firstAlphaLoad.resolve(response({
        contact: contact(11, 'Alpha', 'stale'),
        notes: [{ id: 110, entityType: 'contact', entityId: 11, body: 'Alpha stale note' }],
        activities: [{ id: 110, summary: 'Alpha stale activity' }]
      }))
      await firstAlphaLoad.promise
    })

    expect(screen.getByRole('heading', { name: 'Alpha current' })).toBeInTheDocument()
    expect(screen.getByText('Alpha current note')).toBeInTheDocument()
    expect(screen.queryByText('Alpha stale note')).not.toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('First name'), { target: { value: 'Alpha edited' } })
    fireEvent.click(screen.getByRole('button', { name: 'Update contact' }))
    fireEvent.click(screen.getByRole('button', { name: 'Beta listed' }))
    expect(await screen.findByRole('heading', { name: 'Beta current' })).toBeInTheDocument()

    await act(async () => {
      alphaUpdate.resolve(response({ contact: contact(11, 'Alpha', 'persisted'), activities: [{ id: 113, summary: 'Alpha update finished' }] }))
      await alphaUpdate.promise
    })

    await waitFor(() => expect(screen.getByRole('button', { name: 'Alpha persisted' })).toBeInTheDocument())
    expect(screen.getByRole('heading', { name: 'Beta current' })).toBeInTheDocument()
    expect(screen.getByLabelText('First name')).toHaveValue('Beta')
    expect(screen.queryByText('Alpha update finished')).not.toBeInTheDocument()
  })
})
