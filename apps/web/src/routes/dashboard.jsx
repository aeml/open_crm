import { Card } from '../components/ui/card'

export function DashboardRoute() {
  return (
    <section className="dashboard-grid">
      <Card>
        <h2>Today</h2>
        <p>Keep the next best action obvious for the team.</p>
      </Card>
      <Card>
        <h2>Recent activity</h2>
        <p>Activity history will land here once the first write flows exist.</p>
      </Card>
    </section>
  )
}
