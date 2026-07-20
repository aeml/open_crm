import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useAuth } from '../app/providers'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { usePageTitle } from '../lib/use_page_title'

export function ResetPasswordRoute() {
  const { completePasswordReset } = useAuth()
  const [searchParams, setSearchParams] = useSearchParams()
  const token = searchParams.get('token') || ''
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [error, setError] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [completed, setCompleted] = useState(false)
  usePageTitle('Choose a new password')

  async function handleSubmit(event) {
    event.preventDefault()
    setError('')
    if (password !== confirmation) {
      setError('Passwords do not match.')
      return
    }
    setIsSubmitting(true)
    try {
      await completePasswordReset({ token, password })
      setSearchParams({}, { replace: true })
      setCompleted(true)
    } catch (resetError) {
      setError(resetError.message || 'Unable to reset password.')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="auth-layout">
      <Card className="auth-card">
        <div className="card-stack auth-stack">
          {completed ? (
            <>
              <div>
                <p className="eyebrow">Account recovery</p>
                <h1>Password reset complete</h1>
                <p className="page-description">Your old sessions have been signed out. Use your new password the next time you sign in.</p>
              </div>
              <Link className="button button-primary" to="/login">Sign in</Link>
            </>
          ) : (
            <>
              <div>
                <p className="eyebrow">Account recovery</p>
                <h1>Choose a new password</h1>
                <p className="page-description">Use at least 12 characters. Completing this reset signs you out on every device.</p>
              </div>
              <form className="auth-form" onSubmit={handleSubmit}>
                {!token ? <p className="form-error" role="alert">Reset token is missing. Request a new reset link.</p> : null}
                <Field label="New password">
                  <input
                    className="text-input"
                    type="password"
                    autoComplete="new-password"
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                    minLength={12}
                    required
                    disabled={!token}
                  />
                </Field>
                <Field label="Confirm new password">
                  <input
                    className="text-input"
                    type="password"
                    autoComplete="new-password"
                    value={confirmation}
                    onChange={(event) => setConfirmation(event.target.value)}
                    minLength={12}
                    required
                    disabled={!token}
                  />
                </Field>
                {error ? <p className="form-error" role="alert">{error}</p> : null}
                <Button type="submit" disabled={isSubmitting || !token}>
                  {isSubmitting ? 'Resetting…' : 'Reset password'}
                </Button>
                <Link to="/forgot-password">Request a new link</Link>
              </form>
            </>
          )}
        </div>
      </Card>
    </div>
  )
}
