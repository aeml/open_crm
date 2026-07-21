import { lazy, Suspense } from 'react'
import { usePageTitle } from '../lib/use_page_title'
import { DataQualityPanel } from './data_quality_panel'
import { FollowUpReport } from './follow_up_report'
import { SalesActivityReport } from './sales_activity_report'

const FoundationReportsRoute = import.meta.env.DEV
  ? lazy(async () => {
      const routeModule = await import('./reports_foundation')
      return { default: routeModule.ReportsFoundationRoute }
    })
  : null

export function ReportsRoute() {
  usePageTitle('Reports')
  if (FoundationReportsRoute) {
    return (
      <Suspense fallback={<p className="field-hint" role="status">Loading reports…</p>}>
        <FoundationReportsRoute />
      </Suspense>
    )
  }
  return (
    <section className="dashboard-grid settings-grid">
      <SalesActivityReport />
      <FollowUpReport />
      <DataQualityPanel />
    </section>
  )
}
