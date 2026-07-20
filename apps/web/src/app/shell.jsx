import { AppHeader } from '../components/layout/app_header'
import { SideNav } from '../components/layout/side_nav'
import { PageHeader } from '../components/layout/page_header'
import { WorkspaceAccessBanner } from '../components/layout/workspace_access_banner'
import { useAuth } from './providers'

export function AppShell({ children }) {
  const { session, businessProfile } = useAuth()
  const businessType = businessProfile?.businessType || session?.organization?.businessType || 'general'
  const jobs = businessType === 'services' || businessType === 'construction-services'
  const productSales = businessType === 'product-sales'
  const eyebrow = jobs ? 'Jobs at a glance' : (productSales ? 'Revenue at a glance' : 'Pipeline at a glance')
  const title = jobs ? 'Job pipeline overview' : (productSales ? 'Sales pipeline overview' : 'Pipeline overview')
  const records = jobs ? `clients, jobs, and ${businessType === 'services' ? 'service' : 'site'} tasks` : (productSales ? 'accounts, opportunities, and follow-ups' : 'customers, deals, and follow-ups')

  return (
    <div className="app-shell">
      <a href="#main-content" className="skip-link">Skip to main content</a>
      <SideNav />
      <div className="app-main">
        <AppHeader />
        <WorkspaceAccessBanner />
        <main className="app-content" id="main-content">
          <PageHeader
            eyebrow={eyebrow}
            title={title}
            description={`Track ${records} without drowning in a messy admin panel.`}
          />
          {children}
        </main>
      </div>
    </div>
  )
}
