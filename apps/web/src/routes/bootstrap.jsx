import { useState } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { useAuth } from '../app/providers'
import { usePageTitle } from '../lib/use_page_title'

const businessTypeOptions = [
  { value: 'general', label: 'General CRM' },
  { value: 'services', label: 'Services (Clients + Jobs)' },
  { value: 'product-sales', label: 'Product Sales (Accounts + Opportunities)' },
  { value: 'construction-services', label: 'Construction Services (Clients + Jobs)' }
]

export function BootstrapRoute() {
  const { status, bootstrap, resendVerification, error: authError } = useAuth()
  usePageTitle('Create workspace')
  const [organizationName, setOrganizationName] = useState('')
  const [businessType, setBusinessType] = useState('general')
  const [firstName, setFirstName] = useState('')
  const [lastName, setLastName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [result, setResult] = useState(null)
  const [resendMessage, setResendMessage] = useState('')
  const [idempotencyKey] = useState(() => {
    const existing = window.sessionStorage.getItem('open-crm-bootstrap-key')
    if (existing) return existing
    const suffix = globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random().toString(16).slice(2)}`
    const created = `workspace-${suffix}`
    window.sessionStorage.setItem('open-crm-bootstrap-key', created)
    return created
  })

  if (status === 'authenticated') {
    return <Navigate to="/dashboard" replace />
  }

  async function handleSubmit(event) {
    event.preventDefault()
    setIsSubmitting(true)
    setError('')

    try {
      const created = await bootstrap({ organizationName, businessType, firstName, lastName, email, password, idempotencyKey })
      setResult(created)
      window.sessionStorage.removeItem('open-crm-bootstrap-key')
    } catch (bootstrapError) {
      setError(bootstrapError.message || 'Unable to create workspace.')
    } finally {
      setIsSubmitting(false)
    }
  }

  async function handleResend() {
    setIsSubmitting(true)
    setError('')
    setResendMessage('')
    try {
      const resent = await resendVerification(result.email)
      setResult({ ...result, verificationLink: resent.verificationLink || result.verificationLink })
      setResendMessage('If this address is awaiting verification, another email is on its way. Requests are limited to protect recipients.')
    } catch (resendError) {
      setError(resendError.message || 'Unable to send verification email.')
    } finally {
      setIsSubmitting(false)
    }
  }

  if (result) {
    return (
      <div className="auth-layout">
        <Card className="auth-card">
          <div className="card-stack auth-stack">
            <div>
              <p className="eyebrow">One secure step remains</p>
              <h1>Check your email</h1>
              <p className="page-description">We sent a one-time verification link to <strong>{result.email}</strong>. The link expires in 24 hours; your 14-day trial starts only after verification.</p>
              <p className="page-description">No owner session exists yet, so signing in cannot bypass this step.</p>
            </div>
            {result.verificationLink ? (
              <Link className="button button-primary" to={result.verificationLink}>Verify email locally</Link>
            ) : null}
            <Button className="button-secondary" type="button" onClick={handleResend} disabled={isSubmitting}>
              {isSubmitting ? 'Requesting…' : 'Resend verification email'}
            </Button>
            {resendMessage ? <p className="field-hint" role="status">{resendMessage}</p> : null}
            {error || authError ? <p className="form-error" role="alert">{error || authError}</p> : null}
            <p className="page-description"><Link to="/login">Return to sign in</Link>.</p>
          </div>
        </Card>
      </div>
    )
  }

  return (
    <div className="auth-layout">
      <Card className="auth-card">
        <div className="card-stack auth-stack">
          <div>
            <p className="eyebrow">Start clean</p>
            <h1>Create your workspace</h1>
            <p className="page-description">Set up the company, verify the owner email, and then start a 14-day trial.</p>
            <p className="page-description">Already have an account? <Link to="/login">Sign in</Link>.</p>
          </div>
          <form className="auth-form" onSubmit={handleSubmit}>
            <Field label="Company name">
              <input className="text-input" value={organizationName} onChange={(event) => setOrganizationName(event.target.value)} required />
            </Field>
            <Field label="Business type">
              <select className="text-input" value={businessType} onChange={(event) => setBusinessType(event.target.value)}>
                {businessTypeOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="First name">
              <input className="text-input" value={firstName} onChange={(event) => setFirstName(event.target.value)} required />
            </Field>
            <Field label="Last name">
              <input className="text-input" value={lastName} onChange={(event) => setLastName(event.target.value)} required />
            </Field>
            <Field label="Email">
              <input className="text-input" type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} required />
            </Field>
            <Field label="Password">
              <input className="text-input" type="password" autoComplete="new-password" minLength={12} value={password} onChange={(event) => setPassword(event.target.value)} required />
            </Field>
            <p className="field-hint">Use at least 12 characters.</p>
            {error || authError ? <p className="form-error" role="alert">{error || authError}</p> : null}
            <Button type="submit" disabled={isSubmitting}>{isSubmitting ? 'Creating…' : 'Create workspace'}</Button>
          </form>
        </div>
      </Card>
    </div>
  )
}
