import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createNote, listNotes } from '../lib/notes'
import { listActivities } from '../lib/activities'
import { createTask, listTasks } from '../lib/tasks'
import { useDealSelection } from './use_deal_selection'
import { useDealWork } from './use_deal_work'

vi.mock('../lib/notes', () => ({ createNote: vi.fn(), listNotes: vi.fn() }))
vi.mock('../lib/activities', () => ({ listActivities: vi.fn() }))
vi.mock('../lib/tasks', () => ({ createTask: vi.fn(), listTasks: vi.fn() }))

function deferred() {
  let resolve
  const promise = new Promise((next) => { resolve = next })
  return { promise, resolve }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useDealWork', () => {
  it('loads older history without duplicating the cursor boundary', async () => {
    listNotes.mockResolvedValue({
      notes: [{ id: 1, entityType: 'deal', entityId: 12, body: 'Older note' }],
      meta: { limit: 50, hasMore: false, nextCursor: '' }
    })
    listActivities.mockResolvedValue({
      activities: [{ id: 1, action: 'deal.created', summary: 'Older activity' }],
      meta: { limit: 50, hasMore: false, nextCursor: '' }
    })
    const { result } = renderHook(() => {
      const selection = useDealSelection(12)
      return useDealWork({ selectedDealId: 12, selection, onError: vi.fn() })
    })
    act(() => result.current.load({
      notes: [{ id: 2, entityType: 'deal', entityId: 12, body: 'Newest note' }],
      noteMeta: { limit: 50, hasMore: true, nextCursor: 'notes-next' },
      activities: [{ id: 2, action: 'deal.updated', summary: 'Newest activity' }],
      activityMeta: { limit: 50, hasMore: true, nextCursor: 'activity-next' }
    }))

    await act(async () => {
      await Promise.all([result.current.loadOlderNotes(), result.current.loadOlderActivities()])
    })

    expect(listNotes).toHaveBeenCalledWith('deal', 12, expect.objectContaining({ cursor: 'notes-next', limit: 50 }))
    expect(listActivities).toHaveBeenCalledWith('deal', 12, expect.objectContaining({ cursor: 'activity-next', limit: 50 }))
    expect(result.current.notes.map((note) => note.id)).toEqual([2, 1])
    expect(result.current.activities.map((activity) => activity.id)).toEqual([2, 1])
    expect(result.current.noteMeta.hasMore).toBe(false)
    expect(result.current.activityMeta.hasMore).toBe(false)
  })

  it('rejects work rows returned for another deal', async () => {
    listNotes.mockResolvedValue([{ id: 1, entityType: 'deal', entityId: 11 }])
    listTasks.mockResolvedValue({ tasks: [{ id: 2, entityType: 'deal', entityId: 12 }] })
    const { result } = renderHook(() => {
      const selection = useDealSelection(12)
      return useDealWork({ selectedDealId: 12, selection, onError: vi.fn() })
    })

    await expect(result.current.fetchWork(12)).rejects.toThrow('Unable to load deal work.')
  })

  it('deduplicates creates, rejects late work, and validates the response entity', async () => {
    const noteCreate = deferred()
    createNote.mockReturnValue(noteCreate.promise)
    const onError = vi.fn()
    const { result, rerender } = renderHook(({ dealId }) => {
      const selection = useDealSelection(dealId)
      const work = useDealWork({ selectedDealId: dealId, selection, onError })
      return { selection, work }
    }, { initialProps: { dealId: 11 } })

    act(() => result.current.work.setNoteBody('Alpha note'))
    let firstCreate
    act(() => {
      firstCreate = result.current.work.handleCreateNote({ preventDefault: vi.fn() })
      result.current.work.handleCreateNote({ preventDefault: vi.fn() })
    })
    expect(createNote).toHaveBeenCalledTimes(1)
    expect(result.current.work.isCreatingNote).toBe(true)

    act(() => {
      result.current.selection.begin(12)
      rerender({ dealId: 12 })
      result.current.work.reset()
    })
    await act(async () => {
      noteCreate.resolve({ note: { id: 1, entityType: 'deal', entityId: 11, body: 'Alpha note' } })
      await firstCreate
    })
    expect(result.current.work.notes).toEqual([])
    expect(onError).not.toHaveBeenCalled()

    createTask.mockResolvedValue({ task: { id: 9, entityType: 'deal', entityId: 11 } })
    act(() => result.current.work.setTaskForm((current) => ({ ...current, title: 'Beta follow-up' })))
    await act(async () => {
      await result.current.work.handleCreateTask({ preventDefault: vi.fn() })
    })
    expect(result.current.work.tasks).toEqual([])
    expect(onError).toHaveBeenCalledWith('Unable to create task.')
    await waitFor(() => expect(result.current.work.isCreatingTask).toBe(false))
  })
})
