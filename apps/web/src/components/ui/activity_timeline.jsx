import { useMemo, useState } from 'react'

const actionLabels = {
  'contact.created': 'Contact created',
  'contact.updated': 'Contact updated',
  'company.created': 'Client created',
  'company.updated': 'Client updated',
  'deal.created': 'Deal created',
  'deal.updated': 'Deal updated',
  'deal.stage_changed': 'Stage changed',
  'note.created': 'Note added',
  'task.created': 'Task created',
  'task.updated': 'Task updated',
  'task.completed': 'Task completed',
  'task.reopened': 'Task reopened',
  'task.reassigned': 'Task reassigned'
}

function parseActivityDate(createdAt) {
  if (!createdAt) {
    return null
  }

  const parsed = new Date(createdAt)
  if (Number.isNaN(parsed.getTime())) {
    return null
  }
  return parsed
}

function formatDateHeading(date) {
  if (!date) {
    return 'Date unavailable'
  }
  return date.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric', year: 'numeric' })
}

function formatActivityTime(date) {
  if (!date) {
    return 'Time unavailable'
  }
  return date.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
}

function activityLabel(action) {
  if (actionLabels[action]) {
    return actionLabels[action]
  }
  return String(action || 'Activity').split('.').map((part) => part.charAt(0).toUpperCase() + part.slice(1)).join(' ')
}

function activityGroupLabel(action) {
  return activityLabel(action).split(' ')[0]
}

function groupActivities(activities) {
  const groups = []
  const byKey = new Map()

  for (const activity of activities) {
    const date = parseActivityDate(activity.createdAt)
    const dateLabel = formatDateHeading(date)
    if (!byKey.has(dateLabel)) {
      byKey.set(dateLabel, { dateLabel, items: [] })
      groups.push(byKey.get(dateLabel))
    }
    byKey.get(dateLabel).items.push({ ...activity, date })
  }

  return groups
}

export function ActivityTimeline({ activities = [], emptyMessage = 'No activity yet.', ariaLabel = 'Activity list' }) {
  const [actionFilter, setActionFilter] = useState('all')
  const actionOptions = useMemo(() => {
    const seen = new Set()
    const options = []
    for (const activity of activities) {
      const action = activity.action || 'activity'
      if (seen.has(action)) {
        continue
      }
      seen.add(action)
      options.push({ value: action, label: activityLabel(action) })
    }
    return options
  }, [activities])
  const filteredActivities = actionFilter === 'all' ? activities : activities.filter((activity) => (activity.action || 'activity') === actionFilter)
  const groups = groupActivities(filteredActivities)

  if (activities.length === 0) {
    return (
      <div className="record-list" role="list" aria-label={ariaLabel}>
        <article className="record-row" role="listitem">
          <div>
            <p>{emptyMessage}</p>
          </div>
        </article>
      </div>
    )
  }

  return (
    <div className="activity-timeline">
      {actionOptions.length > 1 ? (
        <label className="activity-filter">
          <span>Activity type</span>
          <select className="text-input" value={actionFilter} onChange={(event) => setActionFilter(event.target.value)} aria-label="Activity type filter">
            <option value="all">All activity</option>
            {actionOptions.map((option) => (
              <option key={option.value} value={option.value}>{option.label}</option>
            ))}
          </select>
        </label>
      ) : null}
      <div className="record-list activity-timeline-list">
        {groups.map((group) => (
          <section className="activity-day" key={group.dateLabel} aria-label={group.dateLabel}>
            <p className="activity-day-heading">{group.dateLabel}</p>
            <div className="activity-day-items" role="list" aria-label={`${ariaLabel}: ${group.dateLabel}`}>
              {group.items.map((activity) => (
                <article className="record-row activity-row" key={activity.id} role="listitem">
                  <div>
                    <p className="activity-summary">{activity.summary || activityLabel(activity.action)}</p>
                    <p className="field-hint">{formatActivityTime(activity.date)}</p>
                  </div>
                  <span className="activity-badge">{activityGroupLabel(activity.action)}</span>
                </article>
              ))}
            </div>
          </section>
        ))}
      </div>
    </div>
  )
}
