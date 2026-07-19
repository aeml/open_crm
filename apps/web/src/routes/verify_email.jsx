import { useEffect, useRef, useState } from 'react'
import { Link, Navigate, useSearchParams } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { useAuth } from '../app/providers'
import { usePageTitle } from '../lib/use_page_title'

export function VerifyEmailRoute() {
  const { status, verifyEmail } = useAuth()
  const [searchParams] = useSearchParams()
  const [error, setError] = useState('')
  const attempted = useRef(false)
  const token = searchParams.get('token') || ''
  usePageTitle('Verify email')

  useEffect(() => {
    if (attempted.current || !token || status === 'checking') return
    attempted.current = true
    verifyEmail(token).catch((verificationError) => {
      setError(verificationError.message || 'Unable to verify workspace email.')
    })
  }, [status, token, verifyEmail])

  if (status === 'authenticated') {
    return <Navigate to="/dashboard" replace />
  }

  return (
    <div className="auth-layout">
      <Card className="auth-card">
        <div className="card-stack auth-stack">
          <div>
            <p className="eyebrow">Workspace verification</p>
            <h1>{error || !token ? 'Verification link unavailable' : 'Verifying your email…'}</h1>
            <p className="page-description">
              {error || !token
                ? (error || 'This verification link is missing its one-time token.')
                : 'We are validating the one-time link and starting your 14-day trial.'}
            </p>
          </div>
          {error || !token ? <p className="form-error" role="alert">Request a new link from the workspace signup confirmation, then use only the newest email.</p> : <p className="field-hint" role="status">Please wait…</p>}
          <p className="page-description"><Link to="/bootstrap">Return to workspace signup</Link>.</p>
        </div>
      </Card>
    </div>
  )
}
