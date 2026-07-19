import { useState } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { useAuth } from '../app/providers'
import { usePageTitle } from '../lib/use_page_title'
import { getPreferences } from '../lib/profile'

export function LoginRoute() {
  const { status, login, resendVerification, error: authError } = useAuth()
  const navigate = useNavigate()
  usePageTitle('Sign in')
  const location = useLocation()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [verificationRequired, setVerificationRequired] = useState(false)
  const [verificationMessage, setVerificationMessage] = useState('')

  const redirectTo = location.state?.from?.pathname || '/dashboard'

  if (status === 'authenticated') {
    return <Navigate to={redirectTo} replace />
  }

  async function handleSubmit(event) {
    event.preventDefault()
    setIsSubmitting(true)
    setError('')
    setVerificationRequired(false)
    setVerificationMessage('')

    try {
      await login({ email, password })

      let destination = redirectTo
      if (!location.state?.from?.pathname) {
        try {
          const prefs = await getPreferences()
          destination = prefs?.defaultLandingView || destination
        } catch {
          // preferences unavailable, use default destination
        }
      }
      navigate(destination, { replace: true })
    } catch (loginError) {
      const message = loginError.message || 'Unable to sign in.'
      setError(message)
      setVerificationRequired(message.toLowerCase().includes('verify your email'))
    } finally {
      setIsSubmitting(false)
    }
  }

  async function handleResend() {
    setIsSubmitting(true)
    setError('')
    setVerificationMessage('')
    try {
      await resendVerification(email)
      setVerificationMessage('If this address is awaiting verification, another email is on its way. Requests are limited to protect recipients.')
    } catch (resendError) {
      setError(resendError.message || 'Unable to send verification email.')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="auth-layout">
      <Card className="auth-card">
        <div className="card-stack auth-stack">
          <div>
            <p className="eyebrow">Welcome back</p>
            <h1>Sign in to Open CRM</h1>
            <p className="page-description">
              Pick up where your pipeline left off without losing the thread.
            </p>
          </div>
          <form className="auth-form" onSubmit={handleSubmit}>
            <p className="page-description">Need a clean start? <a href="/bootstrap">Create a workspace</a>.</p>
            <Field label="Email">
              <input
                className="text-input"
                type="email"
                autoComplete="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                required
              />
            </Field>
            <Field label="Password">
              <input
                className="text-input"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                required
              />
            </Field>
            {error || authError ? <p className="form-error" role="alert">{error || authError}</p> : null}
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? 'Signing in…' : 'Sign in'}
            </Button>
            {verificationRequired ? (
              <Button className="button-secondary" type="button" onClick={handleResend} disabled={isSubmitting}>Resend verification email</Button>
            ) : null}
            {verificationMessage ? <p className="field-hint" role="status">{verificationMessage}</p> : null}
          </form>
        </div>
      </Card>
    </div>
  )
}
