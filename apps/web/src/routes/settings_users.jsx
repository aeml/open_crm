import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { createOrganizationUser, listOrganizationUsers, resendOrganizationUserInvitation, revokeOrganizationUserInvitation, updateOrganizationUserRole, updateOrganizationUserStatus } from '../lib/users'
import { isAbortError } from '../lib/api'
import { useAuth } from '../app/providers'
import { usePageTitle } from '../lib/use_page_title'
import { AdminMemberEmail } from './admin_member_email'

const emptyForm = {
  firstName: '',
  lastName: '',
  email: '',
  role: 'member'
}

const workLabels = [
  ['contacts', 'contacts'],
  ['companies', 'companies'],
  ['deals', 'deals'],
  ['tasks', 'tasks'],
  ['sharedInbox', 'inbox conversations'],
  ['leadRoutingRules', 'routing rules'],
  ['calendarEvents', 'future meetings']
]

const deliverySummaries = {
  complaint: 'Spam complaint · email blocked',
  bounced: 'Email bounced · resend or revoke',
  failed: 'Email failed · resend or revoke'
}

function ownedWorkSummary(ownedWork = {}) {
  const items = workLabels
    .filter(([key]) => Number(ownedWork[key] || 0) > 0)
    .map(([key, label]) => `${ownedWork[key]} ${label}`)
  return items.length > 0 ? items.join(', ') : 'No active assigned work'
}

function invitationSummary(user) {
  const expiresAt = user.invitationExpiresAt ? new Date(user.invitationExpiresAt) : null
  const expires = expiresAt && !Number.isNaN(expiresAt.getTime()) ? expiresAt.toLocaleString() : 'an unknown time'
  const delivery = deliverySummaries[user.invitationDeliveryStatus]
  if (delivery) return delivery
  switch (user.invitationStatus) {
    case 'pending':
      return `Invitation pending · expires ${expires}`
    case 'expired':
      return `Invitation expired ${expires}`
    case 'revoked':
      return 'Invitation revoked'
    default:
      return ''
  }
}

export function SettingsUsersRoute() {
  const { session, canAdminister: canManageUsers } = useAuth()
  usePageTitle('Users')
  const [users, setUsers] = useState([])
  const [error, setError] = useState('')
  const [form, setForm] = useState(emptyForm)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [isLoading, setIsLoading] = useState(false)
  const [savingRoleUserId, setSavingRoleUserId] = useState(0)
  const [changingStatusUserId, setChangingStatusUserId] = useState(0)
  const [deactivatingUserId, setDeactivatingUserId] = useState(0)
  const [resendingUserId, setResendingUserId] = useState(0)
  const [revokingUserId, setRevokingUserId] = useState(0)
  const [replacementUserId, setReplacementUserId] = useState('')
  const [lifecycleStatus, setLifecycleStatus] = useState('')
  const [latestSetupLink, setLatestSetupLink] = useState('')

  async function loadUsers({ signal } = {}) {
    if (!canManageUsers) {
      setError('Admin access required')
      setUsers([])
      return
    }

    setIsLoading(true)
    try {
      const entries = await listOrganizationUsers({ signal, includeDisabled: true })
      setUsers(entries)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load users.')
        setUsers([])
      }
    } finally {
      if (!signal?.aborted) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()

    loadUsers({ signal: controller.signal })
    return () => {
      controller.abort()
    }
  }, [canManageUsers])

  async function handleSubmit(event) {
    event.preventDefault()
    setIsSubmitting(true)
    setError('')
    setLatestSetupLink('')

    try {
      const created = await createOrganizationUser(form)
      setUsers((current) => [...current, created])
      setLatestSetupLink(created?.setupLink || '')
      setLifecycleStatus(`Invited ${created.email}; review delivery below.`)
      setForm(emptyForm)
    } catch (submitError) {
      setError(submitError.message || 'Unable to create user.')
    } finally {
      setIsSubmitting(false)
    }
  }

  async function handleResendInvitation(user) {
    setResendingUserId(user.id)
    setError('')
    setLifecycleStatus('')
    setLatestSetupLink('')
    try {
      const updated = await resendOrganizationUserInvitation(user.id)
      setUsers((current) => current.map((entry) => (entry.id === user.id ? updated : entry)))
      setLatestSetupLink(updated?.setupLink || '')
      setLifecycleStatus(`Invitation sent to ${user.email}; old links are invalid.`)
    } catch (submitError) {
      setError(submitError.message || 'Unable to resend invitation.')
    } finally {
      setResendingUserId(0)
    }
  }

  async function handleRevokeInvitation(user) {
    setChangingStatusUserId(user.id)
    setError('')
    setLifecycleStatus('')
    setLatestSetupLink('')
    try {
      const result = await revokeOrganizationUserInvitation(user.id)
      setUsers((current) => current.map((entry) => (entry.id === user.id ? result.user : entry)))
      setLifecycleStatus(`Invitation revoked for ${user.email}; all links are invalid.`)
      setRevokingUserId(0)
    } catch (submitError) {
      setError(submitError.message || 'Unable to revoke invitation.')
    } finally {
      setChangingStatusUserId(0)
    }
  }

  async function handleRoleChange(userId, role) {
    setSavingRoleUserId(userId)
    setError('')
    try {
      const updated = await updateOrganizationUserRole(userId, role)
      setUsers((current) => current.map((user) => (user.id === userId ? updated : user)))
    } catch (submitError) {
      setError(submitError.message || 'Unable to update user role.')
    } finally {
      setSavingRoleUserId(0)
    }
  }

  async function handleStatusChange(user, status) {
    setChangingStatusUserId(user.id)
    setError('')
    setLifecycleStatus('')
    setLatestSetupLink('')
    try {
      const result = await updateOrganizationUserStatus(user.id, {
        status,
        reassignToUserId: status === 'disabled' ? Number(replacementUserId) || 0 : 0
      })
      setUsers((current) => current.map((entry) => (entry.id === user.id ? result.user : entry)))
      if (status === 'disabled') {
        const reassigned = Object.values(result.reassigned || {}).reduce((total, count) => total + Number(count || 0), 0)
        setLifecycleStatus(`${user.firstName} ${user.lastName} was deactivated. ${reassigned} active work item${reassigned === 1 ? '' : 's'} reassigned; ${result.sessionsInvalidated || 0} session${result.sessionsInvalidated === 1 ? '' : 's'} ended.`)
      } else {
        setLifecycleStatus(user.invitationStatus === 'revoked'
          ? `${user.firstName} ${user.lastName} was reactivated. Resend their invitation before they can finish setup.`
          : `${user.firstName} ${user.lastName} was reactivated. They can sign in again; mailbox sync remains off until it is reviewed.`)
      }
      setDeactivatingUserId(0)
      setReplacementUserId('')
    } catch (submitError) {
      setError(submitError.message || 'Unable to update user access.')
    } finally {
      setChangingStatusUserId(0)
    }
  }

  const activeUsers = users.filter((user) => (user.status || 'active') === 'active')

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Team access</h2>
              <p>Manage who can work inside {session?.organization?.name || 'your workspace'}.</p>
            </div>
          </div>
          {isLoading ? <p className="field-hint">Loading team access...</p> : null}
          {error ? (
            <InlineError message={error} onRetry={canManageUsers ? () => loadUsers() : undefined} retryLabel="Retry users" />
          ) : null}
          {lifecycleStatus ? <p className="inline-note" role="status">{lifecycleStatus}</p> : null}
          <div className="record-list" role="list" aria-label="Organization users">
            {!isLoading && users.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No users found.</p>
                </div>
              </article>
            ) : users.map((user) => {
              const status = user.status || 'active'
              const isSelf = user.id === session?.user?.id
              const isDeactivating = deactivatingUserId === user.id
              const invitation = invitationSummary(user)
              const canManageInvitation = status === 'active' && ['pending', 'expired', 'revoked'].includes(user.invitationStatus)
              const isRevoking = revokingUserId === user.id
              return (
              <article className={`record-row${status === 'disabled' ? ' record-row-alert' : ''}`} key={user.id} role="listitem">
                <div>
                  <h3>
                    {user.firstName} {user.lastName}
                  </h3>
                  <p>{user.email}</p>
                  {invitation ? <p className="field-hint">{invitation}</p> : null}
                  <p className="field-hint"><span className="chip">{status === 'disabled' ? 'Disabled' : 'Active'}</span> · {ownedWorkSummary(user.ownedWork)}</p>
                </div>
                <div className="card-stack">
                  {canManageUsers ? (
                    <select className="text-input" aria-label={`Role for ${user.email}`} value={user.role} onChange={(event) => handleRoleChange(user.id, event.target.value)} disabled={savingRoleUserId === user.id || status === 'disabled'}>
                      <option value="member">Member</option>
                      <option value="admin">Admin</option>
                      <option value="viewer">Viewer</option>
                      <option value="owner">Owner</option>
                    </select>
                  ) : <p>{user.role}</p>}
                  {status === 'disabled' ? (
                    <Button className="button-secondary" type="button" onClick={() => handleStatusChange(user, 'active')} disabled={changingStatusUserId === user.id}>
                      {changingStatusUserId === user.id ? 'Reactivating…' : 'Reactivate'}
                    </Button>
                  ) : canManageInvitation && !isRevoking ? (
                    <div className="button-row">
                      {user.invitationDeliveryStatus !== 'complaint' ? (
                        <Button className="button-secondary" type="button" onClick={() => handleResendInvitation(user)} disabled={resendingUserId === user.id}>
                          {resendingUserId === user.id ? 'Resending…' : 'Resend invitation'}
                        </Button>
                      ) : null}
                      <Button className="button-danger" type="button" onClick={() => { setRevokingUserId(user.id); setLifecycleStatus('') }} disabled={resendingUserId === user.id}>
                        Revoke invitation
                      </Button>
                    </div>
                  ) : !isSelf && !isDeactivating ? (
                    <Button className="button-danger" type="button" onClick={() => { setDeactivatingUserId(user.id); setReplacementUserId(''); setLifecycleStatus('') }}>
                      Deactivate
                    </Button>
                  ) : null}
                  {isDeactivating ? (
                    <div className="inline-note">
                      <p><strong>Deactivate this member?</strong></p>
                      <p>Their sessions will end immediately. Choose where their active work should go; historical authorship is preserved.</p>
                      <label>
                        <span className="field-label">Reassign active work</span>
                        <select className="text-input" aria-label={`Reassign work from ${user.email}`} value={replacementUserId} onChange={(event) => setReplacementUserId(event.target.value)}>
                          <option value="">Leave unassigned</option>
                          {activeUsers.filter((candidate) => candidate.id !== user.id).map((candidate) => (
                            <option key={candidate.id} value={candidate.id}>{candidate.firstName} {candidate.lastName} ({candidate.email})</option>
                          ))}
                        </select>
                      </label>
                      <div className="button-row">
                        <Button className="button-danger" type="button" onClick={() => handleStatusChange(user, 'disabled')} disabled={changingStatusUserId === user.id}>
                          {changingStatusUserId === user.id ? 'Deactivating…' : 'Confirm deactivation'}
                        </Button>
                        <Button className="button-secondary" type="button" onClick={() => { setDeactivatingUserId(0); setReplacementUserId('') }} disabled={changingStatusUserId === user.id}>Cancel</Button>
                      </div>
                    </div>
                  ) : null}
                  {isRevoking ? (
                    <div className="inline-note">
                      <p><strong>Revoke this invitation?</strong></p>
                      <p>The one-time setup link will stop working immediately and this seat will be disabled. Historical records are preserved.</p>
                      <div className="button-row">
                        <Button className="button-danger" type="button" onClick={() => handleRevokeInvitation(user)} disabled={changingStatusUserId === user.id}>
                          {changingStatusUserId === user.id ? 'Revoking…' : 'Confirm revocation'}
                        </Button>
                        <Button className="button-secondary" type="button" onClick={() => setRevokingUserId(0)} disabled={changingStatusUserId === user.id}>Cancel</Button>
                      </div>
                    </div>
                  ) : null}
                </div>
              </article>
              )
            })}
          </div>
        </div>
      </Card>

      {canManageUsers ? (
        <Card>
          <div className="card-stack">
            <div>
              <h2>Invite user</h2>
              <p>Add another user to this organization, then send them the setup link so they can choose their own password.</p>
            </div>
            {latestSetupLink ? (
              <div className="inline-note" role="status">
                <strong>Setup link created:</strong> <code>{latestSetupLink}</code>
              </div>
            ) : null}
            <form className="auth-form" onSubmit={handleSubmit}>
              <Field label="First name">
                <input
                  className="text-input"
                  value={form.firstName}
                  onChange={(event) => setForm((current) => ({ ...current, firstName: event.target.value }))}
                  required
                />
              </Field>
              <Field label="Last name">
                <input
                  className="text-input"
                  value={form.lastName}
                  onChange={(event) => setForm((current) => ({ ...current, lastName: event.target.value }))}
                  required
                />
              </Field>
              <Field label="Email">
                <input
                  className="text-input"
                  type="email"
                  value={form.email}
                  onChange={(event) => setForm((current) => ({ ...current, email: event.target.value }))}
                  required
                />
              </Field>
              <Field label="Role">
                <select
                  className="text-input"
                  value={form.role}
                  onChange={(event) => setForm((current) => ({ ...current, role: event.target.value }))}
                >
                  <option value="member">Member</option>
                  <option value="admin">Admin</option>
                  <option value="viewer">Viewer</option>
                </select>
              </Field>
              <Button type="submit" disabled={isSubmitting}>
                {isSubmitting ? 'Inviting…' : 'Invite user'}
              </Button>
            </form>
          </div>
        </Card>
      ) : null}
      {canManageUsers ? <AdminMemberEmail users={activeUsers} /> : null}
    </section>
  )
}
