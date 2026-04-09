import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { AppShell } from '../app/shell'
import { useAuth } from '../app/providers'

export function RootRoute() {
  const { status } = useAuth()
  const location = useLocation()

  if (status === 'checking') {
    return null
  }

  if (status !== 'authenticated') {
    return <Navigate to="/login" replace state={{ from: location }} />
  }

  return (
    <AppShell>
      <Outlet />
    </AppShell>
  )
}
