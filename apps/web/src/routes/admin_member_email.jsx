import { useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { getUserEmailAccount, saveUserEmailAccount } from '../lib/user_email'

const emptyForm = {
  fromEmail: '',
  fromName: '',
  smtpHost: '',
  smtpPort: 587,
  smtpUsername: '',
  smtpPassword: '',
  smtpUseTls: true
}

export function AdminMemberEmail({ users = [] }) {
  const [selectedUserId, setSelectedUserId] = useState('')
  const [form, setForm] = useState(emptyForm)
  const [configured, setConfigured] = useState(true)
  const [error, setError] = useState('')
  const [status, setStatus] = useState('')
  const [isSaving, setIsSaving] = useState(false)

  async function handleSelect(userId) {
    setSelectedUserId(userId)
    setStatus('')
    setError('')
    setForm(emptyForm)
    if (!userId) {
      return
    }
    try {
      const data = await getUserEmailAccount(userId)
      setConfigured(data.configured)
      if (data.account) {
        setForm({
          fromEmail: data.account.fromEmail || '',
          fromName: data.account.fromName || '',
          smtpHost: data.account.smtpHost || '',
          smtpPort: data.account.smtpPort || 587,
          smtpUsername: data.account.smtpUsername || '',
          smtpPassword: '',
          smtpUseTls: data.account.smtpUseTls !== false
        })
      }
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load email account.')
      }
    }
  }

  async function handleSubmit(event) {
    event.preventDefault()
    if (!selectedUserId) {
      return
    }
    setIsSaving(true)
    setStatus('')
    try {
      await saveUserEmailAccount(selectedUserId, { ...form, smtpPort: Number(form.smtpPort) })
      setForm({ ...form, smtpPassword: '' })
      setStatus('Email connection saved for this member.')
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to save email account.')
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <Card>
      <div className="card-stack">
        <div>
          <h2>Set up email for a member</h2>
          <p>Connect a team member&apos;s mailbox so their customer emails send from their address.</p>
        </div>
        <Field label="Team member">
          <select className="text-input" value={selectedUserId} onChange={(event) => handleSelect(event.target.value)}>
            <option value="">Select a member...</option>
            {users.map((user) => (
              <option key={user.id} value={user.id}>{user.firstName} {user.lastName} ({user.email})</option>
            ))}
          </select>
        </Field>
        {status ? <p className="field-hint" role="status">{status}</p> : null}
        {error ? <InlineError message={error} /> : null}
        {!configured ? (
          <div className="inline-note">Email connections are not enabled on this server yet. Ask your administrator to set <code>CREDENTIAL_ENCRYPTION_KEY</code>.</div>
        ) : null}
        {selectedUserId && configured ? (
          <form className="auth-form card-stack" onSubmit={handleSubmit}>
            <Field label="From email">
              <input className="text-input" type="email" value={form.fromEmail} onChange={(event) => setForm({ ...form, fromEmail: event.target.value })} required />
            </Field>
            <Field label="From name">
              <input className="text-input" value={form.fromName} onChange={(event) => setForm({ ...form, fromName: event.target.value })} />
            </Field>
            <Field label="SMTP host">
              <input className="text-input" value={form.smtpHost} onChange={(event) => setForm({ ...form, smtpHost: event.target.value })} required />
            </Field>
            <Field label="SMTP port">
              <input className="text-input" type="number" value={form.smtpPort} onChange={(event) => setForm({ ...form, smtpPort: event.target.value })} required />
            </Field>
            <Field label="SMTP username">
              <input className="text-input" value={form.smtpUsername} onChange={(event) => setForm({ ...form, smtpUsername: event.target.value })} required />
            </Field>
            <Field label="SMTP password">
              <input className="text-input" type="password" value={form.smtpPassword} onChange={(event) => setForm({ ...form, smtpPassword: event.target.value })} autoComplete="new-password" />
            </Field>
            <label className="checkbox-row">
              <input type="checkbox" checked={form.smtpUseTls} onChange={(event) => setForm({ ...form, smtpUseTls: event.target.checked })} />
              <span>Use TLS / STARTTLS</span>
            </label>
            <Button type="submit" disabled={isSaving}>{isSaving ? 'Saving...' : 'Save member email'}</Button>
          </form>
        ) : null}
      </div>
    </Card>
  )
}
