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
  if (businessType === 'services' || businessType === 'construction-services') {
    return {
      collection: 'Service Tasks',
      createHeading: 'New service task',
      createDescription: 'Assign work against a contact, client, or job.',
      summaryOpen: 'Open service tasks',
      summaryCompleted: 'Completed service tasks',
      searchLabel: 'Search service tasks',
      openHeading: 'Open service tasks',
      completedHeading: 'Completed service tasks',
      showingSuffix: 'service tasks',
      entityTypeLabel: 'Linked record type',
      entityTypeFilterLabel: 'Linked record filter',
      dealOption: 'Job',
      dealLabel: 'Job',
      companyLabel: 'Client',
      viewLabel: 'Service task view',
      overdueHeading: 'Overdue service tasks',
      dueSoonHeading: 'Service tasks due within 24 hours',
      upcomingHeading: 'Upcoming service tasks',
      noDueDateHeading: 'Service tasks without due dates',
      activityAria: 'Service task activity list'
    }
  }

  return {
    collection: 'Tasks',
    createHeading: 'New task',
    createDescription: 'Assign work against a contact, company, or deal.',
    summaryOpen: 'Open tasks',
    summaryCompleted: 'Completed tasks',
    searchLabel: 'Search tasks',
    openHeading: 'Open tasks',
    completedHeading: 'Completed tasks',
    showingSuffix: 'tasks',
    entityTypeLabel: 'Entity type',
    entityTypeFilterLabel: 'Record type filter',
    dealOption: 'Deal',
    dealLabel: 'Deal',
    companyLabel: 'Company',
    viewLabel: 'Task view',
    overdueHeading: 'Overdue tasks',
    dueSoonHeading: 'Tasks due within 24 hours',
    upcomingHeading: 'Upcoming tasks',
    noDueDateHeading: 'Tasks without due dates',
    activityAria: 'Task activity list'
  }
}

export function taskCountLabel(statusFilter, dueView, labels) {
  if (statusFilter === 'completed') {
    return labels.summaryCompleted.toLowerCase()
  }

  if (dueView === 'overdue') {
    return labels.overdueHeading.toLowerCase()
  }
  if (dueView === 'dueSoon') {
    return labels.dueSoonHeading.toLowerCase()
  }
  if (dueView === 'upcoming') {
    return labels.upcomingHeading.toLowerCase()
  }
  if (dueView === 'noDueDate') {
    return labels.noDueDateHeading.toLowerCase()
  }

  return labels.summaryOpen.toLowerCase()
}

export function taskListHeading(statusFilter, dueView, labels) {
  if (statusFilter === 'completed') {
    return labels.completedHeading
  }

  if (dueView === 'overdue') {
    return labels.overdueHeading
  }
  if (dueView === 'dueSoon') {
    return labels.dueSoonHeading
  }
  if (dueView === 'upcoming') {
    return labels.upcomingHeading
  }
  if (dueView === 'noDueDate') {
    return labels.noDueDateHeading
  }

  return labels.openHeading
}

function taskDueSortValue(task) {
  if (!task.dueAt) {
    return Number.POSITIVE_INFINITY
  }

  const dueAt = new Date(task.dueAt)
  if (Number.isNaN(dueAt.getTime())) {
    return Number.POSITIVE_INFINITY
  }

  return dueAt.getTime()
}

export function sortOpenTasks(tasks) {
  return [...tasks].sort((left, right) => {
    const leftDue = taskDueSortValue(left)
    const rightDue = taskDueSortValue(right)

    if (leftDue === rightDue) {
      return (left.id || 0) - (right.id || 0)
    }

    return leftDue - rightDue
  })
}

function taskCompletedSortValue(task) {
  if (!task.completedAt) {
    return Number.NEGATIVE_INFINITY
  }

  const completedAt = new Date(task.completedAt)
  if (Number.isNaN(completedAt.getTime())) {
    return Number.NEGATIVE_INFINITY
  }

  return completedAt.getTime()
}

export function sortCompletedTasks(tasks) {
  return [...tasks].sort((left, right) => {
    const leftCompleted = taskCompletedSortValue(left)
    const rightCompleted = taskCompletedSortValue(right)

    if (leftCompleted === rightCompleted) {
      return (left.id || 0) - (right.id || 0)
    }

    return rightCompleted - leftCompleted
  })
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
  if (statusFilter !== 'open') {
    return `No ${labels.summaryCompleted.toLowerCase()} match the current filters.`
  }

  if (dueView === 'overdue') {
    return `No ${labels.overdueHeading.toLowerCase()} match the current filters.`
  }
  if (dueView === 'dueSoon') {
    return `No ${labels.dueSoonHeading.toLowerCase()} match the current filters.`
  }
  if (dueView === 'upcoming') {
    return `No ${labels.upcomingHeading.toLowerCase()} match the current filters.`
  }
  if (dueView === 'noDueDate') {
    return `No ${labels.noDueDateHeading.toLowerCase()} match the current filters.`
  }

  return hasFilteredTasks ? `No ${labels.summaryOpen.toLowerCase()} match the current filters.` : `No ${labels.summaryOpen.toLowerCase()} yet.`
}

export function emptyTaskListDescription(statusFilter, dueView, labels, hasFilteredTasks = false) {
  if (statusFilter !== 'open') {
    return `Completed ${labels.showingSuffix} will appear here after work is closed.`
  }
  if (dueView !== 'all' || hasFilteredTasks) {
    return 'Change the task view or clear filters to see more work.'
  }
  return `Create the first ${labels.showingSuffix.slice(0, -1)} once there is a real follow-up to track.`
}
