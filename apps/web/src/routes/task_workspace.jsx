import { ActivityTimeline } from '../components/ui/activity_timeline'
import { Card } from '../components/ui/card'
import { TaskForm } from './task_form'

export function TaskCreateWorkspace({ companyOptions, contactOptions, dealOptions, form, isSaving, labels, onSetForm, onSubmit, userOptions }) {
  return (
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
          isSubmitting={isSaving}
          labels={labels}
          onSetForm={onSetForm}
          onSubmit={onSubmit}
          showEntityFields
          submitLabel="Save task"
          userOptions={userOptions}
        />
      </div>
    </Card>
  )
}

export function TaskWorkspace({ activities, canWrite, form, isLoading, isSaving, labels, onArchive, onSetForm, onSubmit, task, userOptions }) {
  return (
    <Card>
      <div className="card-stack">
        {isLoading ? <p className="field-hint">Loading task detail...</p> : null}
        <div className="section-header">
          <div>
            <h2>{task.title}</h2>
            <p>{task.entityLabel || `${task.entityType} #${task.entityId}`}</p>
          </div>
        </div>
        <TaskForm
          canArchive={canWrite}
          canSubmit={canWrite}
          form={form}
          isSubmitting={isSaving}
          labels={labels}
          onArchive={onArchive}
          onSetForm={onSetForm}
          onSubmit={onSubmit}
          showStatusFields
          submitLabel="Update task"
          userOptions={userOptions}
        />
        <Card>
          <div className="card-stack">
            <h3>Activity</h3>
            <ActivityTimeline activities={activities} emptyMessage="No task activity yet." ariaLabel={labels.activityAria} />
          </div>
        </Card>
      </div>
    </Card>
  )
}
