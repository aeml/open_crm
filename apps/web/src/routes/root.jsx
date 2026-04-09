import { Outlet } from 'react-router-dom'
import { AppShell } from '../app/shell'

export function RootRoute() {
  return (
    <AppShell>
      <Outlet />
    </AppShell>
  )
}
