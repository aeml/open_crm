import { useEffect, useRef, useState } from 'react'
import { isAbortError } from '../lib/api'
import { getTask } from '../lib/tasks'
import { taskFormValues } from './task_view'

const emptyTaskForm = {
  title: '',
  entityType: 'deal',
  entityId: '',
  description: '',
  status: 'open',
  dueAt: '',
  completedAt: '',
  assignedToUserId: ''
}

export function useTaskDetail({ isListLoading, routeTaskId, setError, setTasks, tasks }) {
  const [selectedTaskId, setSelectedTaskId] = useState(null)
  const [detail, setDetail] = useState(null)
  const [detailCache, setDetailCache] = useState({})
  const [form, setForm] = useState(emptyTaskForm)
  const [isDetailLoading, setIsDetailLoading] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const visitRef = useRef(null)

  function sync(task, activities) {
    visitRef.current = { taskId: task.id, pending: false }
    setDetailCache((current) => ({ ...current, [task.id]: { task, activities } }))
    setDetail({ task, activities })
    setSelectedTaskId(task.id)
    setForm(taskFormValues(task))
  }

  function clear() {
    visitRef.current = null
    setSelectedTaskId(null)
    setDetail(null)
    setForm(emptyTaskForm)
  }

  function open(task) {
    const cached = detailCache[task.id]
    sync(cached?.task || task, cached?.activities || [])
  }

  function applyExternalUpdate(task, activities) {
    const nextDetail = { task, activities }
    setDetailCache((current) => ({ ...current, [task.id]: nextDetail }))
    setDetail((current) => {
      if (current?.task?.id !== task.id) return current
      setForm(taskFormValues(task))
      return nextDetail
    })
  }
  function removeCached(taskId) {
    setDetailCache((current) => {
      const next = { ...current }
      delete next[taskId]
      return next
    })
  }

  useEffect(() => () => {
    visitRef.current = null
  }, [])

  useEffect(() => {
    const controller = new AbortController()

    async function syncRouteTask() {
      if (isListLoading) return
      if (!Number.isInteger(routeTaskId) || routeTaskId <= 0) {
        if (selectedTaskId) clear()
        return
      }

      if (selectedTaskId === routeTaskId && detail?.task?.id === routeTaskId) return

      const cached = detailCache[routeTaskId]
      if (cached) {
        sync(cached.task, cached.activities || [])
        setError('')
        return
      }

      const routeTask = tasks.find((entry) => entry.id === routeTaskId)
      if (routeTask) {
        sync(routeTask, [])
        setError('')
        return
      }

      try {
        setIsDetailLoading(true)
        const data = await getTask(routeTaskId, { signal: controller.signal })
        if (controller.signal.aborted) return
        setTasks((current) => [data.task, ...current.filter((entry) => entry.id !== routeTaskId)])
        sync(data.task, data.activities || [])
        setError('')
      } catch (loadError) {
        if (!isAbortError(loadError)) setError(loadError.message || 'Unable to load task.')
      } finally {
        if (!controller.signal.aborted) setIsDetailLoading(false)
      }
    }

    syncRouteTask()
    return () => {
      controller.abort()
    }
  }, [detail, detailCache, isListLoading, routeTaskId, selectedTaskId, tasks])

  return {
    applyExternalUpdate,
    clear,
    detail,
    form,
    isDetailLoading,
    isSaving,
    open,
    removeCached,
    selectedTaskId,
    setForm,
    setIsSaving,
    sync,
    visitRef
  }
}
