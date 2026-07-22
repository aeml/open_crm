import { useState } from 'react'
import { Button } from '../components/ui/button'
import { formatRunTime } from './settings_automation_task_model'

const roleLabels = {
  owner: 'workspace owner',
  admin: 'workspace owner or admin',
  record_owner: 'current deal owner'
}

function WorkflowApprovalRow({ approval, isDeciding, onDecide }) {
  const [note, setNote] = useState('')
  return (
    <article className="record-row record-row-alert" role="listitem">
      <div className="card-stack">
        <div>
          <h4>{approval.name}</h4>
          <p>{approval.message}</p>
          <p className="field-hint">{approval.automationName} · {approval.dealName} · requested by {approval.requestedByUserName} {formatRunTime(approval.requestedAt)}</p>
          <p className="field-hint">A {roleLabels[approval.approverRole]} must decide before {approval.pendingTaskCount} {approval.pendingTaskCount === 1 ? 'task is' : 'tasks are'} created.</p>
        </div>
        <label>
          <span className="field-label">Rejection note</span>
          <textarea className="text-input" rows={2} maxLength={1000} value={note} onChange={(event) => setNote(event.target.value)} placeholder="Required only when rejecting" />
        </label>
        <div className="button-row">
          <Button type="button" disabled={isDeciding} onClick={() => onDecide(approval, 'approved', '')}>{isDeciding ? 'Saving…' : 'Approve and create tasks'}</Button>
          <Button className="button-secondary" type="button" disabled={isDeciding || !note.trim()} onClick={() => onDecide(approval, 'rejected', note.trim())}>Reject task plan</Button>
        </div>
      </div>
      <span className="chip">Waiting approval</span>
    </article>
  )
}

export function SettingsWorkflowApprovals({ approvals, decidingApprovalId, isLoading, onDecide }) {
  return (
    <div className="card-stack">
      <div>
        <h3>Pending workflow approvals</h3>
        <p className="field-hint">Only approvals matching your current workspace role or deal ownership appear here.</p>
      </div>
      <div className="record-list" role="list" aria-label="Pending workflow approvals">
        {!isLoading && approvals.length === 0 ? <article className="record-row" role="listitem"><p>No workflow approvals need your decision.</p></article> : null}
        {approvals.map((approval) => (
          <WorkflowApprovalRow approval={approval} isDeciding={decidingApprovalId === approval.id} key={approval.id} onDecide={onDecide} />
        ))}
      </div>
    </div>
  )
}
