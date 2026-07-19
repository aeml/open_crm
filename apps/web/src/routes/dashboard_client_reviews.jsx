import { Link } from 'react-router-dom'
import { Card } from '../components/ui/card'

function formatTimestamp(value) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'Not scheduled' : date.toLocaleString()
}

const recordPaths = { company: 'companies', contact: 'contacts' }

export function DashboardClientReviews({ summary, isLoading }) {
  const data = summary || { total: 0, overdue: 0, dueWithin30Days: 0, later: 0, records: [], semantics: [] }
  return (
    <Card>
      <div className="card-stack">
        <div>
          <p className="eyebrow">Client obligations</p>
          <h2>Reviews and renewals</h2>
          <p>See future customer follow-up without creating a separate calendar or billing system.</p>
        </div>
        {isLoading ? <p className="field-hint" role="status">Loading client obligations...</p> : null}
        <div className="button-row" aria-label="Client obligation counts">
          <span className="chip">{data.overdue} overdue</span>
          <span className="chip">{data.dueWithin30Days} due within 30 days</span>
          <span className="chip">{data.later} later</span>
        </div>
        {!isLoading && data.total === 0 ? <p className="field-hint">No review or renewal tasks are scheduled yet. Add one from a customer record.</p> : null}
        <div className="record-list" role="list" aria-label="Upcoming client reviews and renewals">
          {(data.records || []).map((record) => (
            <article className="record-row" role="listitem" key={`${record.entityType}-${record.entityId}`}>
              <div>
                <h3><Link to={`/${recordPaths[record.entityType]}/${record.entityId}`}>{record.entityLabel}</Link></h3>
                <p>{record.reviewLabel} · {formatTimestamp(record.nextReviewAt)}</p>
                <p className="field-hint">{record.cadenceLabel} · {record.assignedToUserName || 'Assigned team member'}</p>
              </div>
              <Link to={`/tasks/${record.currentTaskId}`}>{record.isOverdue ? 'Review overdue task' : 'Open task'}</Link>
            </article>
          ))}
        </div>
        {data.semantics?.length ? <details><summary>How client obligations are counted</summary><ul>{data.semantics.map((rule) => <li key={rule}>{rule}</li>)}</ul></details> : null}
      </div>
    </Card>
  )
}
