import { afterEach, describe, expect, it, vi } from 'vitest'
import { listReportDefinitions } from './reports'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('saved report definition API', () => {
  it.each([
    ['page identity', { page: 2, pageSize: 50, total: 51 }],
    ['page-size identity', { page: 1, pageSize: 49, total: 51 }],
    ['exact total', { page: 1, pageSize: 50, total: 0 }]
  ])('rejects invalid server metadata for %s', async (_label, meta) => {
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: true,
      json: async () => ({ data: { definitions: [{ id: 1, name: 'Saved report' }], meta } })
    })))

    await expect(listReportDefinitions()).rejects.toThrow('invalid report definition page')
  })
})
