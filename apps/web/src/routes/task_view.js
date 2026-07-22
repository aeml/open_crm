export const unassignedAssigneeFilter = 'unassigned'

export function taskFormValues(task = {}) {
  return {
    title: task.title || '',
    entityType: task.entityType || 'deal',
    entityId: String(task.entityId || ''),
    description: task.description || '',
    status: task.status || 'open',
    dueAt: task.dueAt ? task.dueAt.slice(0, 16) : '',
    completedAt: task.completedAt ? task.completedAt.slice(0, 16) : '',
    assignedToUserId: task.assignedToUserId ? String(task.assignedToUserId) : ''
  }
}

export function normalizeTaskStatusFilter(value) {
  return value === 'completed' ? 'completed' : 'open'
}

export function normalizeDueView(value) {
  if (value === 'dueToday') return 'dueSoon'
  return ['all', 'overdue', 'dueSoon', 'upcoming', 'noDueDate'].includes(value) ? value : 'all'
}

export function normalizeAssigneeFilter(value) {
  if (value === unassignedAssigneeFilter) {
    return value
  }

  return /^\d+$/.test(String(value || '')) ? String(value) : 'all'
}

export function normalizeEntityTypeFilter(value) {
  return ['all', 'deal', 'company', 'contact'].includes(value) ? value : 'all'
}

export function normalizeEntityIdFilter(value) {
  return /^\d+$/.test(String(value || '')) ? String(value) : ''
}

export function formatDueLabel(task) {
  if (task.completedAt) {
    return `Completed ${new Date(task.completedAt).toLocaleString()}`
  }
  if (!task.dueAt) {
    return 'No due date'
  }
  return `Due ${new Date(task.dueAt).toLocaleString()}`
}

export function taskLabels(businessType) {
  const usesServiceLanguage = businessType === 'services' || businessType === 'construction-services'
  const noun = usesServiceLanguage ? 'service task' : 'task'
  const plural = `${noun}s`
  const titleNoun = usesServiceLanguage ? 'Service task' : 'Task'
  const titlePlural = usesServiceLanguage ? 'Service tasks' : 'Tasks'
  const dealLabel = usesServiceLanguage ? 'Job' : 'Deal'

  return {
    collection: usesServiceLanguage ? 'Service Tasks' : titlePlural,
    createDescription: usesServiceLanguage ? 'Assign work against a contact, client, or job.' : 'Assign work against a contact, company, or deal.',
    entityTypeLabel: usesServiceLanguage ? 'Linked record type' : 'Entity type',
    entityTypeFilterLabel: usesServiceLanguage ? 'Linked record filter' : 'Record type filter',
    dealOption: dealLabel,
    companyLabel: usesServiceLanguage ? 'Client' : 'Company',
    noun,
    plural,
    titleNoun,
    titlePlural
  }
}

export function taskCountLabel(statusFilter, dueView, labels) {
  return taskListHeading(statusFilter, dueView, labels).toLowerCase()
}

export function taskListHeading(statusFilter, dueView, labels) {
  if (statusFilter === 'completed') {
    return `Completed ${labels.plural}`
  }

  if (dueView === 'overdue') {
    return `Overdue ${labels.plural}`
  }
  if (dueView === 'dueSoon') {
    return `${labels.titlePlural} due within 24 hours`
  }
  if (dueView === 'upcoming') {
    return `Upcoming ${labels.plural}`
  }
  if (dueView === 'noDueDate') {
    return `${labels.titlePlural} without due dates`
  }

  return `Open ${labels.plural}`
}

function sortTasks(tasks, field, missingValue, direction) {
  return [...tasks].sort((left, right) => {
    const leftDate = Date.parse(left[field])
    const rightDate = Date.parse(right[field])
    const leftValue = Number.isNaN(leftDate) ? missingValue : leftDate
    const rightValue = Number.isNaN(rightDate) ? missingValue : rightDate

    if (leftValue === rightValue) {
      return (left.id || 0) - (right.id || 0)
    }
    return direction * (leftValue - rightValue)
  })
}

export function sortOpenTasks(tasks) {
  return sortTasks(tasks, 'dueAt', Number.POSITIVE_INFINITY, 1)
}

export function sortCompletedTasks(tasks) {
  return sortTasks(tasks, 'completedAt', Number.NEGATIVE_INFINITY, -1)
}

export function matchesAssignee(task, assigneeFilter) {
  if (assigneeFilter === 'all') {
    return true
  }

  if (assigneeFilter === unassignedAssigneeFilter) {
    return !task.assignedToUserId
  }

  return String(task.assignedToUserId || '') === assigneeFilter
}

export function matchesEntityType(task, entityTypeFilter) {
  if (entityTypeFilter === 'all') {
    return true
  }

  return task.entityType === entityTypeFilter
}

export function matchesStatus(task, statusFilter) {
  if (statusFilter === 'completed') {
    return task.status === 'completed'
  }

  return task.status !== 'completed'
}

export function emptyTaskListMessage(statusFilter, dueView, labels, hasFilteredTasks = false) {
  const heading = taskListHeading(statusFilter, dueView, labels).toLowerCase()
  const isFiltered = statusFilter !== 'open' || dueView !== 'all' || hasFilteredTasks
  return `No ${heading}${isFiltered ? ' match the current filters.' : ' yet.'}`
}

export function emptyTaskListDescription(statusFilter, dueView, labels, hasFilteredTasks = false) {
  if (statusFilter !== 'open') {
    return `Completed ${labels.plural} will appear here after work is closed.`
  }
  if (dueView !== 'all' || hasFilteredTasks) {
    return 'Change the task view or clear filters to see more work.'
  }
  return `Create the first ${labels.noun} once there is a real follow-up to track.`
}
