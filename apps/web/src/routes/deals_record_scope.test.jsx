import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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

const pipeline = {
  id: 1,
  name: 'Sales pipeline',
  position: 1,
  isDefault: true,
  stages: [
    { id: 1, pipelineId: 1, name: 'Lead', position: 1, isClosed: false, isWon: false },
    { id: 2, pipelineId: 1, name: 'Qualified', position: 2, isClosed: false, isWon: false }
  ]
}

function deal(id, name) {
  return {
    id,
    name,
    stageId: id === 11 ? 1 : 2,
    stageName: id === 11 ? 'Lead' : 'Qualified',
    companyId: 0,
    companyName: '',
    primaryContactId: 0,
    primaryContactName: '',
    status: 'open',
    valueAmount: id === 11 ? '1000.00' : '2000.00',
    valueCurrency: 'USD',
    expectedCloseDate: '2026-08-01',
    ownerUserId: 1,
    ownerUserName: 'Demo Owner'
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('deal record scoping', () => {
  it('rejects late A -> B -> A loads and applies an old save only to its directory record', async () => {
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
      if (requestURL.pathname.endsWith('/api/deal-pipelines')) return response({ pipelines: [pipeline] })
      if (requestURL.pathname.endsWith('/api/deals') && method === 'GET') {
        return response({
          deals: [deal(11, 'Alpha listed'), deal(12, 'Beta listed')],
          meta: { page: 1, pageSize: 20, total: 2, openCount: 2, wonCount: 0, pipelineValue: '3000.00' }
        })
      }
      if (requestURL.pathname.endsWith('/api/companies')) return response({ companies: [], meta: { page: 1, pageSize: 20, total: 0 } })
      if (requestURL.pathname.endsWith('/api/contacts')) return response({ contacts: [], meta: { page: 1, pageSize: 20, total: 0 } })
      if (requestURL.pathname.endsWith('/api/users')) {
        return response({ users: [{ id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' }] })
      }
      if (requestURL.pathname.endsWith('/api/product-catalog-items')) return response({ items: [] })
      if (requestURL.pathname.endsWith('/api/quote-templates/policy')) return response({ policy: { approvalRequired: false, activeApprovers: 2 } })
      if (requestURL.pathname.endsWith('/api/quote-templates')) return response({ templates: [] })
      if (requestURL.pathname.endsWith('/api/notes')) {
        const entityId = Number(requestURL.searchParams.get('entityId'))
        return response({ notes: [{ id: entityId * 10 + alphaReads, entityType: 'deal', entityId, body: entityId === 11 && alphaReads > 1 ? 'Alpha current note' : `Deal ${entityId} note` }] })
      }
      if (requestURL.pathname.endsWith('/api/tasks')) return response({ tasks: [] })
      if (requestURL.pathname.endsWith('/api/deals/11') && method === 'GET') {
        alphaReads += 1
        if (alphaReads === 1) return firstAlphaLoad.promise
        return response({ deal: deal(11, 'Alpha current'), activities: [{ id: 112, summary: 'Alpha current activity' }] })
      }
      if (requestURL.pathname.endsWith('/api/deals/12') && method === 'GET') {
        return response({ deal: deal(12, 'Beta current'), activities: [{ id: 121, summary: 'Beta current activity' }] })
      }
      if (requestURL.pathname.endsWith('/api/deals/11') && method === 'PATCH') return alphaUpdate.promise

      throw new Error(`Unexpected fetch: ${method} ${requestURL.pathname}${requestURL.search}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/deals')
    render(<AppRouter />)

    fireEvent.click(await screen.findByRole('button', { name: 'Alpha listed' }))
    await waitFor(() => expect(alphaReads).toBe(1))
    fireEvent.click(screen.getByRole('button', { name: 'Beta listed' }))

    let detailForm = await screen.findByRole('form', { name: /deal details form/i })
    await waitFor(() => expect(within(detailForm).getByLabelText(/deal name/i)).toHaveValue('Beta current'))
    fireEvent.click(screen.getByRole('button', { name: 'Alpha listed' }))

    detailForm = await screen.findByRole('form', { name: /deal details form/i })
    await waitFor(() => expect(within(detailForm).getByLabelText(/deal name/i)).toHaveValue('Alpha current'))
    await act(async () => {
      firstAlphaLoad.resolve(response({ deal: deal(11, 'Alpha stale'), activities: [{ id: 111, summary: 'Alpha stale activity' }] }))
      await firstAlphaLoad.promise
    })

    expect(within(detailForm).getByLabelText(/deal name/i)).toHaveValue('Alpha current')
    expect(screen.queryByText('Alpha stale activity')).not.toBeInTheDocument()
    expect(screen.getByText('Alpha current note')).toBeInTheDocument()

    fireEvent.change(within(detailForm).getByLabelText(/deal name/i), { target: { value: 'Alpha edited' } })
    fireEvent.click(within(detailForm).getByRole('button', { name: /update deal/i }))
    fireEvent.click(screen.getByRole('button', { name: 'Beta current' }))
    detailForm = await screen.findByRole('form', { name: /deal details form/i })
    await waitFor(() => expect(within(detailForm).getByLabelText(/deal name/i)).toHaveValue('Beta current'))

    await act(async () => {
      alphaUpdate.resolve(response({ deal: deal(11, 'Alpha persisted'), activities: [{ id: 113, summary: 'Alpha update finished' }] }))
      await alphaUpdate.promise
    })

    await waitFor(() => expect(screen.getByRole('button', { name: 'Alpha persisted' })).toBeInTheDocument())
    expect(within(detailForm).getByLabelText(/deal name/i)).toHaveValue('Beta current')
    expect(screen.queryByText('Alpha update finished')).not.toBeInTheDocument()
  })

  it('does not select a newly created deal after the user opens another deal during quote preparation', async () => {
    const firstCatalogLoad = deferred()
    let catalogReads = 0
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
      if (requestURL.pathname.endsWith('/api/deal-pipelines')) return response({ pipelines: [pipeline] })
      if (requestURL.pathname.endsWith('/api/deals') && method === 'GET') {
        return response({
          deals: [deal(11, 'Alpha listed'), deal(12, 'Beta listed')],
          meta: { page: 1, pageSize: 20, total: 2, openCount: 2, wonCount: 0, pipelineValue: '3000.00' }
        })
      }
      if (requestURL.pathname.endsWith('/api/deals') && method === 'POST') {
        return response({ deal: deal(13, 'Created while loading'), activities: [{ id: 131, summary: 'Created deal activity' }] }, 201)
      }
      if (requestURL.pathname.endsWith('/api/companies')) return response({ companies: [], meta: { page: 1, pageSize: 20, total: 0 } })
      if (requestURL.pathname.endsWith('/api/contacts')) return response({ contacts: [], meta: { page: 1, pageSize: 20, total: 0 } })
      if (requestURL.pathname.endsWith('/api/users')) {
        return response({ users: [{ id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' }] })
      }
      if (requestURL.pathname.endsWith('/api/product-catalog-items')) {
        catalogReads += 1
        if (catalogReads === 1) return firstCatalogLoad.promise
        return response({ items: [] })
      }
      if (requestURL.pathname.endsWith('/api/quote-templates/policy')) return response({ policy: { approvalRequired: false, activeApprovers: 2 } })
      if (requestURL.pathname.endsWith('/api/quote-templates')) return response({ templates: [] })
      if (requestURL.pathname.endsWith('/api/notes')) return response({ notes: [] })
      if (requestURL.pathname.endsWith('/api/tasks')) return response({ tasks: [] })
      if (requestURL.pathname.endsWith('/api/deals/12') && method === 'GET') {
        return response({ deal: deal(12, 'Beta current'), activities: [{ id: 121, summary: 'Beta current activity' }] })
      }

      throw new Error(`Unexpected fetch: ${method} ${requestURL.pathname}${requestURL.search}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/deals')
    render(<AppRouter />)

    fireEvent.change(await screen.findByLabelText(/deal name/i), { target: { value: 'Created while loading' } })
    fireEvent.click(screen.getByRole('button', { name: /save deal/i }))
    await waitFor(() => expect(catalogReads).toBe(1))

    fireEvent.click(screen.getByRole('button', { name: 'Beta listed' }))
    const detailForm = await screen.findByRole('form', { name: /deal details form/i })
    await waitFor(() => expect(within(detailForm).getByLabelText(/deal name/i)).toHaveValue('Beta current'))
    expect(window.location.pathname).toBe('/deals/12')

    await act(async () => {
      firstCatalogLoad.resolve(response({ items: [] }))
      await firstCatalogLoad.promise
    })

    await waitFor(() => expect(within(detailForm).getByLabelText(/deal name/i)).toHaveValue('Beta current'))
    expect(window.location.pathname).toBe('/deals/12')
    expect(screen.getByRole('button', { name: 'Created while loading' })).toBeInTheDocument()
  })
})
