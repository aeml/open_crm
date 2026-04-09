import { useAuth } from '../../app/providers'

export function AppHeader() {
  const { session } = useAuth()
  const organizationName = session?.organization?.name || 'Open CRM'
  const userRole = session?.membership?.role || 'Member'

  return (
    <header className="app-header">
      <div>
        <p className="eyebrow">Workspace</p>
        <h1>{organizationName}</h1>
      </div>
      <div className="user-pill">{userRole}</div>
    </header>
  )
}
