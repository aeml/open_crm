import { useCallback, useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { listNotifications, markNotificationRead, markAllNotificationsRead } from '../lib/notifications'
import { usePageTitle } from '../lib/use_page_title'

function formatNotificationTime(value) {
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleString()
}

export function NotificationsRoute() {
  usePageTitle('Notifications')
  const [notifications, setNotifications] = useState([])
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isMarkingAll, setIsMarkingAll] = useState(false)

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

          {isLoading ? (
            <p className="field-hint">Loading notifications…</p>
          ) : notifications.length === 0 ? (
            <p className="field-hint">No notifications yet.</p>
          ) : (
            <ul className="notification-list">
              {notifications.map((n) => (
                <li key={n.id} className={`notification-item${n.readAt ? '' : ' notification-item--unread'}`}>
                  <div className="notification-body">
                    <p className="notification-summary">{n.summary}</p>
                    <p className="notification-time field-hint">{formatNotificationTime(n.createdAt)}</p>
                  </div>
                  {!n.readAt ? (
                    <button
                      className="button-ghost"
                      onClick={() => handleMarkRead(n.id)}
                      aria-label="Mark as read"
                    >
                      Mark read
                    </button>
                  ) : null}
                </li>
              ))}
            </ul>
          )}
        </div>
      </Card>
    </section>
  )
}
