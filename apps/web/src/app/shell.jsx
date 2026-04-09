import { Button } from '../components/ui/button'
import { AppHeader } from '../components/layout/app_header'
import { SideNav } from '../components/layout/side_nav'
import { PageHeader } from '../components/layout/page_header'
import { useAuth } from './providers'

const profileCopy = {
  general: {
    eyebrow: 'Pipeline at a glance',
    title: 'Pipeline overview',
    description: 'Track customers, deals, and follow-ups without drowning in a messy admin panel.',
    action: 'New Contact'
  },
  services: {
    eyebrow: 'Service work at a glance',
    title: 'Service pipeline overview',
    description: 'Track clients, engagements, and follow-ups without drowning in a messy admin panel.',
    action: 'New Contact'
  },
  'product-sales': {
    eyebrow: 'Revenue at a glance',
    title: 'Sales pipeline overview',
    description: 'Track accounts, opportunities, and follow-ups without drowning in a messy admin panel.',
    action: 'New Contact'
  },
  'construction-services': {
    eyebrow: 'Jobs at a glance',
    title: 'Job pipeline overview',
    description: 'Track clients, jobs, and site tasks without drowning in a messy admin panel.',
    action: 'New Contact'
  }
}

export function AppShell({ children }) {
  const { session, businessProfile } = useAuth()
  const businessType = businessProfile?.businessType || session?.organization?.businessType || 'general'
  const copy = profileCopy[businessType] || profileCopy.general

  return (
    <div className="app-shell">
      <SideNav />
      <div className="app-main">
        <AppHeader />
        <main className="app-content">
          <PageHeader
            eyebrow={copy.eyebrow}
            title={copy.title}
            description={copy.description}
            actions={<Button>{copy.action}</Button>}
          />
          {children}
        </main>
      </div>
    </div>
  )
}
