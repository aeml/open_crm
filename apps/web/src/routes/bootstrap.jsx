import { useState } from 'react'
import { Link, Navigate, useNavigate } from 'react-router-dom'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { useAuth } from '../app/providers'

const businessTypeOptions = [
  { value: 'general', label: 'General CRM' },
  { value: 'services', label: 'Services' },
  { value: 'product-sales', label: 'Product Sales' },
  { value: 'construction-services', label: 'Construction Services' }
]

export function BootstrapRoute() {
  const { status, bootstrap, error: authError } = useAuth()
  const navigate = useNavigate()
  const [organizationName, setOrganizationName] = useState('')
  const [businessType, setBusinessType] = useState('general')
  const [firstName, setFirstName] = useState('')
  const [lastName, setLastName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  if (status === 'authenticated') {
    return <Navigate to="/dashboard" replace />
  }

  async function handleSubmit(event) {
    event.preventDefault()
    setIsSubmitting(true)
    setError('')

    try {
      await bootstrap({ organizationName, businessType, firstName, lastName, email, password })
      navigate('/dashboard', { replace: true })
    } catch (bootstrapError) {
      setError(bootstrapError.message || 'Unable to create workspace.')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="auth-layout">
      <Card className="auth-card">
        <div className="card-stack auth-stack">
          <div>
            <p className="eyebrow">Start clean</p>
            <h1>Create your workspace</h1>
            <p className="page-description">Set up the company, choose the business type, and land as the owner in one pass.</p>
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
              <input className="text-input" type="password" autoComplete="new-password" value={password} onChange={(event) => setPassword(event.target.value)} required />
            </Field>
            {error || authError ? <p className="form-error">{error || authError}</p> : null}
            <Button type="submit" disabled={isSubmitting}>{isSubmitting ? 'Creating…' : 'Create workspace'}</Button>
          </form>
        </div>
      </Card>
    </div>
  )
}
