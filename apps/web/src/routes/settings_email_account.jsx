import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { getMyEmailAccount, saveMyEmailAccount, deleteMyEmailAccount } from '../lib/user_email'
import { usePageTitle } from '../lib/use_page_title'

const emptyForm = {
  fromEmail: '',
  fromName: '',
  smtpHost: '',
  smtpPort: 587,
  smtpUsername: '',
  smtpPassword: '',
  smtpUseTls: true
}

export function SettingsEmailAccountRoute() {
  const { session } = useAuth()
  usePageTitle('My Email')
  const [form, setForm] = useState(emptyForm)
  const [configured, setConfigured] = useState(true)
  const [hasAccount, setHasAccount] = useState(false)
  const [error, setError] = useState('')
  const [status, setStatus] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    async function load() {
      setIsLoading(true)
      try {
        const data = await getMyEmailAccount({ signal: controller.signal })
        setConfigured(data.configured)
        if (data.account) {
          setHasAccount(true)
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
        setError('')
      } catch (loadError) {
        if (!isAbortError(loadError)) {
          setError(loadError.message || 'Unable to load email account.')
        }
      } finally {
        if (!controller.signal.aborted) {
          setIsLoading(false)
        }
      }
    }
    load()
    return () => {
      controller.abort()
    }
  }, [])

  async function handleSubmit(event) {
    event.preventDefault()
    setIsSaving(true)
    setStatus('')
    try {
      await saveMyEmailAccount({ ...form, smtpPort: Number(form.smtpPort) })
      setHasAccount(true)
      setForm({ ...form, smtpPassword: '' })
      setStatus('Email account saved. Emails you send to contacts will come from your address.')
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to save email account.')
    } finally {
      setIsSaving(false)
    }
  }

  async function handleDelete() {
    try {
      await deleteMyEmailAccount()
      setHasAccount(false)
      setForm(emptyForm)
      setStatus('Email account removed.')
      setError('')
    } catch (deleteError) {
      setError(deleteError.message || 'Unable to remove email account.')
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>My email connection</h2>
              <p>Connect your mailbox so emails you send to contacts come from you ({session?.user?.email}). System emails like invites are sent separately by Open CRM.</p>
            </div>
          </div>
          {isLoading ? <p className="field-hint">Loading...</p> : null}
          {!isLoading && !configured ? (
            <div className="inline-note">Email connections are not enabled on this server yet. Ask your administrator to set <code>CREDENTIAL_ENCRYPTION_KEY</code>.</div>
          ) : null}
          {status ? <p className="field-hint" role="status">{status}</p> : null}
          {error ? <InlineError message={error} /> : null}
          {!isLoading && configured ? (
            <form className="auth-form card-stack" onSubmit={handleSubmit}>
              <Field label="From email">
                <input className="text-input" type="email" value={form.fromEmail} onChange={(event) => setForm({ ...form, fromEmail: event.target.value })} required />
              </Field>
              <Field label="From name">
                <input className="text-input" value={form.fromName} onChange={(event) => setForm({ ...form, fromName: event.target.value })} />
              </Field>
              <Field label="SMTP host">
                <input className="text-input" value={form.smtpHost} onChange={(event) => setForm({ ...form, smtpHost: event.target.value })} placeholder="smtp.gmail.com" required />
              </Field>
              <Field label="SMTP port">
                <input className="text-input" type="number" value={form.smtpPort} onChange={(event) => setForm({ ...form, smtpPort: event.target.value })} required />
              </Field>
              <Field label="SMTP username">
                <input className="text-input" value={form.smtpUsername} onChange={(event) => setForm({ ...form, smtpUsername: event.target.value })} required />
              </Field>
              <Field label={hasAccount ? 'SMTP password (leave blank to keep current)' : 'SMTP password'}>
                <input className="text-input" type="password" value={form.smtpPassword} onChange={(event) => setForm({ ...form, smtpPassword: event.target.value })} autoComplete="new-password" />
              </Field>
              <label className="checkbox-row">
                <input type="checkbox" checked={form.smtpUseTls} onChange={(event) => setForm({ ...form, smtpUseTls: event.target.checked })} />
                <span>Use TLS / STARTTLS</span>
              </label>
              <div>
                <Button type="submit" disabled={isSaving}>{isSaving ? 'Saving...' : 'Save connection'}</Button>
                {hasAccount ? <Button className="button-secondary" type="button" onClick={handleDelete}>Remove</Button> : null}
              </div>
            </form>
          ) : null}
        </div>
      </Card>
    </section>
  )
}
