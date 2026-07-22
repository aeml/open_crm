import { useEffect, useMemo } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { archiveTask, createTask, updateTask } from '../lib/tasks'
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
  taskLabels
} from './task_view'
import { useTaskDetail } from './use_task_detail'
import { useTaskDirectory } from './use_task_directory'
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
  const {
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
  } = useTaskDirectory({
    initialAssigneeFilter,
    initialDueView,
    initialEntityIdFilter,
    initialEntityTypeFilter,
    initialSearch,
    initialStatusFilter,
    navigate,
    routeTaskId
  })
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

  useEffect(() => {
    setForm((current) => {
      let entityId = current.entityId
      if (!entityId && current.entityType === 'deal' && dealOptions[0]) entityId = String(dealOptions[0].id)
      if (!entityId && current.entityType === 'company' && companyOptions[0]) entityId = String(companyOptions[0].id)
      if (!entityId && current.entityType === 'contact' && contactOptions[0]) entityId = String(contactOptions[0].id)
      const assignedToUserId = current.assignedToUserId || (userOptions[0] ? String(userOptions[0].id) : '')
      if (entityId === current.entityId && assignedToUserId === current.assignedToUserId) return current
      return { ...current, entityId, assignedToUserId }
    })
  }, [companyOptions, contactOptions, dealOptions, userOptions])

  const selectedTask = detail?.task || null
  const selectedActivities = detail?.activities || []
  const statusTasks = useMemo(() => tasks.filter((task) => matchesStatus(task, statusFilter)), [statusFilter, tasks])
  const visibleTasks = useMemo(() => {
    const filteredTasks = statusTasks.filter((task) => matchesAssignee(task, assigneeFilter) &&
      matchesEntityType(task, entityTypeFilter) && (!entityIdFilter || String(task.entityId || '') === entityIdFilter))
    return statusFilter === 'open' ? sortOpenTasks(filteredTasks) : sortCompletedTasks(filteredTasks)
  }, [assigneeFilter, entityIdFilter, entityTypeFilter, statusFilter, statusTasks])
  const hasFilteredTasks = statusTasks.length > 0 || assigneeFilter !== 'all'
  const emptyMessage = useMemo(() => emptyTaskListMessage(statusFilter, dueView, labels, hasFilteredTasks), [dueView, hasFilteredTasks, labels, statusFilter])
  const emptyDescription = useMemo(() => emptyTaskListDescription(statusFilter, dueView, labels, hasFilteredTasks), [dueView, hasFilteredTasks, labels, statusFilter])

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
      setTasks((current) => [data.task, ...current.filter((task) => task.id !== data.task.id)])
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

  function handleOpenTask(task) {
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
    if (!selectedTaskId) return
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
        canExport={['owner', 'admin'].includes(session?.membership?.role || '')}
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
        onApplySavedView={(filters) => handleApplySavedView(filters, clearSelectedTask)}
        onAssigneeFilterChange={handleAssigneeFilterChange}
        onBulkChanged={() => reloadTasks(search, statusFilter, entityTypeFilter, entityIdFilter, assigneeFilter)}
        onDueViewChange={handleDueViewChange}
        onEntityIdFilterChange={handleEntityIdFilterChange}
        onEntityTypeFilterChange={handleEntityTypeFilterChange}
        onOpenTask={handleOpenTask}
        onQuickAssign={handleQuickAssign}
        onQuickComplete={handleQuickComplete}
        onQuickReopen={handleQuickReopen}
        onReset={handleReset}
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
