import { BulkActions, bulkStatusOptions } from '../components/ui/bulk_actions'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { CRMExportActions } from '../components/ui/crm_export_actions'
import { EmptyState } from '../components/ui/empty_state'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { SavedViews } from '../components/ui/saved_views'
import { tasksExportURL } from '../lib/tasks'
import { crmExportSetupURL } from '../lib/crm_exports'
import { formatDueLabel, taskCountLabel, taskListHeading, unassignedAssigneeFilter } from './task_view'

export function TaskDirectory({
  assigneeFilter,
  canWrite,
  canExport,
  companyOptions,
  contactOptions,
  currentUserId,
  dealOptions,
  dueView,
  emptyDescription,
  emptyMessage,
  entityIdFilter,
  entityTypeFilter,
  error,
  isListLoading,
  isTaskPending,
  labels,
  meta,
  onApplySavedView,
  onAssigneeFilterChange,
  onBulkChanged,
  onDueViewChange,
  onEntityIdFilterChange,
  onEntityTypeFilterChange,
  onOpenTask,
  onQuickAssign,
  onQuickComplete,
  onQuickReopen,
  onReset,
  onRetry,
  onSearchChange,
  onSelectionChange,
  onToggleStatus,
  search,
  selectedTaskIds,
  statusFilter,
  statusTasks,
  userOptions,
  visibleTasks
}) {
  const entityOptions = entityTypeFilter === 'deal' ? dealOptions : entityTypeFilter === 'company' ? companyOptions : contactOptions
  const hasResettableFilter = search.trim() || assigneeFilter !== 'all' || entityTypeFilter !== 'all' || entityIdFilter || dueView !== 'all' || statusFilter !== 'open'

  return (
    <Card>
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h2>{labels.collection}</h2>
            <p>Keep the next real action visible and close it cleanly.</p>
          </div>
          <div className="button-row">
            <CRMExportActions canExport={canExport} directURL={tasksExportURL({ search, status: statusFilter, due: statusFilter === 'open' ? dueView : '', assignee: assigneeFilter === 'all' ? '' : assigneeFilter, entityType: entityTypeFilter === 'all' ? '' : entityTypeFilter, entityId: entityIdFilter })} durableURL={crmExportSetupURL({ resource: 'tasks', search, status: statusFilter, due: statusFilter === 'open' ? dueView : '', assignee: assigneeFilter === 'all' ? '' : assigneeFilter, entityType: entityTypeFilter === 'all' ? '' : entityTypeFilter, entityId: Number(entityIdFilter) || 0 })} />
            <Button className={statusFilter === 'open' ? '' : 'button-secondary'} onClick={() => onToggleStatus('open')}>Show open</Button>
            <Button className={statusFilter === 'completed' ? '' : 'button-secondary'} onClick={() => onToggleStatus('completed')}>Show completed</Button>
          </div>
        </div>
        <div className="record-list" role="list" aria-label="Task summary list">
          <article className="record-row" role="listitem">
            <div><p>Open {labels.plural}</p></div>
            <div><p>{meta.openCount}</p></div>
          </article>
          <article className="record-row" role="listitem">
            <div><p>Completed {labels.plural}</p></div>
            <div><p>{meta.completedCount}</p></div>
          </article>
        </div>
        <Field label={`Search ${labels.plural}`}>
          <input className="text-input" type="search" value={search} onChange={onSearchChange} />
        </Field>
        <SavedViews
          entityType="tasks"
          canManage={canWrite}
          currentFilters={{ q: search, status: statusFilter, due: dueView, assignee: assigneeFilter, entityType: entityTypeFilter, entityId: entityIdFilter }}
          onApply={onApplySavedView}
          defaultName={`${labels.collection} view`}
        />
        <Field label="Assignee">
          <div className="button-row">
            <select className="text-input" value={assigneeFilter} onChange={(event) => onAssigneeFilterChange(event.target.value)}>
              <option value="all">All assignees</option>
              <option value={unassignedAssigneeFilter}>Unassigned</option>
              {userOptions.map((user) => (
                <option key={user.id} value={user.id}>{`${user.firstName} ${user.lastName}`.trim() || user.email}</option>
              ))}
            </select>
            {currentUserId ? (
              <Button className={assigneeFilter === currentUserId ? '' : 'button-secondary'} type="button" onClick={() => onAssigneeFilterChange(currentUserId)}>
                My tasks
              </Button>
            ) : null}
            <Button className={assigneeFilter === unassignedAssigneeFilter ? '' : 'button-secondary'} type="button" onClick={() => onAssigneeFilterChange(unassignedAssigneeFilter)}>
              Unassigned
            </Button>
          </div>
        </Field>
        <Field label={labels.entityTypeFilterLabel}>
          <select className="text-input" value={entityTypeFilter} onChange={(event) => onEntityTypeFilterChange(event.target.value)}>
            <option value="all">All record types</option>
            <option value="deal">{labels.dealOption}</option>
            <option value="company">{labels.companyLabel}</option>
            <option value="contact">Contact</option>
          </select>
        </Field>
        {entityTypeFilter !== 'all' ? (
          <Field label="Record">
            <select className="text-input" value={entityIdFilter} onChange={(event) => onEntityIdFilterChange(event.target.value)}>
              <option value="">All {entityTypeFilter === 'deal' ? `${labels.dealOption.toLowerCase()}s` : entityTypeFilter === 'company' ? `${labels.companyLabel.toLowerCase()}s` : 'contacts'}</option>
              {entityOptions.map((entity) => (
                <option key={entity.id} value={entity.id}>
                  {entityTypeFilter === 'contact' ? `${entity.firstName || ''} ${entity.lastName || ''}`.trim() : entity.name}
                </option>
              ))}
            </select>
          </Field>
        ) : null}
        {statusFilter === 'open' ? (
          <Field label={`${labels.titleNoun} view`} hint="Due soon is the next 24 hours.">
            <select className="text-input" value={dueView} onChange={(event) => onDueViewChange(event.target.value)}>
              <option value="all">All open</option>
              <option value="overdue">Overdue</option>
              <option value="dueSoon">Due within 24 hours</option>
              <option value="upcoming">Later</option>
              <option value="noDueDate">No due date</option>
            </select>
            <p className="field-hint">Overdue {meta.overdueCount || 0} · Due soon {meta.dueSoonCount || 0}</p>
          </Field>
        ) : null}
        {isListLoading ? <p className="field-hint">Loading {labels.plural}...</p> : null}
        {error ? (
          <InlineError message={error} onRetry={onRetry} retryLabel={`Retry ${labels.plural}`} />
        ) : null}
        <h3>{taskListHeading(statusFilter, dueView, labels)}</h3>
        <p className="field-hint">Showing {visibleTasks.length} of {statusTasks.length} {taskCountLabel(statusFilter, dueView, labels)}.</p>
        {canWrite ? (
          <BulkActions
            entityType="task"
            selectedIds={selectedTaskIds}
            visibleIds={visibleTasks.map((task) => task.id)}
            onSelectionChange={onSelectionChange}
            onChanged={onBulkChanged}
            statuses={bulkStatusOptions.task}
            userOptions={userOptions}
          />
        ) : null}
        <div className="record-list" role="list" aria-label="Tasks list">
          {visibleTasks.length === 0 && (!isListLoading || statusTasks.length > 0) ? (
            <EmptyState
              title={emptyMessage}
              description={emptyDescription}
              actionLabel={hasResettableFilter ? 'Reset task view' : ''}
              onAction={onReset}
            />
          ) : visibleTasks.map((task) => (
            <article className="record-row" key={task.id} role="listitem">
              <div>
                {canWrite ? (
                  <input
                    className="record-select-checkbox"
                    type="checkbox"
                    aria-label={`Select ${task.title}`}
                    checked={selectedTaskIds.includes(task.id)}
                    onChange={() => onSelectionChange((current) => current.includes(task.id) ? current.filter((id) => id !== task.id) : [...current, task.id])}
                  />
                ) : null}
                <button className="button button-ghost contact-link" type="button" onClick={() => onOpenTask(task)}>
                  {task.title}
                </button>
                <p>{task.entityLabel || `${task.entityType} #${task.entityId}`}</p>
                {canWrite ? statusFilter === 'open' ? (
                  <Button className="button-secondary" type="button" disabled={isTaskPending(task.id)} onClick={() => onQuickComplete(task)} aria-label={`Complete ${task.title}`}>
                    {isTaskPending(task.id) ? 'Saving…' : 'Complete'}
                  </Button>
                ) : (
                  <Button className="button-secondary" type="button" disabled={isTaskPending(task.id)} onClick={() => onQuickReopen(task)} aria-label={`Reopen ${task.title}`}>
                    {isTaskPending(task.id) ? 'Saving…' : 'Reopen'}
                  </Button>
                ) : null}
              </div>
              <div>
                {canWrite ? (
                  <select className="text-input" aria-label={`Assign ${task.title}`} disabled={isTaskPending(task.id)} value={task.assignedToUserId ? String(task.assignedToUserId) : ''} onChange={(event) => onQuickAssign(task, event.target.value)}>
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
  )
}
