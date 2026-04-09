import { NavLink } from 'react-router-dom'

const links = [
  { to: '/dashboard', label: 'Dashboard' },
  { to: '/contacts', label: 'Contacts' },
  { to: '/companies', label: 'Companies' },
  { to: '/deals', label: 'Deals' },
  { to: '/tasks', label: 'Tasks' },
  { to: '/settings/users', label: 'Users' }
]

export function SideNav() {
  return (
    <aside className="side-nav">
      <div className="brand-mark">OC</div>
      <nav aria-label="Primary">
        <ul>
          {links.map((link) => (
            <li key={link.to}>
              <NavLink to={link.to} className="side-nav-link">
                {link.label}
              </NavLink>
            </li>
          ))}
        </ul>
      </nav>
    </aside>
  )
}
