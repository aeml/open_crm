import { afterEach, describe, expect, it, vi } from 'vitest'
import { APIError, apiRequest, apiURL, getErrorMessage, isAbortError, readJSON } from './api'

afterEach(() => {
  vi.restoreAllMocks()
})

describe('api client', () => {
  it('reads empty JSON for 204 responses', async () => {
    await expect(readJSON({ status: 204, json: vi.fn() })).resolves.toEqual({})
  })

  it('extracts API error messages with fallback support', () => {
    expect(getErrorMessage({ error: { message: 'Specific failure' } }, 'Fallback')).toBe('Specific failure')
    expect(getErrorMessage({}, 'Fallback')).toBe('Fallback')
  })

  it('detects abort errors', () => {
    expect(isAbortError(new DOMException('Cancelled', 'AbortError'))).toBe(true)
    expect(isAbortError(new Error('Failed'))).toBe(false)
  })

  it('builds absolute API URLs for browser navigations', () => {
    expect(apiURL('/api/export/contacts')).toBe('https://crmserver.mendola.tech/api/export/contacts')
  })

  it('sends credentialed JSON requests', async () => {
    const payload = { data: { ok: true } }
    const controller = new AbortController()
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => payload
    })

    await expect(apiRequest('/api/example', { method: 'POST', body: { name: 'Demo' }, signal: controller.signal })).resolves.toEqual(payload)

    expect(fetchMock).toHaveBeenCalledWith(
      'https://crmserver.mendola.tech/api/example',
      expect.objectContaining({
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        signal: controller.signal,
        body: JSON.stringify({ name: 'Demo' })
      })
    )
  })

  it('throws APIError with status and payload for failed responses', async () => {
    const payload = { error: { message: 'No access' } }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: false,
      status: 403,
      json: async () => payload
    })

    await expect(apiRequest('/api/private', { fallbackMessage: 'Unable to load.' })).rejects.toMatchObject({
      name: 'APIError',
      message: 'No access',
      status: 403,
      payload
    })
  })

  it('exposes APIError for direct callers', () => {
    const error = new APIError('Failed', { status: 500, payload: { error: { code: 'INTERNAL' } } })

    expect(error).toBeInstanceOf(Error)
    expect(error.status).toBe(500)
  })
})
