import { useEffect, useRef, useState } from 'react'
import { updateTask } from '../lib/tasks'

// Quick actions may finish after the user opens another task. Keep one
// mutation in flight per task and return the result without changing selection;
// the route decides whether the matching detail is still active.
export function useTaskQuickActions({ onUpdated, onError }) {
  const pendingRef = useRef(new Set())
  const mountedRef = useRef(true)
  const [pendingTaskIds, setPendingTaskIds] = useState([])

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  function publishPending() {
    if (mountedRef.current) {
      setPendingTaskIds([...pendingRef.current])
    }
  }

  async function runQuickAction(task, input) {
    const taskId = task?.id
    if (!taskId || pendingRef.current.has(taskId)) {
      return
    }

    pendingRef.current.add(taskId)
    publishPending()
    try {
      const data = await updateTask(taskId, input)
      if (!mountedRef.current) {
        return
      }
      if (!data?.task?.id || data.task.id !== taskId) {
        throw new Error('Unable to update task.')
      }
      onUpdated(task, data)
      onError('')
    } catch (saveError) {
      if (mountedRef.current) {
        onError(saveError.message || 'Unable to update task.')
      }
    } finally {
      pendingRef.current.delete(taskId)
      publishPending()
    }
  }

  function handleQuickStatus(task, nextStatus) {
    return runQuickAction(task, {
      title: task.title,
      description: task.description || '',
      status: nextStatus,
      dueAt: task.dueAt || '',
      completedAt: nextStatus === 'completed' ? new Date().toISOString() : '',
      assignedToUserId: task.assignedToUserId || 0
    })
  }

  function handleQuickComplete(task) {
    return handleQuickStatus(task, 'completed')
  }

  function handleQuickReopen(task) {
    return handleQuickStatus(task, 'open')
  }

  function handleQuickAssign(task, nextAssignedToUserId) {
    return runQuickAction(task, {
      title: task.title,
      description: task.description || '',
      status: task.status,
      dueAt: task.dueAt || '',
      completedAt: task.completedAt || '',
      assignedToUserId: Number.parseInt(nextAssignedToUserId, 10) || 0
    })
  }

  function isTaskPending(taskId) {
    return pendingTaskIds.includes(taskId)
  }

  return { handleQuickAssign, handleQuickComplete, handleQuickReopen, isTaskPending }
}
