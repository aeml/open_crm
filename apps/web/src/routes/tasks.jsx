import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { archiveTask, createTask, listTasks, updateTask } from '../lib/tasks'
import { listDeals } from '../lib/deals'
import { listCompanies } from '../lib/companies'
import { listContacts } from '../lib/contacts'
import { listOrganizationUsers } from '../lib/users'
import { isAbortError } from '../lib/api'
import { useAuth } from '../app/providers'
import { usePageTitle } from '../lib/use_page_title'
import { TaskDirectory } from './task_directory'
import { TaskCreateWorkspace, TaskWorkspace } from './task_workspace'
import {
  emptyTaskListDescription,
  emptyTaskListMessage,
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
  taskLabels,
  unassignedAssigneeFilter
} from './task_view'
import { useTaskDetail } from './use_task_detail'
import { useTaskQuickActions } from './use_task_quick_actions'

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
  const [dealOptions, setDealOptions] = useState([])
  const [companyOptions, setCompanyOptions] = useState([])
  const [contactOptions, setContactOptions] = useState([])
  const [userOptions, setUserOptions] = useState([])
  const [error, setError] = useState('')
  const [isListLoading, setIsListLoading] = useState(true)
  const listControllerRef = useRef(null)
  const {
    applyExternalUpdate: applyExternalTaskUpdate,
    clear: clearSelectedTask,
    detail,
    form,
    isDetailLoading,
    isLoadingOlderActivities,
    isSaving: isSavingTask,
    open: openTaskDetail,
    loadOlderActivities,
    removeCached: removeCachedTask,
    selectedTaskId,
    setForm,
    setIsSaving: setIsSavingTask,
    sync: syncTaskIntoState,
    visitRef: taskVisitRef
  } = useTaskDetail({ isListLoading, routeTaskId, setError, setTasks })
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
    }
  }, [])

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
      syncTaskIntoState(data.task, data.activities || [], data.activityMeta)
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
      syncTaskIntoState(data.task, data.activities || [], data.activityMeta)
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
    openTaskDetail(task)
    navigate(buildTasksPath(task.id))
  }

  function handleQuickTaskUpdated(previousTask, data) {
    const nextTask = data.task
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
    applyExternalTaskUpdate(nextTask, data.activities || [], data.activityMeta)
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
      removeCachedTask(selectedTaskId)
      clearSelectedTask()
      navigate(buildTasksPath(null))
      setError('')
    } catch (archiveError) {
      setError(archiveError.message || 'Unable to archive task.')
    }
  }

  return (
    <section className="dashboard-grid contacts-grid">
      <TaskDirectory
        assigneeFilter={assigneeFilter}
        canWrite={canWrite}
        companyOptions={companyOptions}
        contactOptions={contactOptions}
        currentUserId={currentUserId}
        dealOptions={dealOptions}
        dueView={dueView}
        emptyDescription={emptyDescription}
        emptyMessage={emptyMessage}
        entityIdFilter={entityIdFilter}
        entityTypeFilter={entityTypeFilter}
        error={error}
        isListLoading={isListLoading}
        isTaskPending={isTaskPending}
        labels={labels}
        meta={meta}
        onApplySavedView={handleApplySavedView}
        onAssigneeFilterChange={handleAssigneeFilterChange}
        onBulkChanged={() => reloadTasks(search, statusFilter, entityTypeFilter, entityIdFilter, assigneeFilter)}
        onDueViewChange={handleDueViewChange}
        onEntityIdFilterChange={handleEntityIdFilterChange}
        onEntityTypeFilterChange={handleEntityTypeFilterChange}
        onOpenTask={handleOpenTask}
        onQuickAssign={handleQuickAssign}
        onQuickComplete={handleQuickComplete}
        onQuickReopen={handleQuickReopen}
        onReset={() => {
          setSearch('')
          setStatusFilter('open')
          setDueView('all')
          setAssigneeFilter('all')
          setEntityTypeFilter('all')
          setEntityIdFilter('')
          navigate(buildTasksPath(null, '', 'open', 'all', 'all', 'all', ''), { replace: true })
          reloadTasks('', 'open', 'all', '', 'all', 'all')
        }}
        onRetry={() => reloadTasks(search, statusFilter, entityTypeFilter, entityIdFilter, assigneeFilter)}
        onSearchChange={handleSearchChange}
        onSelectionChange={setSelectedTaskIds}
        onToggleStatus={handleToggleStatus}
        search={search}
        selectedTaskIds={selectedTaskIds}
        statusFilter={statusFilter}
        statusTasks={statusTasks}
        userOptions={userOptions}
        visibleTasks={visibleTasks}
      />

      {canWrite ? (
        <TaskCreateWorkspace
          companyOptions={companyOptions}
          contactOptions={contactOptions}
          dealOptions={dealOptions}
          form={form}
          isSaving={isSavingTask}
          labels={labels}
          onSetForm={setForm}
          onSubmit={handleCreate}
          userOptions={userOptions}
        />
      ) : null}

      {selectedTask ? (
        <TaskWorkspace
          activities={selectedActivities}
          activityMeta={detail?.activityMeta}
          canWrite={canWrite}
          form={form}
          isLoading={isDetailLoading}
          isLoadingOlderActivities={isLoadingOlderActivities}
          isSaving={isSavingTask}
          labels={labels}
          onArchive={handleArchive}
          onLoadOlderActivities={loadOlderActivities}
          onSetForm={setForm}
          onSubmit={handleUpdate}
          task={selectedTask}
          userOptions={userOptions}
        />
      ) : null}
    </section>
  )
}
