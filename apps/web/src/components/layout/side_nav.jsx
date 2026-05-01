import { NavLink } from 'react-router-dom'
import { useAuth } from '../../app/providers'

const baseLinks = [
  { to: '/dashboard', labelKey: 'dashboard', fallback: 'Dashboard' },
  { to: '/companies', labelKey: 'companies', fallback: 'Companies' },
  { to: '/deals', labelKey: 'deals', fallback: 'Deals' },
  { to: '/tasks', labelKey: 'tasks', fallback: 'Tasks' },
  { to: '/settings/users', labelKey: 'users', fallback: 'Users' },
  { to: '/settings/business-profile', labelKey: 'businessProfile', fallback: 'Business Profile' },
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
    <aside className="side-nav">
      <div className="brand-mark">OC</div>
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
