import { useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { completeUserSetup } from '../lib/users'

export function SetupPasswordRoute() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const token = searchParams.get('token') || ''
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  async function handleSubmit(event) {
    event.preventDefault()
    setIsSubmitting(true)
    setError('')

    try {
      await completeUserSetup({ token, password })
      navigate('/login', { replace: true })
    } catch (setupError) {
      setError(setupError.message || 'Unable to complete password setup.')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="auth-layout">
      <Card className="auth-card">
        <div className="card-stack auth-stack">
          <div>
            <p className="eyebrow">Account setup</p>
            <h1>Choose your password</h1>
            <p className="page-description">Finish activating your Open CRM account with a password only you know.</p>
          </div>
          <form className="auth-form" onSubmit={handleSubmit}>
            {!token ? <p className="form-error">Setup token is missing. Ask an admin for a new setup link.</p> : null}
            <Field label="New password">
              <input
                className="text-input"
                type="password"
                autoComplete="new-password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                required
                disabled={!token}
              />
            </Field>
            {error ? <p className="form-error">{error}</p> : null}
            <Button type="submit" disabled={isSubmitting || !token}>
              {isSubmitting ? 'Setting password...' : 'Set password'}
            </Button>
            <p className="page-description">
              Already activated? <Link to="/login">Sign in</Link>.
            </p>
          </form>
        </div>
      </Card>
    </div>
  )
}
