import { useState } from 'react'
import { isAbortError } from '../lib/api'
import { createNote, listNotes } from '../lib/notes'
import { createTask, listTasks } from '../lib/tasks'

function emptyTaskForm(assignedToUserId = '') {
  return { title: '', description: '', dueAt: '', assignedToUserId }
}

// Notes, tasks, and activity belong to one deal visit. Late mutations may have
// succeeded on their original record, but must never appear on a newer detail.
export function useDealWork({ defaultAssignedToUserId = '', selectedDealId, selection, onError }) {
  const [notes, setNotes] = useState([])
  const [tasks, setTasks] = useState([])
  const [noteBody, setNoteBody] = useState('')
  const [taskForm, setTaskForm] = useState(emptyTaskForm(defaultAssignedToUserId))
  const [activities, setActivities] = useState([])
  const [isCreatingNote, setIsCreatingNote] = useState(false)
  const [isCreatingTask, setIsCreatingTask] = useState(false)

  async function fetchWork(dealId, { signal } = {}) {
    const [loadedNotes, taskData] = await Promise.all([
      listNotes('deal', dealId, { signal }),
      listTasks({ status: 'open', entityType: 'deal', entityId: dealId }, { signal })
    ])
    const loadedTasks = taskData.tasks || []
    const hasWrongNote = loadedNotes.some((note) => note.entityType !== 'deal' || note.entityId !== dealId)
    const hasWrongTask = loadedTasks.some((task) => task.entityType !== 'deal' || task.entityId !== dealId)
    if (hasWrongNote || hasWrongTask) throw new Error('Unable to load deal work.')
    return { notes: loadedNotes, tasks: loadedTasks }
  }

  function load({ activities: nextActivities = [], notes: nextNotes = [], tasks: nextTasks = [] } = {}) {
    setActivities(nextActivities)
    setNotes(nextNotes)
    setTasks(nextTasks)
    setNoteBody('')
    setTaskForm(emptyTaskForm(defaultAssignedToUserId))
    setIsCreatingNote(false)
    setIsCreatingTask(false)
  }

  function reset() {
    load()
  }

  async function handleCreateNote(event) {
    event.preventDefault()
    const operation = selection.start('note', selectedDealId)
    const body = noteBody.trim()
    if (!operation || !body) {
      selection.finish(operation)
      return
    }

    setIsCreatingNote(true)
    try {
      const data = await createNote({ entityType: 'deal', entityId: operation.dealId, body })
      if (!data?.note?.id || data.note.entityType !== 'deal' || data.note.entityId !== operation.dealId) {
        throw new Error('Unable to add note.')
      }
      if (!selection.isCurrent(operation.selection)) {
        return
      }
      setNotes((current) => [data.note, ...current])
      if (data.activity) setActivities((current) => [data.activity, ...current])
      setNoteBody('')
      onError('')
    } catch (noteError) {
      if (!isAbortError(noteError) && selection.isCurrent(operation.selection)) {
        onError(noteError.message || 'Unable to add note.')
      }
    } finally {
      selection.finish(operation)
      if (selection.isCurrent(operation.selection)) setIsCreatingNote(false)
    }
  }

  async function handleCreateTask(event) {
    event.preventDefault()
    const operation = selection.start('task', selectedDealId)
    const title = taskForm.title.trim()
    if (!operation || !title) {
      selection.finish(operation)
      return
    }

    setIsCreatingTask(true)
    try {
      const data = await createTask({
        entityType: 'deal',
        entityId: operation.dealId,
        title,
        description: taskForm.description.trim(),
        status: 'open',
        dueAt: taskForm.dueAt ? `${taskForm.dueAt}:00Z` : '',
        assignedToUserId: Number.parseInt(taskForm.assignedToUserId, 10) || 0
      })
      if (!data?.task?.id || data.task.entityType !== 'deal' || data.task.entityId !== operation.dealId) {
        throw new Error('Unable to create task.')
      }
      if (!selection.isCurrent(operation.selection)) {
        return
      }
      setTasks((current) => [data.task, ...current.filter((task) => task.id !== data.task.id)])
      setActivities((current) => [...(data.activities || []), ...current])
      setTaskForm(emptyTaskForm(defaultAssignedToUserId))
      onError('')
    } catch (taskError) {
      if (!isAbortError(taskError) && selection.isCurrent(operation.selection)) {
        onError(taskError.message || 'Unable to create task.')
      }
    } finally {
      selection.finish(operation)
      if (selection.isCurrent(operation.selection)) setIsCreatingTask(false)
    }
  }

  return {
    activities,
    fetchWork,
    handleCreateNote,
    handleCreateTask,
    isCreatingNote,
    isCreatingTask,
    load,
    noteBody,
    notes,
    reset,
    setActivities,
    setNoteBody,
    setTaskForm,
    taskForm,
    tasks
  }
}
