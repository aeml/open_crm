import { Link } from 'react-router-dom'
import { Field } from '../components/ui/field'

export const emptyCloseReview = { closeReasonCode: '', closeNotes: '' }

const reasonOptions = {
  won: [
    ['relationship', 'Existing relationship'],
    ['solution_fit', 'Best solution fit'],
    ['price_value', 'Price / value'],
    ['service_quality', 'Service quality'],
    ['timing', 'Timing'],
    ['other', 'Other']
  ],
  lost: [
    ['budget', 'Budget / price'],
    ['competitor', 'Competitor'],
    ['no_decision', 'No decision'],
    ['timing', 'Timing'],
    ['scope_fit', 'Scope / solution fit'],
    ['unresponsive', 'Unresponsive'],
    ['other', 'Other']
  ]
}

export function stageOutcome(stage) {
  if (!stage?.isClosed) return 'open'
  return stage.isWon ? 'won' : 'lost'
}

export function CloseReviewFields({ outcome, value, onChange }) {
  if (outcome !== 'won' && outcome !== 'lost') return null
  const outcomeLabel = outcome === 'won' ? 'Won' : 'Lost'
  return (
    <div className="inline-note card-stack" aria-label={`${outcomeLabel} close review`}>
      <div>
        <h3>{outcomeLabel} close review</h3>
        <p className="field-hint">Required at the stage transition so outcome reporting stays explainable.</p>
        {outcome === 'won' ? <p className="field-hint">Won deals need a company or primary contact for customer handoff.</p> : null}
      </div>
      <Field label={`${outcomeLabel} reason`}>
        <select className="text-input" value={value.closeReasonCode} onChange={(event) => onChange({ ...value, closeReasonCode: event.target.value })} required>
          <option value="">Choose a reason</option>
          {reasonOptions[outcome].map(([code, label]) => <option key={code} value={code}>{label}</option>)}
        </select>
      </Field>
      <Field label="Close notes">
        <textarea className="text-input" value={value.closeNotes} maxLength={2000} onChange={(event) => onChange({ ...value, closeNotes: event.target.value })} placeholder="Optional decision context for the team" />
      </Field>
    </div>
  )
}

function closeTimestamp(value) {
  const timestamp = new Date(value)
  return Number.isNaN(timestamp.getTime()) ? 'time not captured' : timestamp.toLocaleString()
}

export function DealCloseSummary({ deal }) {
  const outcome = deal?.status === 'won' || deal?.status === 'lost' ? deal.status : 'open'
  if (outcome === 'open') {
    return <p className="inline-note" aria-label="Deal outcome">Open outcome · derived from {deal?.stageName || 'the current stage'}.</p>
  }
  const label = deal.closeReasonLabel || 'Not captured before close-reason tracking'
  const accountPath = deal.companyId ? `/companies/${deal.companyId}` : deal.primaryContactId ? `/contacts/${deal.primaryContactId}` : ''
  return (
    <div className="inline-note card-stack" aria-label="Deal close review">
      <div>
        <h3>{outcome === 'won' ? 'Won' : 'Lost'} outcome</h3>
        <p><strong>{label}</strong></p>
        <p className="field-hint">Closed {closeTimestamp(deal.closedAt)}{deal.closedByUserName ? ` by ${deal.closedByUserName}` : ''}. Outcome is derived from the current stage.</p>
      </div>
      {deal.closeNotes ? <p>{deal.closeNotes}</p> : <p className="field-hint">No close notes were recorded.</p>}
      {outcome === 'won' && accountPath ? <Link className="button button-secondary" to={accountPath}>Open customer account</Link> : null}
    </div>
  )
}
