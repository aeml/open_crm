import { NavLink } from 'react-router-dom'
import { useAuth } from '../../app/providers'

const foundationLinks = import.meta.env.DEV ? [
  ['/settings/marketing-email-campaigns', 'Email Campaigns'],
  ['/settings/nurture-campaigns', 'Nurture Campaigns'],
  ['/settings/calendar', 'Booking Links']
] : []

const baseLinks = [
  ['/dashboard', 'Dashboard'],
  ['/reports', 'Reports'],
  ['/companies', 'Companies', false, 'companies'],
  ['/deals', 'Deals', false, 'deals'],
  ['/tasks', 'Tasks', false, 'tasks'],
  ['/mailbox', 'Mailbox'],
  ['/team-inbox', 'Team Inbox'],
  ['/settings/profile', 'My Profile'],
  ['/settings/email-account', 'My Email'],
  ['/settings/users', 'Users'],
  ['/settings/business-profile', 'Business Profile'],
  ['/settings/email-templates', 'Email Templates'],
  ['/settings/email-sequences', 'Email Sequences'],
  ['/settings/product-catalog', 'Product Catalog'],
  ['/settings/lead-forms', 'Lead Forms'],
  ['/settings/landing-pages', 'Landing Pages'],
  ['/settings/lead-audiences', 'Audiences'],
  ...foundationLinks,
  ['/settings/lead-scoring', 'Lead Scoring'],
  ['/settings/lead-widgets', 'Website Widgets'],
  ['/settings/automations', 'Automations'],
  ['/settings/email-log', 'Email Log'],
  ['/settings/billing', 'Plan & Billing'],
  ['/settings/audit', 'Audit Trail'],
  ['/settings/imports', 'Data Imports', true],
  ['/settings/data-quality', 'Data Quality', true],
  ['/settings/custom-fields', 'Custom Fields', true],
  ['/settings/pipelines', 'Pipelines', true],
  ['/settings/archived-records', 'Archived Records'],
  ['/settings/operations', 'Operations', true]
]

const defaultLabels = {
  general: { companies: 'Clients' },
  services: { companies: 'Clients', deals: 'Jobs', tasks: 'Service Tasks' },
  'construction-services': { companies: 'Clients', deals: 'Jobs', tasks: 'Site Tasks' },
  'product-sales': { companies: 'Accounts', deals: 'Opportunities', tasks: 'Follow-ups' }
}

export function SideNav() {
  const { session, businessProfile } = useAuth()
  const businessType = businessProfile?.businessType || session?.organization?.businessType || 'general'
  const labels = {
    ...(defaultLabels[businessType] || defaultLabels.general),
    ...(businessProfile?.labels || {})
  }
  const isAdmin = ['owner', 'admin'].includes(session?.membership?.role || '')

  return (
    <aside className="side-nav" aria-label="Main navigation">
      <div className="brand-mark" aria-hidden="true">OC</div>
      <nav aria-label="Primary">
        <ul>
          {baseLinks.filter(([, , adminOnly]) => !adminOnly || isAdmin).map(([to, fallback, , labelKey]) => (
            <li key={to}>
              <NavLink to={to} className="side-nav-link">
                {labels[labelKey] || fallback}
              </NavLink>
            </li>
          ))}
        </ul>
      </nav>
    </aside>
  )
}
