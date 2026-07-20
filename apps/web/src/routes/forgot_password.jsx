import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useAuth } from '../app/providers'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { usePageTitle } from '../lib/use_page_title'

export function ForgotPasswordRoute() {
  const { requestPasswordReset } = useAuth()
  const [email, setEmail] = useState('')
  const [error, setError] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [result, setResult] = useState(null)
  usePageTitle('Reset password')

  async function handleSubmit(event) {
    event.preventDefault()
    setIsSubmitting(true)
    setError('')
    try {
      setResult(await requestPasswordReset(email))
    } catch (requestError) {
      setError(requestError.message || 'Unable to request a password reset.')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="auth-layout">
      <Card className="auth-card">
        <div className="card-stack auth-stack">
          <div>
            <p className="eyebrow">Account recovery</p>
            <h1>{result ? 'Check your email' : 'Reset your password'}</h1>
            <p className="page-description">
              {result
                ? 'If an active Open CRM account matches that address, a one-time reset link is on its way. It expires in 1 hour.'
                : 'Enter your account email. For privacy, the result is the same whether or not an active account exists.'}
            </p>
          </div>
          {result ? (
            <div className="auth-form">
              {result.resetLink ? <Link className="button button-primary" to={result.resetLink}>Reset password locally</Link> : null}
              <Link to="/login">Return to sign in</Link>
            </div>
          ) : (
            <form className="auth-form" onSubmit={handleSubmit}>
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
              {error ? <p className="form-error" role="alert">{error}</p> : null}
              <Button type="submit" disabled={isSubmitting}>
                {isSubmitting ? 'Requesting…' : 'Send reset link'}
              </Button>
              <Link to="/login">Return to sign in</Link>
            </form>
          )}
        </div>
      </Card>
    </div>
  )
}
