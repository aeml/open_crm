import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { AppProviders } from './providers'
import { RootRoute } from '../routes/root'
import { DashboardRoute } from '../routes/dashboard'
import { ContactsRoute } from '../routes/contacts'
import { CompaniesRoute } from '../routes/companies'
import { DealsRoute } from '../routes/deals'
import { TasksRoute } from '../routes/tasks'
import { LoginRoute } from '../routes/login'
import { SettingsUsersRoute } from '../routes/settings_users'

export function AppRouter() {
  return (
    <BrowserRouter>
      <AppProviders>
        <Routes>
          <Route path="/login" element={<LoginRoute />} />
          <Route path="/" element={<RootRoute />}>
            <Route index element={<Navigate to="/dashboard" replace />} />
            <Route path="dashboard" element={<DashboardRoute />} />
            <Route path="contacts" element={<ContactsRoute />} />
            <Route path="companies" element={<CompaniesRoute />} />
            <Route path="deals" element={<DealsRoute />} />
            <Route path="tasks" element={<TasksRoute />} />
            <Route path="settings/users" element={<SettingsUsersRoute />} />
          </Route>
          <Route path="*" element={<Navigate to="/dashboard" replace />} />
        </Routes>
      </AppProviders>
    </BrowserRouter>
  )
}
