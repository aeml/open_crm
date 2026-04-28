import { useEffect, useMemo, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { createOrganizationUser, listOrganizationUsers } from '../lib/users'
import { isAbortError } from '../lib/api'
import { useAuth } from '../app/providers'

const emptyForm = {
  firstName: '',
  lastName: '',
  email: '',
  role: 'member'
}

export function SettingsUsersRoute() {
  const { session } = useAuth()
  const role = session?.membership?.role || ''
  const canManageUsers = useMemo(() => ['owner', 'admin'].includes(role), [role])
  const [users, setUsers] = useState([])
  const [error, setError] = useState('')
  const [form, setForm] = useState(emptyForm)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [isLoading, setIsLoading] = useState(false)
  const [latestSetupLink, setLatestSetupLink] = useState('')

  async function loadUsers({ signal } = {}) {
    if (!canManageUsers) {
      setError('Admin access required')
      setUsers([])
      return
    }

    setIsLoading(true)
    try {
      const entries = await listOrganizationUsers({ signal })
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
            <div className="card-stack">
              <p className="form-error">{error}</p>
              {canManageUsers ? (
                <div>
                  <Button className="button-secondary" type="button" onClick={() => loadUsers()}>
                    Retry users
                  </Button>
                </div>
              ) : null}
            </div>
          ) : null}
          <div className="record-list" role="list" aria-label="Organization users">
            {!isLoading && users.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No users found.</p>
                </div>
              </article>
            ) : users.map((user) => (
              <article className="record-row" key={user.email} role="listitem">
                <div>
                  <h3>
                    {user.firstName} {user.lastName}
                  </h3>
                  <p>{user.email}</p>
                </div>
                <div>
                  <p>{user.role}</p>
                </div>
              </article>
            ))}
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
    </section>
  )
}
