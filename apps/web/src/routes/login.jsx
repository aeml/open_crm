import { useState } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { useAuth } from '../app/providers'
import { usePageTitle } from '../lib/use_page_title'
import { getPreferences } from '../lib/profile'

export function LoginRoute() {
  const { status, login, error: authError } = useAuth()
  const navigate = useNavigate()
  usePageTitle('Sign in')
  const location = useLocation()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  const redirectTo = location.state?.from?.pathname || '/dashboard'

  if (status === 'authenticated') {
    return <Navigate to={redirectTo} replace />
  }

  async function handleSubmit(event) {
    event.preventDefault()
    setIsSubmitting(true)
    setError('')

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
      setError(loginError.message || 'Unable to sign in.')
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
          </form>
        </div>
      </Card>
    </div>
  )
}
