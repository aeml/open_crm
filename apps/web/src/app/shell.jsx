import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { AppHeader } from '../components/layout/app_header'
import { SideNav } from '../components/layout/side_nav'
import { PageHeader } from '../components/layout/page_header'

export function AppShell({ children }) {
  return (
    <div className="app-shell">
      <SideNav />
      <div className="app-main">
        <AppHeader />
        <main className="app-content">
          <PageHeader
            eyebrow="Pipeline at a glance"
            title="Open CRM"
            description="Track customers, deals, and follow-ups without drowning in a messy admin panel."
            actions={<Button>New Contact</Button>}
          />
          <Card className="hero-card">
            <div className="hero-grid">
              <div>
                <p className="metric-label">Open pipeline</p>
                <p className="metric-value">$182,400</p>
              </div>
              <div>
                <p className="metric-label">Due today</p>
                <p className="metric-value">7 tasks</p>
              </div>
              <div>
                <p className="metric-label">New contacts</p>
                <p className="metric-value">14 this week</p>
              </div>
            </div>
          </Card>
          {children}
        </main>
      </div>
    </div>
  )
}
