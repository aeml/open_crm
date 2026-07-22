import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { Button } from '../components/ui/button'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { listEmailSequenceEnrollmentPage } from '../lib/email_sequence_enrollments'

const emptyMeta = { limit: 50, hasMore: false, nextCursor: '' }

function formatDateTime(value) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

function outcomeLabel(enrollment) {
  if (enrollment.needsReview > 0) return 'Needs review'
  if (enrollment.complaints > 0) return 'Complaint'
  if (enrollment.bouncedMessages > 0) return 'Bounced'
  if (enrollment.status === 'completed' && enrollment.completionReason === 'replied') return 'Replied'
  if (enrollment.status === 'completed' && enrollment.completionReason === 'finished') return 'Finished'
  if (enrollment.status === 'completed' && enrollment.completionReason === 'suppressed') return 'Suppressed'
  if (enrollment.status === 'completed') return 'Completed (unclassified)'
  if (enrollment.status === 'cancelled') return 'Cancelled'
  if (enrollment.status === 'paused') return `Paused at step ${enrollment.currentStepOrder}`
  return `Active at step ${enrollment.currentStepOrder}`
}

function deliverySummary(enrollment) {
  const parts = []
  if (enrollment.providerAccepted) parts.push(`${enrollment.providerAccepted} accepted`)
  if (enrollment.bouncedMessages) parts.push(`${enrollment.bouncedMessages} bounced`)
  if (enrollment.complaints) parts.push(`${enrollment.complaints} complaint`)
  if (enrollment.suppressedMessages) parts.push(`${enrollment.suppressedMessages} suppressed send`)
  if (enrollment.queuedMessages) parts.push(`${enrollment.queuedMessages} queued`)
  if (enrollment.needsReview) parts.push(`${enrollment.needsReview} review`)
  return parts.join(' · ') || 'No provider attempts'
}

function mergeByID(current, next) {
  const ids = new Set(current.map((enrollment) => enrollment.id))
  return [...current, ...next.filter((enrollment) => !ids.has(enrollment.id))]
}

export function EmailSequenceEnrollmentHistory({ sequence }) {
  const [isExpanded, setIsExpanded] = useState(false)
  const [enrollments, setEnrollments] = useState([])
  const [meta, setMeta] = useState(emptyMeta)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [isLoadingOlder, setIsLoadingOlder] = useState(false)
  const requestRef = useRef({ token: 0, controller: null })

  useEffect(() => {
    if (!isExpanded) return undefined
    const token = requestRef.current.token + 1
    const controller = new AbortController()
    requestRef.current.controller?.abort()
    requestRef.current = { token, controller }
    setEnrollments([])
    setMeta(emptyMeta)
    setIsLoading(true)
    setError('')
    listEmailSequenceEnrollmentPage({ sequenceId: sequence.id }, { signal: controller.signal })
      .then((page) => {
        if (requestRef.current.token !== token || controller.signal.aborted) return
        setEnrollments(page.enrollments)
        setMeta(page.meta)
      })
      .catch((loadError) => {
        if (requestRef.current.token === token && !isAbortError(loadError)) {
          setError(loadError.message || 'Unable to load sequence enrollment history.')
        }
      })
      .finally(() => {
        if (requestRef.current.token === token && !controller.signal.aborted) setIsLoading(false)
      })
    return () => {
      controller.abort()
      if (requestRef.current.token === token) requestRef.current.token += 1
    }
  }, [isExpanded, sequence.id])

  function toggleHistory() {
    if (isExpanded) {
      requestRef.current.controller?.abort()
      requestRef.current.token += 1
      setIsLoading(false)
      setIsLoadingOlder(false)
      setError('')
    }
    setIsExpanded((current) => !current)
  }

  async function loadOlder() {
    const cursor = meta.nextCursor
    if (!isExpanded || !meta.hasMore || !cursor || isLoading || isLoadingOlder) return
    const token = requestRef.current.token + 1
    const controller = new AbortController()
    requestRef.current.controller?.abort()
    requestRef.current = { token, controller }
    setIsLoadingOlder(true)
    setError('')
    try {
      const page = await listEmailSequenceEnrollmentPage({ sequenceId: sequence.id, cursor, limit: meta.limit || 50 }, { signal: controller.signal })
      if (requestRef.current.token !== token || controller.signal.aborted) return
      setEnrollments((current) => mergeByID(current, page.enrollments))
      setMeta((current) => current.nextCursor === cursor ? page.meta : current)
    } catch (loadError) {
      if (requestRef.current.token === token && !isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load sequence enrollment history.')
      }
    } finally {
      if (requestRef.current.token === token && !controller.signal.aborted) setIsLoadingOlder(false)
    }
  }

  return (
    <div className="sequence-enrollment-history">
      <Button
        className="button-secondary"
        type="button"
        aria-expanded={isExpanded}
        aria-controls={`sequence-${sequence.id}-enrollments`}
        onClick={toggleHistory}
      >
        {isExpanded ? 'Hide enrollments' : 'View enrollments'}
      </Button>
      {isExpanded ? (
        <div id={`sequence-${sequence.id}-enrollments`} className="card-stack" aria-label={`${sequence.name} enrollment history`}>
          {isLoading ? <p className="field-hint" role="status">Loading enrollment history...</p> : null}
          {error ? <InlineError message={error} /> : null}
          <div className="record-list" role="list" aria-label={`${sequence.name} enrollments`} aria-busy={isLoading || isLoadingOlder}>
            {!isLoading && enrollments.length === 0 && !error ? (
              <article className="record-row" role="listitem"><p>No enrollment history.</p></article>
            ) : enrollments.map((enrollment) => (
              <article className="record-row sequence-enrollment-row" role="listitem" key={enrollment.id}>
                <div>
                  <h4><Link to={`/contacts/${enrollment.contactId}`}>{enrollment.contactName || enrollment.contactEmail || `Contact #${enrollment.contactId}`}</Link></h4>
                  {enrollment.contactEmail ? <p className="field-hint">{enrollment.contactEmail}</p> : null}
                  <p className="field-hint">{outcomeLabel(enrollment)} · {deliverySummary(enrollment)}</p>
                  <p className="field-hint">Enrolled {formatDateTime(enrollment.createdAt)}{enrollment.enrolledByName ? ` by ${enrollment.enrolledByName}` : ''}</p>
                  {enrollment.completedAt ? <p className="field-hint">Completed {formatDateTime(enrollment.completedAt)}</p> : null}
                  {enrollment.cancelledAt ? <p className="field-hint">Cancelled {formatDateTime(enrollment.cancelledAt)}</p> : null}
                </div>
                {enrollment.needsReview > 0 ? <Link className="button button-secondary" to="/settings/operations">Review delivery</Link> : null}
              </article>
            ))}
          </div>
          {meta.hasMore && meta.nextCursor ? (
            <Button className="button-secondary" type="button" disabled={isLoading || isLoadingOlder} onClick={loadOlder}>
              {isLoadingOlder ? 'Loading older enrollments...' : 'Load older enrollments'}
            </Button>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
