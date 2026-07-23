import { Card } from '../components/ui/card'

export function hasAttribution(contact = {}) {
  return !!(contact.leadSource || contact.firstSourceUrl || contact.utmSource || contact.utmMedium || contact.utmCampaign || contact.utmTerm || contact.utmContent)
}

export function attributionSummary(contact = {}) {
  const parts = []
  if (contact.leadSource) parts.push(contact.leadSource)
  if (contact.utmCampaign) parts.push(contact.utmCampaign)
  if (contact.utmSource) parts.push(contact.utmMedium ? `${contact.utmSource} / ${contact.utmMedium}` : contact.utmSource)
  return parts.join(' | ')
}

export function hasLeadScore(contact = {}) {
  return !!(contact.leadScoredAt || contact.leadScore || contact.leadGrade)
}

export function leadScoreLabel(contact = {}) {
  const score = contact.leadScore || 0
  return contact.leadGrade ? `${score} (${contact.leadGrade})` : String(score)
}

function formatLeadScoreTime(value) {
  if (!value) {
    return 'Not scored yet'
  }
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'Not scored yet' : `Last scored ${date.toLocaleString()}`
}

function safeHTTPURL(value = '') {
  try {
    const parsed = new URL(value)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? value : ''
  } catch {
    return ''
  }
}

export function ContactLeadScoreEvidenceCard({ contact }) {
  if (!hasLeadScore(contact)) {
    return null
  }

  return (
    <Card>
      <div className="card-stack">
        <div>
          <h3>Lead score evidence</h3>
          <p className="field-hint">Retained from the last scoring evaluation.</p>
        </div>
        <div className="record-list" role="list" aria-label="Lead score summary">
          <article className="record-row" role="listitem">
            <div>
              <p>{`Score ${leadScoreLabel(contact)}`}</p>
              <p className="field-hint">{formatLeadScoreTime(contact.leadScoredAt)}</p>
            </div>
            <div><span className="chip">Owner: {contact.ownerUserName || 'Unassigned'}</span></div>
          </article>
        </div>
      </div>
    </Card>
  )
}

export function ContactAttributionCard({ contact }) {
  if (!hasAttribution(contact)) {
    return null
  }
  return (
    <Card>
      <div className="card-stack">
        <div>
          <h3>Attribution</h3>
          <p className="field-hint">First captured lead source and campaign details.</p>
        </div>
        <div className="record-list" role="list" aria-label="Lead attribution">
          {contact.leadSource ? <AttributionRow label="Lead source" value={contact.leadSource} /> : null}
          {contact.utmCampaign ? <AttributionRow label="Campaign" value={contact.utmCampaign} /> : null}
          {(contact.utmSource || contact.utmMedium) ? <AttributionRow label="Source / medium" value={[contact.utmSource, contact.utmMedium].filter(Boolean).join(' / ')} /> : null}
          {(contact.utmTerm || contact.utmContent) ? <AttributionRow label="Term / content" value={[contact.utmTerm, contact.utmContent].filter(Boolean).join(' / ')} /> : null}
          {contact.firstSourceUrl ? (
            <AttributionRow
              label="First source URL"
              value={safeHTTPURL(contact.firstSourceUrl) ? <a href={contact.firstSourceUrl} target="_blank" rel="noreferrer">{contact.firstSourceUrl}</a> : contact.firstSourceUrl}
            />
          ) : null}
        </div>
      </div>
    </Card>
  )
}

function AttributionRow({ label, value }) {
  return (
    <article className="record-row" role="listitem">
      <div>
        <p>{label}</p>
        <p className="field-hint">{value}</p>
      </div>
    </article>
  )
}
