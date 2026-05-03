import { useNavigate } from 'react-router-dom'
import { Button } from '../ui/button'
import { useAuth } from '../../app/providers'

export function AppHeader() {
  const navigate = useNavigate()
  const { session, logout } = useAuth()
  const organizationName = session?.organization?.name || 'Open CRM'
  const userRole = session?.membership?.role || 'Member'

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
        <div className="user-pill">{userRole}</div>
        <Button className="button-secondary" onClick={handleLogout}>Log out</Button>
      </div>
    </header>
  )
}
