import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Button } from '../ui/button'
import { useAuth } from '../../app/providers'
import { getNotificationUnreadCount } from '../../lib/notifications'

export function AppHeader() {
  const navigate = useNavigate()
  const { session, logout } = useAuth()
  const organizationName = session?.organization?.name || 'Open CRM'
  const userRole = session?.membership?.role || 'Member'
  const [unreadCount, setUnreadCount] = useState(0)

  useEffect(() => {
    if (!session) return
    const controller = new AbortController()
    getNotificationUnreadCount({ signal: controller.signal })
      .then((count) => setUnreadCount(count))
      .catch(() => {})
    const interval = setInterval(() => {
      getNotificationUnreadCount()
        .then((count) => setUnreadCount(count))
        .catch(() => {})
    }, 60000)
    return () => {
      controller.abort()
      clearInterval(interval)
    }
  }, [session])

  async function handleLogout() {
    try {
      await logout()
      navigate('/login', { replace: true })
    } catch {
      // Keep the current session visible when logout fails.
    }
  }

  return (
    <header className="app-header" aria-label="Site header">
      <div>
        <p className="eyebrow">Workspace</p>
        <p className="org-name">{organizationName}</p>
      </div>
      <div className="app-header-actions">
        <Link to="/notifications" className="notification-bell" aria-label={`Notifications${unreadCount > 0 ? `, ${unreadCount} unread` : ''}`}>
          {unreadCount > 0 ? (
            <span className="notification-badge">{unreadCount > 99 ? '99+' : unreadCount}</span>
          ) : null}
          <span aria-hidden="true">&#x1F514;</span>
        </Link>
        <div className="user-pill">{userRole}</div>
        <Button className="button-secondary" onClick={handleLogout}>Log out</Button>
      </div>
    </header>
  )
}
