import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { updateTask } from '../lib/tasks'
import { useTaskQuickActions } from './use_task_quick_actions'

vi.mock('../lib/tasks', () => ({ updateTask: vi.fn() }))

function deferred() {
  let resolve
  const promise = new Promise((next) => {
    resolve = next
  })
  return { promise, resolve }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useTaskQuickActions', () => {
  it('allows only one in-flight mutation per task', async () => {
    const update = deferred()
    updateTask.mockReturnValue(update.promise)
    const onUpdated = vi.fn()
    const onError = vi.fn()
    const task = { id: 7, title: 'Call client', status: 'open', assignedToUserId: 1 }
    const { result } = renderHook(() => useTaskQuickActions({ onUpdated, onError }))

    let updatePromise
    act(() => {
      updatePromise = result.current.handleQuickComplete(task)
      result.current.handleQuickAssign(task, '2')
    })

    expect(updateTask).toHaveBeenCalledTimes(1)
    expect(result.current.isTaskPending(7)).toBe(true)

    await act(async () => {
      update.resolve({ task: { ...task, status: 'completed' }, activities: [{ id: 9 }] })
      await updatePromise
    })

    expect(onUpdated).toHaveBeenCalledWith(task, expect.objectContaining({ task: expect.objectContaining({ id: 7, status: 'completed' }) }))
    expect(onError).toHaveBeenLastCalledWith('')
    expect(result.current.isTaskPending(7)).toBe(false)
  })

  it('rejects a mismatched task response and recovers the control', async () => {
    updateTask.mockResolvedValue({ task: { id: 8 } })
    const onUpdated = vi.fn()
    const onError = vi.fn()
    const { result } = renderHook(() => useTaskQuickActions({ onUpdated, onError }))

    await act(async () => {
      await result.current.handleQuickReopen({ id: 7, title: 'Call client', status: 'completed' })
    })

    expect(onUpdated).not.toHaveBeenCalled()
    expect(onError).toHaveBeenCalledWith('Unable to update task.')
    await waitFor(() => expect(result.current.isTaskPending(7)).toBe(false))
  })
})
