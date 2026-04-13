import { useEffect, useMemo, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { createTask, listTasks, updateTask } from '../lib/tasks'
import { listDeals } from '../lib/deals'
import { listCompanies } from '../lib/companies'
import { listContacts } from '../lib/contacts'
import { listOrganizationUsers } from '../lib/users'
import { useAuth } from '../app/providers'

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

export function TasksRoute() {
  const { session, businessProfile } = useAuth()
  const businessType = businessProfile?.businessType || session?.organization?.businessType || 'general'
  const labels = taskLabels(businessType)
  const [tasks, setTasks] = useState([])
  const [meta, setMeta] = useState({ page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0 })
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('open')
  const [dueView, setDueView] = useState('all')
  const [assigneeFilter, setAssigneeFilter] = useState('all')
  const [entityTypeFilter, setEntityTypeFilter] = useState('all')
  const [selectedTaskId, setSelectedTaskId] = useState(null)
  const [detail, setDetail] = useState(null)
  const [detailCache, setDetailCache] = useState({})
  const [dealOptions, setDealOptions] = useState([])
  const [companyOptions, setCompanyOptions] = useState([])
  const [contactOptions, setContactOptions] = useState([])
  const [userOptions, setUserOptions] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [error, setError] = useState('')

  const selectedTask = detail?.task || null
  const selectedActivities = detail?.activities || []
  const visibleTasks = useMemo(() => {
    const filteredTasks = tasks.filter((task) => matchesAssignee(task, assigneeFilter) && matchesEntityType(task, entityTypeFilter))

    if (statusFilter !== 'open') {
      return sortCompletedTasks(filteredTasks)
    }

    return sortOpenTasks(filteredTasks.filter((task) => matchesDueView(task, dueView)))
  }, [assigneeFilter, dueView, entityTypeFilter, statusFilter, tasks])

  async function loadTasks(nextSearch = search, nextStatus = statusFilter) {
    const data = await listTasks({ search: nextSearch, status: nextStatus })
    setTasks(data.tasks || [])
    setMeta(data.meta || { page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0 })
  }

  async function loadDealOptions() {
    const data = await listDeals()
    const nextDeals = data.deals || []
    setDealOptions(nextDeals)
    setForm((current) => {
      if (current.entityType !== 'deal' || current.entityId || nextDeals.length === 0) {
        return current
      }
      return { ...current, entityId: String(nextDeals[0].id) }
    })
  }

  async function loadCompanyOptions() {
    const data = await listCompanies()
    const nextCompanies = data.companies || []
    setCompanyOptions(nextCompanies)
    setForm((current) => {
      if (current.entityType !== 'company' || current.entityId || nextCompanies.length === 0) {
        return current
      }
      return { ...current, entityId: String(nextCompanies[0].id) }
    })
  }

  async function loadContactOptions() {
    const data = await listContacts()
    const nextContacts = data.contacts || []
    setContactOptions(nextContacts)
    setForm((current) => {
      if (current.entityType !== 'contact' || current.entityId || nextContacts.length === 0) {
        return current
      }
      return { ...current, entityId: String(nextContacts[0].id) }
    })
  }

  async function loadUserOptions() {
    const nextUsers = await listOrganizationUsers()
    setUserOptions(nextUsers)
    setForm((current) => {
      if (current.assignedToUserId || nextUsers.length === 0) {
        return current
      }
      return { ...current, assignedToUserId: String(nextUsers[0].id) }
    })
  }

  useEffect(() => {
    let cancelled = false

    async function run() {
      try {
        await Promise.all([loadTasks('', 'open'), loadDealOptions(), loadCompanyOptions(), loadContactOptions(), loadUserOptions()])
        if (!cancelled) {
          setError('')
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(loadError.message || 'Unable to load tasks.')
        }
      }
    }

    run()
    return () => {
      cancelled = true
    }
  }, [])

  async function handleSearchChange(event) {
    const value = event.target.value
    setSearch(value)
    try {
      await loadTasks(value, statusFilter)
      if (statusFilter === 'open') {
        setDueView('all')
      }
      setError('')
    } catch (loadError) {
      setError(loadError.message || 'Unable to load tasks.')
    }
  }

  async function handleToggleStatus(nextStatus) {
    setStatusFilter(nextStatus)
    try {
      await loadTasks(search, nextStatus)
      setError('')
    } catch (loadError) {
      setError(loadError.message || 'Unable to load tasks.')
    }
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
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to update task.')
    }
  }

  async function handleOpenTask(task) {
    const cached = detailCache[task.id]
    if (cached) {
      syncTaskIntoState(cached.task, cached.activities || [])
      return
    }

    syncTaskIntoState(task, [])
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
            <input className="text-input" value={search} onChange={handleSearchChange} />
          </Field>
          <Field label="Assignee">
            <select className="text-input" value={assigneeFilter} onChange={(event) => setAssigneeFilter(event.target.value)}>
              <option value="all">All assignees</option>
              <option value={unassignedAssigneeFilter}>Unassigned</option>
              {userOptions.map((user) => (
                <option key={user.id} value={user.id}>{`${user.firstName} ${user.lastName}`.trim() || user.email}</option>
              ))}
            </select>
          </Field>
          <Field label={labels.entityTypeFilterLabel}>
            <select className="text-input" value={entityTypeFilter} onChange={(event) => setEntityTypeFilter(event.target.value)}>
              <option value="all">All record types</option>
              <option value="deal">{labels.dealOption}</option>
              <option value="company">{labels.companyLabel}</option>
              <option value="contact">Contact</option>
            </select>
          </Field>
          {statusFilter === 'open' ? (
            <Field label={labels.viewLabel}>
              <select className="text-input" value={dueView} onChange={(event) => setDueView(event.target.value)}>
                <option value="all">All open</option>
                <option value="overdue">Overdue</option>
                <option value="dueToday">Due today</option>
                <option value="upcoming">Upcoming</option>
                <option value="noDueDate">No due date</option>
              </select>
            </Field>
          ) : null}
          {error ? <p className="form-error">{error}</p> : null}
          <h3>{summaryLabel}</h3>
          <p className="field-hint">Showing {visibleTasks.length} of {tasks.length} {taskCountLabel(statusFilter, dueView, labels)}.</p>
          <div className="record-list" role="list" aria-label="Tasks list">
            {visibleTasks.map((task) => (
              <article className="record-row" key={task.id} role="listitem">
                <div>
                  <button className="button button-ghost contact-link" type="button" onClick={() => handleOpenTask(task)}>
                    {task.title}
                  </button>
                  <p>{task.entityLabel || `${task.entityType} #${task.entityId}`}</p>
                </div>
                <div>
                  <p>{task.assignedToUserName || 'Unassigned'}</p>
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
            </form>
            <Card>
              <div className="card-stack">
                <h3>Activity</h3>
                <div className="record-list" role="list" aria-label={labels.activityAria}>
                  {selectedActivities.map((activity) => (
                    <article className="record-row" key={activity.id} role="listitem">
                      <div>
                        <p>{activity.summary}</p>
                      </div>
                    </article>
                  ))}
                </div>
              </div>
            </Card>
          </div>
        </Card>
      ) : null}
    </section>
  )
}
