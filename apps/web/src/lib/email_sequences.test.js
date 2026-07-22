import { afterEach, describe, expect, it, vi } from 'vitest'
import { deleteEmailSequence, listEmailSequences, transitionEmailSequence, updateEmailSequence } from './email_sequences'

afterEach(() => vi.unstubAllGlobals())

describe('email sequence API', () => {
  it('loads only the bounded active enrollment catalog', async () => {
    const fetchMock = vi.fn(async () => ({
      ok: true,
      json: async () => ({
        data: {
          sequences: [{ id: 7, name: 'Pilot cadence', status: 'active' }],
          meta: { page: 1, pageSize: 100, total: 1 }
        }
      })
    }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(listEmailSequences()).resolves.toEqual([{ id: 7, name: 'Pilot cadence', status: 'active' }])
    const requestURL = new URL(String(fetchMock.mock.calls[0][0]), 'http://localhost')
    expect(requestURL.pathname).toBe('/api/email-sequences')
    expect(Object.fromEntries(requestURL.searchParams)).toEqual({ status: 'active', page: '1', pageSize: '100' })
  })

  it('preserves access to legacy active sequences above the current ceiling', async () => {
    const sequences = Array.from({ length: 101 }, (_, index) => ({ id: index + 1, name: `Cadence ${index + 1}`, status: 'active' }))
    const fetchMock = vi.fn(async (url) => {
      const page = Number(new URL(String(url), 'http://localhost').searchParams.get('page'))
      return {
        ok: true,
        json: async () => ({ data: { sequences: sequences.slice((page - 1) * 100, page * 100), meta: { page, pageSize: 100, total: sequences.length } } })
      }
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(listEmailSequences()).resolves.toEqual(sequences)
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('fails visibly when the active catalog total changes between pages', async () => {
    const firstPage = Array.from({ length: 100 }, (_, index) => ({ id: index + 1, name: `Cadence ${index + 1}`, status: 'active' }))
    const fetchMock = vi.fn(async (url) => {
      const page = Number(new URL(String(url), 'http://localhost').searchParams.get('page'))
      return {
        ok: true,
        json: async () => ({ data: { sequences: page === 1 ? firstPage : [], meta: { page, pageSize: 100, total: page === 1 ? 101 : 100 } } })
      }
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(listEmailSequences()).rejects.toThrow('changed while options were loading')
  })

  it('binds edits, deletes, and approvals to the revision the operator reviewed', async () => {
    const fetchMock = vi.fn(async (_url, options) => options.method === 'DELETE'
      ? { ok: true, status: 204 }
      : { ok: true, json: async () => ({ data: { sequence: { id: 9, revision: 4 } } }) })
    vi.stubGlobal('fetch', fetchMock)

    await updateEmailSequence(9, { name: 'Reviewed', expectedRevision: 4, steps: [] })
    await deleteEmailSequence(9, 4)
    await transitionEmailSequence(9, 'approve', 4)
    await transitionEmailSequence(9, 'pause', 4)

    expect(fetchMock.mock.calls.map((call) => {
      const requestURL = new URL(String(call[0]), 'http://localhost')
      return [`${requestURL.pathname}${requestURL.search}`, call[1].method]
    })).toEqual([
      ['/api/email-sequences/9', 'PATCH'],
      ['/api/email-sequences/9?revision=4', 'DELETE'],
      ['/api/email-sequences/9/approve?revision=4', 'POST'],
      ['/api/email-sequences/9/pause', 'POST']
    ])
    expect(JSON.parse(fetchMock.mock.calls[0][1].body).expectedRevision).toBe(4)
  })
})
