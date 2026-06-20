import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { AppProviders } from './providers'
import { RootRoute } from '../routes/root'
import { DashboardRoute } from '../routes/dashboard'
import { ContactsRoute } from '../routes/contacts'
import { CompaniesRoute } from '../routes/companies'
import { DealsRoute } from '../routes/deals'
import { TasksRoute } from '../routes/tasks'
import { MailboxRoute } from '../routes/mailbox'
import { TeamInboxRoute } from '../routes/team_inbox'
import { LoginRoute } from '../routes/login'
import { BootstrapRoute } from '../routes/bootstrap'
import { SetupPasswordRoute } from '../routes/setup_password'
import { SettingsProfileRoute } from '../routes/settings_profile'
import { SettingsUsersRoute } from '../routes/settings_users'
import { BusinessProfileRoute } from '../routes/business_profile'
import { SettingsAuditRoute } from '../routes/settings_audit'
import { SettingsBillingRoute } from '../routes/settings_billing'
import { SettingsEmailTemplatesRoute } from '../routes/settings_email_templates'
import { SettingsEmailSequencesRoute } from '../routes/settings_email_sequences'
import { SettingsProductCatalogRoute } from '../routes/settings_product_catalog'
import { SettingsCalendarRoute } from '../routes/settings_calendar'
import { SettingsEmailAccountRoute } from '../routes/settings_email_account'
import { SettingsEmailLogRoute } from '../routes/settings_email_log'
import { NotificationsRoute } from '../routes/notifications'

export function AppRouter() {
  return (
    <BrowserRouter>
      <AppProviders>
        <Routes>
          <Route path="/login" element={<LoginRoute />} />
          <Route path="/bootstrap" element={<BootstrapRoute />} />
          <Route path="/setup-password" element={<SetupPasswordRoute />} />
          <Route path="/" element={<RootRoute />}>
            <Route index element={<Navigate to="/dashboard" replace />} />
            <Route path="dashboard" element={<DashboardRoute />} />
            <Route path="contacts" element={<Navigate to="/companies" replace />} />
            <Route path="contacts/:contactId" element={<ContactsRoute />} />
            <Route path="companies" element={<CompaniesRoute />} />
            <Route path="companies/:companyId" element={<CompaniesRoute />} />
            <Route path="deals" element={<DealsRoute />} />
            <Route path="deals/:dealId" element={<DealsRoute />} />
            <Route path="tasks" element={<TasksRoute />} />
            <Route path="tasks/:taskId" element={<TasksRoute />} />
            <Route path="mailbox" element={<MailboxRoute />} />
            <Route path="team-inbox" element={<TeamInboxRoute />} />
            <Route path="settings/profile" element={<SettingsProfileRoute />} />
            <Route path="settings/users" element={<SettingsUsersRoute />} />
            <Route path="settings/business-profile" element={<BusinessProfileRoute />} />
            <Route path="settings/billing" element={<SettingsBillingRoute />} />
            <Route path="settings/email-templates" element={<SettingsEmailTemplatesRoute />} />
            <Route path="settings/email-sequences" element={<SettingsEmailSequencesRoute />} />
            <Route path="settings/product-catalog" element={<SettingsProductCatalogRoute />} />
            <Route path="settings/calendar" element={<SettingsCalendarRoute />} />
            <Route path="settings/email-account" element={<SettingsEmailAccountRoute />} />
            <Route path="settings/email-log" element={<SettingsEmailLogRoute />} />
            <Route path="settings/audit" element={<SettingsAuditRoute />} />
            <Route path="notifications" element={<NotificationsRoute />} />
          </Route>
          <Route path="*" element={<Navigate to="/dashboard" replace />} />
        </Routes>
      </AppProviders>
    </BrowserRouter>
  )
}
