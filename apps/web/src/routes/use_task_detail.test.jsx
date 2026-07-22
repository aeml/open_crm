import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { listActivities } from '../lib/activities'
import { getTask } from '../lib/tasks'
import { useTaskDetail } from './use_task_detail'

vi.mock('../lib/activities', () => ({ listActivities: vi.fn() }))
vi.mock('../lib/tasks', () => ({ getTask: vi.fn() }))

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useTaskDetail activity history', () => {
  it('loads and deduplicates an older cursor page', async () => {
    getTask.mockResolvedValue({
      task: { id: 9, entityType: 'deal', entityId: 7, title: 'Follow up', status: 'open' },
      activities: [{ id: 2, action: 'task.updated', summary: 'Newest' }],
      activityMeta: { limit: 50, hasMore: true, nextCursor: 'task-next' }
    })
    listActivities.mockResolvedValue({
      activities: [
        { id: 2, action: 'task.updated', summary: 'Duplicate boundary' },
        { id: 1, action: 'task.created', summary: 'Older' }
      ],
      meta: { limit: 50, hasMore: false, nextCursor: '' }
    })
    const setError = vi.fn()
    const { result } = renderHook(() => useTaskDetail({
      isListLoading: false,
      routeTaskId: 9,
      setError,
      setTasks: vi.fn()
    }))

    await waitFor(() => expect(result.current.detail?.task?.id).toBe(9))
    await act(async () => result.current.loadOlderActivities())

    expect(listActivities).toHaveBeenCalledWith('task', 9, expect.objectContaining({ cursor: 'task-next', limit: 50 }))
    expect(result.current.detail.activities.map((activity) => activity.id)).toEqual([2, 1])
    expect(result.current.detail.activityMeta.hasMore).toBe(false)
    expect(setError).toHaveBeenLastCalledWith('')
  })
})
