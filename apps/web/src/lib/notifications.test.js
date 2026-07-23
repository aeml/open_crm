import { afterEach, describe, expect, it, vi } from 'vitest'
import { listNotifications } from './notifications'

afterEach(() => vi.unstubAllGlobals())

describe('notification API', () => {
  it('uses the exact bounded snapshot contract', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ data: { notifications: [{ id: 9 }], unreadCount: 73, window: { limit: 50 } } })
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(listNotifications()).resolves.toEqual({ notifications: [{ id: 9 }], unreadCount: 73, limit: 50 })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('remains usable while an older API release is rolling out', async () => {
    const controller = new AbortController()
    const fetchMock = vi.fn(async (input) => {
      const path = new URL(String(input), 'http://localhost').pathname
      if (path === '/api/notifications') return { ok: true, json: async () => ({ data: { notifications: [{ id: 8 }] } }) }
      if (path === '/api/notifications/unread-count') return { ok: true, json: async () => ({ data: { unreadCount: 51 } }) }
      throw new Error(`Unexpected request: ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(listNotifications({ signal: controller.signal })).resolves.toEqual({ notifications: [{ id: 8 }], unreadCount: 51, limit: 50 })
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(fetchMock.mock.calls.every((call) => call[1].signal === controller.signal)).toBe(true)
  })

  it('fails visibly when a new snapshot response is only partially shaped', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ data: { notifications: [], unreadCount: 3 } })
    }))

    await expect(listNotifications()).rejects.toThrow('notification response was incomplete')
  })
})
