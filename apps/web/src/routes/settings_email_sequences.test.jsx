import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

function sessionResponse() {
  return {
    ok: true,
    json: async () => ({
      data: {
        user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
        organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
        membership: { role: 'owner' }
      }
    })
  }
}

function jsonResponse(data) {
  return { ok: true, json: async () => ({ data }) }
}

function deferred() {
  let resolve
  const promise = new Promise((next) => { resolve = next })
  return { promise, resolve }
}

describe('settings email sequences route', () => {
  it('lists sequences and creates a new draft sequence', async () => {
    const fetchMock = vi.fn(async (url, options = {}) => {
      const path = new URL(String(url), 'http://localhost').pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) {
        return sessionResponse()
      }
      if (path.endsWith('/api/email-sequences') && method === 'POST') {
        return {
          ok: true,
          json: async () => ({
            data: {
              sequence: {
                id: 5,
                name: 'Trial nurture',
                description: 'Warm new trials',
                status: 'draft',
                steps: [{ id: 10, stepOrder: 1, delayDays: 2, subject: 'Checking in', body: 'Hi {{first_name}}' }]
              }
            }
          })
        }
      }
      if (path.endsWith('/api/email-sequences')) {
        return {
          ok: true,
          json: async () => ({
            data: {
              sequences: [
                { id: 3, name: 'Welcome cadence', description: 'First-touch follow-up', status: 'draft', revision: 1, steps: [{ id: 7, stepOrder: 1, delayDays: 0, subject: 'Welcome', body: 'Hello' }] }
              ]
            }
          })
        }
      }
      return { ok: true, json: async () => ({ data: { unreadCount: 0 } }) }
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/email-sequences')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /email sequences/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /welcome cadence/i })).toBeInTheDocument()
    expect(screen.getByText(/draft · revision 1 · approval required · steps 1/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/sequence name/i), { target: { value: 'Trial nurture' } })
    fireEvent.change(screen.getByLabelText(/description/i), { target: { value: 'Warm new trials' } })
    fireEvent.change(screen.getByLabelText(/step 1 delay days/i), { target: { value: '2' } })
    fireEvent.change(screen.getByLabelText(/step 1 subject/i), { target: { value: 'Checking in' } })
    fireEvent.change(screen.getByLabelText(/step 1 body/i), { target: { value: 'Hi {{first_name}}' } })
    fireEvent.click(screen.getByRole('button', { name: /create sequence/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/email-sequences') && call[1]?.method === 'POST'
      )
      expect(createCall).toBeTruthy()
      expect(JSON.parse(createCall[1].body)).toEqual({
        name: 'Trial nurture',
        description: 'Warm new trials',
        status: 'draft',
        steps: [{ delayDays: 2, subject: 'Checking in', body: 'Hi {{first_name}}' }]
      })
    })
    expect(await screen.findByRole('heading', { name: /trial nurture/i })).toBeInTheDocument()
  })

  it('requires an admin action to activate an exact draft revision', async () => {
    const draft = {
      id: 8,
      name: 'Approved follow-up',
      description: 'Controlled outreach',
      status: 'draft',
      revision: 3,
      outcomes: { enrolled: 12, active: 3, providerAccepted: 17, bouncedMessages: 2, complaints: 1, replied: 4, cadenceFinished: 2, cancelled: 2, suppressedExits: 1, suppressedMessages: 1, queuedMessages: 2, needsReview: 0, unclassifiedCompleted: 0 },
      steps: [{ id: 12, stepOrder: 1, delayDays: 0, subject: 'Hello', body: 'Hi' }]
    }
    const fetchMock = vi.fn(async (url, options = {}) => {
      const path = new URL(String(url), 'http://localhost').pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/email-sequences/8/approve') && method === 'POST') {
        return {
          ok: true,
          json: async () => ({ data: { sequence: { ...draft, status: 'active', approvedRevision: 3, approvedAt: '2026-07-20T12:00:00Z' } } })
        }
      }
      if (path.endsWith('/api/email-sequences')) {
        return { ok: true, json: async () => ({ data: { sequences: [draft] } }) }
      }
      return { ok: true, json: async () => ({ data: { unreadCount: 0 } }) }
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/email-sequences')
    render(<AppRouter />)

    expect(await screen.findByText(/draft · revision 3 · approval required/i)).toBeInTheDocument()
    expect(screen.getByText(/12 enrolled · 17 accepted · 2 bounced · 1 complaints · 4 replied · 2 finished/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /approve & activate/i }))

    await waitFor(() => {
      expect(fetchMock.mock.calls.some((call) => String(call[0]).endsWith('/api/email-sequences/8/approve') && call[1]?.method === 'POST')).toBe(true)
    })
    expect(await screen.findByText(/active · revision 3/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /pause sending/i })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^edit$/i })).not.toBeInTheDocument()
  })

  it('drills into bounded enrollment outcomes and deduplicates continuation rows', async () => {
    const sequence = {
      id: 8,
      name: 'Outcome cadence',
      description: 'Visible execution history',
      status: 'active',
      revision: 2,
      approvedRevision: 2,
      approvedAt: '2026-07-22T10:00:00Z',
      outcomes: { enrolled: 51, active: 0, providerAccepted: 52, bouncedMessages: 1, complaints: 0, replied: 0, cadenceFinished: 49, suppressedExits: 0, needsReview: 1 },
      steps: [{ id: 12, stepOrder: 1, delayDays: 0, subject: 'Hello', body: 'Hi' }]
    }
    const fetchMock = vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      if (requestURL.pathname.endsWith('/auth/me')) return sessionResponse()
      if (requestURL.pathname.endsWith('/api/email-sequence-enrollments')) {
        expect(requestURL.searchParams.get('sequenceId')).toBe('8')
        expect(requestURL.searchParams.get('limit')).toBe('50')
        if (requestURL.searchParams.get('cursor') === 'older-page') {
          return jsonResponse({
            enrollments: [
              { id: 50, sequenceId: 8, contactId: 70, contactName: 'Review Buyer', contactEmail: 'review@example.test', status: 'completed', completionReason: 'finished', currentStepOrder: 1, providerAccepted: 1, needsReview: 1, createdAt: '2026-07-22T09:00:00Z', completedAt: '2026-07-22T09:01:00Z' },
              { id: 49, sequenceId: 8, contactId: 69, contactName: 'Bounced Buyer', contactEmail: 'bounce@example.test', status: 'completed', completionReason: 'suppressed', currentStepOrder: 1, providerAccepted: 1, bouncedMessages: 1, createdAt: '2026-07-21T09:00:00Z', completedAt: '2026-07-21T09:01:00Z' }
            ],
            meta: { limit: 50, hasMore: false, nextCursor: '' }
          })
        }
        return jsonResponse({
          enrollments: [
            { id: 50, sequenceId: 8, contactId: 70, contactName: 'Review Buyer', contactEmail: 'review@example.test', status: 'completed', completionReason: 'finished', currentStepOrder: 1, providerAccepted: 1, needsReview: 1, createdAt: '2026-07-22T09:00:00Z', completedAt: '2026-07-22T09:01:00Z', enrolledByName: 'Sequence Operator' }
          ],
          meta: { limit: 50, hasMore: true, nextCursor: 'older-page' }
        })
      }
      if (requestURL.pathname.endsWith('/api/email-sequences')) return jsonResponse({ sequences: [sequence] })
      return jsonResponse({ unreadCount: 0 })
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/email-sequences')
    render(<AppRouter />)

    fireEvent.click(await screen.findByRole('button', { name: 'View enrollments' }))
    expect(await screen.findByRole('link', { name: 'Review Buyer' })).toHaveAttribute('href', '/contacts/70')
    expect(screen.getByText('review@example.test')).toBeInTheDocument()
    expect(screen.getByText(/Needs review · 1 accepted · 1 review/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Review delivery' })).toHaveAttribute('href', '/settings/operations')

    fireEvent.click(screen.getByRole('button', { name: 'Load older enrollments' }))
    expect(await screen.findByRole('link', { name: 'Bounced Buyer' })).toHaveAttribute('href', '/contacts/69')
    expect(screen.getByText(/Bounced · 1 accepted · 1 bounced/)).toBeInTheDocument()
    expect(screen.getAllByRole('link', { name: 'Review Buyer' })).toHaveLength(1)
    expect(screen.queryByRole('button', { name: 'Load older enrollments' })).not.toBeInTheDocument()
  })

  it('discards an enrollment response after the drill-down closes', async () => {
    const firstHistory = deferred()
    let historyRequests = 0
    const sequence = {
      id: 8,
      name: 'Stale-safe cadence',
      status: 'paused',
      revision: 1,
      outcomes: { enrolled: 1 },
      steps: [{ id: 12, stepOrder: 1, delayDays: 0, subject: 'Hello', body: 'Hi' }]
    }
    const fetchMock = vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      if (requestURL.pathname.endsWith('/auth/me')) return sessionResponse()
      if (requestURL.pathname.endsWith('/api/email-sequence-enrollments')) {
        historyRequests += 1
        if (historyRequests === 1) return firstHistory.promise
        return jsonResponse({
          enrollments: [{ id: 2, sequenceId: 8, contactId: 72, contactName: 'Current Buyer', status: 'active', currentStepOrder: 1, createdAt: '2026-07-22T10:00:00Z' }],
          meta: { limit: 50, hasMore: false, nextCursor: '' }
        })
      }
      if (requestURL.pathname.endsWith('/api/email-sequences')) return jsonResponse({ sequences: [sequence] })
      return jsonResponse({ unreadCount: 0 })
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/email-sequences')
    render(<AppRouter />)

    fireEvent.click(await screen.findByRole('button', { name: 'View enrollments' }))
    fireEvent.click(screen.getByRole('button', { name: 'Hide enrollments' }))
    await act(async () => {
      firstHistory.resolve(jsonResponse({
        enrollments: [{ id: 1, sequenceId: 8, contactId: 71, contactName: 'Stale Buyer', status: 'active', currentStepOrder: 1, createdAt: '2026-07-22T09:00:00Z' }],
        meta: { limit: 50, hasMore: false, nextCursor: '' }
      }))
      await Promise.resolve()
    })
    expect(screen.queryByText('Stale Buyer')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'View enrollments' }))
    expect(await screen.findByRole('link', { name: 'Current Buyer' })).toBeInTheDocument()
    expect(screen.queryByText('Stale Buyer')).not.toBeInTheDocument()
  })
})
