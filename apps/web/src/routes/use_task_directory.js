import { useEffect, useRef, useState } from 'react'
import { isAbortError } from '../lib/api'
import { listCompanies } from '../lib/companies'
import { listContacts } from '../lib/contacts'
import { listDeals } from '../lib/deals'
import { listTasks } from '../lib/tasks'
import { listOrganizationUsers } from '../lib/users'
import {
  normalizeAssigneeFilter,
  normalizeDueView,
  normalizeEntityIdFilter,
  normalizeEntityTypeFilter,
  normalizeTaskStatusFilter,
  unassignedAssigneeFilter
} from './task_view'

export const emptyTaskMeta = {
  page: 1,
  pageSize: 20,
  total: 0,
  openCount: 0,
  completedCount: 0,
  overdueCount: 0,
  dueSoonCount: 0,
  upcomingCount: 0,
  noDueDateCount: 0
}

export function taskDirectoryPath({ assignee = 'all', due = 'all', entityId = '', entityType = 'all', search = '', status = 'open', taskId = null } = {}) {
  const params = new URLSearchParams()
  if (search) params.set('q', search)
  if (status !== 'open') params.set('status', status)
  if (due !== 'all') params.set('due', due)
  if (assignee !== 'all') params.set('assignee', assignee)
  if (entityType !== 'all') params.set('entityType', entityType)
  if (entityType !== 'all' && entityId) params.set('entityId', entityId)
  const suffix = params.toString() ? `?${params.toString()}` : ''
  return `${taskId ? `/tasks/${taskId}` : '/tasks'}${suffix}`
}

export function useTaskDirectory({
  initialAssigneeFilter,
  initialDueView,
  initialEntityIdFilter,
  initialEntityTypeFilter,
  initialSearch,
  initialStatusFilter,
  navigate,
  routeTaskId
}) {
  const [tasks, setTasks] = useState([])
  const [selectedTaskIds, setSelectedTaskIds] = useState([])
  const [meta, setMeta] = useState(emptyTaskMeta)
  const [search, setSearch] = useState(initialSearch)
  const [statusFilter, setStatusFilter] = useState(initialStatusFilter)
  const [dueView, setDueView] = useState(initialDueView)
  const [assigneeFilter, setAssigneeFilter] = useState(initialAssigneeFilter)
  const [entityTypeFilter, setEntityTypeFilter] = useState(initialEntityTypeFilter)
  const [entityIdFilter, setEntityIdFilter] = useState(initialEntityIdFilter)
  const [dealOptions, setDealOptions] = useState([])
  const [companyOptions, setCompanyOptions] = useState([])
  const [contactOptions, setContactOptions] = useState([])
  const [userOptions, setUserOptions] = useState([])
  const [error, setError] = useState('')
  const [isListLoading, setIsListLoading] = useState(true)
  const listControllerRef = useRef(null)
  const listRequestRef = useRef(null)

  function buildTasksPath(nextTaskId = routeTaskId) {
    return taskDirectoryPath({
      assignee: assigneeFilter,
      due: dueView,
      entityId: entityIdFilter,
      entityType: entityTypeFilter,
      search,
      status: statusFilter,
      taskId: nextTaskId
    })
  }

  function applyTasks(data) {
    setTasks(data.tasks || [])
    setSelectedTaskIds([])
    setMeta(data.meta || emptyTaskMeta)
  }

  async function loadTasks(nextSearch = search, nextStatus = statusFilter, nextEntityTypeFilter = entityTypeFilter, nextEntityIdFilter = entityIdFilter, nextAssigneeFilter = assigneeFilter, { request = {}, signal } = {}, nextDueView = dueView) {
    const isUnassigned = nextAssigneeFilter === unassignedAssigneeFilter
    const data = await listTasks({
      search: nextSearch,
      status: nextStatus,
      entityType: nextEntityTypeFilter === 'all' ? '' : nextEntityTypeFilter,
      entityId: nextEntityTypeFilter === 'all' ? 0 : Number.parseInt(nextEntityIdFilter, 10) || 0,
      unassigned: isUnassigned,
      assignedToUserId: isUnassigned ? 0 : (Number.parseInt(nextAssigneeFilter, 10) || 0),
      due: nextStatus === 'open' ? nextDueView : 'all'
    }, { signal })
    if (listRequestRef.current !== request || signal?.aborted) return false
    applyTasks(data)
    return true
  }

  async function loadOptions({ signal }) {
    function applyOptionLoad(promise, apply) {
      return promise.then((data) => {
        if (!signal.aborted) apply(data)
      })
    }

    await Promise.all([
      applyOptionLoad(listDeals({}, { signal }), (data) => setDealOptions(data.deals || [])),
      applyOptionLoad(listCompanies('', { signal }), (data) => setCompanyOptions(data.companies || [])),
      applyOptionLoad(listContacts('', { signal }), (data) => setContactOptions(data.contacts || [])),
      applyOptionLoad(listOrganizationUsers({ signal }), setUserOptions)
    ])
  }

  useEffect(() => {
    const controller = new AbortController()
    const request = {}
    listRequestRef.current = request

    async function run() {
      setIsListLoading(true)
      try {
        const [applied] = await Promise.all([
          loadTasks(initialSearch, initialStatusFilter, initialEntityTypeFilter, initialEntityIdFilter, initialAssigneeFilter, { request, signal: controller.signal }, initialDueView),
          loadOptions({ signal: controller.signal })
        ])
        if (applied) setError('')
      } catch (loadError) {
        if (!controller.signal.aborted && !isAbortError(loadError) && listRequestRef.current === request) {
          setError(loadError.message || 'Unable to load tasks.')
        }
      } finally {
        if (listRequestRef.current === request) setIsListLoading(false)
      }
    }

    run()
    return () => {
      controller.abort()
      listControllerRef.current?.abort()
      listRequestRef.current = null
    }
  }, [])

  async function reloadTasks(nextSearch = search, nextStatus = statusFilter, nextEntityTypeFilter = entityTypeFilter, nextEntityIdFilter = entityIdFilter, nextAssigneeFilter = assigneeFilter, nextDueView = dueView) {
    listControllerRef.current?.abort()
    const controller = new AbortController()
    const request = {}
    listControllerRef.current = controller
    listRequestRef.current = request
    setIsListLoading(true)
    try {
      const applied = await loadTasks(nextSearch, nextStatus, nextEntityTypeFilter, nextEntityIdFilter, nextAssigneeFilter, { request, signal: controller.signal }, nextDueView)
      if (applied) setError('')
    } catch (loadError) {
      if (!isAbortError(loadError) && listRequestRef.current === request) {
        setError(loadError.message || 'Unable to load tasks.')
      }
    } finally {
      if (listRequestRef.current === request) setIsListLoading(false)
      if (listControllerRef.current === controller) listControllerRef.current = null
    }
  }

  function applyFilters({
    assignee = assigneeFilter,
    due = dueView,
    entityId = entityIdFilter,
    entityType = entityTypeFilter,
    search: nextSearch = search,
    status = statusFilter
  }, { clearSelectedTask, taskId = routeTaskId, updatePath = true } = {}) {
    setSearch(nextSearch)
    setStatusFilter(status)
    setDueView(due)
    setAssigneeFilter(assignee)
    setEntityTypeFilter(entityType)
    setEntityIdFilter(entityId)
    clearSelectedTask?.()
    if (updatePath) navigate(taskDirectoryPath({ assignee, due, entityId, entityType, search: nextSearch, status, taskId }), { replace: true })
    return reloadTasks(nextSearch, status, entityType, entityId, assignee, due)
  }

  useEffect(() => {
    const alreadySynchronized = search === initialSearch && statusFilter === initialStatusFilter &&
      dueView === initialDueView && assigneeFilter === initialAssigneeFilter &&
      entityTypeFilter === initialEntityTypeFilter && entityIdFilter === initialEntityIdFilter
    if (alreadySynchronized) return

    applyFilters({
      assignee: initialAssigneeFilter,
      due: initialDueView,
      entityId: initialEntityIdFilter,
      entityType: initialEntityTypeFilter,
      search: initialSearch,
      status: initialStatusFilter
    }, { updatePath: false })
  }, [initialAssigneeFilter, initialDueView, initialEntityIdFilter, initialEntityTypeFilter, initialSearch, initialStatusFilter])

  function handleSearchChange(event) {
    return applyFilters({ search: event.target.value })
  }

  function handleToggleStatus(status) {
    return applyFilters({ status, due: status === 'open' ? dueView : 'all' })
  }

  function handleAssigneeFilterChange(assignee) {
    return applyFilters({ assignee })
  }

  function handleEntityTypeFilterChange(entityType) {
    return applyFilters({ entityType, entityId: entityType === entityTypeFilter ? entityIdFilter : '' })
  }

  function handleDueViewChange(due) {
    return applyFilters({ due })
  }

  function handleEntityIdFilterChange(entityId) {
    return applyFilters({ entityId })
  }

  function handleApplySavedView(filters, clearSelectedTask) {
    const status = normalizeTaskStatusFilter(filters.status)
    const entityType = normalizeEntityTypeFilter(filters.entityType)
    return applyFilters({
      assignee: normalizeAssigneeFilter(filters.assignee),
      due: status === 'open' ? normalizeDueView(filters.due) : 'all',
      entityId: entityType === 'all' ? '' : normalizeEntityIdFilter(filters.entityId),
      entityType,
      search: filters.q || '',
      status
    }, { clearSelectedTask, taskId: null })
  }

  function handleReset() {
    return applyFilters({ assignee: 'all', due: 'all', entityId: '', entityType: 'all', search: '', status: 'open' }, { taskId: null })
  }

  return {
    assigneeFilter,
    buildTasksPath,
    companyOptions,
    contactOptions,
    dealOptions,
    dueView,
    entityIdFilter,
    entityTypeFilter,
    error,
    handleApplySavedView,
    handleAssigneeFilterChange,
    handleDueViewChange,
    handleEntityIdFilterChange,
    handleEntityTypeFilterChange,
    handleReset,
    handleSearchChange,
    handleToggleStatus,
    isListLoading,
    meta,
    reloadTasks,
    search,
    selectedTaskIds,
    setError,
    setMeta,
    setSelectedTaskIds,
    setTasks,
    statusFilter,
    tasks,
    userOptions
  }
}
