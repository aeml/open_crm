import { useCallback, useEffect, useState } from 'react'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { InlineError } from '../components/ui/inline_error'
import { getSessions, revokeOtherSessions, revokeSession } from '../lib/profile'
import { isAbortError } from '../lib/api'

function formatSessionTime(value) {
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return 'Unavailable'
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short'
  }).format(parsed)
}

export function SessionManagement() {
  const [sessions, setSessions] = useState([])
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState('')
  const [status, setStatus] = useState('')
  const [confirmSessionId, setConfirmSessionId] = useState(null)
  const [confirmOthers, setConfirmOthers] = useState(false)
  const [pendingAction, setPendingAction] = useState('')

  const loadSessions = useCallback(async (signal) => {
    setIsLoading(true)
    setError('')
    try {
      setSessions(await getSessions({ signal }))
    } catch (err) {
      if (!isAbortError(err)) setError(err.message || 'Unable to load active sign-ins.')
    } finally {
      if (!signal?.aborted) setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    loadSessions(controller.signal)
    return () => controller.abort()
  }, [loadSessions])

  async function handleRevokeSession(sessionId) {
    setPendingAction(`session-${sessionId}`)
    setError('')
    setStatus('')
    try {
      await revokeSession(sessionId)
      setSessions((current) => current.filter((session) => session.id !== sessionId))
      setConfirmSessionId(null)
      setStatus('That sign-in has been ended.')
    } catch (err) {
      setError(err.message || 'Unable to sign out that session.')
    } finally {
      setPendingAction('')
    }
  }

  async function handleRevokeOthers() {
    setPendingAction('others')
    setError('')
    setStatus('')
    try {
      const revoked = await revokeOtherSessions()
      setSessions((current) => current.filter((session) => session.current))
      setConfirmOthers(false)
      setStatus(revoked === 1 ? 'One other sign-in has been ended.' : `${revoked} other sign-ins have been ended.`)
    } catch (err) {
      setError(err.message || 'Unable to sign out other sessions.')
    } finally {
      setPendingAction('')
    }
  }

  const otherSessionCount = sessions.filter((session) => !session.current).length

  return (
    <Card>
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h2>Active sign-ins</h2>
            <p>Review and end other server-side sessions for your account.</p>
          </div>
        </div>
        <p className="field-hint">
          Open CRM does not retain IP addresses or browser fingerprints. Use workspace and timing to identify older sign-ins.
        </p>
        {isLoading ? <p className="field-hint" role="status">Loading active sign-ins…</p> : null}
        {error ? (
          <div className="card-stack">
            <InlineError message={error} />
            {sessions.length === 0 ? <Button className="button-secondary" type="button" onClick={() => loadSessions()}>Retry</Button> : null}
          </div>
        ) : null}
        {status ? <p className="field-hint" role="status">{status}</p> : null}
        {!isLoading && sessions.length === 0 && !error ? <p className="empty-state">No active sign-ins were returned.</p> : null}
        {sessions.length > 0 ? (
          <ul className="session-list" aria-label="Active sign-ins">
            {sessions.map((session) => (
              <li className="session-row" key={session.id}>
                <div className="session-row-copy">
                  <div className="session-row-title">
                    <strong>{session.organization?.name || 'Workspace'}</strong>
                    {session.current ? <span className="badge">This sign-in</span> : null}
                  </div>
                  <dl className="session-times">
                    <div><dt>Last active</dt><dd><time dateTime={session.lastSeenAt}>{formatSessionTime(session.lastSeenAt)}</time></dd></div>
                    <div><dt>Started</dt><dd><time dateTime={session.createdAt}>{formatSessionTime(session.createdAt)}</time></dd></div>
                    <div><dt>Expires</dt><dd><time dateTime={session.expiresAt}>{formatSessionTime(session.expiresAt)}</time></dd></div>
                  </dl>
                </div>
                {!session.current ? (
                  confirmSessionId === session.id ? (
                    <div className="button-row" aria-label={`Confirm sign out from ${session.organization?.name || 'workspace'}`}>
                      <Button className="button-danger" type="button" disabled={pendingAction !== ''} onClick={() => handleRevokeSession(session.id)}>
                        {pendingAction === `session-${session.id}` ? 'Signing out…' : 'Confirm sign out'}
                      </Button>
                      <Button className="button-secondary" type="button" disabled={pendingAction !== ''} onClick={() => setConfirmSessionId(null)}>Cancel</Button>
                    </div>
                  ) : (
                    <Button className="button-secondary" type="button" disabled={pendingAction !== ''} onClick={() => { setConfirmSessionId(session.id); setConfirmOthers(false) }}>Sign out</Button>
                  )
                ) : null}
              </li>
            ))}
          </ul>
        ) : null}
        {otherSessionCount > 0 ? (
          confirmOthers ? (
            <div className="button-row" aria-label="Confirm sign out all other sessions">
              <Button className="button-danger" type="button" disabled={pendingAction !== ''} onClick={handleRevokeOthers}>
                {pendingAction === 'others' ? 'Signing out…' : 'Confirm sign out all others'}
              </Button>
              <Button className="button-secondary" type="button" disabled={pendingAction !== ''} onClick={() => setConfirmOthers(false)}>Cancel</Button>
            </div>
          ) : (
            <Button className="button-secondary" type="button" disabled={pendingAction !== ''} onClick={() => { setConfirmOthers(true); setConfirmSessionId(null) }}>
              Sign out all other sessions
            </Button>
          )
        ) : null}
        <p className="field-hint">To end this sign-in, use Log out.</p>
      </div>
    </Card>
  )
}
