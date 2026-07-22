import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { createIdempotencyKey } from '../lib/idempotency'
import { listLeadSubmissionReviews, reviewLeadSubmission } from '../lib/lead_forms'

const emptyPage = { submissions: [], counts: { unreviewed: 0, legitimate: 0, spam: 0 }, limit: 50, meta: { limit: 50, hasMore: false, nextCursor: '' } }

function reviewLabel(status) {
	if (status === 'spam') return 'Spam'
	if (status === 'legitimate') return 'Legitimate'
	return 'Needs review'
}

function submissionDetails(submission) {
	return Object.entries(submission.values || {})
		.filter(([, value]) => String(value || '').trim())
		.slice(0, 8)
}

function effectMessage(submission, desiredStatus) {
	const effects = submission?.effects || {}
	if (desiredStatus === 'spam') {
		const completed = Number(effects.completedRuns || 0)
		return `Lead quarantined. ${Number(effects.cancelledRuns || 0)} queued follow-up${Number(effects.cancelledRuns || 0) === 1 ? '' : 's'} cancelled.${completed ? ` ${completed} completed follow-up${completed === 1 ? ' remains' : 's remain'} as history.` : ''}`
	}
	return `Lead restored as legitimate. ${Number(effects.recoveredRuns || 0)} follow-up${Number(effects.recoveredRuns || 0) === 1 ? '' : 's'} rescheduled.`
}

export function LeadSubmissionReview({ forms = [] }) {
	const [page, setPage] = useState(emptyPage)
	const [statusFilter, setStatusFilter] = useState('unreviewed')
	const [formFilter, setFormFilter] = useState('')
	const [notes, setNotes] = useState({})
	const [isLoading, setIsLoading] = useState(true)
	const [isLoadingOlder, setIsLoadingOlder] = useState(false)
	const [pendingId, setPendingId] = useState(null)
	const [error, setError] = useState('')
	const [status, setStatus] = useState('')
	const operationPending = useRef(false)

	async function requestReviews({ signal, cursor = '', append = false } = {}) {
		append ? setIsLoadingOlder(true) : setIsLoading(true)
		try {
			const next = await listLeadSubmissionReviews({ status: statusFilter, formId: formFilter, cursor, limit: append ? (page.meta?.limit || page.limit || 50) : undefined, signal })
			if (signal?.aborted) return
			next.meta ||= { limit: next.limit || 50, hasMore: false, nextCursor: '' }
			if (append) {
				setPage((current) => {
					const ids = new Set(current.submissions.map((submission) => submission.id))
					return { ...next, submissions: [...current.submissions, ...next.submissions.filter((submission) => !ids.has(submission.id))] }
				})
			} else {
				setPage(next)
			}
			setError('')
		} catch (loadError) {
			if (!isAbortError(loadError)) setError(loadError.message || 'Unable to load lead submissions.')
		} finally {
			if (!signal?.aborted) append ? setIsLoadingOlder(false) : setIsLoading(false)
		}
	}

	async function loadReviews(options = {}) {
		const userRequested = !options.signal
		if (userRequested && operationPending.current) return
		if (userRequested) operationPending.current = true
		try {
			await requestReviews(options)
		} finally {
			if (userRequested) operationPending.current = false
		}
	}

	useEffect(() => {
		const controller = new AbortController()
		loadReviews({ signal: controller.signal })
		return () => controller.abort()
	}, [statusFilter, formFilter])

	async function review(submission, desiredStatus) {
		if (pendingId !== null || operationPending.current) return
		if (desiredStatus === 'spam' && !window.confirm(`Quarantine ${submission.contactName || 'this captured lead'} as spam? The captured contact will be archived and queued follow-up work will be cancelled.`)) return
		operationPending.current = true
		setPendingId(submission.id)
		setStatus('')
		try {
			const updated = await reviewLeadSubmission(submission.id, {
				status: desiredStatus,
				note: notes[submission.id] || ''
			}, createIdempotencyKey('lead-review'))
			if (updated?.id !== submission.id || updated.reviewStatus !== desiredStatus) throw new Error('Unable to verify the reviewed submission. Refresh and try again.')
			setStatus(effectMessage(updated, desiredStatus))
			setNotes((current) => ({ ...current, [submission.id]: '' }))
			await requestReviews()
		} catch (reviewError) {
			setError(reviewError.message || 'Unable to review lead submission.')
		} finally {
			operationPending.current = false
			setPendingId(null)
		}
	}

	const counts = page.counts || emptyPage.counts
	const matchingTotal = statusFilter ? Number(counts[statusFilter] || 0) : Number(counts.unreviewed || 0) + Number(counts.legitimate || 0) + Number(counts.spam || 0)
	const pageMeta = page.meta || emptyPage.meta
	return (
		<div className="card-stack">
			<div className="section-header">
				<div>
					<h2>Lead submission review</h2>
					<p>Review captured inquiries, quarantine spam before delayed follow-up runs, and recover mistakes without deleting history.</p>
				</div>
			</div>
			<div className="filters-grid">
				<Field label="Review status">
					<select className="text-input" value={statusFilter} disabled={pendingId !== null || isLoadingOlder} onChange={(event) => setStatusFilter(event.target.value)}>
						<option value="unreviewed">Needs review ({counts.unreviewed || 0})</option>
						<option value="spam">Spam ({counts.spam || 0})</option>
						<option value="legitimate">Legitimate ({counts.legitimate || 0})</option>
						<option value="">All submissions</option>
					</select>
				</Field>
				<Field label="Lead form">
					<select className="text-input" value={formFilter} disabled={pendingId !== null || isLoadingOlder} onChange={(event) => setFormFilter(event.target.value)}>
						<option value="">All lead forms</option>
						{forms.map((form) => <option key={form.id} value={form.id}>{form.name}</option>)}
					</select>
				</Field>
			</div>
			{status ? <p className="field-hint" role="status">{status}</p> : null}
			{error ? <InlineError message={error} onRetry={() => loadReviews()} retryLabel="Retry lead submissions" /> : null}
			{isLoading ? <p className="field-hint">Loading lead submissions...</p> : null}
			<div className="record-list" role="list" aria-label="Lead submissions awaiting review" aria-busy={isLoading || isLoadingOlder}>
				{!isLoading && page.submissions.length === 0 ? (
					<article className="record-row" role="listitem"><div><p>No submissions match this review filter.</p></div></article>
				) : page.submissions.map((submission) => (
					<article className={submission.reviewStatus === 'spam' ? 'record-row record-row-alert' : 'record-row'} key={submission.id} role="listitem">
						<div className="card-stack">
							<div>
								<h3>{submission.contactName || `Submission #${submission.id}`}</h3>
								<p className="field-hint">{submission.formName} · {new Date(submission.createdAt).toLocaleString()} · {reviewLabel(submission.reviewStatus)}</p>
								{submission.contactEmail ? <p className="field-hint">{submission.contactEmail}</p> : null}
								{submission.leadSource || submission.utmSource || submission.utmCampaign ? <p className="field-hint">Source: {[submission.leadSource, submission.utmSource, submission.utmCampaign].filter(Boolean).join(' · ')}</p> : null}
							</div>
							{submissionDetails(submission).length ? <dl className="details-grid">{submissionDetails(submission).map(([key, value]) => <div key={key}><dt>{key}</dt><dd>{value}</dd></div>)}</dl> : null}
							<p className="field-hint">Follow-up: {submission.queuedFollowUpRuns || 0} queued · {submission.completedFollowUpRuns || 0} completed · {submission.cancelledFollowUpRuns || 0} cancelled</p>
							{submission.reviewNote ? <p className="field-hint">Review note: {submission.reviewNote}</p> : null}
							<Field label={`Review note for ${submission.contactName || `submission ${submission.id}`}`} hint="Optional internal context; maximum 500 characters.">
								<input className="text-input" maxLength={500} value={notes[submission.id] || ''} onChange={(event) => setNotes((current) => ({ ...current, [submission.id]: event.target.value }))} />
							</Field>
						</div>
						<div>
							<span className="chip">{reviewLabel(submission.reviewStatus)}</span>
							{submission.contactActive ? <Link className="button button-secondary" to={`/contacts/${submission.contactId}`}>Open contact</Link> : null}
							{submission.reviewStatus !== 'legitimate' ? <Button className="button-secondary" type="button" disabled={pendingId !== null} onClick={() => review(submission, 'legitimate')}>{submission.reviewStatus === 'spam' ? 'Recover as legitimate' : 'Mark legitimate'}</Button> : null}
							{submission.reviewStatus !== 'spam' ? <Button className="button-danger" type="button" disabled={pendingId !== null} onClick={() => review(submission, 'spam')}>Mark spam</Button> : null}
						</div>
					</article>
				))}
			</div>
			{pageMeta.hasMore && pageMeta.nextCursor ? <Button className="button-secondary" type="button" disabled={isLoading || isLoadingOlder || pendingId !== null} onClick={() => loadReviews({ cursor: pageMeta.nextCursor, append: true })}>{isLoadingOlder ? 'Loading...' : 'Load older submissions'}</Button> : null}
			<p className="field-hint">Showing {page.submissions.length} of {matchingTotal} matching submissions. Completed follow-up tasks are retained as auditable history.</p>
		</div>
	)
}
