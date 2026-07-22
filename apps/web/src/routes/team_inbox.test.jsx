import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('team inbox route', () => {
  it('lists shared inbound messages and updates assignment/status', async () => {
    const jsonResponse = (payload) => ({ ok: true, json: async () => payload })
    let currentMessage = { id: 12, direction: 'inbound', visibility: 'shared', fromEmail: 'lead@example.test', toEmail: 'team@acme.test', subject: 'Pricing question', status: 'received', sharedInboxStatus: 'open', sharedInboxUpdatedAt: '2026-05-03T12:01:00.123456Z', receivedAt: '2026-05-03T12:00:00Z', createdAt: '2026-05-03T12:01:00Z' }
    let updateCount = 0
    const fetchMock = vi.fn(async (url, options = {}) => {
      const path = new URL(String(url), 'http://localhost').pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) {
        return jsonResponse({ data: {
          user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
          organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
          membership: { role: 'member' }
        } })
      }
      if (path.endsWith('/api/shared-inbox/email-messages')) {
        return jsonResponse({ data: { messages: [currentMessage], meta: { limit: 50, hasMore: false, nextCursor: '' } } })
      }
      if (path.endsWith('/api/email-messages/12/shared-inbox') && method === 'PATCH') {
        const input = JSON.parse(options.body)
        expect(input.expectedUpdatedAt).toBe(currentMessage.sharedInboxUpdatedAt)
        updateCount += 1
        currentMessage = { ...currentMessage, sharedInboxStatus: input.status || currentMessage.sharedInboxStatus, sharedInboxAssignedToUserId: input.assignedToUserId || currentMessage.sharedInboxAssignedToUserId || 0, sharedInboxAssignedToUserName: input.assignedToUserId ? 'Demo Owner' : currentMessage.sharedInboxAssignedToUserName || '', sharedInboxUpdatedAt: `2026-05-03T12:01:0${updateCount}.123456Z` }
        return jsonResponse({ data: { message: { ...currentMessage, body: 'Can you send pricing?' } } })
      }
      if (path.endsWith('/api/email-threads/12')) {
        return jsonResponse({ data: { messages: [{ ...currentMessage, body: 'Can you send pricing?' }], replies: [] } })
      }
      return jsonResponse({ data: { unreadCount: 0 } })
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/team-inbox')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /team inbox/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /pricing question/i })).toBeInTheDocument()
    expect(screen.getByText(/from lead@example.test/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /assign to me/i }))
    await waitFor(() => {
      const assignCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/email-messages/12/shared-inbox') && JSON.parse(call[1].body).assignedToUserId === 1)
      expect(assignCall).toBeTruthy()
      expect(JSON.parse(assignCall[1].body).expectedUpdatedAt).toBe('2026-05-03T12:01:00.123456Z')
    })
    expect(await screen.findByText(/demo owner/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /^close$/i }))
    await waitFor(() => {
      const closeCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/email-messages/12/shared-inbox') && JSON.parse(call[1].body).status === 'closed')
      expect(closeCall).toBeTruthy()
      expect(JSON.parse(closeCall[1].body).expectedUpdatedAt).toBe('2026-05-03T12:01:01.123456Z')
    })

    fireEvent.click(screen.getByRole('button', { name: /view details/i }))
    expect(await screen.findByText(/can you send pricing/i)).toBeInTheDocument()
  })

  it('loads older messages once and preserves a newer loaded version of a duplicate', async () => {
    const current = { id: 21, direction: 'inbound', visibility: 'shared', fromEmail: 'new@example.test', subject: 'Newest question', sharedInboxStatus: 'open', sharedInboxAssignedToUserName: 'Current owner', sharedInboxUpdatedAt: '2026-07-22T12:00:00.123456Z', receivedAt: '2026-07-22T11:00:00.123456Z', createdAt: '2026-07-22T11:00:00.123456Z' }
    const older = { id: 20, direction: 'inbound', visibility: 'shared', fromEmail: 'older@example.test', subject: 'Older question', sharedInboxStatus: 'closed', sharedInboxUpdatedAt: '2026-07-20T12:00:00Z', receivedAt: '2026-07-20T11:00:00Z', createdAt: '2026-07-20T11:00:00Z' }
    const fetchMock = vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      if (requestURL.pathname.endsWith('/auth/me')) {
        return { ok: true, json: async () => ({ data: {
          user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
          organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
          membership: { role: 'member' }
        } }) }
      }
      if (requestURL.pathname.endsWith('/api/shared-inbox/email-messages')) {
        if (requestURL.searchParams.get('cursor')) {
          expect(requestURL.searchParams.get('cursor')).toBe('older-page')
          expect(requestURL.searchParams.get('limit')).toBe('2')
          return { ok: true, json: async () => ({ data: {
            messages: [{ ...current, subject: 'Stale duplicate', sharedInboxAssignedToUserName: 'Stale owner', sharedInboxUpdatedAt: '2026-07-22T12:00:00.123455Z' }, older],
            meta: { limit: 2, hasMore: false, nextCursor: '' }
          } }) }
        }
        return { ok: true, json: async () => ({ data: {
          messages: [current], meta: { limit: 2, hasMore: true, nextCursor: 'older-page' }
        } }) }
      }
      return { ok: true, json: async () => ({ data: { unreadCount: 0 } }) }
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/team-inbox')
    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: 'Newest question' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Load older messages' }))

    expect(await screen.findByRole('heading', { name: 'Older question' })).toBeInTheDocument()
    expect(screen.getAllByRole('heading', { name: 'Newest question' })).toHaveLength(1)
    expect(screen.getByText(/current owner/i)).toBeInTheDocument()
    expect(screen.queryByText(/stale owner/i)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Load older messages' })).not.toBeInTheDocument()
  })

  it('keeps an older backend response without page metadata usable', async () => {
    const fetchMock = vi.fn(async (url) => {
      const path = new URL(String(url), 'http://localhost').pathname
      if (path.endsWith('/auth/me')) {
        return { ok: true, json: async () => ({ data: {
          user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
          organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
          membership: { role: 'member' }
        } }) }
      }
      if (path.endsWith('/api/shared-inbox/email-messages')) {
        return { ok: true, json: async () => ({ data: { messages: [{ id: 31, direction: 'inbound', visibility: 'shared', fromEmail: 'legacy@example.test', subject: 'Compatible response', sharedInboxStatus: 'open', createdAt: '2026-07-22T12:01:00Z' }] } }) }
      }
      return { ok: true, json: async () => ({ data: { unreadCount: 0 } }) }
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/team-inbox')
    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: 'Compatible response' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Load older messages' })).not.toBeInTheDocument()
  })

  it('keeps coordination controls read-only for viewers', async () => {
    const fetchMock = vi.fn(async (url) => {
      const path = new URL(String(url), 'http://localhost').pathname
      if (path.endsWith('/auth/me')) {
        return { ok: true, json: async () => ({ data: {
          user: { id: 2, email: 'viewer@acme.test', firstName: 'Read', lastName: 'Only' },
          organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
          membership: { role: 'viewer' }
        } }) }
      }
      if (path.endsWith('/api/shared-inbox/email-messages')) {
        return { ok: true, json: async () => ({ data: { messages: [{ id: 13, direction: 'inbound', visibility: 'shared', fromEmail: 'lead@example.test', subject: 'Read-only question', sharedInboxStatus: 'open', sharedInboxUpdatedAt: '2026-05-03T12:01:00Z', createdAt: '2026-05-03T12:01:00Z' }], meta: { limit: 50, hasMore: false, nextCursor: '' } } }) }
      }
      return { ok: true, json: async () => ({ data: { unreadCount: 0 } }) }
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/team-inbox')
    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /read-only question/i })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /assign to me/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^close$/i })).not.toBeInTheDocument()
  })
})
