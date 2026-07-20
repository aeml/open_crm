import { Link } from 'react-router-dom'
import { useAuth } from '../../app/providers'

export function WorkspaceAccessBanner() {
  const { workspaceAccess, canManageBilling } = useAuth()
  const unavailable = workspaceAccess?.state === 'unavailable'
  if (!unavailable && workspaceAccess?.state !== 'read_only') return null
  const title = unavailable ? 'Write access is temporarily unavailable' : 'Workspace is read-only'
  const message = unavailable
    ? 'Open CRM could not verify billing state. Reads and CSV exports remain available while writes fail closed.'
    : `The hosted subscription is inactive. Reads and CSV exports remain available.${canManageBilling ? '' : ' Ask an owner or admin to restore access.'}`

  return (
    <section className="card workspace-access-banner" aria-labelledby="workspace-access-title" role={unavailable ? 'status' : 'alert'}>
      <div>
        <strong id="workspace-access-title">{title}</strong>
        <p>{message}</p>
      </div>
      <Link className="button button-secondary" to="/settings/billing">Review plan &amp; billing</Link>
    </section>
  )
}
