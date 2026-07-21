import { lazy, Suspense } from 'react'
import { usePageTitle } from '../lib/use_page_title'

const SavedReportsRoute = lazy(async () => {
  const routeModule = await import('./reports_foundation')
  return { default: routeModule.ReportsFoundationRoute }
})

export function ReportsRoute() {
  usePageTitle('Reports')
  return (
    <Suspense fallback={<p className="field-hint" role="status">Loading reports…</p>}>
      <SavedReportsRoute />
    </Suspense>
  )
}
