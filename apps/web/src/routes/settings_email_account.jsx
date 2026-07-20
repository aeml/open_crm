import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { getMyEmailAccount, getMyEmailSyncStatus, startMyEmailOAuth, checkMyEmailSync, runMyEmailSync, saveMyEmailAccount, deleteMyEmailAccount } from '../lib/user_email'
import { usePageTitle } from '../lib/use_page_title'

const emptyForm = {
  fromEmail: '',
  fromName: '',
  smtpHost: '',
  smtpPort: 587,
  smtpUsername: '',
  smtpPassword: '',
  smtpUseTls: true,
  syncEnabled: false,
  provider: 'imap',
  authMethod: 'password',
  imapHost: '',
  imapPort: 993,
  imapUsername: '',
  imapPassword: '',
  imapUseTls: true
}

function syncStatusText(account) {
  if (!account?.syncEnabled) {
    return 'Sync disabled'
  }
  if (account.syncStatus === 'pending') {
    return 'Pending first sync'
  }
  if (account.syncStatus === 'error') {
    return 'Sync needs attention'
  }
  return account.syncStatus || 'Sync enabled'
}

function initialOAuthResultMessage() {
  if (typeof window === 'undefined') {
    return { status: '', error: '' }
  }
  const result = new URLSearchParams(window.location.search).get('emailSync')
  if (result === 'oauth_connected') {
    return { status: 'OAuth connected for sending and sync.', error: '' }
  }
  if (result === 'oauth_invalid_state') {
    return { status: '', error: 'Mailbox OAuth expired or could not be verified. Start the connection again.' }
  }
  if (result === 'oauth_not_configured') {
    return { status: '', error: 'Mailbox OAuth is not configured on this server yet.' }
  }
  if (result === 'oauth_missing_account') {
    return { status: '', error: 'Save your email account before connecting OAuth mailbox sync.' }
  }
  if (result === 'oauth_token_missing') {
    return { status: '', error: 'The provider did not return a refresh token. Try reconnecting and approving offline mailbox access.' }
  }
  if (result === 'oauth_scope_missing') {
    return { status: '', error: 'Mail sending permission was not granted. Reconnect and approve send and read access.' }
  }
  if (result === 'oauth_exchange_failed' || result === 'oauth_error') {
    return { status: '', error: 'Mailbox OAuth connection failed. Try again from this page.' }
  }
  return { status: '', error: '' }
}

export function SettingsEmailAccountRoute() {
  const { session, workspaceWritable } = useAuth()
  usePageTitle('My Email')
  const initialOAuthResult = initialOAuthResultMessage()
  const [form, setForm] = useState(emptyForm)
  const [configured, setConfigured] = useState(true)
  const [hasAccount, setHasAccount] = useState(false)
  const [error, setError] = useState(initialOAuthResult.error)
  const [status, setStatus] = useState(initialOAuthResult.status)
  const [syncStatus, setSyncStatus] = useState({ account: null, oauthProviders: [] })
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  const [isCheckingSync, setIsCheckingSync] = useState(false)
  const [isRunningSync, setIsRunningSync] = useState(false)
  const [startingOAuthProvider, setStartingOAuthProvider] = useState('')

  useEffect(() => {
    const controller = new AbortController()
    async function load() {
      setIsLoading(true)
      try {
        const [data, nextSyncStatus] = await Promise.all([
          getMyEmailAccount({ signal: controller.signal }),
          getMyEmailSyncStatus({ signal: controller.signal })
        ])
        setSyncStatus({ account: nextSyncStatus.account || null, oauthProviders: nextSyncStatus.oauthProviders || [] })
        setConfigured(data.configured)
        const account = data.account || nextSyncStatus.account
        if (account) {
          setHasAccount(true)
          setForm({
            fromEmail: account.fromEmail || '',
            fromName: account.fromName || '',
            smtpHost: account.smtpHost || '',
            smtpPort: account.smtpPort || 587,
            smtpUsername: account.smtpUsername || '',
            smtpPassword: '',
            smtpUseTls: account.smtpUseTls !== false,
            syncEnabled: !!account.syncEnabled,
            provider: account.provider && account.provider !== 'smtp' ? account.provider : 'imap',
            authMethod: account.authMethod || 'password',
            imapHost: account.imapHost || '',
            imapPort: account.imapPort || 993,
            imapUsername: account.imapUsername || '',
            imapPassword: '',
            imapUseTls: account.imapUseTls !== false
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
      const account = await saveMyEmailAccount({ ...form, smtpPort: Number(form.smtpPort), imapPort: Number(form.imapPort) })
      setHasAccount(true)
      setForm({ ...form, smtpPassword: '', imapPassword: '' })
      if (account) {
        setSyncStatus({ account, oauthProviders: syncStatus.oauthProviders || [] })
      }
      setStatus(form.authMethod === 'oauth' ? 'Mailbox settings saved. Connect OAuth below to send and sync.' : 'Email account saved. Emails you send to contacts will come from your address.')
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to save email account.')
    } finally {
      setIsSaving(false)
    }
  }

  async function handleStartOAuth(provider) {
    setStartingOAuthProvider(provider.provider)
    setStatus('')
    setError('')
    try {
      const data = await startMyEmailOAuth(provider.provider)
      if (!data.authorizationUrl) {
        throw new Error('Provider did not return an authorization URL.')
      }
      window.location.assign(data.authorizationUrl)
    } catch (oauthError) {
      setError(oauthError.message || 'Unable to start mailbox OAuth.')
      setStartingOAuthProvider('')
    }
  }

  async function handleCheckSync() {
    setIsCheckingSync(true)
    setStatus('')
    setError('')
    try {
      const data = await checkMyEmailSync()
      if (data.account) {
        setSyncStatus({ account: data.account, oauthProviders: syncStatus.oauthProviders || [] })
      }
      if (data.status === 'ready') {
        setStatus('Mailbox sync settings are ready. You can manually import generic IMAP messages now.')
      } else {
        setError(data.error || 'Mailbox sync is not ready yet.')
      }
    } catch (syncError) {
      setError(syncError.message || 'Unable to check mailbox sync.')
    } finally {
      setIsCheckingSync(false)
    }
  }

  async function handleRunSync() {
    setIsRunningSync(true)
    setStatus('')
    setError('')
    try {
      const data = await runMyEmailSync()
      if (data.account) {
        setSyncStatus({ account: data.account, oauthProviders: syncStatus.oauthProviders || [] })
      }
      if (data.status === 'ready') {
        const imported = Number(data.imported || 0)
        setStatus(`Mailbox sync imported ${imported} new ${imported === 1 ? 'message' : 'messages'}.`)
      } else {
        setError(data.error || 'Mailbox sync did not complete.')
      }
    } catch (syncError) {
      setError(syncError.message || 'Unable to run mailbox sync.')
    } finally {
      setIsRunningSync(false)
    }
  }

  async function handleDelete() {
    try {
      await deleteMyEmailAccount()
      setHasAccount(false)
      setForm(emptyForm)
      setSyncStatus({ account: null, oauthProviders: syncStatus.oauthProviders || [] })
      setStatus('Email account removed.')
      setError('')
    } catch (deleteError) {
      setError(deleteError.message || 'Unable to remove email account.')
    }
  }

  const usesOAuthDelivery = form.syncEnabled && (form.provider === 'google' || form.provider === 'microsoft')

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
              {!usesOAuthDelivery ? (
                <div className="card-stack">
                  <Field label="SMTP host">
                    <input className="text-input" value={form.smtpHost} onChange={(event) => setForm({ ...form, smtpHost: event.target.value })} placeholder="smtp.example.com" required />
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
                </div>
              ) : (
                <p className="field-hint">Outbound email uses the provider API; no SMTP password is stored.</p>
              )}
              <Card>
                <div className="card-stack">
                  <div>
                    <h3>Mailbox sync</h3>
                    <p className="field-hint">Import replies and mailbox history manually or with the background worker.</p>
                    {syncStatus.account ? <p className="field-hint">Status: {syncStatusText(syncStatus.account)}</p> : null}
                  </div>
                  <label className="checkbox-row">
                    <input type="checkbox" checked={form.syncEnabled} onChange={(event) => setForm({ ...form, syncEnabled: event.target.checked })} />
                    <span>Enable mailbox sync metadata</span>
                  </label>
                  <div>
                    <Button type="button" className="button-secondary" disabled={!workspaceWritable || !hasAccount || !form.syncEnabled || isCheckingSync} onClick={handleCheckSync}>
                      {isCheckingSync ? 'Checking...' : 'Check sync readiness'}
                    </Button>
                    <Button type="button" className="button-secondary" disabled={!workspaceWritable || !hasAccount || !form.syncEnabled || isRunningSync} onClick={handleRunSync}>
                      {isRunningSync ? 'Syncing...' : 'Run sync now'}
                    </Button>
                  </div>
                  {form.syncEnabled ? (
                    <div className="card-stack">
                      <Field label="Sync provider">
                        <select className="text-input" value={form.provider} onChange={(event) => setForm({ ...form, provider: event.target.value, authMethod: event.target.value === 'imap' ? 'password' : 'oauth' })}>
                          <option value="imap">Generic IMAP</option>
                          <option value="google">Google Workspace / Gmail OAuth</option>
                          <option value="microsoft">Microsoft 365 / Outlook OAuth</option>
                        </select>
                      </Field>
                      {form.provider === 'imap' ? (
                        <div className="card-stack">
                          <Field label="IMAP host">
                            <input className="text-input" value={form.imapHost} onChange={(event) => setForm({ ...form, imapHost: event.target.value })} placeholder="imap.gmail.com" required />
                          </Field>
                          <Field label="IMAP port">
                            <input className="text-input" type="number" value={form.imapPort} onChange={(event) => setForm({ ...form, imapPort: event.target.value })} required />
                          </Field>
                          <Field label="IMAP username">
                            <input className="text-input" value={form.imapUsername} onChange={(event) => setForm({ ...form, imapUsername: event.target.value })} required />
                          </Field>
                          <Field label={hasAccount ? 'IMAP password (leave blank to keep current)' : 'IMAP password'}>
                            <input className="text-input" type="password" value={form.imapPassword} onChange={(event) => setForm({ ...form, imapPassword: event.target.value })} autoComplete="new-password" />
                          </Field>
                          <label className="checkbox-row">
                            <input type="checkbox" checked={form.imapUseTls} onChange={(event) => setForm({ ...form, imapUseTls: event.target.checked })} />
                            <span>Use IMAP TLS / SSL</span>
                          </label>
                        </div>
                      ) : (
                        <p className="field-hint">Save, then connect OAuth below for sending and mailbox sync.</p>
                      )}
                    </div>
                  ) : null}
                  {syncStatus.oauthProviders?.length > 0 ? (
                    <div className="record-list" role="list" aria-label="OAuth mailbox providers">
                      {syncStatus.oauthProviders.map((provider) => (
                        <article className="record-row" key={provider.provider} role="listitem">
                          <div>
                            <p>{provider.label}</p>
                            <p className="field-hint">{provider.configured ? 'OAuth client configured' : 'OAuth client not configured'} · {provider.status}</p>
                            {syncStatus.account?.provider === provider.provider && syncStatus.account?.oauthSendReady ? <p className="field-hint">Connected for sending and sync</p> : null}
                            {syncStatus.account?.provider === provider.provider && syncStatus.account?.oauthConnected && !syncStatus.account?.oauthSendReady ? <p className="field-hint">Reconnect to approve provider sending</p> : null}
                          </div>
                          <Button type="button" className="button-secondary" disabled={!workspaceWritable || !provider.configured || !hasAccount || form.provider !== provider.provider || !form.syncEnabled || startingOAuthProvider === provider.provider} onClick={() => handleStartOAuth(provider)}>
                            {startingOAuthProvider === provider.provider ? 'Starting...' : `Connect ${provider.provider === 'google' ? 'Google' : 'Microsoft'}`}
                          </Button>
                        </article>
                      ))}
                    </div>
                  ) : null}
                </div>
              </Card>
              <div>
                <Button type="submit" disabled={!workspaceWritable || isSaving}>{isSaving ? 'Saving...' : 'Save connection'}</Button>
                {hasAccount ? <Button className="button-secondary" type="button" onClick={handleDelete} disabled={!workspaceWritable}>Remove</Button> : null}
              </div>
            </form>
          ) : null}
        </div>
      </Card>
    </section>
  )
}
