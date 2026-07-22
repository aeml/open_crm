import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  deleteEmailSnippet,
  deleteEmailTemplate,
  listEmailSnippetPage,
  listEmailSnippets,
  listEmailTemplatePage,
  listEmailTemplates,
  updateEmailSnippet,
  updateEmailTemplate
} from './email_templates'

afterEach(() => vi.unstubAllGlobals())

function response(data) {
  return { ok: true, json: async () => ({ data }) }
}

describe('email template and snippet API', () => {
  it('builds bounded management page queries', async () => {
    const fetchMock = vi.fn(async (url) => String(url).includes('/email-snippets')
      ? response({ snippets: [{ id: 2 }], meta: { page: 3, pageSize: 25, total: 51 } })
      : response({ templates: [{ id: 1 }], meta: { page: 2, pageSize: 25, total: 26 } }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(listEmailTemplatePage({ search: 'Welcome 100%', page: 2, pageSize: 25 })).resolves.toEqual({
      templates: [{ id: 1 }], meta: { page: 2, pageSize: 25, total: 26 }
    })
    await expect(listEmailSnippetPage({ search: 'Closing note', page: 3, pageSize: 25 })).resolves.toEqual({
      snippets: [{ id: 2 }], meta: { page: 3, pageSize: 25, total: 51 }
    })

    const queries = fetchMock.mock.calls.map((call) => Object.fromEntries(new URL(String(call[0]), 'http://localhost').searchParams))
    expect(queries).toEqual([
      { q: 'Welcome 100%', page: '2', pageSize: '25' },
      { q: 'Closing note', page: '3', pageSize: '25' }
    ])
  })

  it.each([
    ['template', listEmailTemplates, 'templates', '/api/email-templates'],
    ['snippet', listEmailSnippets, 'snippets', '/api/email-snippets']
  ])('preserves composer access to legacy %s overflow', async (_kind, loadAll, field, pathname) => {
    const rows = Array.from({ length: 101 }, (_, index) => ({ id: index + 1, name: `Definition ${index + 1}` }))
    const fetchMock = vi.fn(async (url) => {
      const page = Number(new URL(String(url), 'http://localhost').searchParams.get('page'))
      return response({ [field]: rows.slice((page - 1) * 100, page * 100), meta: { page, pageSize: 100, total: rows.length } })
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(loadAll()).resolves.toEqual(rows)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    const firstRequest = new URL(String(fetchMock.mock.calls[0][0]), 'http://localhost')
    expect(firstRequest.pathname).toBe(pathname)
    expect(Object.fromEntries(firstRequest.searchParams)).toEqual({ page: '1', pageSize: '100' })
  })

  it.each([
    ['template', listEmailTemplates, 'templates'],
    ['snippet', listEmailSnippets, 'snippets']
  ])('fails visibly when the %s catalog changes between pages', async (_kind, loadAll, field) => {
    const firstPage = Array.from({ length: 100 }, (_, index) => ({ id: index + 1 }))
    const fetchMock = vi.fn(async (url) => {
      const page = Number(new URL(String(url), 'http://localhost').searchParams.get('page'))
      return response({ [field]: page === 1 ? firstPage : [], meta: { page, pageSize: 100, total: page === 1 ? 101 : 100 } })
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(loadAll()).rejects.toThrow('changed while options were loading')
  })

  it('binds edits and deletes to the revision the operator reviewed', async () => {
    const fetchMock = vi.fn(async (_url, options) => options.method === 'DELETE'
      ? { ok: true, status: 204 }
      : response({ template: { id: 9, revision: 5 }, snippet: { id: 10, revision: 7 } }))
    vi.stubGlobal('fetch', fetchMock)

    await updateEmailTemplate(9, { name: 'Reviewed', subject: 'Hello', body: 'Body', expectedRevision: 4 })
    await deleteEmailTemplate(9, 5)
    await updateEmailSnippet(10, { name: 'Reviewed CTA', body: 'Body', expectedRevision: 6 })
    await deleteEmailSnippet(10, 7)

    expect(fetchMock.mock.calls.map((call) => {
      const requestURL = new URL(String(call[0]), 'http://localhost')
      return [`${requestURL.pathname}${requestURL.search}`, call[1].method]
    })).toEqual([
      ['/api/email-templates/9', 'PATCH'],
      ['/api/email-templates/9?revision=5', 'DELETE'],
      ['/api/email-snippets/10', 'PATCH'],
      ['/api/email-snippets/10?revision=7', 'DELETE']
    ])
    expect(JSON.parse(fetchMock.mock.calls[0][1].body).expectedRevision).toBe(4)
    expect(JSON.parse(fetchMock.mock.calls[2][1].body).expectedRevision).toBe(6)
  })
})
