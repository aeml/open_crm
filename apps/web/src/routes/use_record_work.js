import { useState } from 'react'
import { isAbortError } from '../lib/api'
import { listActivities } from '../lib/activities'
import { createNote, listNotes } from '../lib/notes'
import { createTask, listTasks } from '../lib/tasks'

function emptyTaskForm(assignedToUserId = '') {
  return { title: '', description: '', dueAt: '', assignedToUserId }
}

const emptyHistoryMeta = { limit: 50, hasMore: false, nextCursor: '' }

function mergeUniqueById(current, next) {
  const ids = new Set(current.map((entry) => entry.id))
  return [...current, ...next.filter((entry) => !ids.has(entry.id))]
}

export function requireRecordWork({ notes = [], tasks = [] }, entityType, entityId) {
  const wrongNote = notes.some((note) => note.entityType !== entityType || note.entityId !== entityId)
  const wrongTask = tasks.some((task) => task.entityType !== entityType || task.entityId !== entityId)
  if (wrongNote || wrongTask) throw new Error(`Unable to load ${entityType} work.`)
  return { notes, tasks }
}

// Notes, tasks, and activity belong to one record visit. Mutations that finish
// after navigation may have succeeded, but cannot appear on the active record.
export function useRecordWork({ defaultAssignedToUserId = '', entityType, selectedEntityId, selection, onError }) {
  const [notes, setNotes] = useState([])
  const [tasks, setTasks] = useState([])
  const [noteBody, setNoteBody] = useState('')
  const [taskForm, setTaskForm] = useState(emptyTaskForm(defaultAssignedToUserId))
  const [activities, setActivities] = useState([])
  const [activityMeta, setActivityMeta] = useState(emptyHistoryMeta)
  const [noteMeta, setNoteMeta] = useState(emptyHistoryMeta)
  const [isCreatingNote, setIsCreatingNote] = useState(false)
  const [isCreatingTask, setIsCreatingTask] = useState(false)
  const [isLoadingOlderActivities, setIsLoadingOlderActivities] = useState(false)
  const [isLoadingOlderNotes, setIsLoadingOlderNotes] = useState(false)

  async function fetchTasks(entityId, { signal } = {}) {
    const taskData = await listTasks({ status: 'open', entityType, entityId }, { signal })
    return requireRecordWork({ tasks: taskData.tasks || [] }, entityType, entityId).tasks
  }

  async function fetchWork(entityId, { signal } = {}) {
    const [loadedNotes, loadedTasks] = await Promise.all([listNotes(entityType, entityId, { signal }), fetchTasks(entityId, { signal })])
    const notePage = Array.isArray(loadedNotes) ? { notes: loadedNotes, meta: emptyHistoryMeta } : loadedNotes
    return {
      ...requireRecordWork({ notes: notePage.notes || [], tasks: loadedTasks }, entityType, entityId),
      noteMeta: notePage.meta || emptyHistoryMeta
    }
  }

  function load({ activities: nextActivities = [], activityMeta: nextActivityMeta = emptyHistoryMeta, notes: nextNotes = [], noteMeta: nextNoteMeta = emptyHistoryMeta, tasks: nextTasks = [] } = {}) {
    setActivities(nextActivities)
    setActivityMeta(nextActivityMeta)
    setNotes(nextNotes)
    setNoteMeta(nextNoteMeta)
    setTasks(nextTasks)
    setNoteBody('')
    setTaskForm(emptyTaskForm(defaultAssignedToUserId))
    setIsCreatingNote(false)
    setIsCreatingTask(false)
    setIsLoadingOlderActivities(false)
    setIsLoadingOlderNotes(false)
  }

  function reset() {
    load()
  }

  async function refreshTasks() {
    const operation = selection.start('task-refresh', selectedEntityId)
    if (!operation) return
    try {
      const nextTasks = await fetchTasks(operation.entityId)
      if (selection.isCurrent(operation.selection)) setTasks(nextTasks)
    } finally {
      selection.finish(operation)
    }
  }

  async function loadOlderNotes() {
    if (!noteMeta.hasMore || !noteMeta.nextCursor) return
    const operation = selection.start('notes-older', selectedEntityId)
    if (!operation) return
    setIsLoadingOlderNotes(true)
    try {
      const page = await listNotes(entityType, operation.entityId, { cursor: noteMeta.nextCursor, limit: noteMeta.limit || 50 })
      const nextNotes = requireRecordWork({ notes: page.notes || [] }, entityType, operation.entityId).notes
      if (!selection.isCurrent(operation.selection)) return
      setNotes((current) => mergeUniqueById(current, nextNotes))
      setNoteMeta(page.meta || emptyHistoryMeta)
      onError('')
    } catch (loadError) {
      if (!isAbortError(loadError) && selection.isCurrent(operation.selection)) onError(loadError.message || 'Unable to load older notes.')
    } finally {
      selection.finish(operation)
      if (selection.isCurrent(operation.selection)) setIsLoadingOlderNotes(false)
    }
  }

  async function loadOlderActivities() {
    if (!activityMeta.hasMore || !activityMeta.nextCursor) return
    const operation = selection.start('activities-older', selectedEntityId)
    if (!operation) return
    setIsLoadingOlderActivities(true)
    try {
      const page = await listActivities(entityType, operation.entityId, { cursor: activityMeta.nextCursor, limit: activityMeta.limit || 50 })
      if (!selection.isCurrent(operation.selection)) return
      setActivities((current) => mergeUniqueById(current, page.activities || []))
      setActivityMeta(page.meta || emptyHistoryMeta)
      onError('')
    } catch (loadError) {
      if (!isAbortError(loadError) && selection.isCurrent(operation.selection)) onError(loadError.message || 'Unable to load older activity.')
    } finally {
      selection.finish(operation)
      if (selection.isCurrent(operation.selection)) setIsLoadingOlderActivities(false)
    }
  }

  async function handleCreateNote(event) {
    event.preventDefault()
    const operation = selection.start('note', selectedEntityId)
    const body = noteBody.trim()
    if (!operation || !body) {
      selection.finish(operation)
      return
    }

    setIsCreatingNote(true)
    try {
      const data = await createNote({ entityType, entityId: operation.entityId, body })
      if (!data?.note?.id || data.note.entityType !== entityType || data.note.entityId !== operation.entityId) throw new Error('Unable to add note.')
      if (!selection.isCurrent(operation.selection)) return
      setNotes((current) => [data.note, ...current])
      if (data.activity) setActivities((current) => [data.activity, ...current])
      setNoteBody('')
      onError('')
    } catch (noteError) {
      if (!isAbortError(noteError) && selection.isCurrent(operation.selection)) onError(noteError.message || 'Unable to add note.')
    } finally {
      selection.finish(operation)
      if (selection.isCurrent(operation.selection)) setIsCreatingNote(false)
    }
  }

  async function handleCreateTask(event) {
    event.preventDefault()
    const operation = selection.start('task', selectedEntityId)
    const title = taskForm.title.trim()
    if (!operation || !title) {
      selection.finish(operation)
      return
    }

    setIsCreatingTask(true)
    try {
      const data = await createTask({
        entityType,
        entityId: operation.entityId,
        title,
        description: taskForm.description.trim(),
        status: 'open',
        dueAt: taskForm.dueAt ? `${taskForm.dueAt}:00Z` : '',
        assignedToUserId: Number.parseInt(taskForm.assignedToUserId, 10) || 0
      })
      if (!data?.task?.id || data.task.entityType !== entityType || data.task.entityId !== operation.entityId) throw new Error('Unable to create task.')
      if (!selection.isCurrent(operation.selection)) return
      setTasks((current) => [data.task, ...current.filter((task) => task.id !== data.task.id)])
      setActivities((current) => [...(data.activities || []), ...current])
      setTaskForm(emptyTaskForm(defaultAssignedToUserId))
      onError('')
    } catch (taskError) {
      if (!isAbortError(taskError) && selection.isCurrent(operation.selection)) onError(taskError.message || 'Unable to create task.')
    } finally {
      selection.finish(operation)
      if (selection.isCurrent(operation.selection)) setIsCreatingTask(false)
    }
  }

  return {
    activities,
    activityMeta,
    fetchTasks,
    fetchWork,
    handleCreateNote,
    handleCreateTask,
    isCreatingNote,
    isCreatingTask,
    isLoadingOlderActivities,
    isLoadingOlderNotes,
    load,
    loadOlderActivities,
    loadOlderNotes,
    noteBody,
    notes,
    noteMeta,
    refreshTasks,
    reset,
    setActivities,
    setNoteBody,
    setTaskForm,
    taskForm,
    tasks
  }
}
