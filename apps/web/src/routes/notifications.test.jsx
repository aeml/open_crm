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
      if (url.includes('/api/notifications/5/read')) {
        return { ok: true, status: 204, json: async () => ({}) }
      }
      if (url.includes('/api/notifications')) {
        return {
          ok: true,
          json: async () => ({ data: { notifications: [
            { id: 5, eventType: 'record.mentioned', entityType: 'contact', entityId: 17, summary: 'Casey mentioned you on Ada Lovelace', readAt: null, createdAt: '2026-07-19T08:00:00Z' },
            { id: 6, eventType: 'task.assigned', entityType: 'task', entityId: 22, summary: 'You were assigned a task', readAt: '2026-07-19T08:10:00Z', createdAt: '2026-07-19T08:00:00Z' }
          ] } })
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
    expect(await screen.findByText('Ada Lovelace: Note added')).toBeInTheDocument()
    expect(screen.getByRole('list', { name: /activity digest summary/i })).toHaveTextContent('Activity1')

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
})
