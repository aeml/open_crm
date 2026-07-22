import { useEffect, useRef, useState } from 'react'
import { isAbortError } from '../lib/api'
import { listActivities } from '../lib/activities'
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

const emptyActivityMeta = { limit: 50, hasMore: false, nextCursor: '' }

export function useTaskDetail({ isListLoading, routeTaskId, setError, setTasks }) {
  const [selectedTaskId, setSelectedTaskId] = useState(null)
  const [detail, setDetail] = useState(null)
  const [detailCache, setDetailCache] = useState({})
  const [form, setForm] = useState(emptyTaskForm)
  const [isDetailLoading, setIsDetailLoading] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const [isLoadingOlderActivities, setIsLoadingOlderActivities] = useState(false)
  const visitRef = useRef(null)

  function sync(task, activities, activityMeta = emptyActivityMeta) {
    visitRef.current = { taskId: task.id, pending: false }
    setDetailCache((current) => ({ ...current, [task.id]: { task, activities, activityMeta, loaded: true } }))
    setDetail({ task, activities, activityMeta, loaded: true })
    setSelectedTaskId(task.id)
    setForm(taskFormValues(task))
    setIsLoadingOlderActivities(false)
  }

  function clear() {
    visitRef.current = null
    setSelectedTaskId(null)
    setDetail(null)
    setForm(emptyTaskForm)
  }

  function open(task) {
    const cached = detailCache[task.id]
    if (cached) {
      sync(cached.task, cached.activities || [], cached.activityMeta)
      return
    }
    visitRef.current = { taskId: task.id, pending: false }
    setSelectedTaskId(task.id)
    setDetail({ task, activities: [], activityMeta: emptyActivityMeta, loaded: false })
    setForm(taskFormValues(task))
  }

  function applyExternalUpdate(task, activities, activityMeta = emptyActivityMeta) {
    const nextDetail = { task, activities, activityMeta, loaded: true }
    setDetailCache((current) => ({ ...current, [task.id]: nextDetail }))
    setDetail((current) => {
      if (current?.task?.id !== task.id) return current
      setForm(taskFormValues(task))
      return nextDetail
    })
  }

  async function loadOlderActivities() {
    const current = detail
    const visit = visitRef.current
    if (!current?.activityMeta?.hasMore || !current.activityMeta.nextCursor || visit?.taskId !== current.task.id || isLoadingOlderActivities) return
    setIsLoadingOlderActivities(true)
    try {
      const page = await listActivities('task', current.task.id, { cursor: current.activityMeta.nextCursor, limit: current.activityMeta.limit || 50 })
      if (visitRef.current !== visit) return
      const ids = new Set(current.activities.map((entry) => entry.id))
      const activities = [...current.activities, ...(page.activities || []).filter((entry) => !ids.has(entry.id))]
      const nextDetail = { ...current, activities, activityMeta: page.meta || emptyActivityMeta }
      setDetail(nextDetail)
      setDetailCache((cache) => ({ ...cache, [current.task.id]: nextDetail }))
      setError('')
    } catch (loadError) {
      if (visitRef.current === visit && !isAbortError(loadError)) setError(loadError.message || 'Unable to load older task activity.')
    } finally {
      if (visitRef.current === visit) setIsLoadingOlderActivities(false)
    }
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

      if (selectedTaskId === routeTaskId && detail?.task?.id === routeTaskId && detail.loaded !== false) return

      const cached = detailCache[routeTaskId]
      if (cached) {
        sync(cached.task, cached.activities || [], cached.activityMeta)
        setError('')
        return
      }

      try {
        setIsDetailLoading(true)
        const data = await getTask(routeTaskId, { signal: controller.signal })
        if (controller.signal.aborted) return
        setTasks((current) => [data.task, ...current.filter((entry) => entry.id !== routeTaskId)])
        sync(data.task, data.activities || [], data.activityMeta)
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
  }, [detail, detailCache, isListLoading, routeTaskId, selectedTaskId])

  return {
    applyExternalUpdate,
    clear,
    detail,
    form,
    isDetailLoading,
    isLoadingOlderActivities,
    isSaving,
    open,
    loadOlderActivities,
    removeCached,
    selectedTaskId,
    setForm,
    setIsSaving,
    sync,
    visitRef
  }
}
