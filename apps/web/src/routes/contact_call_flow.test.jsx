import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('contact call flow', () => {
  it('starts a call from contact detail and logs the outcome', async () => {
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
      if (path.endsWith('/api/calls/start') && method === 'POST') {
        return jsonResponse({ data: { call: { id: 44, entityType: 'contact', entityId: 7, direction: 'outbound', phoneNumber: '+15551234567', status: 'initiated', createdByUserName: 'Demo Owner', startedAt: '2026-06-19T19:00:00Z', createdAt: '2026-06-19T19:00:00Z', updatedAt: '2026-06-19T19:00:00Z' }, dialUrl: 'tel:+15551234567' } }, { status: 201 })
      }
      if (path.endsWith('/api/calls/44/complete') && method === 'PATCH') {
        return jsonResponse({ data: { call: { id: 44, entityType: 'contact', entityId: 7, direction: 'outbound', phoneNumber: '+15551234567', status: 'completed', disposition: 'Connected', notes: 'Asked for a quote', createdByUserName: 'Demo Owner', startedAt: '2026-06-19T19:00:00Z', completedAt: '2026-06-19T19:05:00Z', createdAt: '2026-06-19T19:00:00Z', updatedAt: '2026-06-19T19:05:00Z' } } })
      }
      return jsonResponse({ data: { tasks: [], deals: [], users: [], unreadCount: 0 } })
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/contacts/7')

    render(<AppRouter />)

    const startButton = await screen.findByRole('button', { name: /^start call$/i })
    fireEvent.click(startButton)

    await waitFor(() => {
      const startCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/calls/start'))
      expect(startCall).toBeTruthy()
      expect(JSON.parse(startCall[1].body)).toEqual({ entityType: 'contact', entityId: 7, phoneNumber: '+15551234567' })
    })
    expect(await screen.findByRole('link', { name: /open dialer/i })).toHaveAttribute('href', 'tel:+15551234567')

    fireEvent.change(screen.getByLabelText(/disposition/i), { target: { value: 'Connected' } })
    fireEvent.change(screen.getByLabelText(/call notes/i), { target: { value: 'Asked for a quote' } })
    fireEvent.click(screen.getByRole('button', { name: /log call outcome/i }))

    await waitFor(() => {
      const completeCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/calls/44/complete'))
      expect(completeCall).toBeTruthy()
      expect(JSON.parse(completeCall[1].body)).toEqual({ status: 'completed', disposition: 'Connected', notes: 'Asked for a quote' })
    })
    expect(await screen.findByText(/call outcome logged/i)).toBeInTheDocument()
    expect(screen.getByText(/connected/i)).toBeInTheDocument()
  })

  it('logs an inbound call from contact detail', async () => {
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
      if (path.endsWith('/api/calls/log') && method === 'POST') {
        return jsonResponse({ data: { call: { id: 45, entityType: 'contact', entityId: 7, direction: 'inbound', phoneNumber: '+15551234567', status: 'completed', disposition: 'Voicemail', notes: 'Asked for callback', createdByUserName: 'Demo Owner', startedAt: '2026-06-19T20:00:00Z', completedAt: '2026-06-19T20:00:00Z', createdAt: '2026-06-19T20:00:00Z', updatedAt: '2026-06-19T20:00:00Z' } } }, { status: 201 })
      }
      return jsonResponse({ data: { tasks: [], deals: [], users: [], unreadCount: 0 } })
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/contacts/7')

    render(<AppRouter />)

    const inboundButton = await screen.findByRole('button', { name: /add inbound call/i })
    fireEvent.click(inboundButton)
    fireEvent.change(screen.getByLabelText(/inbound disposition/i), { target: { value: 'Voicemail' } })
    fireEvent.change(screen.getByLabelText(/inbound notes/i), { target: { value: 'Asked for callback' } })
    fireEvent.click(screen.getByRole('button', { name: /save inbound call/i }))

    await waitFor(() => {
      const logCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/calls/log'))
      expect(logCall).toBeTruthy()
      expect(JSON.parse(logCall[1].body)).toEqual({ entityType: 'contact', entityId: 7, direction: 'inbound', phoneNumber: '+15551234567', status: 'completed', disposition: 'Voicemail', notes: 'Asked for callback' })
    })
    expect(await screen.findByText(/inbound call logged/i)).toBeInTheDocument()
    expect(screen.getByText(/voicemail/i)).toBeInTheDocument()
    expect(screen.getByText(/asked for callback/i)).toBeInTheDocument()
  })
})
