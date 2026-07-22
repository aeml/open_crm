import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

function response(payload) {
  return { ok: true, json: async () => payload }
}

function sessionResponse() {
  return response({
    data: {
      user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
      organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
      membership: { role: 'owner' }
    }
  })
}

function paged(rows, requestURL, field) {
  const search = requestURL.searchParams.get('q') || ''
  const page = Number(requestURL.searchParams.get('page') || 1)
  const pageSize = Number(requestURL.searchParams.get('pageSize') || 50)
  const filtered = rows.filter((row) => row.name.toLowerCase().includes(search.toLowerCase()))
  const start = (page - 1) * pageSize
  return response({ data: { [field]: filtered.slice(start, start + pageSize), meta: { page, pageSize, total: filtered.length } } })
}

describe('settings email templates route', () => {
  it('lists definitions and reconciles newly created templates and snippets', async () => {
    let templates = [{ id: 3, name: 'Welcome', subject: 'Hi there', body: 'Hello {{first_name}}', revision: 1 }]
    let snippets = [{ id: 4, name: 'Trial CTA', body: 'Want to try it this week?', revision: 1 }]
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return response({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/email-templates/merge-fields')) return response({ data: { groups: [
        { key: 'contact', label: 'Contact fields', fields: [{ token: '{{first_name}}', label: 'First name' }] },
        { key: 'contact_custom', label: 'Contact custom fields', fields: [{ token: '{{contact.custom.region}}', label: 'Region' }] }
      ] } })
      if (path.endsWith('/api/email-snippets') && method === 'POST') {
        const input = JSON.parse(options.body)
        const created = { id: 6, ...input, revision: 1 }
        snippets = [created, ...snippets]
        return response({ data: { snippet: created } })
      }
      if (path.endsWith('/api/email-snippets')) return paged(snippets, requestURL, 'snippets')
      if (path.endsWith('/api/email-templates') && method === 'POST') {
        const input = JSON.parse(options.body)
        const created = { id: 5, ...input, revision: 1 }
        templates = [created, ...templates]
        return response({ data: { template: created } })
      }
      if (path.endsWith('/api/email-templates')) return paged(templates, requestURL, 'templates')
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/email-templates')
    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /welcome/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /trial cta/i })).toBeInTheDocument()
    expect(await screen.findByText('{{first_name}}')).toBeInTheDocument()
    expect(await screen.findByText('{{contact.custom.region}}')).toBeInTheDocument()
    expect(screen.getByLabelText(/^name$/i)).toHaveAttribute('maxlength', '120')
    expect(screen.getByLabelText(/^subject$/i)).toHaveAttribute('maxlength', '500')
    expect(screen.getByLabelText(/^body$/i)).toHaveAttribute('maxlength', '10000')

    fireEvent.change(screen.getByLabelText(/^name$/i), { target: { value: 'Follow up' } })
    fireEvent.change(screen.getByLabelText(/^subject$/i), { target: { value: 'Checking in' } })
    fireEvent.change(screen.getByLabelText(/^body$/i), { target: { value: 'Hi {{first_name}}' } })
    fireEvent.click(screen.getByRole('button', { name: /create template/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/email-templates') && call[1]?.method === 'POST')
      expect(JSON.parse(createCall[1].body)).toEqual({ name: 'Follow up', subject: 'Checking in', body: 'Hi {{first_name}}' })
    })
    expect(await screen.findByRole('heading', { name: /follow up/i })).toBeInTheDocument()
    expect(screen.getByText('Email template created.')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/snippet name/i), { target: { value: 'Scheduling CTA' } })
    fireEvent.change(screen.getByLabelText(/snippet body/i), { target: { value: 'Would next week work?' } })
    fireEvent.click(screen.getByRole('button', { name: /create snippet/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/email-snippets') && call[1]?.method === 'POST')
      expect(JSON.parse(createCall[1].body)).toEqual({ name: 'Scheduling CTA', body: 'Would next week work?' })
    })
    expect(await screen.findByRole('heading', { name: /scheduling cta/i })).toBeInTheDocument()
    expect(screen.getByText('Email snippet created.')).toBeInTheDocument()
  })

  it('pages and searches template and snippet catalogs independently', async () => {
    const templates = Array.from({ length: 51 }, (_, index) => ({
      id: index + 1,
      name: `Message template ${String(index + 1).padStart(3, '0')}`,
      subject: `Subject ${index + 1}`,
      body: 'Body',
      revision: 1
    }))
    const snippets = Array.from({ length: 51 }, (_, index) => ({
      id: index + 101,
      name: `Reusable snippet ${String(index + 1).padStart(3, '0')}`,
      body: 'Snippet body',
      revision: 1
    }))
    const fetchMock = vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return response({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/email-templates/merge-fields')) return response({ data: { groups: [] } })
      if (path.endsWith('/api/email-templates')) return paged(templates, requestURL, 'templates')
      if (path.endsWith('/api/email-snippets')) return paged(snippets, requestURL, 'snippets')
      throw new Error(`Unexpected fetch: ${path}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/email-templates')
    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: 'Message template 001' })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: 'Reusable snippet 001' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Message template 051' })).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Reusable snippet 051' })).not.toBeInTheDocument()
    expect(screen.getByText(/Showing 50 of 51 email templates/)).toBeInTheDocument()
    expect(screen.getByText(/Showing 50 of 51 email snippets/)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Next template page' }))
    expect(await screen.findByRole('heading', { name: 'Message template 051' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Reusable snippet 051' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Next snippet page' }))
    expect(await screen.findByRole('heading', { name: 'Reusable snippet 051' })).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Search email templates'), { target: { value: 'Message template 025' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply template search' }))
    expect(await screen.findByText(/Showing 1 of 1 email templates matching/)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Search email snippets'), { target: { value: 'Reusable snippet 030' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply snippet search' }))
    expect(await screen.findByText(/Showing 1 of 1 email snippets matching/)).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/email-templates\?q=Message\+template\+025&page=1&pageSize=50/), expect.any(Object))
    expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/email-snippets\?q=Reusable\+snippet\+030&page=1&pageSize=50/), expect.any(Object))
    expect(screen.getByText(/Up to 100 templates may be stored/)).toBeInTheDocument()
    expect(screen.getByText(/Up to 100 snippets may be stored/)).toBeInTheDocument()
  })

  it('discards an obsolete template search response', async () => {
    let resolveFirstSearch
    let resolveSecondSearch
    const fetchMock = vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const search = requestURL.searchParams.get('q') || ''
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return response({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/email-templates/merge-fields')) return response({ data: { groups: [] } })
      if (path.endsWith('/api/email-snippets')) return response({ data: { snippets: [], meta: { page: 1, pageSize: 50, total: 0 } } })
      if (path.endsWith('/api/email-templates') && search === 'First') {
        return new Promise((resolve) => { resolveFirstSearch = resolve })
      }
      if (path.endsWith('/api/email-templates') && search === 'Second') {
        return new Promise((resolve) => { resolveSecondSearch = resolve })
      }
      if (path.endsWith('/api/email-templates')) return response({ data: { templates: [], meta: { page: 1, pageSize: 50, total: 0 } } })
      throw new Error(`Unexpected fetch: ${path}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/email-templates')
    render(<AppRouter />)
    await screen.findByText('No email templates yet.')
    const searchInput = screen.getByLabelText('Search email templates')
    const searchForm = searchInput.closest('form')

    fireEvent.change(searchInput, { target: { value: 'First' } })
    fireEvent.submit(searchForm)
    await waitFor(() => expect(resolveFirstSearch).toBeTypeOf('function'))
    fireEvent.change(searchInput, { target: { value: 'Second' } })
    fireEvent.submit(searchForm)
    await waitFor(() => expect(resolveSecondSearch).toBeTypeOf('function'))

    resolveSecondSearch(response({ data: { templates: [{ id: 2, name: 'Second result', subject: 'Current', body: 'Body', revision: 1 }], meta: { page: 1, pageSize: 50, total: 1 } } }))
    expect(await screen.findByRole('heading', { name: 'Second result' })).toBeInTheDocument()
    resolveFirstSearch(response({ data: { templates: [{ id: 1, name: 'Stale first result', subject: 'Old', body: 'Body', revision: 1 }], meta: { page: 1, pageSize: 50, total: 1 } } }))
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Stale first result' })).not.toBeInTheDocument())
    expect(screen.getByRole('heading', { name: 'Second result' })).toBeInTheDocument()
  })

  it('serializes template and snippet writes through one operation boundary', async () => {
    let completeTemplateCreate
    let templates = []
    let snippets = []
    const createdTemplate = { id: 8, name: 'Serialized template', subject: 'Subject', body: 'Body', revision: 1 }
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return response({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/email-templates/merge-fields')) return response({ data: { groups: [] } })
      if (path.endsWith('/api/email-templates') && method === 'POST') {
        return new Promise((resolve) => {
          completeTemplateCreate = () => {
            templates = [createdTemplate]
            resolve(response({ data: { template: createdTemplate } }))
          }
        })
      }
      if (path.endsWith('/api/email-snippets') && method === 'POST') {
        const input = JSON.parse(options.body)
        const created = { id: 9, ...input, revision: 1 }
        snippets = [created]
        return response({ data: { snippet: created } })
      }
      if (path.endsWith('/api/email-templates')) return paged(templates, requestURL, 'templates')
      if (path.endsWith('/api/email-snippets')) return paged(snippets, requestURL, 'snippets')
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/email-templates')
    render(<AppRouter />)
    await screen.findByText('No email templates yet.')
    fireEvent.change(screen.getByLabelText(/^name$/i), { target: { value: 'Serialized template' } })
    fireEvent.change(screen.getByLabelText(/^subject$/i), { target: { value: 'Subject' } })
    fireEvent.change(screen.getByLabelText(/^body$/i), { target: { value: 'Body' } })
    fireEvent.change(screen.getByLabelText(/snippet name/i), { target: { value: 'Blocked overlap' } })
    fireEvent.change(screen.getByLabelText(/snippet body/i), { target: { value: 'Body' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create template' }))
    await waitFor(() => expect(completeTemplateCreate).toBeTypeOf('function'))

    fireEvent.submit(screen.getByLabelText(/snippet name/i).closest('form'))
    expect(fetchMock.mock.calls.filter((call) => String(call[0]).endsWith('/api/email-snippets') && call[1]?.method === 'POST')).toHaveLength(0)
    completeTemplateCreate()
    expect(await screen.findByText('Email template created.')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Create snippet' }))
    await waitFor(() => expect(fetchMock.mock.calls.filter((call) => String(call[0]).endsWith('/api/email-snippets') && call[1]?.method === 'POST')).toHaveLength(1))
  })
})
