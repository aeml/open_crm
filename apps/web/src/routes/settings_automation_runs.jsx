import { Link } from 'react-router-dom'
import { formatRunTime } from './settings_automation_task_model'

function actionStatusText(action) {
  const attempts = Number(action.attempts) || 0
  const attemptText = attempts > 0 ? ` · ${attempts} ${attempts === 1 ? 'attempt' : 'attempts'}` : ''
  return `Action ${action.status}${attemptText}`
}

function RunActionOutcome({ action }) {
  return (
    <li className="card-stack">
      <div className="automation-action-header">
        <p><strong>{action.position}. {action.label}</strong></p>
        <span className="chip">{actionStatusText(action)}</span>
      </div>
      {action.status === 'queued' && action.scheduledAt ? <p className="field-hint">Scheduled for {formatRunTime(action.scheduledAt)}</p> : null}
      {action.approval ? <p className="field-hint">Approval {action.approval.status} · {action.approval.approverRole.replace('_', ' ')}{action.approval.decidedAt ? ` · decided ${formatRunTime(action.approval.decidedAt)}` : ''}</p> : null}
      {action.approval?.decisionNote ? <p className="field-hint">Decision note: {action.approval.decisionNote}</p> : null}
      {action.taskDueAt ? <p className="field-hint">Task due {formatRunTime(action.taskDueAt)}</p> : null}
      {action.notificationCount ? <p className="field-hint">Delivered to {action.notificationCount} eligible {action.notificationCount === 1 ? 'teammate' : 'teammates'}.</p> : null}
      {action.lastError ? <p>Action issue: {action.lastError}</p> : null}
      {action.taskId ? <Link to={`/tasks/${action.taskId}`}>Open created task</Link> : null}
    </li>
  )
}

export function SettingsAutomationRuns({ canManage, isLoading, runs }) {
  return (
    <>
      <div>
        <h3>Recent automation runs</h3>
        <p className="field-hint">Each supported action retains its own schedule, attempts, outcome, and bounded result evidence.</p>
      </div>
      <div className="record-list" role="list" aria-label="Task automation runs">
        {!isLoading && runs.length === 0 ? (
          <article className="record-row" role="listitem"><p>No task automation runs yet.</p></article>
        ) : runs.map((run) => (
          <article className={run.status === 'failed' ? 'record-row record-row-alert' : 'record-row'} key={run.id} role="listitem">
            <div>
              <p>{run.automationName}</p>
              <p className="field-hint">{formatRunTime(run.createdAt)} · {run.actionsCompleted ?? 0}/{run.actionsTotal ?? 0} actions completed</p>
              {run.causalDepth > 0 ? <p className="field-hint">Nested depth {run.causalDepth} · caused by run #{run.causationRunId}, action {run.causationActionPosition}.</p> : <p className="field-hint">Root event · no workflow action caused this run.</p>}
              {['Automation re-entry prevented.', 'Workflow causal depth limit reached.'].includes(run.triggerPayload?.skipReason) ? <p className="field-hint">Loop guard: {run.triggerPayload.skipReason}</p> : null}
              {run.status === 'waiting_approval' ? <p className="field-hint">Paused safely until an eligible teammate decides the retained approval.</p> : null}
              {run.status === 'queued' && run.scheduledAt ? <p className="field-hint">Scheduled for {formatRunTime(run.scheduledAt)}</p> : null}
              {run.operation ? <p className="field-hint">Durable attempt {run.operation.attempts} of {run.operation.maxAttempts} · {run.operation.status}</p> : null}
              <p>{run.lastError || run.triggerEventKey}</p>
              {run.actions?.length ? (
                <details className="workflow-run-details">
                  <summary>Inspect {run.actions.length} action {run.actions.length === 1 ? 'outcome' : 'outcomes'}</summary>
                  <ol className="automation-action-outcomes card-stack" aria-label={`${run.automationName} run actions`}>
                    {run.actions.map((action) => <RunActionOutcome action={action} key={action.id || action.position} />)}
                  </ol>
                </details>
              ) : null}
            </div>
            <div>
              <span className="chip">{run.status}</span>
              {run.operation?.status === 'dead' && canManage ? <a className="button button-secondary" href="/settings/operations">Review and replay in Operations</a> : run.operation?.status === 'dead' ? <span className="field-hint">Admin replay required</span> : null}
            </div>
          </article>
        ))}
      </div>
    </>
  )
}
