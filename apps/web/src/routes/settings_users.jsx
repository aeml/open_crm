import { useEffect, useMemo, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { createOrganizationUser, listOrganizationUsers, updateOrganizationUserRole, updateOrganizationUserStatus } from '../lib/users'
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

function ownedWorkSummary(ownedWork = {}) {
  const items = workLabels
    .filter(([key]) => Number(ownedWork[key] || 0) > 0)
    .map(([key, label]) => `${ownedWork[key]} ${label}`)
  return items.length > 0 ? items.join(', ') : 'No active assigned work'
}

export function SettingsUsersRoute() {
  const { session } = useAuth()
  usePageTitle('Users')
  const role = session?.membership?.role || ''
  const canManageUsers = useMemo(() => ['owner', 'admin'].includes(role), [role])
  const [users, setUsers] = useState([])
  const [error, setError] = useState('')
  const [form, setForm] = useState(emptyForm)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [isLoading, setIsLoading] = useState(false)
  const [savingRoleUserId, setSavingRoleUserId] = useState(0)
  const [changingStatusUserId, setChangingStatusUserId] = useState(0)
  const [deactivatingUserId, setDeactivatingUserId] = useState(0)
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

    try {
      const created = await createOrganizationUser(form)
      setUsers((current) => [...current, created])
      setLatestSetupLink(created?.setupLink || '')
      setForm(emptyForm)
    } catch (submitError) {
      setError(submitError.message || 'Unable to create user.')
    } finally {
      setIsSubmitting(false)
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
        setLifecycleStatus(`${user.firstName} ${user.lastName} was reactivated. They can sign in again; mailbox sync remains off until it is reviewed.`)
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
              return (
              <article className={`record-row${status === 'disabled' ? ' record-row-alert' : ''}`} key={user.id} role="listitem">
                <div>
                  <h3>
                    {user.firstName} {user.lastName}
                  </h3>
                  <p>{user.email}</p>
                  {user.setupPending ? <p className="field-hint">Pending setup</p> : null}
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
