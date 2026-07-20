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

function company(id, name) {
  return {
    id,
    name,
    clientType: 'organization',
    addressLine1: '',
    addressLine2: '',
    city: '',
    state: '',
    postalCode: '',
    country: '',
    industry: 'Services',
    phone: '',
    website: `https://${name.toLowerCase().split(' ')[0]}.example`,
    status: 'prospect',
    ownerUserId: 1,
    ownerUserName: 'Demo Owner',
    customFields: {}
  }
}

function detail(id, name, activity) {
  return { company: company(id, name), linkedContacts: [], activities: [{ id: id * 10, summary: activity }] }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('company record scoping', () => {
  it('rejects late A -> B -> A loads and applies an old save only to its company directory record', async () => {
    const firstAlphaLoad = deferred()
    const alphaUpdate = deferred()
    let alphaReads = 0
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const method = options.method || 'GET'

      if (requestURL.pathname.endsWith('/auth/me')) {
        return response({
          user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
          organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
          membership: { role: 'owner' }
        })
      }
      if (requestURL.pathname.endsWith('/api/notifications/unread-count')) return response({ unreadCount: 0 })
      if (requestURL.pathname.endsWith('/api/companies') && method === 'GET') {
        return response({ companies: [company(11, 'Alpha listed'), company(12, 'Beta listed')], meta: { page: 1, pageSize: 20, total: 2 } })
      }
      if (requestURL.pathname.endsWith('/api/contacts')) return response({ contacts: [], meta: { page: 1, pageSize: 20, total: 0 } })
      if (requestURL.pathname.endsWith('/api/users')) {
        return response({ users: [{ id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner', status: 'active' }] })
      }
      if (requestURL.pathname.endsWith('/api/custom-fields')) return response({ definitions: [] })
      if (requestURL.pathname.endsWith('/api/reports/client-health')) {
        return response({ entityType: 'company', status: 'all', count: 0, totals: { total: 0, healthy: 0, watch: 0, needsAttention: 0 }, records: [], semantics: [] })
      }
      if (requestURL.pathname.includes('/api/touchpoints/')) return response({ staleDays: 30, isStale: false, recent: [], semantics: [] })
      if (requestURL.pathname.endsWith('/api/notes')) {
        const entityId = Number(requestURL.searchParams.get('entityId'))
        const body = entityId === 11 && alphaReads > 1 ? 'Alpha current note' : `Company ${entityId} note`
        return response({ notes: [{ id: entityId * 100 + alphaReads, entityType: 'company', entityId, body }] })
      }
      if (requestURL.pathname.endsWith('/api/tasks')) return response({ tasks: [] })
      if (requestURL.pathname.endsWith('/api/deals')) return response({ deals: [], meta: { page: 1, pageSize: 20, total: 0 } })
      if (requestURL.pathname.endsWith('/api/companies/11') && method === 'GET') {
        alphaReads += 1
        if (alphaReads === 1) return firstAlphaLoad.promise
        return response(detail(11, 'Alpha current', 'Alpha current activity'))
      }
      if (requestURL.pathname.endsWith('/api/companies/12') && method === 'GET') {
        return response(detail(12, 'Beta current', 'Beta current activity'))
      }
      if (requestURL.pathname.endsWith('/api/companies/11') && method === 'PATCH') return alphaUpdate.promise

      throw new Error(`Unexpected fetch: ${method} ${requestURL.pathname}${requestURL.search}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/companies/11')
    render(<AppRouter />)

    await waitFor(() => expect(alphaReads).toBe(1))
    fireEvent.click(await screen.findByRole('button', { name: 'Beta listed' }))
    expect(await screen.findByRole('heading', { name: 'Beta current' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Alpha listed' }))
    expect(await screen.findByRole('heading', { name: 'Alpha current' })).toBeInTheDocument()

    await act(async () => {
      firstAlphaLoad.resolve(response(detail(11, 'Alpha stale', 'Alpha stale activity')))
      await firstAlphaLoad.promise
    })

    expect(screen.getByRole('heading', { name: 'Alpha current' })).toBeInTheDocument()
    expect(screen.getByText('Alpha current note')).toBeInTheDocument()
    expect(screen.queryByText('Alpha stale activity')).not.toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Client name'), { target: { value: 'Alpha edited' } })
    fireEvent.click(screen.getByRole('button', { name: 'Update client' }))
    fireEvent.click(screen.getByRole('button', { name: 'Beta listed' }))
    expect(await screen.findByRole('heading', { name: 'Beta current' })).toBeInTheDocument()

    await act(async () => {
      alphaUpdate.resolve(response(detail(11, 'Alpha persisted', 'Alpha update finished')))
      await alphaUpdate.promise
    })

    await waitFor(() => expect(screen.getByRole('button', { name: 'Alpha persisted' })).toBeInTheDocument())
    expect(screen.getByRole('heading', { name: 'Beta current' })).toBeInTheDocument()
    expect(screen.getByLabelText('Client name')).toHaveValue('Beta current')
    expect(screen.queryByText('Alpha update finished')).not.toBeInTheDocument()
  })
})
