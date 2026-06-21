import { NavLink } from 'react-router-dom'
import { useAuth } from '../../app/providers'

const baseLinks = [
  { to: '/dashboard', labelKey: 'dashboard', fallback: 'Dashboard' },
  { to: '/companies', labelKey: 'companies', fallback: 'Companies' },
  { to: '/deals', labelKey: 'deals', fallback: 'Deals' },
  { to: '/tasks', labelKey: 'tasks', fallback: 'Tasks' },
  { to: '/mailbox', labelKey: 'mailbox', fallback: 'Mailbox' },
  { to: '/team-inbox', labelKey: 'teamInbox', fallback: 'Team Inbox' },
  { to: '/settings/profile', labelKey: 'myProfile', fallback: 'My Profile' },
  { to: '/settings/email-account', labelKey: 'myEmail', fallback: 'My Email' },
  { to: '/settings/users', labelKey: 'users', fallback: 'Users' },
  { to: '/settings/business-profile', labelKey: 'businessProfile', fallback: 'Business Profile' },
  { to: '/settings/email-templates', labelKey: 'emailTemplates', fallback: 'Email Templates' },
  { to: '/settings/email-sequences', labelKey: 'emailSequences', fallback: 'Email Sequences' },
  { to: '/settings/product-catalog', labelKey: 'productCatalog', fallback: 'Product Catalog' },
  { to: '/settings/lead-forms', labelKey: 'leadForms', fallback: 'Lead Forms' },
  { to: '/settings/calendar', labelKey: 'calendar', fallback: 'Booking Links' },
  { to: '/settings/email-log', labelKey: 'emailLog', fallback: 'Email Log' },
  { to: '/settings/billing', labelKey: 'billing', fallback: 'Plan & Billing' },
  { to: '/settings/audit', labelKey: 'audit', fallback: 'Audit Trail' }
]

function defaultLabelsForBusinessType(businessType) {
  if (businessType === 'services') {
    return {
      companies: 'Clients',
      deals: 'Jobs',
      tasks: 'Service Tasks'
    }
  }

  if (businessType === 'construction-services') {
    return {
      companies: 'Clients',
      deals: 'Jobs',
      tasks: 'Site Tasks'
    }
  }

  if (businessType === 'product-sales') {
    return {
      companies: 'Accounts',
      deals: 'Opportunities',
      tasks: 'Follow-ups'
    }
  }

  return {}
}

function defaultFallbacksForBusinessType(businessType) {
  if (businessType === 'product-sales') {
    return {
      companies: 'Accounts'
    }
  }

  return {
    companies: 'Clients'
  }
}

export function SideNav() {
  const { session, businessProfile } = useAuth()
  const businessType = businessProfile?.businessType || session?.organization?.businessType || 'general'
  const labels = {
    ...defaultLabelsForBusinessType(businessType),
    ...(businessProfile?.labels || {})
  }
  const fallbacks = defaultFallbacksForBusinessType(businessType)

  return (
    <aside className="side-nav" aria-label="Main navigation">
      <div className="brand-mark" aria-hidden="true">OC</div>
      <nav aria-label="Primary">
        <ul>
          {baseLinks.map((link) => (
            <li key={link.to}>
              <NavLink to={link.to} className="side-nav-link">
                {labels[link.labelKey] || fallbacks[link.labelKey] || link.fallback}
              </NavLink>
            </li>
          ))}
        </ul>
      </nav>
    </aside>
  )
}
