import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { listNotifications, markNotificationRead, markAllNotificationsRead } from '../lib/notifications'
import { getActivityDigest } from '../lib/collaboration'
import { listOrganizationUsers } from '../lib/users'
import { usePageTitle } from '../lib/use_page_title'

function formatNotificationTime(value) {
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleString()
}

function entityPath(entityType, entityId) {
  if (!entityId) return ''
  if (entityType === 'contact') return `/contacts/${entityId}`
  if (entityType === 'company') return `/companies/${entityId}`
  if (entityType === 'deal') return `/deals/${entityId}`
  if (entityType === 'task') return `/tasks/${entityId}`
  return ''
}

function matchesNotificationFilter(notification, filter) {
  if (filter === 'mentions') return notification.eventType === 'record.mentioned'
  if (filter === 'following') return notification.eventType === 'record.activity'
  if (filter === 'assignments') return !notification.eventType.startsWith('record.')
  return true
}

export function NotificationsRoute() {
  usePageTitle('Notifications')
  const navigate = useNavigate()
  const [notifications, setNotifications] = useState([])
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isMarkingAll, setIsMarkingAll] = useState(false)
  const [filter, setFilter] = useState('all')
  const [digestScope, setDigestScope] = useState('following')
  const [digestDays, setDigestDays] = useState(7)
  const [digestActorUserId, setDigestActorUserId] = useState(0)
  const [users, setUsers] = useState([])
  const [digest, setDigest] = useState({ totalActivities: 0, activeRecords: 0, activePeople: 0, activities: [] })
  const [digestError, setDigestError] = useState('')
  const [isDigestLoading, setIsDigestLoading] = useState(true)

  const load = useCallback(({ signal } = {}) => {
    setIsLoading(true)
    listNotifications({ signal })
      .then((list) => {
        setNotifications(list)
        setError('')
        setIsLoading(false)
      })
      .catch((err) => {
        if (!isAbortError(err)) {
          setError(err.message || 'Unable to load notifications.')
          setIsLoading(false)
        }
      })
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    load({ signal: controller.signal })
    return () => controller.abort()
  }, [load])

  useEffect(() => {
    const controller = new AbortController()
    listOrganizationUsers({ signal: controller.signal })
      .then(setUsers)
      .catch((err) => {
        if (!isAbortError(err)) setUsers([])
      })
    return () => controller.abort()
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    setIsDigestLoading(true)
    getActivityDigest({ scope: digestScope, days: digestDays, actorUserId: digestActorUserId, signal: controller.signal })
      .then((result) => {
        setDigest(result)
        setDigestError('')
        setIsDigestLoading(false)
      })
      .catch((err) => {
        if (!isAbortError(err)) {
          setDigestError(err.message || 'Unable to load the activity digest.')
          setIsDigestLoading(false)
        }
      })
    return () => controller.abort()
  }, [digestActorUserId, digestDays, digestScope])

  async function handleMarkRead(id) {
    try {
      await markNotificationRead(id)
      setNotifications((prev) =>
        prev.map((n) => (n.id === id ? { ...n, readAt: new Date().toISOString() } : n))
      )
    } catch {
      // best-effort
    }
  }

  async function handleOpen(notification) {
    if (!notification.readAt) await handleMarkRead(notification.id)
    const path = entityPath(notification.entityType, notification.entityId)
    if (path) navigate(path)
  }

  async function handleMarkAll() {
    setIsMarkingAll(true)
    try {
      await markAllNotificationsRead()
      setNotifications((prev) => prev.map((n) => ({ ...n, readAt: n.readAt || new Date().toISOString() })))
    } catch (err) {
      setError(err.message || 'Unable to mark notifications read.')
    } finally {
      setIsMarkingAll(false)
    }
  }

  const unreadCount = notifications.filter((n) => !n.readAt).length
  const visibleNotifications = notifications.filter((notification) => matchesNotificationFilter(notification, filter))

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Notifications</h2>
              <p>Assignments and activity that mention you.</p>
            </div>
            {unreadCount > 0 ? (
              <Button
                className="button-secondary"
                onClick={handleMarkAll}
                disabled={isMarkingAll}
              >
                {isMarkingAll ? 'Marking…' : 'Mark all read'}
              </Button>
            ) : null}
          </div>

          {error ? <InlineError message={error} /> : null}

          <label className="field-label">
            Show
            <select className="text-input" value={filter} onChange={(event) => setFilter(event.target.value)}>
              <option value="all">All notifications</option>
              <option value="mentions">Mentions</option>
              <option value="following">Followed record activity</option>
              <option value="assignments">Assignments and reminders</option>
            </select>
          </label>

          {isLoading ? (
            <p className="field-hint">Loading notifications…</p>
          ) : visibleNotifications.length === 0 ? (
            <p className="field-hint">No notifications match this view.</p>
          ) : (
            <ul className="notification-list">
              {visibleNotifications.map((n) => (
                <li key={n.id} className={`notification-item${n.readAt ? '' : ' notification-item--unread'}`}>
                  <div className="notification-body">
                    <p className="notification-summary">{n.summary}</p>
                    <p className="notification-time field-hint">{formatNotificationTime(n.createdAt)}</p>
                  </div>
                  <div className="button-row">
                    {entityPath(n.entityType, n.entityId) ? (
                      <button className="button-ghost" onClick={() => handleOpen(n)}>Open record</button>
                    ) : null}
                    {!n.readAt ? (
                      <button className="button-ghost" onClick={() => handleMarkRead(n.id)} aria-label="Mark as read">Mark read</button>
                    ) : null}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      </Card>
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Activity digest</h2>
              <p>Review recent work across records you follow or the whole team.</p>
            </div>
          </div>
          <div className="form-grid form-grid--two">
            <label className="field-label">
              Records
              <select className="text-input" value={digestScope} onChange={(event) => setDigestScope(event.target.value)}>
                <option value="following">I follow</option>
                <option value="team">Whole team</option>
              </select>
            </label>
            <label className="field-label">
              Window
              <select className="text-input" value={digestDays} onChange={(event) => setDigestDays(Number(event.target.value))}>
                <option value={1}>Last day</option>
                <option value={7}>Last 7 days</option>
                <option value={30}>Last 30 days</option>
              </select>
            </label>
            <label className="field-label">
              Teammate
              <select className="text-input" value={digestActorUserId} onChange={(event) => setDigestActorUserId(Number(event.target.value))}>
                <option value={0}>Anyone</option>
                {users.map((user) => (
                  <option key={user.id} value={user.id}>{`${user.firstName || ''} ${user.lastName || ''}`.trim() || user.email}</option>
                ))}
              </select>
            </label>
          </div>
          {digestError ? <InlineError message={digestError} /> : null}
          {isDigestLoading ? (
            <p className="field-hint">Loading activity digest…</p>
          ) : (
            <>
              <div className="record-list" role="list" aria-label="Activity digest summary">
                <article className="record-row" role="listitem"><p>Activity</p><strong>{digest.totalActivities}</strong></article>
                <article className="record-row" role="listitem"><p>Records</p><strong>{digest.activeRecords}</strong></article>
                <article className="record-row" role="listitem"><p>Teammates</p><strong>{digest.activePeople}</strong></article>
              </div>
              {digest.activities.length === 0 ? (
                <p className="field-hint">No activity matches this digest.</p>
              ) : (
                <ul className="notification-list" aria-label="Activity digest">
                  {digest.activities.map((activity) => (
                    <li className="notification-item" key={activity.id}>
                      <div className="notification-body">
                        <p className="notification-summary">{activity.entityLabel}: {activity.summary}</p>
                        <p className="notification-time field-hint">{activity.actorName} · {formatNotificationTime(activity.createdAt)}</p>
                      </div>
                      {entityPath(activity.entityType, activity.entityId) ? (
                        <button className="button-ghost" onClick={() => navigate(entityPath(activity.entityType, activity.entityId))}>Open record</button>
                      ) : null}
                    </li>
                  ))}
                </ul>
              )}
            </>
          )}
        </div>
      </Card>
    </section>
  )
}
