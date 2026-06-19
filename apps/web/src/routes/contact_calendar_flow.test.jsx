import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('contact calendar flow', () => {
  it('schedules and cancels a meeting from contact detail', async () => {
    const startAt = new Date('2026-06-20T14:00').toISOString()
    const endAt = new Date('2026-06-20T14:30').toISOString()
    const jsonResponse = (payload, init = {}) => ({
      ok: init.ok ?? true,
      status: init.status ?? 200,
      json: async () => payload
    })

    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const method = options.method || 'GET'
      const path = requestURL.pathname

      if (path.endsWith('/auth/me')) {
        return jsonResponse({ data: {
          user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
          organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' },
          membership: { role: 'owner' }
        } })
      }
      if (path.endsWith('/api/contacts/7')) {
        return jsonResponse({ data: {
          contact: { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '+15551234567', status: 'lead' },
          notes: [],
          activities: []
        } })
      }
      if (path.endsWith('/api/contacts') && method === 'GET') {
        return jsonResponse({ data: { contacts: [{ id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '+15551234567', status: 'lead' }], meta: { page: 1, pageSize: 20, total: 1 } } })
      }
      if (path.endsWith('/api/calendar-events') && method === 'POST') {
        return jsonResponse({ data: { event: { id: 60, entityType: 'contact', entityId: 7, title: 'Intro meeting', description: 'Discuss timeline', location: 'Zoom', startAt, endAt, timezone: 'UTC', status: 'scheduled', visibility: 'shared', createdByUserName: 'Demo Owner', createdAt: startAt, updatedAt: startAt } } }, { status: 201 })
      }
      if (path.endsWith('/api/calendar-events/60/cancel') && method === 'PATCH') {
        return jsonResponse({ data: { event: { id: 60, entityType: 'contact', entityId: 7, title: 'Intro meeting', description: 'Discuss timeline', location: 'Zoom', startAt, endAt, timezone: 'UTC', status: 'cancelled', visibility: 'shared', createdByUserName: 'Demo Owner', createdAt: startAt, updatedAt: endAt } } })
      }
      return jsonResponse({ data: { tasks: [], deals: [], users: [], unreadCount: 0, events: [], messages: [] } })
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/contacts/7')

    render(<AppRouter />)

    await screen.findByRole('heading', { name: /meetings/i })
    fireEvent.change(screen.getByLabelText(/meeting title/i), { target: { value: 'Intro meeting' } })
    fireEvent.change(screen.getByLabelText(/meeting start/i), { target: { value: '2026-06-20T14:00' } })
    fireEvent.change(screen.getByLabelText(/meeting end/i), { target: { value: '2026-06-20T14:30' } })
    fireEvent.change(screen.getByLabelText(/meeting location/i), { target: { value: 'Zoom' } })
    fireEvent.change(screen.getByLabelText(/meeting notes/i), { target: { value: 'Discuss timeline' } })
    fireEvent.click(screen.getByRole('button', { name: /schedule meeting/i }))

    await waitFor(() => {
      const scheduleCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/calendar-events') && call[1]?.method === 'POST')
      expect(scheduleCall).toBeTruthy()
      const body = JSON.parse(scheduleCall[1].body)
      expect(body).toMatchObject({ entityType: 'contact', entityId: 7, title: 'Intro meeting', description: 'Discuss timeline', location: 'Zoom', startAt, endAt, visibility: 'shared' })
      expect(body.timezone).toEqual(expect.any(String))
    })
    expect(await screen.findByText(/meeting scheduled/i)).toBeInTheDocument()
    expect(screen.getByText(/intro meeting/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /cancel meeting/i }))

    await waitFor(() => {
      const cancelCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/calendar-events/60/cancel'))
      expect(cancelCall).toBeTruthy()
    })
    expect(await screen.findByText(/meeting cancelled/i)).toBeInTheDocument()
    expect(screen.getByText(/cancelled .* Zoom .* shared/i)).toBeInTheDocument()
  })
})
