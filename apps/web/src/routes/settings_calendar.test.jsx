import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

function jsonResponse(payload, init = {}) {
  return {
    ok: init.ok ?? true,
    status: init.status ?? 200,
    json: async () => payload
  }
}

describe('settings calendar route', () => {
  it('creates a booking link and saves weekly availability', async () => {
    const now = '2026-06-20T14:00:00Z'
    const fetchMock = vi.fn(async (url, options = {}) => {
      const path = new URL(String(url), 'http://localhost').pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) {
        return jsonResponse({ data: {
          user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
          organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
          membership: { role: 'owner' }
        } })
      }
      if (path.endsWith('/api/calendar-booking-links') && method === 'POST') {
        return jsonResponse({ data: { link: {
          id: 44,
          slug: 'discovery-call',
          name: 'Discovery call',
          description: 'Intro meeting',
          durationMinutes: 30,
          bufferMinutes: 10,
          timezone: 'UTC',
          assignmentMode: 'round_robin',
          isActive: true,
          createdByUserId: 1,
          members: [
            { userId: 1, firstName: 'Demo', lastName: 'Owner', email: 'owner@acme.test', role: 'owner', position: 1 },
            { userId: 2, firstName: 'Morgan', lastName: 'Member', email: 'morgan@acme.test', role: 'member', position: 2 }
          ],
          createdAt: now,
          updatedAt: now
        } } }, { status: 201 })
      }
      if (path.endsWith('/api/calendar-booking-links')) {
        return jsonResponse({ data: { links: [] } })
      }
      if (path.endsWith('/api/me/calendar-availability') && method === 'PUT') {
        return jsonResponse({ data: { blocks: [{ id: 3, dayOfWeek: 1, startMinute: 540, endMinute: 1020, timezone: 'UTC', createdAt: now, updatedAt: now }] } })
      }
      if (path.endsWith('/api/me/calendar-availability')) {
        return jsonResponse({ data: { blocks: [] } })
      }
      if (path.endsWith('/api/users')) {
        return jsonResponse({ data: { users: [
          { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' },
          { id: 2, email: 'morgan@acme.test', firstName: 'Morgan', lastName: 'Member', role: 'member' }
        ] } })
      }
      return jsonResponse({ data: { unreadCount: 0 } })
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/calendar')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /booking links/i })).toBeInTheDocument()
    expect(await screen.findByText(/morgan member/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/booking name/i), { target: { value: 'Discovery call' } })
    fireEvent.change(screen.getByLabelText(/booking slug/i), { target: { value: 'discovery-call' } })
    fireEvent.change(screen.getByLabelText(/description/i), { target: { value: 'Intro meeting' } })
    fireEvent.change(screen.getByLabelText(/buffer minutes/i), { target: { value: '10' } })
    fireEvent.change(screen.getByLabelText(/^timezone$/i), { target: { value: 'UTC' } })
    fireEvent.change(screen.getByLabelText(/assignment mode/i), { target: { value: 'round_robin' } })
    fireEvent.click(screen.getByLabelText(/morgan member/i))
    fireEvent.click(screen.getByRole('button', { name: /create booking link/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/calendar-booking-links') && call[1]?.method === 'POST')
      expect(createCall).toBeTruthy()
      expect(JSON.parse(createCall[1].body)).toMatchObject({
        name: 'Discovery call',
        slug: 'discovery-call',
        description: 'Intro meeting',
        bufferMinutes: 10,
        timezone: 'UTC',
        assignmentMode: 'round_robin',
        memberUserIds: [1, 2]
      })
    })
    expect(await screen.findByText(/booking link created/i)).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /discovery call/i })).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/availability timezone/i), { target: { value: 'UTC' } })
    fireEvent.click(screen.getByRole('button', { name: /add availability block/i }))
    fireEvent.click(screen.getByRole('button', { name: /save availability/i }))

    await waitFor(() => {
      const saveAvailabilityCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/me/calendar-availability') && call[1]?.method === 'PUT')
      expect(saveAvailabilityCall).toBeTruthy()
      expect(JSON.parse(saveAvailabilityCall[1].body)).toEqual({ blocks: [{ dayOfWeek: 1, startMinute: 540, endMinute: 1020, timezone: 'UTC' }] })
    })
    expect(await screen.findByText(/availability updated/i)).toBeInTheDocument()
  })

  it('disables only new catalog entries at capacity and keeps editing available', async () => {
    const link = {
      id: 9, slug: 'discovery', name: 'Discovery', description: '', durationMinutes: 30, bufferMinutes: 0,
      timezone: 'UTC', assignmentMode: 'owner', isActive: true, createdByUserId: 1,
      members: [{ userId: 1, firstName: 'Demo', lastName: 'Owner', email: 'owner@acme.test', role: 'owner', position: 1 }]
    }
    const fetchMock = vi.fn(async (url) => {
      const path = new URL(String(url), 'http://localhost').pathname
      if (path.endsWith('/auth/me')) {
        return jsonResponse({ data: {
          user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
          organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
          membership: { role: 'owner' }
        } })
      }
      if (path.endsWith('/api/calendar-booking-links')) {
        return jsonResponse({ data: { links: [link], capacity: { maxLinks: 1, maxMembers: 1 } } })
      }
      if (path.endsWith('/api/me/calendar-availability')) {
        return jsonResponse({ data: { blocks: [{ id: 2, dayOfWeek: 1, startMinute: 540, endMinute: 1020, timezone: 'UTC' }], capacity: { maxBlocks: 1 } } })
      }
      if (path.endsWith('/api/users')) {
        return jsonResponse({ data: { users: [
          { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' },
          { id: 2, email: 'member@acme.test', firstName: 'Morgan', lastName: 'Member', role: 'member' }
        ] } })
      }
      return jsonResponse({ data: { unreadCount: 0 } })
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/calendar')

    render(<AppRouter />)

    expect(await screen.findByText('1 of 1 booking links')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /new booking link/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /create booking link/i })).toBeDisabled()
    expect(screen.getByText(/edit an existing link to recover space/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /add availability block/i })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: /^edit$/i }))
    expect(await screen.findByRole('heading', { name: /edit booking link/i })).toBeInTheDocument()
    expect(screen.getByLabelText(/morgan member/i)).toBeDisabled()
    expect(screen.getByRole('button', { name: /save booking link/i })).toBeEnabled()
  })
})
