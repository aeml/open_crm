import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { EmptyState } from '../components/ui/empty_state'
import { SavedViews } from '../components/ui/saved_views'
import { ActivityTimeline } from '../components/ui/activity_timeline'
import { InlineError } from '../components/ui/inline_error'
import { archiveTask, createTask, getTask, listTasks, tasksExportURL, updateTask } from '../lib/tasks'
import { listDeals } from '../lib/deals'
import { listCompanies } from '../lib/companies'
import { listContacts } from '../lib/contacts'
import { listOrganizationUsers } from '../lib/users'
import { isAbortError } from '../lib/api'
import { useAuth } from '../app/providers'
import { usePageTitle } from '../lib/use_page_title'

const emptyForm = {
  title: '',
  entityType: 'deal',
  entityId: '',
  description: '',
  status: 'open',
  dueAt: '',
  completedAt: '',
  assignedToUserId: ''
}

const unassignedAssigneeFilter = 'unassigned'

function normalizeTaskStatusFilter(value) {
  return value === 'completed' ? 'completed' : 'open'
}

function normalizeDueView(value) {
  return ['all', 'overdue', 'dueToday', 'upcoming', 'noDueDate'].includes(value) ? value : 'all'
}

function normalizeAssigneeFilter(value) {
  if (value === unassignedAssigneeFilter) {
    return value
  }

  return /^\d+$/.test(String(value || '')) ? String(value) : 'all'
}

function normalizeEntityTypeFilter(value) {
  return ['all', 'deal', 'company', 'contact'].includes(value) ? value : 'all'
}

function normalizeEntityIdFilter(value) {
  return /^\d+$/.test(String(value || '')) ? String(value) : ''
}

function formatDueLabel(task) {
  if (task.completedAt) {
    return `Completed ${new Date(task.completedAt).toLocaleString()}`
  }
  if (!task.dueAt) {
    return 'No due date'
  }
  return `Due ${new Date(task.dueAt).toLocaleString()}`
}

function taskLabels(businessType) {
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
      dueTodayHeading: 'Service tasks due today',
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
    dueTodayHeading: 'Tasks due today',
    upcomingHeading: 'Upcoming tasks',
    noDueDateHeading: 'Tasks without due dates',
    activityAria: 'Task activity list'
  }
}

function taskCountLabel(statusFilter, dueView, labels) {
  if (statusFilter === 'completed') {
    return labels.summaryCompleted.toLowerCase()
  }

  if (dueView === 'overdue') {
    return labels.overdueHeading.toLowerCase()
  }
  if (dueView === 'dueToday') {
    return labels.dueTodayHeading.toLowerCase()
  }
  if (dueView === 'upcoming') {
    return labels.upcomingHeading.toLowerCase()
  }
  if (dueView === 'noDueDate') {
    return labels.noDueDateHeading.toLowerCase()
  }

  return labels.summaryOpen.toLowerCase()
}

function taskListHeading(statusFilter, dueView, labels) {
  if (statusFilter === 'completed') {
    return labels.completedHeading
  }

  if (dueView === 'overdue') {
    return labels.overdueHeading
  }
  if (dueView === 'dueToday') {
    return labels.dueTodayHeading
  }
  if (dueView === 'upcoming') {
    return labels.upcomingHeading
  }
  if (dueView === 'noDueDate') {
    return labels.noDueDateHeading
  }

  return labels.openHeading
}

function startOfToday(now) {
  const next = new Date(now)
  next.setHours(0, 0, 0, 0)
  return next
}

function endOfToday(now) {
  const next = new Date(now)
  next.setHours(23, 59, 59, 999)
  return next
}

function matchesDueView(task, dueView) {
  if (dueView === 'all') {
    return true
  }

  if (!task.dueAt) {
    return dueView === 'noDueDate'
  }

  const dueAt = new Date(task.dueAt)
  if (Number.isNaN(dueAt.getTime())) {
    return dueView === 'noDueDate'
  }

  const now = new Date()
  const dayStart = startOfToday(now)
  const dayEnd = endOfToday(now)

  if (dueView === 'overdue') {
    return dueAt < dayStart
  }
  if (dueView === 'dueToday') {
    return dueAt >= dayStart && dueAt <= dayEnd
  }
  if (dueView === 'upcoming') {
    return dueAt > dayEnd
  }

  return false
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

function sortOpenTasks(tasks) {
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

function sortCompletedTasks(tasks) {
  return [...tasks].sort((left, right) => {
    const leftCompleted = taskCompletedSortValue(left)
    const rightCompleted = taskCompletedSortValue(right)

    if (leftCompleted === rightCompleted) {
      return (left.id || 0) - (right.id || 0)
    }

    return rightCompleted - leftCompleted
  })
}

function matchesAssignee(task, assigneeFilter) {
  if (assigneeFilter === 'all') {
    return true
  }

  if (assigneeFilter === unassignedAssigneeFilter) {
    return !task.assignedToUserId
  }

  return String(task.assignedToUserId || '') === assigneeFilter
}

function matchesEntityType(task, entityTypeFilter) {
  if (entityTypeFilter === 'all') {
    return true
  }

  return task.entityType === entityTypeFilter
}

function matchesStatus(task, statusFilter) {
  if (statusFilter === 'completed') {
    return task.status === 'completed'
  }

  return task.status !== 'completed'
}

function emptyTaskListMessage(statusFilter, dueView, labels, hasFilteredTasks = false) {
  if (statusFilter !== 'open') {
    return `No ${labels.summaryCompleted.toLowerCase()} match the current filters.`
  }

  if (dueView === 'overdue') {
    return `No ${labels.overdueHeading.toLowerCase()} match the current filters.`
  }
  if (dueView === 'dueToday') {
    return `No ${labels.dueTodayHeading.toLowerCase()} match the current filters.`
  }
  if (dueView === 'upcoming') {
    return `No ${labels.upcomingHeading.toLowerCase()} match the current filters.`
  }
  if (dueView === 'noDueDate') {
    return `No ${labels.noDueDateHeading.toLowerCase()} match the current filters.`
  }

  return hasFilteredTasks ? `No ${labels.summaryOpen.toLowerCase()} match the current filters.` : `No ${labels.summaryOpen.toLowerCase()} yet.`
}

function emptyTaskListDescription(statusFilter, dueView, labels, hasFilteredTasks = false) {
  if (statusFilter !== 'open') {
    return `Completed ${labels.showingSuffix} will appear here after work is closed.`
  }
  if (dueView !== 'all' || hasFilteredTasks) {
    return 'Change the task view or clear filters to see more work.'
  }
  return `Create the first ${labels.showingSuffix.slice(0, -1)} once there is a real follow-up to track.`
}

export function TasksRoute() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { taskId } = useParams()
  const routeTaskId = Number.parseInt(taskId || '', 10)
  const { session, businessProfile } = useAuth()
  const businessType = businessProfile?.businessType || session?.organization?.businessType || 'general'
  const currentUserId = session?.user?.id ? String(session.user.id) : ''
  const labels = taskLabels(businessType)
  usePageTitle(labels.collection)
  const initialSearch = searchParams.get('q') || ''
  const initialStatusFilter = normalizeTaskStatusFilter(searchParams.get('status'))
  const initialDueView = initialStatusFilter === 'open' ? normalizeDueView(searchParams.get('due')) : 'all'
  const initialAssigneeFilter = normalizeAssigneeFilter(searchParams.get('assignee'))
  const initialEntityTypeFilter = normalizeEntityTypeFilter(searchParams.get('entityType'))
  const initialEntityIdFilter = initialEntityTypeFilter === 'all' ? '' : normalizeEntityIdFilter(searchParams.get('entityId'))
  const [tasks, setTasks] = useState([])
  const [meta, setMeta] = useState({ page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0 })
  const [search, setSearch] = useState(initialSearch)
  const [statusFilter, setStatusFilter] = useState(initialStatusFilter)
  const [dueView, setDueView] = useState(initialDueView)
  const [assigneeFilter, setAssigneeFilter] = useState(initialAssigneeFilter)
  const [entityTypeFilter, setEntityTypeFilter] = useState(initialEntityTypeFilter)
  const [entityIdFilter, setEntityIdFilter] = useState(initialEntityIdFilter)
  const [selectedTaskId, setSelectedTaskId] = useState(null)
  const [detail, setDetail] = useState(null)
  const [detailCache, setDetailCache] = useState({})
  const [dealOptions, setDealOptions] = useState([])
  const [companyOptions, setCompanyOptions] = useState([])
  const [contactOptions, setContactOptions] = useState([])
  const [userOptions, setUserOptions] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [error, setError] = useState('')
  const [isListLoading, setIsListLoading] = useState(true)
  const [isDetailLoading, setIsDetailLoading] = useState(false)
  const listControllerRef = useRef(null)

  const selectedTask = detail?.task || null
  const selectedActivities = detail?.activities || []
  const statusTasks = useMemo(() => tasks.filter((task) => matchesStatus(task, statusFilter)), [statusFilter, tasks])
  const visibleTasks = useMemo(() => {
    const filteredTasks = statusTasks.filter((task) => {
      if (!matchesAssignee(task, assigneeFilter) || !matchesEntityType(task, entityTypeFilter)) {
        return false
      }

      if (!entityIdFilter) {
        return true
      }

      return String(task.entityId || '') === entityIdFilter
    })

    if (statusFilter !== 'open') {
      return sortCompletedTasks(filteredTasks)
    }

    return sortOpenTasks(filteredTasks.filter((task) => matchesDueView(task, dueView)))
  }, [assigneeFilter, dueView, entityIdFilter, entityTypeFilter, statusFilter, statusTasks])
  const hasFilteredTasks = statusTasks.length > 0
  const emptyMessage = useMemo(() => emptyTaskListMessage(statusFilter, dueView, labels, hasFilteredTasks), [dueView, hasFilteredTasks, labels, statusFilter])
  const emptyDescription = useMemo(() => emptyTaskListDescription(statusFilter, dueView, labels, hasFilteredTasks), [dueView, hasFilteredTasks, labels, statusFilter])

  function buildTasksPath(nextTaskId = routeTaskId, nextSearch = search, nextStatusFilter = statusFilter, nextDueView = dueView, nextAssigneeFilter = assigneeFilter, nextEntityTypeFilter = entityTypeFilter, nextEntityIdFilter = entityIdFilter) {
    const params = new URLSearchParams()
    if (nextSearch) {
      params.set('q', nextSearch)
    }
    if (nextStatusFilter !== 'open') {
      params.set('status', nextStatusFilter)
    }
    if (nextDueView !== 'all') {
      params.set('due', nextDueView)
    }
    if (nextAssigneeFilter !== 'all') {
      params.set('assignee', nextAssigneeFilter)
    }
    if (nextEntityTypeFilter !== 'all') {
      params.set('entityType', nextEntityTypeFilter)
    }
    if (nextEntityTypeFilter !== 'all' && nextEntityIdFilter) {
      params.set('entityId', nextEntityIdFilter)
    }

    const suffix = params.toString() ? `?${params.toString()}` : ''
    const pathname = nextTaskId ? `/tasks/${nextTaskId}` : '/tasks'
    return `${pathname}${suffix}`
  }

  async function loadTasks(nextSearch = search, nextStatus = statusFilter, nextEntityTypeFilter = entityTypeFilter, nextEntityIdFilter = entityIdFilter, { signal } = {}) {
    const data = await listTasks({
      search: nextSearch,
      status: nextStatus,
      entityType: nextEntityTypeFilter === 'all' ? '' : nextEntityTypeFilter,
      entityId: nextEntityTypeFilter === 'all' ? 0 : Number.parseInt(nextEntityIdFilter, 10) || 0
    }, { signal })
    setTasks(data.tasks || [])
    setMeta(data.meta || { page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0 })
  }

  async function loadDealOptions({ signal } = {}) {
    const data = await listDeals({}, { signal })
    const nextDeals = data.deals || []
    setDealOptions(nextDeals)
    setForm((current) => {
      if (current.entityType !== 'deal' || current.entityId || nextDeals.length === 0) {
        return current
      }
      return { ...current, entityId: String(nextDeals[0].id) }
    })
  }

  async function loadCompanyOptions({ signal } = {}) {
    const data = await listCompanies('', { signal })
    const nextCompanies = data.companies || []
    setCompanyOptions(nextCompanies)
    setForm((current) => {
      if (current.entityType !== 'company' || current.entityId || nextCompanies.length === 0) {
        return current
      }
      return { ...current, entityId: String(nextCompanies[0].id) }
    })
  }

  async function loadContactOptions({ signal } = {}) {
    const data = await listContacts('', { signal })
    const nextContacts = data.contacts || []
    setContactOptions(nextContacts)
    setForm((current) => {
      if (current.entityType !== 'contact' || current.entityId || nextContacts.length === 0) {
        return current
      }
      return { ...current, entityId: String(nextContacts[0].id) }
    })
  }

  async function loadUserOptions({ signal } = {}) {
    const nextUsers = await listOrganizationUsers({ signal })
    setUserOptions(nextUsers)
    setForm((current) => {
      if (current.assignedToUserId || nextUsers.length === 0) {
        return current
      }
      return { ...current, assignedToUserId: String(nextUsers[0].id) }
    })
  }

  useEffect(() => {
    const controller = new AbortController()

    async function run() {
      setIsListLoading(true)
      try {
        await Promise.all([
          loadTasks(initialSearch, initialStatusFilter, initialEntityTypeFilter, initialEntityIdFilter, { signal: controller.signal }),
          loadDealOptions({ signal: controller.signal }),
          loadCompanyOptions({ signal: controller.signal }),
          loadContactOptions({ signal: controller.signal }),
          loadUserOptions({ signal: controller.signal })
        ])
        setError('')
      } catch (loadError) {
        if (!isAbortError(loadError)) {
          setError(loadError.message || 'Unable to load tasks.')
        }
      } finally {
        if (!controller.signal.aborted) {
          setIsListLoading(false)
        }
      }
    }

    run()
    return () => {
      controller.abort()
    }
  }, [])

  useEffect(() => {
    const controller = new AbortController()

    async function syncRouteTask() {
      if (!Number.isInteger(routeTaskId) || routeTaskId <= 0) {
        if (selectedTaskId) {
          clearSelectedTask()
        }
        return
      }

      if (selectedTaskId === routeTaskId && detail?.task?.id === routeTaskId) {
        return
      }

      const cached = detailCache[routeTaskId]
      if (cached) {
        syncTaskIntoState(cached.task, cached.activities || [])
        setError('')
        return
      }

      const routeTask = tasks.find((entry) => entry.id === routeTaskId)
      if (routeTask) {
        syncTaskIntoState(routeTask, [])
        setError('')
        return
      }

      try {
        setIsDetailLoading(true)
        const data = await getTask(routeTaskId, { signal: controller.signal })
        if (controller.signal.aborted) {
          return
        }
        setTasks((current) => {
          const next = current.filter((entry) => entry.id !== routeTaskId)
          return [data.task, ...next]
        })
        syncTaskIntoState(data.task, data.activities || [])
        setError('')
      } catch (loadError) {
        if (!isAbortError(loadError)) {
          setError(loadError.message || 'Unable to load task.')
        }
      } finally {
        if (!controller.signal.aborted) {
          setIsDetailLoading(false)
        }
      }
    }

    syncRouteTask()
    return () => {
      controller.abort()
    }
  }, [detail, detailCache, routeTaskId, selectedTaskId, tasks])

  async function reloadTasks(nextSearch = search, nextStatus = statusFilter, nextEntityTypeFilter = entityTypeFilter, nextEntityIdFilter = entityIdFilter) {
    listControllerRef.current?.abort()
    const controller = new AbortController()
    listControllerRef.current = controller
    setIsListLoading(true)
    try {
      await loadTasks(nextSearch, nextStatus, nextEntityTypeFilter, nextEntityIdFilter, { signal: controller.signal })
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load tasks.')
      }
    } finally {
      if (listControllerRef.current === controller) {
        listControllerRef.current = null
      }
      if (!controller.signal.aborted) {
        setIsListLoading(false)
      }
    }
  }

  async function handleSearchChange(event) {
    const value = event.target.value
    setSearch(value)
    navigate(buildTasksPath(selectedTaskId, value, statusFilter, dueView, assigneeFilter, entityTypeFilter, entityIdFilter), { replace: true })
    await reloadTasks(value, statusFilter, entityTypeFilter, entityIdFilter)
  }

  async function handleToggleStatus(nextStatus) {
    const nextDueView = nextStatus === 'open' ? dueView : 'all'
    setStatusFilter(nextStatus)
    setDueView(nextDueView)
    navigate(buildTasksPath(selectedTaskId, search, nextStatus, nextDueView, assigneeFilter, entityTypeFilter, entityIdFilter), { replace: true })
    await reloadTasks(search, nextStatus, entityTypeFilter, entityIdFilter)
  }

  function handleAssigneeFilterChange(nextAssigneeFilter) {
    setAssigneeFilter(nextAssigneeFilter)
    navigate(buildTasksPath(selectedTaskId, search, statusFilter, dueView, nextAssigneeFilter, entityTypeFilter, entityIdFilter), { replace: true })
  }

  async function handleEntityTypeFilterChange(nextEntityTypeFilter) {
    const nextEntityIdFilter = nextEntityTypeFilter === entityTypeFilter ? entityIdFilter : ''
    setEntityTypeFilter(nextEntityTypeFilter)
    setEntityIdFilter(nextEntityIdFilter)
    navigate(buildTasksPath(selectedTaskId, search, statusFilter, dueView, assigneeFilter, nextEntityTypeFilter, nextEntityIdFilter), { replace: true })
    await reloadTasks(search, statusFilter, nextEntityTypeFilter, nextEntityIdFilter)
  }

  function handleDueViewChange(nextDueView) {
    setDueView(nextDueView)
    navigate(buildTasksPath(selectedTaskId, search, statusFilter, nextDueView, assigneeFilter, entityTypeFilter, entityIdFilter), { replace: true })
  }

  async function handleEntityIdFilterChange(nextEntityIdFilter) {
    setEntityIdFilter(nextEntityIdFilter)
    navigate(buildTasksPath(selectedTaskId, search, statusFilter, dueView, assigneeFilter, entityTypeFilter, nextEntityIdFilter), { replace: true })
    await reloadTasks(search, statusFilter, entityTypeFilter, nextEntityIdFilter)
  }

  async function handleApplySavedView(filters) {
    const nextSearch = filters.q || ''
    const nextStatus = normalizeTaskStatusFilter(filters.status)
    const nextDueView = nextStatus === 'open' ? normalizeDueView(filters.due) : 'all'
    const nextAssigneeFilter = normalizeAssigneeFilter(filters.assignee)
    const nextEntityTypeFilter = normalizeEntityTypeFilter(filters.entityType)
    const nextEntityIdFilter = nextEntityTypeFilter === 'all' ? '' : normalizeEntityIdFilter(filters.entityId)

    setSearch(nextSearch)
    setStatusFilter(nextStatus)
    setDueView(nextDueView)
    setAssigneeFilter(nextAssigneeFilter)
    setEntityTypeFilter(nextEntityTypeFilter)
    setEntityIdFilter(nextEntityIdFilter)
    clearSelectedTask()
    navigate(buildTasksPath(null, nextSearch, nextStatus, nextDueView, nextAssigneeFilter, nextEntityTypeFilter, nextEntityIdFilter), { replace: true })
    await reloadTasks(nextSearch, nextStatus, nextEntityTypeFilter, nextEntityIdFilter)
  }

  function getDefaultEntityId(nextEntityType) {
    if (nextEntityType === 'deal') {
      return dealOptions[0] ? String(dealOptions[0].id) : ''
    }
    if (nextEntityType === 'company') {
      return companyOptions[0] ? String(companyOptions[0].id) : ''
    }
    if (nextEntityType === 'contact') {
      return contactOptions[0] ? String(contactOptions[0].id) : ''
    }
    return ''
  }

  function syncTaskIntoState(task, activities) {
    setDetailCache((current) => ({ ...current, [task.id]: { task, activities } }))
    setDetail({ task, activities })
    setSelectedTaskId(task.id)
    setForm({
      title: task.title || '',
      entityType: task.entityType || 'deal',
      entityId: String(task.entityId || ''),
      description: task.description || '',
      status: task.status || 'open',
      dueAt: task.dueAt ? task.dueAt.slice(0, 16) : '',
      completedAt: task.completedAt ? task.completedAt.slice(0, 16) : '',
      assignedToUserId: task.assignedToUserId ? String(task.assignedToUserId) : ''
    })
  }

  function clearSelectedTask() {
    setSelectedTaskId(null)
    setDetail(null)
    setForm(emptyForm)
  }

  async function handleCreate(event) {
    event.preventDefault()
    try {
      const data = await createTask({
        entityType: form.entityType,
        entityId: Number.parseInt(form.entityId, 10),
        title: form.title,
        description: form.description,
        status: form.status,
        dueAt: form.dueAt ? `${form.dueAt}:00Z` : '',
        assignedToUserId: Number.parseInt(form.assignedToUserId, 10) || 0
      })
      const nextTasks = [data.task, ...tasks.filter((task) => task.id !== data.task.id)]
      setTasks(nextTasks)
      setMeta((current) => ({ ...current, total: current.total + 1, openCount: current.openCount + (data.task.status === 'completed' ? 0 : 1), completedCount: current.completedCount + (data.task.status === 'completed' ? 1 : 0) }))
      syncTaskIntoState(data.task, data.activities || [])
      navigate(buildTasksPath(data.task.id))
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to create task.')
    }
  }

  async function handleUpdate(event) {
    event.preventDefault()
    if (!selectedTaskId) {
      return
    }

    try {
      const data = await updateTask(selectedTaskId, {
        title: form.title,
        description: form.description,
        status: form.status,
        dueAt: form.dueAt ? `${form.dueAt}:00Z` : '',
        completedAt: form.completedAt ? `${form.completedAt}:00Z` : '',
        assignedToUserId: Number.parseInt(form.assignedToUserId, 10) || 0
      })
      setTasks((current) => current.map((task) => (task.id === selectedTaskId ? data.task : task)))
      syncTaskIntoState(data.task, data.activities || [])
      navigate(buildTasksPath(data.task.id))
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to update task.')
    }
  }

  async function handleOpenTask(task) {
    const cached = detailCache[task.id]
    if (cached) {
      syncTaskIntoState(cached.task, cached.activities || [])
      navigate(buildTasksPath(task.id))
      return
    }

    syncTaskIntoState(task, [])
    navigate(buildTasksPath(task.id))
  }

  async function handleQuickStatus(task, nextStatus) {
    try {
      const completedAt = nextStatus === 'completed' ? new Date().toISOString() : ''
      const data = await updateTask(task.id, {
        title: task.title,
        description: task.description || '',
        status: nextStatus,
        dueAt: task.dueAt || '',
        completedAt,
        assignedToUserId: task.assignedToUserId || 0
      })

      const wasCompleted = task.status === 'completed'
      const isCompleted = data.task.status === 'completed'
      setTasks((current) => current.map((currentTask) => (currentTask.id === task.id ? data.task : currentTask)))
      setMeta((current) => ({
        ...current,
        openCount: Math.max(0, current.openCount + (wasCompleted ? 1 : 0) - (isCompleted ? 1 : 0)),
        completedCount: Math.max(0, current.completedCount + (isCompleted ? 1 : 0) - (wasCompleted ? 1 : 0))
      }))
      if (selectedTaskId === task.id) {
        syncTaskIntoState(data.task, data.activities || [])
      } else {
        setDetailCache((current) => ({ ...current, [task.id]: { task: data.task, activities: data.activities || [] } }))
      }
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to update task.')
    }
  }

  function handleQuickComplete(task) {
    return handleQuickStatus(task, 'completed')
  }

  function handleQuickReopen(task) {
    return handleQuickStatus(task, 'open')
  }

  async function handleQuickAssign(task, nextAssignedToUserId) {
    try {
      const data = await updateTask(task.id, {
        title: task.title,
        description: task.description || '',
        status: task.status,
        dueAt: task.dueAt || '',
        completedAt: task.completedAt || '',
        assignedToUserId: Number.parseInt(nextAssignedToUserId, 10) || 0
      })

      setTasks((current) => current.map((currentTask) => (currentTask.id === task.id ? data.task : currentTask)))
      if (selectedTaskId === task.id) {
        syncTaskIntoState(data.task, data.activities || [])
      } else {
        setDetailCache((current) => ({ ...current, [task.id]: { task: data.task, activities: data.activities || [] } }))
      }
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to update task.')
    }
  }

  async function handleArchive() {
    if (!selectedTaskId) {
      return
    }

    try {
      const archivedTask = detail?.task
      await archiveTask(selectedTaskId)
      setTasks((current) => current.filter((task) => task.id !== selectedTaskId))
      setMeta((current) => ({
        ...current,
        total: Math.max(0, current.total - 1),
        openCount: Math.max(0, current.openCount - (archivedTask?.status === 'completed' ? 0 : 1)),
        completedCount: Math.max(0, current.completedCount - (archivedTask?.status === 'completed' ? 1 : 0))
      }))
      setDetailCache((current) => {
        const next = { ...current }
        delete next[selectedTaskId]
        return next
      })
      clearSelectedTask()
      navigate(buildTasksPath(null))
      setError('')
    } catch (archiveError) {
      setError(archiveError.message || 'Unable to archive task.')
    }
  }

  const summaryLabel = useMemo(() => taskListHeading(statusFilter, dueView, labels), [dueView, labels, statusFilter])

  return (
    <section className="dashboard-grid contacts-grid">
      <Card>
        <div className="card-stack">
            <div className="section-header">
              <div>
                <h2>{labels.collection}</h2>
                <p>Keep the next real action visible and close it cleanly.</p>
              </div>
            <div className="button-row">
              <a className="button button-secondary" href={tasksExportURL({ search, status: statusFilter, due: statusFilter === 'open' ? dueView : '', assignee: assigneeFilter === 'all' ? '' : assigneeFilter, entityType: entityTypeFilter === 'all' ? '' : entityTypeFilter, entityId: entityIdFilter })}>
                Export CSV
              </a>
              <Button className={statusFilter === 'open' ? '' : 'button-secondary'} onClick={() => handleToggleStatus('open')}>Show open</Button>
              <Button className={statusFilter === 'completed' ? '' : 'button-secondary'} onClick={() => handleToggleStatus('completed')}>Show completed</Button>
            </div>
          </div>
          <div className="record-list" role="list" aria-label="Task summary list">
            <article className="record-row" role="listitem">
              <div>
                <p>{labels.summaryOpen}</p>
              </div>
              <div>
                <p>{meta.openCount}</p>
              </div>
            </article>
            <article className="record-row" role="listitem">
              <div>
                <p>{labels.summaryCompleted}</p>
              </div>
              <div>
                <p>{meta.completedCount}</p>
              </div>
            </article>
          </div>
          <Field label={labels.searchLabel}>
            <input className="text-input" type="search" value={search} onChange={handleSearchChange} />
          </Field>
          <SavedViews
            entityType="tasks"
            currentFilters={{ q: search, status: statusFilter, due: dueView, assignee: assigneeFilter, entityType: entityTypeFilter, entityId: entityIdFilter }}
            onApply={handleApplySavedView}
            defaultName={`${labels.collection} view`}
          />
          <Field label="Assignee">
            <div className="button-row">
              <select className="text-input" value={assigneeFilter} onChange={(event) => handleAssigneeFilterChange(event.target.value)}>
                <option value="all">All assignees</option>
                <option value={unassignedAssigneeFilter}>Unassigned</option>
                {userOptions.map((user) => (
                  <option key={user.id} value={user.id}>{`${user.firstName} ${user.lastName}`.trim() || user.email}</option>
                ))}
              </select>
              {currentUserId ? (
                <Button className={assigneeFilter === currentUserId ? '' : 'button-secondary'} type="button" onClick={() => handleAssigneeFilterChange(currentUserId)}>
                  My tasks
                </Button>
              ) : null}
              <Button className={assigneeFilter === unassignedAssigneeFilter ? '' : 'button-secondary'} type="button" onClick={() => handleAssigneeFilterChange(unassignedAssigneeFilter)}>
                Unassigned
              </Button>
            </div>
          </Field>
          <Field label={labels.entityTypeFilterLabel}>
            <select className="text-input" value={entityTypeFilter} onChange={(event) => handleEntityTypeFilterChange(event.target.value)}>
              <option value="all">All record types</option>
              <option value="deal">{labels.dealOption}</option>
              <option value="company">{labels.companyLabel}</option>
              <option value="contact">Contact</option>
            </select>
          </Field>
          {entityTypeFilter !== 'all' ? (
            <Field label="Record">
              <select className="text-input" value={entityIdFilter} onChange={(event) => handleEntityIdFilterChange(event.target.value)}>
                <option value="">All {entityTypeFilter === 'deal' ? `${labels.dealOption.toLowerCase()}s` : entityTypeFilter === 'company' ? `${labels.companyLabel.toLowerCase()}s` : 'contacts'}</option>
                {(entityTypeFilter === 'deal' ? dealOptions : entityTypeFilter === 'company' ? companyOptions : contactOptions).map((entity) => (
                  <option key={entity.id} value={entity.id}>
                    {entityTypeFilter === 'contact' ? `${entity.firstName || ''} ${entity.lastName || ''}`.trim() : entity.name}
                  </option>
                ))}
              </select>
            </Field>
          ) : null}
          {statusFilter === 'open' ? (
            <Field label={labels.viewLabel}>
              <select className="text-input" value={dueView} onChange={(event) => handleDueViewChange(event.target.value)}>
                <option value="all">All open</option>
                <option value="overdue">Overdue</option>
                <option value="dueToday">Due today</option>
                <option value="upcoming">Upcoming</option>
                <option value="noDueDate">No due date</option>
              </select>
            </Field>
          ) : null}
          {isListLoading ? <p className="field-hint">Loading {labels.showingSuffix}...</p> : null}
          {error ? (
            <InlineError message={error} onRetry={() => reloadTasks(search, statusFilter, entityTypeFilter, entityIdFilter)} retryLabel={`Retry ${labels.showingSuffix}`} />
          ) : null}
          <h3>{summaryLabel}</h3>
          <p className="field-hint">Showing {visibleTasks.length} of {statusTasks.length} {taskCountLabel(statusFilter, dueView, labels)}.</p>
          <div className="record-list" role="list" aria-label="Tasks list">
            {visibleTasks.length === 0 && (!isListLoading || statusTasks.length > 0) ? (
              <EmptyState
                title={emptyMessage}
                description={emptyDescription}
                actionLabel={search.trim() || assigneeFilter !== 'all' || entityTypeFilter !== 'all' || entityIdFilter || dueView !== 'all' || statusFilter !== 'open' ? 'Reset task view' : ''}
                onAction={() => {
                  setSearch('')
                  setStatusFilter('open')
                  setDueView('all')
                  setAssigneeFilter('all')
                  setEntityTypeFilter('all')
                  setEntityIdFilter('')
                  navigate(buildTasksPath(null, '', 'open', 'all', 'all', 'all', ''), { replace: true })
                  reloadTasks('', 'open', 'all', '')
                }}
              />
            ) : visibleTasks.map((task) => (
              <article className="record-row" key={task.id} role="listitem">
                <div>
                  <button className="button button-ghost contact-link" type="button" onClick={() => handleOpenTask(task)}>
                    {task.title}
                  </button>
                  <p>{task.entityLabel || `${task.entityType} #${task.entityId}`}</p>
                  {statusFilter === 'open' ? (
                    <Button className="button-secondary" type="button" onClick={() => handleQuickComplete(task)} aria-label={`Complete ${task.title}`}>
                      Complete
                    </Button>
                  ) : (
                    <Button className="button-secondary" type="button" onClick={() => handleQuickReopen(task)} aria-label={`Reopen ${task.title}`}>
                      Reopen
                    </Button>
                  )}
                </div>
                <div>
                  <select className="text-input" aria-label={`Assign ${task.title}`} value={task.assignedToUserId ? String(task.assignedToUserId) : ''} onChange={(event) => handleQuickAssign(task, event.target.value)}>
                    <option value="">Unassigned</option>
                    {userOptions.map((user) => (
                      <option key={user.id} value={user.id}>{`${user.firstName} ${user.lastName}`.trim() || user.email}</option>
                    ))}
                  </select>
                  <p>{formatDueLabel(task)}</p>
                </div>
              </article>
            ))}
          </div>
        </div>
      </Card>

      <Card>
        <div className="card-stack">
          <div>
            <h2>{labels.createHeading}</h2>
            <p>{labels.createDescription}</p>
          </div>
          <form className="auth-form" onSubmit={handleCreate}>
            <Field label="Task title">
              <input className="text-input" value={form.title} onChange={(event) => setForm((current) => ({ ...current, title: event.target.value }))} required />
            </Field>
            <Field label={labels.entityTypeLabel}>
              <select className="text-input" value={form.entityType} onChange={(event) => setForm((current) => ({ ...current, entityType: event.target.value, entityId: getDefaultEntityId(event.target.value) }))}>
                <option value="deal">{labels.dealOption}</option>
                <option value="company">{labels.companyLabel}</option>
                <option value="contact">Contact</option>
              </select>
            </Field>
            {form.entityType === 'deal' ? (
              <Field label={labels.dealLabel}>
                <select className="text-input" value={form.entityId} onChange={(event) => setForm((current) => ({ ...current, entityId: event.target.value }))} required>
                  {dealOptions.map((deal) => (
                    <option key={deal.id} value={deal.id}>{deal.name}</option>
                  ))}
                </select>
              </Field>
            ) : null}
            {form.entityType === 'company' ? (
              <Field label={labels.companyLabel}>
                <select className="text-input" value={form.entityId} onChange={(event) => setForm((current) => ({ ...current, entityId: event.target.value }))} required>
                  {companyOptions.map((company) => (
                    <option key={company.id} value={company.id}>{company.name}</option>
                  ))}
                </select>
              </Field>
            ) : null}
            {form.entityType === 'contact' ? (
              <Field label="Contact">
                <select className="text-input" value={form.entityId} onChange={(event) => setForm((current) => ({ ...current, entityId: event.target.value }))} required>
                  {contactOptions.map((contact) => (
                    <option key={contact.id} value={contact.id}>{`${contact.firstName} ${contact.lastName}`.trim()}</option>
                  ))}
                </select>
              </Field>
            ) : null}
            <Field label="Description">
              <textarea className="text-input" value={form.description} onChange={(event) => setForm((current) => ({ ...current, description: event.target.value }))} />
            </Field>
            <Field label="Assigned to">
              <select className="text-input" value={form.assignedToUserId} onChange={(event) => setForm((current) => ({ ...current, assignedToUserId: event.target.value }))}>
                {userOptions.map((user) => (
                  <option key={user.id} value={user.id}>{`${user.firstName} ${user.lastName}`.trim() || user.email}</option>
                ))}
              </select>
            </Field>
            <Field label="Due at">
              <input className="text-input" type="datetime-local" value={form.dueAt} onChange={(event) => setForm((current) => ({ ...current, dueAt: event.target.value }))} />
            </Field>
            <Button type="submit">Save task</Button>
          </form>
        </div>
      </Card>

      {selectedTask ? (
        <Card>
          <div className="card-stack">
            {isDetailLoading ? <p className="field-hint">Loading task detail...</p> : null}
            <div className="section-header">
              <div>
                <h2>{selectedTask.title}</h2>
                <p>{selectedTask.entityLabel || `${selectedTask.entityType} #${selectedTask.entityId}`}</p>
              </div>
            </div>
            <form className="auth-form" onSubmit={handleUpdate}>
              <Field label="Task title">
                <input className="text-input" value={form.title} onChange={(event) => setForm((current) => ({ ...current, title: event.target.value }))} required />
              </Field>
              <Field label="Description">
                <textarea className="text-input" value={form.description} onChange={(event) => setForm((current) => ({ ...current, description: event.target.value }))} />
              </Field>
              <Field label="Assigned to">
                <select className="text-input" value={form.assignedToUserId} onChange={(event) => setForm((current) => ({ ...current, assignedToUserId: event.target.value }))}>
                  {userOptions.map((user) => (
                    <option key={user.id} value={user.id}>{`${user.firstName} ${user.lastName}`.trim() || user.email}</option>
                  ))}
                </select>
              </Field>
              <Field label="Status">
                <select className="text-input" value={form.status} onChange={(event) => setForm((current) => ({ ...current, status: event.target.value }))}>
                  <option value="open">Open</option>
                  <option value="completed">Completed</option>
                </select>
              </Field>
              <Field label="Due at">
                <input className="text-input" type="datetime-local" value={form.dueAt} onChange={(event) => setForm((current) => ({ ...current, dueAt: event.target.value }))} />
              </Field>
              <Field label="Completed at">
                <input className="text-input" type="datetime-local" value={form.completedAt} onChange={(event) => setForm((current) => ({ ...current, completedAt: event.target.value }))} />
              </Field>
              <Button type="submit">Update task</Button>
              <Button className="button-danger" type="button" onClick={handleArchive}>Archive task</Button>
            </form>
            <Card>
              <div className="card-stack">
                <h3>Activity</h3>
                <ActivityTimeline activities={selectedActivities} emptyMessage="No task activity yet." ariaLabel={labels.activityAria} />
              </div>
            </Card>
          </div>
        </Card>
      ) : null}
    </section>
  )
}
