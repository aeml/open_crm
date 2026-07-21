import { lazy, Suspense } from 'react'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { AppProviders } from './providers'

function lazyRoute(loader, exportName) {
  return lazy(async () => {
    const routeModule = await loader()
    return { default: routeModule[exportName] }
  })
}

const showFoundationRoutes = import.meta.env.DEV

const RootRoute = lazyRoute(() => import('../routes/root'), 'RootRoute')
const DashboardRoute = lazyRoute(() => import('../routes/dashboard'), 'DashboardRoute')
const ReportsRoute = lazyRoute(() => import('../routes/reports'), 'ReportsRoute')
const ContactsRoute = lazyRoute(() => import('../routes/contacts'), 'ContactsRoute')
const CompaniesRoute = lazyRoute(() => import('../routes/companies'), 'CompaniesRoute')
const DealsRoute = lazyRoute(() => import('../routes/deals'), 'DealsRoute')
const TasksRoute = lazyRoute(() => import('../routes/tasks'), 'TasksRoute')
const MailboxRoute = lazyRoute(() => import('../routes/mailbox'), 'MailboxRoute')
const TeamInboxRoute = lazyRoute(() => import('../routes/team_inbox'), 'TeamInboxRoute')
const LoginRoute = lazyRoute(() => import('../routes/login'), 'LoginRoute')
const PublicLandingPageRoute = lazyRoute(() => import('../routes/public_landing_page'), 'PublicLandingPageRoute')
const PublicLeadWidgetRoute = lazyRoute(() => import('../routes/public_lead_widget'), 'PublicLeadWidgetRoute')
const PublicQuoteRoute = lazyRoute(() => import('../routes/public_quote'), 'PublicQuoteRoute')
const BootstrapRoute = lazyRoute(() => import('../routes/bootstrap'), 'BootstrapRoute')
const VerifyEmailRoute = lazyRoute(() => import('../routes/verify_email'), 'VerifyEmailRoute')
const SetupPasswordRoute = lazyRoute(() => import('../routes/setup_password'), 'SetupPasswordRoute')
const ForgotPasswordRoute = lazyRoute(() => import('../routes/forgot_password'), 'ForgotPasswordRoute')
const ResetPasswordRoute = lazyRoute(() => import('../routes/reset_password'), 'ResetPasswordRoute')
const SettingsProfileRoute = lazyRoute(() => import('../routes/settings_profile'), 'SettingsProfileRoute')
const SettingsUsersRoute = lazyRoute(() => import('../routes/settings_users'), 'SettingsUsersRoute')
const BusinessProfileRoute = lazyRoute(() => import('../routes/business_profile'), 'BusinessProfileRoute')
const SettingsAuditRoute = lazyRoute(() => import('../routes/settings_audit'), 'SettingsAuditRoute')
const SettingsBillingRoute = lazyRoute(() => import('../routes/settings_billing'), 'SettingsBillingRoute')
const SettingsEmailTemplatesRoute = lazyRoute(() => import('../routes/settings_email_templates'), 'SettingsEmailTemplatesRoute')
const SettingsEmailSequencesRoute = lazyRoute(() => import('../routes/settings_email_sequences'), 'SettingsEmailSequencesRoute')
const SettingsProductCatalogRoute = lazyRoute(() => import('../routes/settings_product_catalog'), 'SettingsProductCatalogRoute')
const SettingsLeadFormsRoute = lazyRoute(() => import('../routes/settings_lead_forms'), 'SettingsLeadFormsRoute')
const SettingsLandingPagesRoute = lazyRoute(() => import('../routes/settings_landing_pages'), 'SettingsLandingPagesRoute')
const SettingsLeadAudiencesRoute = lazyRoute(() => import('../routes/settings_lead_audiences'), 'SettingsLeadAudiencesRoute')
const SettingsMarketingEmailCampaignsRoute = showFoundationRoutes ? lazyRoute(() => import('../routes/settings_marketing_email_campaigns'), 'SettingsMarketingEmailCampaignsRoute') : null
const SettingsNurtureCampaignsRoute = showFoundationRoutes ? lazyRoute(() => import('../routes/settings_nurture_campaigns'), 'SettingsNurtureCampaignsRoute') : null
const SettingsLeadScoringRoute = lazyRoute(() => import('../routes/settings_lead_scoring'), 'SettingsLeadScoringRoute')
const SettingsLeadWidgetsRoute = lazyRoute(() => import('../routes/settings_lead_widgets'), 'SettingsLeadWidgetsRoute')
const SettingsAutomationsRoute = lazyRoute(() => import('../routes/settings_automations'), 'SettingsAutomationsRoute')
const SettingsCalendarRoute = showFoundationRoutes ? lazyRoute(() => import('../routes/settings_calendar'), 'SettingsCalendarRoute') : null
const SettingsEmailAccountRoute = lazyRoute(() => import('../routes/settings_email_account'), 'SettingsEmailAccountRoute')
const SettingsEmailLogRoute = lazyRoute(() => import('../routes/settings_email_log'), 'SettingsEmailLogRoute')
const SettingsOperationsRoute = lazyRoute(() => import('../routes/settings_operations'), 'SettingsOperationsRoute')
const SettingsImportsRoute = lazyRoute(() => import('../routes/settings_imports'), 'SettingsImportsRoute')
const SettingsDuplicatesRoute = lazyRoute(() => import('../routes/settings_duplicates'), 'SettingsDuplicatesRoute')
const SettingsCustomFieldsRoute = lazyRoute(() => import('../routes/settings_custom_fields'), 'SettingsCustomFieldsRoute')
const SettingsPipelinesRoute = lazyRoute(() => import('../routes/settings_pipelines'), 'SettingsPipelinesRoute')
const SettingsArchivedRecordsRoute = lazyRoute(() => import('../routes/settings_archived_records'), 'SettingsArchivedRecordsRoute')
const NotificationsRoute = lazyRoute(() => import('../routes/notifications'), 'NotificationsRoute')

function RouteLoadingState() {
  return <div className="route-loading" role="status">Loading Open CRM…</div>
}

export function AppRouter() {
  return (
    <BrowserRouter>
      <AppProviders>
        <Suspense fallback={<RouteLoadingState />}>
          <Routes>
            <Route path="/login" element={<LoginRoute />} />
            <Route path="/lp/:slug" element={<PublicLandingPageRoute />} />
            <Route path="/widget/:publicId" element={<PublicLeadWidgetRoute />} />
            <Route path="/quote" element={<PublicQuoteRoute />} />
            <Route path="/bootstrap" element={<BootstrapRoute />} />
            <Route path="/verify-email" element={<VerifyEmailRoute />} />
            <Route path="/setup-password" element={<SetupPasswordRoute />} />
            <Route path="/forgot-password" element={<ForgotPasswordRoute />} />
            <Route path="/reset-password" element={<ResetPasswordRoute />} />
            <Route path="/" element={<RootRoute />}>
              <Route index element={<Navigate to="/dashboard" replace />} />
              <Route path="dashboard" element={<DashboardRoute />} />
              <Route path="reports" element={<ReportsRoute />} />
              <Route path="contacts" element={<ContactsRoute />} />
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
              <Route path="settings/lead-forms" element={<SettingsLeadFormsRoute />} />
              <Route path="settings/landing-pages" element={<SettingsLandingPagesRoute />} />
              <Route path="settings/lead-audiences" element={<SettingsLeadAudiencesRoute />} />
              {showFoundationRoutes ? <Route path="settings/marketing-email-campaigns" element={<SettingsMarketingEmailCampaignsRoute />} /> : null}
              {showFoundationRoutes ? <Route path="settings/nurture-campaigns" element={<SettingsNurtureCampaignsRoute />} /> : null}
              <Route path="settings/lead-scoring" element={<SettingsLeadScoringRoute />} />
              <Route path="settings/lead-widgets" element={<SettingsLeadWidgetsRoute />} />
              <Route path="settings/automations" element={<SettingsAutomationsRoute />} />
              {showFoundationRoutes ? <Route path="settings/calendar" element={<SettingsCalendarRoute />} /> : null}
              <Route path="settings/email-account" element={<SettingsEmailAccountRoute />} />
              <Route path="settings/email-log" element={<SettingsEmailLogRoute />} />
              <Route path="settings/audit" element={<SettingsAuditRoute />} />
              <Route path="settings/operations" element={<SettingsOperationsRoute />} />
              <Route path="settings/imports" element={<SettingsImportsRoute />} />
              <Route path="settings/data-quality" element={<SettingsDuplicatesRoute />} />
              <Route path="settings/custom-fields" element={<SettingsCustomFieldsRoute />} />
              <Route path="settings/pipelines" element={<SettingsPipelinesRoute />} />
              <Route path="settings/archived-records" element={<SettingsArchivedRecordsRoute />} />
              <Route path="notifications" element={<NotificationsRoute />} />
            </Route>
            <Route path="*" element={<Navigate to="/dashboard" replace />} />
          </Routes>
        </Suspense>
      </AppProviders>
    </BrowserRouter>
  )
}
