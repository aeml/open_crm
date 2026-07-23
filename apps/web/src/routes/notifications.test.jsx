import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { NotificationsRoute } from './notifications'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('notifications collaboration route', () => {
  it('links useful notifications and filters a focused activity digest', async () => {
    const fetchMock = vi.fn(async (input, options = {}) => {
      const url = String(input)
      if (url.endsWith('/api/notifications/read-all')) {
        return { ok: true, status: 204, json: async () => ({}) }
      }
      if (url.match(/\/api\/notifications\/(5|7)\/read/)) {
        return { ok: true, status: 204, json: async () => ({}) }
      }
      if (url.includes('/api/notifications')) {
        return {
          ok: true,
          json: async () => ({ data: { notifications: [
            { id: 5, eventType: 'record.mentioned', entityType: 'contact', entityId: 17, summary: 'Casey mentioned you on Ada Lovelace', readAt: null, createdAt: '2026-07-19T08:00:00Z' },
            { id: 6, eventType: 'task.assigned', entityType: 'task', entityId: 22, summary: 'You were assigned a task', readAt: '2026-07-19T08:10:00Z', createdAt: '2026-07-19T08:00:00Z' },
            { id: 7, eventType: 'deal.assigned', entityType: 'deal', entityId: 23, summary: 'You were assigned a deal: Renewal', readAt: null, createdAt: '2026-07-19T08:20:00Z' }
          ], unreadCount: 2, window: { limit: 50 } } })
        }
      }
      if (url.includes('/api/users')) {
        return { ok: true, json: async () => ({ data: { users: [{ id: 2, firstName: 'Casey', lastName: 'Example', email: 'casey@example.test' }] } }) }
      }
      if (url.includes('/api/collaboration/activity-digest')) {
        return {
          ok: true,
          json: async () => ({ data: {
            scope: url.includes('scope=team') ? 'team' : 'following',
            days: 7,
            totalActivities: 1,
            activeRecords: 1,
            activePeople: 1,
            activities: [{ id: 71, action: 'note.created', summary: 'Note added', entityType: 'contact', entityId: 17, entityLabel: 'Ada Lovelace', actorName: 'Casey Example', createdAt: '2026-07-19T08:00:00Z' }]
          } })
        }
      }
      throw new Error(`Unexpected request: ${url} ${options.method || 'GET'}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<MemoryRouter><NotificationsRoute /></MemoryRouter>)

    expect(await screen.findByText(/casey mentioned you on ada lovelace/i)).toBeInTheDocument()
    expect(screen.getByText('2 unread notifications.')).toBeInTheDocument()
    expect(await screen.findByText('Ada Lovelace: Note added')).toBeInTheDocument()
    expect(screen.getByRole('list', { name: /activity digest summary/i })).toHaveTextContent('Activity1')

    fireEvent.change(screen.getByLabelText('Show'), { target: { value: 'assignments' } })
    expect(screen.getByText(/you were assigned a deal: renewal/i)).toBeInTheDocument()
    expect(screen.getByText(/you were assigned a task/i)).toBeInTheDocument()
    expect(screen.queryByText(/casey mentioned you/i)).not.toBeInTheDocument()
    fireEvent.click(screen.getAllByRole('button', { name: 'Open record' })[1])
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining('/api/notifications/7/read'), expect.objectContaining({ method: 'PATCH' }))
    })

    fireEvent.change(screen.getByLabelText('Show'), { target: { value: 'mentions' } })
    expect(screen.getByText(/casey mentioned you/i)).toBeInTheDocument()
    expect(screen.queryByText(/you were assigned a task/i)).not.toBeInTheDocument()

    fireEvent.click(screen.getAllByRole('button', { name: 'Open record' })[0])
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining('/api/notifications/5/read'), expect.objectContaining({ method: 'PATCH' }))
    })

    fireEvent.change(screen.getByLabelText('Records'), { target: { value: 'team' } })
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/activity-digest\?.*scope=team/), expect.any(Object))
    })
    fireEvent.change(screen.getByLabelText('Teammate'), { target: { value: '2' } })
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/activity-digest\?.*actorUserId=2/), expect.any(Object))
    })
  })

  it('discloses the fixed latest window and marks the complete unread backlog', async () => {
    const notifications = Array.from({ length: 50 }, (_, index) => ({
      id: index + 1,
      eventType: 'task.assigned',
      entityType: 'task',
      entityId: index + 100,
      summary: `Assigned task ${index + 1}`,
      readAt: null,
      createdAt: '2026-07-19T08:00:00Z'
    }))
    const fetchMock = vi.fn(async (input) => {
      const url = String(input)
      if (url.endsWith('/api/notifications/read-all')) return { ok: true, status: 204, json: async () => ({}) }
      if (url.endsWith('/api/notifications')) return { ok: true, json: async () => ({ data: { notifications, unreadCount: 51, window: { limit: 50 } } }) }
      if (url.includes('/api/users')) return { ok: true, json: async () => ({ data: { users: [] } }) }
      if (url.includes('/api/collaboration/activity-digest')) return { ok: true, json: async () => ({ data: { totalActivities: 0, activeRecords: 0, activePeople: 0, activities: [] } }) }
      throw new Error(`Unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<MemoryRouter><NotificationsRoute /></MemoryRouter>)

    expect(await screen.findByText(/51 unread notifications/i)).toHaveTextContent('Showing the latest 50 retained notifications.')
    expect(screen.getByText(/1 unread item is older than this window/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Mark all read' }))
    await waitFor(() => expect(screen.getByText(/0 unread notifications/i)).toBeInTheDocument())
    expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining('/api/notifications/read-all'), expect.objectContaining({ method: 'POST' }))
  })

  it('keeps a failed acknowledgement visible and offers a retryable error', async () => {
    const fetchMock = vi.fn(async (input) => {
      const url = String(input)
      if (url.endsWith('/api/notifications/8/read')) return { ok: false, status: 504, json: async () => ({ error: { message: 'Notification processing exceeded the five-second query limit; retry safely' } }) }
      if (url.endsWith('/api/notifications')) return { ok: true, json: async () => ({ data: { notifications: [{ id: 8, eventType: 'task.assigned', entityType: 'task', entityId: 18, summary: 'Assigned task', readAt: null, createdAt: '2026-07-19T08:00:00Z' }], unreadCount: 1, window: { limit: 50 } } }) }
      if (url.includes('/api/users')) return { ok: true, json: async () => ({ data: { users: [] } }) }
      if (url.includes('/api/collaboration/activity-digest')) return { ok: true, json: async () => ({ data: { totalActivities: 0, activeRecords: 0, activePeople: 0, activities: [] } }) }
      throw new Error(`Unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<MemoryRouter><NotificationsRoute /></MemoryRouter>)
    fireEvent.click(await screen.findByRole('button', { name: 'Mark as read' }))

    expect(await screen.findByText(/notification processing exceeded the five-second query limit/i)).toBeInTheDocument()
    expect(screen.getByText('1 unread notification.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Mark as read' })).toBeEnabled()
  })
})
