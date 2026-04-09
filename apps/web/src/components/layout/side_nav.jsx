import { NavLink } from 'react-router-dom'
import { useAuth } from '../../app/providers'

const baseLinks = [
  { to: '/dashboard', labelKey: 'dashboard', fallback: 'Dashboard' },
  { to: '/contacts', labelKey: 'contacts', fallback: 'Contacts' },
  { to: '/companies', labelKey: 'companies', fallback: 'Companies' },
  { to: '/deals', labelKey: 'deals', fallback: 'Deals' },
  { to: '/tasks', labelKey: 'tasks', fallback: 'Tasks' },
  { to: '/settings/users', labelKey: 'users', fallback: 'Users' },
  { to: '/settings/business-profile', labelKey: 'businessProfile', fallback: 'Business Profile' }
]

export function SideNav() {
  const { businessProfile } = useAuth()
  const labels = businessProfile?.labels || {}

  return (
    <aside className="side-nav">
      <div className="brand-mark">OC</div>
      <nav aria-label="Primary">
        <ul>
          {baseLinks.map((link) => (
            <li key={link.to}>
              <NavLink to={link.to} className="side-nav-link">
                {labels[link.labelKey] || link.fallback}
              </NavLink>
            </li>
          ))}
        </ul>
      </nav>
    </aside>
  )
}
