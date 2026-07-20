import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { EmptyState } from '../components/ui/empty_state'
import { SavedViews } from '../components/ui/saved_views'
import { ActivityTimeline } from '../components/ui/activity_timeline'
import { InlineError } from '../components/ui/inline_error'
import { BulkActions, bulkStatusOptions } from '../components/ui/bulk_actions'
import { archiveTask, createTask, getTask, listTasks, tasksExportURL, updateTask } from '../lib/tasks'
import { listDeals } from '../lib/deals'
import { listCompanies } from '../lib/companies'
import { listContacts } from '../lib/contacts'
import { listOrganizationUsers } from '../lib/users'
import { isAbortError } from '../lib/api'
import { useAuth } from '../app/providers'
import { usePageTitle } from '../lib/use_page_title'
import { TaskForm } from './task_form'
import {
  emptyTaskListDescription,
  emptyTaskListMessage,
  formatDueLabel,
  matchesAssignee,
  matchesEntityType,
  matchesStatus,
  normalizeAssigneeFilter,
  normalizeDueView,
  normalizeEntityIdFilter,
  normalizeEntityTypeFilter,
  normalizeTaskStatusFilter,
  sortCompletedTasks,
  sortOpenTasks,
  taskFormValues,
  taskCountLabel,
  taskLabels,
  taskListHeading,
  unassignedAssigneeFilter
} from './task_view'
import { useTaskQuickActions } from './use_task_quick_actions'

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


export function TasksRoute() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { taskId } = useParams()
  const routeTaskId = Number.parseInt(taskId || '', 10)
  const { session, businessProfile, canWrite } = useAuth()
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
  const [selectedTaskIds, setSelectedTaskIds] = useState([])
  const [meta, setMeta] = useState({ page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0, overdueCount: 0, dueSoonCount: 0, upcomingCount: 0, noDueDateCount: 0 })
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
  const [isSavingTask, setIsSavingTask] = useState(false)
  const listControllerRef = useRef(null)
  const taskVisitRef = useRef(null)
  const { handleQuickAssign, handleQuickComplete, handleQuickReopen, isTaskPending } = useTaskQuickActions({ onUpdated: handleQuickTaskUpdated, onError: setError })

  const selectedTask = detail?.task || null
  const selectedActivities = detail?.activities || []
  const statusTasks = useMemo(() => tasks.filter((task) => matchesStatus(task, statusFilter)), [statusFilter, tasks])
  const visibleTasks = useMemo(() => {
    const filteredTasks = statusTasks.filter((task) => {
      if (!matchesAssignee(task, assigneeFilter)) {
        return false
      }

      if (!matchesEntityType(task, entityTypeFilter)) {
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

    return sortOpenTasks(filteredTasks)
  }, [assigneeFilter, dueView, entityIdFilter, entityTypeFilter, statusFilter, statusTasks])
  const hasFilteredTasks = statusTasks.length > 0 || assigneeFilter !== 'all'
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

  async function loadTasks(nextSearch = search, nextStatus = statusFilter, nextEntityTypeFilter = entityTypeFilter, nextEntityIdFilter = entityIdFilter, nextAssigneeFilter = assigneeFilter, { signal } = {}, nextDueView = dueView) {
    const isUnassigned = nextAssigneeFilter === unassignedAssigneeFilter
    const assignedToUserId = isUnassigned ? 0 : (Number.parseInt(nextAssigneeFilter, 10) || 0)
    const data = await listTasks({
      search: nextSearch,
      status: nextStatus,
      entityType: nextEntityTypeFilter === 'all' ? '' : nextEntityTypeFilter,
      entityId: nextEntityTypeFilter === 'all' ? 0 : Number.parseInt(nextEntityIdFilter, 10) || 0,
      unassigned: isUnassigned,
      assignedToUserId,
      due: nextStatus === 'open' ? nextDueView : 'all'
    }, { signal })
    setTasks(data.tasks || [])
    setSelectedTaskIds([])
    setMeta(data.meta || { page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0, overdueCount: 0, dueSoonCount: 0, upcomingCount: 0, noDueDateCount: 0 })
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
          loadTasks(initialSearch, initialStatusFilter, initialEntityTypeFilter, initialEntityIdFilter, initialAssigneeFilter, { signal: controller.signal }, initialDueView),
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
      taskVisitRef.current = null
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

  async function reloadTasks(nextSearch = search, nextStatus = statusFilter, nextEntityTypeFilter = entityTypeFilter, nextEntityIdFilter = entityIdFilter, nextAssigneeFilter = assigneeFilter, nextDueView = dueView) {
    listControllerRef.current?.abort()
    const controller = new AbortController()
    listControllerRef.current = controller
    setIsListLoading(true)
    try {
      await loadTasks(nextSearch, nextStatus, nextEntityTypeFilter, nextEntityIdFilter, nextAssigneeFilter, { signal: controller.signal }, nextDueView)
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
    await reloadTasks(value, statusFilter, entityTypeFilter, entityIdFilter, assigneeFilter)
  }

  async function handleToggleStatus(nextStatus) {
    const nextDueView = nextStatus === 'open' ? dueView : 'all'
    setStatusFilter(nextStatus)
    setDueView(nextDueView)
    navigate(buildTasksPath(selectedTaskId, search, nextStatus, nextDueView, assigneeFilter, entityTypeFilter, entityIdFilter), { replace: true })
    await reloadTasks(search, nextStatus, entityTypeFilter, entityIdFilter, assigneeFilter, nextDueView)
  }

  async function handleAssigneeFilterChange(nextAssigneeFilter) {
    setAssigneeFilter(nextAssigneeFilter)
    navigate(buildTasksPath(selectedTaskId, search, statusFilter, dueView, nextAssigneeFilter, entityTypeFilter, entityIdFilter), { replace: true })
    await reloadTasks(search, statusFilter, entityTypeFilter, entityIdFilter, nextAssigneeFilter)
  }

  async function handleEntityTypeFilterChange(nextEntityTypeFilter) {
    const nextEntityIdFilter = nextEntityTypeFilter === entityTypeFilter ? entityIdFilter : ''
    setEntityTypeFilter(nextEntityTypeFilter)
    setEntityIdFilter(nextEntityIdFilter)
    navigate(buildTasksPath(selectedTaskId, search, statusFilter, dueView, assigneeFilter, nextEntityTypeFilter, nextEntityIdFilter), { replace: true })
    await reloadTasks(search, statusFilter, nextEntityTypeFilter, nextEntityIdFilter)
  }

  async function handleDueViewChange(nextDueView) {
    setDueView(nextDueView)
    navigate(buildTasksPath(selectedTaskId, search, statusFilter, nextDueView, assigneeFilter, entityTypeFilter, entityIdFilter), { replace: true })
    await reloadTasks(search, statusFilter, entityTypeFilter, entityIdFilter, assigneeFilter, nextDueView)
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
    await reloadTasks(nextSearch, nextStatus, nextEntityTypeFilter, nextEntityIdFilter, nextAssigneeFilter, nextDueView)
  }

  function syncTaskIntoState(task, activities) {
    taskVisitRef.current = { taskId: task.id, pending: false }
    setDetailCache((current) => ({ ...current, [task.id]: { task, activities } }))
    setDetail({ task, activities })
    setSelectedTaskId(task.id)
    setForm(taskFormValues(task))
  }

  function clearSelectedTask() {
    taskVisitRef.current = null
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
    const visit = taskVisitRef.current
    if (!selectedTaskId || visit?.taskId !== selectedTaskId || visit.pending) return

    visit.pending = true
    setIsSavingTask(true)
    try {
      const data = await updateTask(selectedTaskId, {
        title: form.title,
        description: form.description,
        status: form.status,
        dueAt: form.dueAt ? `${form.dueAt}:00Z` : '',
        completedAt: form.completedAt ? `${form.completedAt}:00Z` : '',
        assignedToUserId: Number.parseInt(form.assignedToUserId, 10) || 0
      })
      if (!data?.task?.id || data.task.id !== selectedTaskId) throw new Error('Unable to update task.')
      if (!taskVisitRef.current) return
      setTasks((current) => current.map((task) => (task.id === selectedTaskId ? data.task : task)))
      if (taskVisitRef.current !== visit) return
      setIsSavingTask(false)
      syncTaskIntoState(data.task, data.activities || [])
      navigate(buildTasksPath(data.task.id))
      setError('')
    } catch (saveError) {
      if (taskVisitRef.current === visit) setError(saveError.message || 'Unable to update task.')
    } finally {
      visit.pending = false
      if (taskVisitRef.current === visit) setIsSavingTask(false)
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

  function handleQuickTaskUpdated(previousTask, data) {
    const nextTask = data.task
    const nextDetail = { task: nextTask, activities: data.activities || [] }
    const wasCompleted = previousTask.status === 'completed'
    const isCompleted = nextTask.status === 'completed'
    setTasks((current) => current.map((task) => (task.id === nextTask.id ? nextTask : task)))
    if (wasCompleted !== isCompleted) {
      setMeta((current) => ({
        ...current,
        openCount: Math.max(0, current.openCount + (wasCompleted ? 1 : 0) - (isCompleted ? 1 : 0)),
        completedCount: Math.max(0, current.completedCount + (isCompleted ? 1 : 0) - (wasCompleted ? 1 : 0))
      }))
    }
    setDetailCache((current) => ({ ...current, [nextTask.id]: nextDetail }))
    setDetail((current) => {
      if (current?.task?.id !== nextTask.id) return current
      setForm(taskFormValues(nextTask))
      return nextDetail
    })
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
            canManage={canWrite}
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
            <Field label={labels.viewLabel} hint="Due soon is the next 24 hours.">
              <select className="text-input" value={dueView} onChange={(event) => handleDueViewChange(event.target.value)}>
                <option value="all">All open</option>
                <option value="overdue">Overdue</option>
                <option value="dueSoon">Due within 24 hours</option>
                <option value="upcoming">Later</option>
                <option value="noDueDate">No due date</option>
              </select>
              <p className="field-hint">Overdue {meta.overdueCount || 0} · Due soon {meta.dueSoonCount || 0}</p>
            </Field>
          ) : null}
          {isListLoading ? <p className="field-hint">Loading {labels.showingSuffix}...</p> : null}
          {error ? (
            <InlineError message={error} onRetry={() => reloadTasks(search, statusFilter, entityTypeFilter, entityIdFilter, assigneeFilter)} retryLabel={`Retry ${labels.showingSuffix}`} />
          ) : null}
          <h3>{summaryLabel}</h3>
          <p className="field-hint">Showing {visibleTasks.length} of {statusTasks.length} {taskCountLabel(statusFilter, dueView, labels)}.</p>
          {canWrite ? <BulkActions entityType="task" selectedIds={selectedTaskIds} visibleIds={visibleTasks.map((task) => task.id)} onSelectionChange={setSelectedTaskIds} onChanged={() => reloadTasks(search, statusFilter, entityTypeFilter, entityIdFilter, assigneeFilter)} statuses={bulkStatusOptions.task} userOptions={userOptions} /> : null}
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
                  reloadTasks('', 'open', 'all', '', 'all', 'all')
                }}
              />
            ) : visibleTasks.map((task) => (
              <article className="record-row" key={task.id} role="listitem">
                <div>
                  {canWrite ? <input type="checkbox" aria-label={`Select ${task.title}`} checked={selectedTaskIds.includes(task.id)} onChange={() => setSelectedTaskIds((current) => current.includes(task.id) ? current.filter((id) => id !== task.id) : [...current, task.id])} /> : null}
                  <button className="button button-ghost contact-link" type="button" onClick={() => handleOpenTask(task)}>
                    {task.title}
                  </button>
                  <p>{task.entityLabel || `${task.entityType} #${task.entityId}`}</p>
                  {statusFilter === 'open' ? (
                    canWrite ? (
                      <Button className="button-secondary" type="button" disabled={isTaskPending(task.id)} onClick={() => handleQuickComplete(task)} aria-label={`Complete ${task.title}`}>
                        {isTaskPending(task.id) ? 'Saving…' : 'Complete'}
                      </Button>
                    ) : null
                  ) : (
                    canWrite ? (
                      <Button className="button-secondary" type="button" disabled={isTaskPending(task.id)} onClick={() => handleQuickReopen(task)} aria-label={`Reopen ${task.title}`}>
                        {isTaskPending(task.id) ? 'Saving…' : 'Reopen'}
                      </Button>
                    ) : null
                  )}
                </div>
                <div>
                  {canWrite ? (
                    <select className="text-input" aria-label={`Assign ${task.title}`} disabled={isTaskPending(task.id)} value={task.assignedToUserId ? String(task.assignedToUserId) : ''} onChange={(event) => handleQuickAssign(task, event.target.value)}>
                      <option value="">Unassigned</option>
                      {userOptions.map((user) => (
                        <option key={user.id} value={user.id}>{`${user.firstName} ${user.lastName}`.trim() || user.email}</option>
                      ))}
                    </select>
                  ) : null}
                  <p>{formatDueLabel(task)}</p>
                </div>
              </article>
            ))}
          </div>
        </div>
      </Card>

      {canWrite ? (
        <Card>
          <div className="card-stack">
            <div>
              <h2>{labels.createHeading}</h2>
              <p>{labels.createDescription}</p>
            </div>
            <TaskForm
              companyOptions={companyOptions}
              contactOptions={contactOptions}
              dealOptions={dealOptions}
              form={form}
              isSubmitting={isSavingTask}
              labels={labels}
              onSetForm={setForm}
              onSubmit={handleCreate}
              showEntityFields
              submitLabel="Save task"
              userOptions={userOptions}
            />
          </div>
        </Card>
      ) : null}

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
            <TaskForm
              canArchive={canWrite}
              canSubmit={canWrite}
              form={form}
              isSubmitting={isSavingTask}
              labels={labels}
              onArchive={handleArchive}
              onSetForm={setForm}
              onSubmit={handleUpdate}
              showStatusFields
              submitLabel="Update task"
              userOptions={userOptions}
            />
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
