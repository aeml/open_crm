import { useEffect, useRef, useState } from 'react'
import { Card } from './ui/card'
import { Button } from './ui/button'
import { Field } from './ui/field'
import { InlineError } from './ui/inline_error'
import { MergeFieldCatalog } from './merge_field_catalog'
import { isAbortError } from '../lib/api'
import { listEmailMessages, listRecordEmailDeliveries, resolveRecordEmailDelivery } from '../lib/email_messages'
import { listEmailTemplates, listEmailTemplateMergeFields, listEmailSnippets } from '../lib/email_templates'

const emptyForm = { subject: '', body: '', trackEngagement: false }

function newRecordEmailKey() {
  return `record-email-${globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random()}`}`
}

function deliveryStateLabel(delivery) {
  if (delivery.status === 'uncertain') return 'Outcome uncertain — check the sender’s Sent folder before resolving.'
  if (delivery.status === 'sending') return 'Sending is in progress. Refresh before taking another action.'
  return 'Prepared for delivery.'
}

function hasDefiniteRecordEmailFailure(error) {
  const code = error?.payload?.error?.code
  return Boolean(code) && code !== 'EMAIL_DELIVERY_UNCERTAIN'
}

export function RecordEmailComposer(props) {
  return <RecordEmailComposerState key={`${props.entityType || 'record'}:${props.entityId || 0}`} {...props} />
}

function RecordEmailComposerState({ entityType, entityId, canWrite, recipientOptions = [], sendEmail, emptyMessage, mergeFieldHint, onDeliveryChanged }) {
  const [open, setOpen] = useState(false)
  const [templates, setTemplates] = useState([])
  const [snippets, setSnippets] = useState([])
  const [mergeFieldGroups, setMergeFieldGroups] = useState([])
  const [history, setHistory] = useState([])
  const [deliveries, setDeliveries] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [selectedRecipientId, setSelectedRecipientId] = useState('')
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [isSending, setIsSending] = useState(false)
  const deliveryKey = useRef(newRecordEmailKey())

  useEffect(() => {
    if (recipientOptions.length === 0) {
      setSelectedRecipientId('')
      return
    }
    setSelectedRecipientId((current) => {
      if (recipientOptions.some((recipient) => String(recipient.id) === String(current))) {
        return current
      }
      return String(recipientOptions[0].id)
    })
  }, [recipientOptions])

  async function loadHistory() {
    if (!entityType || !entityId) {
      return
    }
    const [messages, pendingDeliveries] = await Promise.all([
      listEmailMessages({ entityType, entityId }),
      listRecordEmailDeliveries({ entityType, entityId })
    ])
    setHistory(messages)
    setDeliveries(pendingDeliveries)
  }

  async function handleToggle() {
    const nextOpen = !open
    setOpen(nextOpen)
    setStatus('')
    setError('')
    if (!nextOpen) {
      return
    }
    const loads = [loadHistory()]
    if (templates.length === 0) {
      loads.push(listEmailTemplates().then(setTemplates))
    }
    if (snippets.length === 0) {
      loads.push(listEmailSnippets().then(setSnippets))
    }
    if (mergeFieldGroups.length === 0) {
      loads.push(listEmailTemplateMergeFields().then(setMergeFieldGroups))
    }
    const results = await Promise.allSettled(loads)
    const failure = results.find((result) => result.status === 'rejected' && !isAbortError(result.reason))
    if (failure) {
      setError(failure.reason?.message || 'Unable to load email tools.')
    }
  }

  async function handleRefreshHistory() {
    setError('')
    try {
      await loadHistory()
    } catch (loadError) {
      if (!isAbortError(loadError)) setError(loadError.message || 'Unable to refresh email delivery status.')
    }
  }

  function applyTemplate(templateId) {
    const template = templates.find((item) => String(item.id) === String(templateId))
    if (template) {
      setForm((current) => ({ ...current, subject: template.subject, body: template.body }))
    }
  }

  function insertSnippet(snippetId) {
    const snippet = snippets.find((item) => String(item.id) === String(snippetId))
    if (!snippet) {
      return
    }
    setForm((current) => {
      const body = current.body.trimRight()
      return { ...current, body: body ? `${body}\n\n${snippet.body}` : snippet.body }
    })
  }

  async function handleSubmit(event) {
    event.preventDefault()
    if (!entityId || !sendEmail || recipientOptions.length === 0) {
      return
    }
    setIsSending(true)
    setStatus('')
    try {
      const contactId = Number.parseInt(selectedRecipientId, 10) || 0
      const input = entityType === 'contact' ? { ...form } : { ...form, contactId }
      const result = await sendEmail(entityId, input, deliveryKey.current)
      if (result?.status === 'accepted' || result?.sent) {
        setStatus(`Email sent to ${result?.to || 'recipient'}.`)
        setForm(emptyForm)
        deliveryKey.current = newRecordEmailKey()
        onDeliveryChanged?.()
      } else if (result?.status === 'uncertain') {
        setStatus('Delivery outcome is uncertain. Check Sent mail before resolving it below.')
      } else if (result?.status === 'failed') {
        setStatus('Email was not sent. You can compose another email.')
        deliveryKey.current = newRecordEmailKey()
      }
      setError('')
      try {
        await loadHistory()
      } catch (historyError) {
        if (!isAbortError(historyError)) {
          // History refresh is best-effort after a successful send.
        }
      }
    } catch (sendError) {
      if (hasDefiniteRecordEmailFailure(sendError)) {
        deliveryKey.current = newRecordEmailKey()
      }
      try {
        await loadHistory()
      } catch (historyError) {
        if (!isAbortError(historyError)) {
          // The original delivery error remains the actionable message.
        }
      }
      setError(sendError.message || 'Unable to send email.')
    } finally {
      setIsSending(false)
    }
  }

  async function resolveDelivery(delivery, resolution) {
    const warning = resolution === 'retry'
      ? 'Retry this email? The earlier provider outcome is unknown, so this can send a duplicate.'
      : resolution === 'confirmed_sent'
        ? 'Confirm this email was sent after checking the sender’s Sent folder? This records it without another provider call.'
        : 'Mark this email not sent? No provider call will be made.'
    if (!window.confirm(warning)) return
    setIsSending(true)
    setError('')
    try {
      const result = await resolveRecordEmailDelivery(delivery.id, resolution)
      if (result?.status === 'accepted') setStatus(`Email to ${result.to || delivery.to} confirmed sent.`)
      if (result?.status === 'failed') setStatus('Email marked not sent. You can compose another email.')
      if (result?.status === 'accepted' || result?.status === 'failed') {
        deliveryKey.current = newRecordEmailKey()
        onDeliveryChanged?.()
      }
      await loadHistory()
    } catch (resolveError) {
      if (!isAbortError(resolveError)) setError(resolveError.message || 'Unable to resolve email delivery.')
    } finally {
      setIsSending(false)
    }
  }

  const currentUserHasUnresolvedDelivery = deliveries.some((delivery) => delivery.ownedByCurrentUser && ['prepared', 'sending', 'uncertain'].includes(delivery.status))

  return (
    <Card>
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h3>Email</h3>
            <p>Send customer email from your connected mailbox and keep it on this record.</p>
          </div>
          {canWrite ? (
            <Button className="button-secondary" type="button" onClick={handleToggle}>
              {open ? 'Close' : 'Send email'}
            </Button>
          ) : null}
        </div>
        {status ? <p className="field-hint" role="status">{status}</p> : null}
        {error ? <InlineError message={error} /> : null}
        {canWrite && open && recipientOptions.length === 0 ? (
          <p className="field-hint">{emptyMessage || 'Add a contact with an email address before sending email.'}</p>
        ) : null}
        {open && deliveries.length > 0 ? (
          <div className="card-stack">
            <div className="record-list" role="list" aria-label="Unresolved record email deliveries">
              {deliveries.map((delivery) => (
                <article className="record-row record-row-alert" key={`delivery-${delivery.id}`} role="listitem">
                  <div className="card-stack">
                    <div>
                      <p>{delivery.subject}</p>
                      <p className="field-hint">To {delivery.to} · {deliveryStateLabel(delivery)}</p>
                    </div>
                    {delivery.lastError ? <InlineError message={delivery.lastError} /> : null}
                    {delivery.status === 'uncertain' && delivery.canResolve ? (
                      <div className="button-row">
                        {delivery.canRetry ? <Button className="button-secondary" type="button" onClick={() => resolveDelivery(delivery, 'retry')} disabled={isSending}>Retry explicitly</Button> : null}
                        <Button className="button-secondary" type="button" onClick={() => resolveDelivery(delivery, 'confirmed_sent')} disabled={isSending}>Confirm sent</Button>
                        <Button className="button-secondary" type="button" onClick={() => resolveDelivery(delivery, 'not_sent')} disabled={isSending}>Mark not sent</Button>
                      </div>
                    ) : null}
                  </div>
                </article>
              ))}
            </div>
            <Button className="button-secondary" type="button" onClick={handleRefreshHistory} disabled={isSending}>Refresh delivery status</Button>
          </div>
        ) : null}
        {currentUserHasUnresolvedDelivery ? <p className="inline-note">Resolve your in-progress or uncertain email before composing another email for this record.</p> : null}
        {canWrite && open && recipientOptions.length > 0 && !currentUserHasUnresolvedDelivery ? (
          <form className="auth-form" onSubmit={handleSubmit}>
            <Field label="Recipient">
              <select className="text-input" value={selectedRecipientId} onChange={(event) => setSelectedRecipientId(event.target.value)}>
                {recipientOptions.map((recipient) => (
                  <option key={recipient.id} value={recipient.id}>{recipient.label}</option>
                ))}
              </select>
            </Field>
            {templates.length > 0 ? (
              <Field label="Template">
                <select className="text-input" defaultValue="" onChange={(event) => applyTemplate(event.target.value)}>
                  <option value="">Start from scratch</option>
                  {templates.map((template) => (
                    <option key={template.id} value={template.id}>{template.name}</option>
                  ))}
                </select>
              </Field>
            ) : null}
            {snippets.length > 0 ? (
              <Field label="Snippet">
                <select className="text-input" defaultValue="" onChange={(event) => {
                  insertSnippet(event.target.value)
                  event.target.value = ''
                }}>
                  <option value="">Insert reusable snippet</option>
                  {snippets.map((snippet) => (
                    <option key={snippet.id} value={snippet.id}>{snippet.name}</option>
                  ))}
                </select>
              </Field>
            ) : null}
            <Field label="Subject">
              <input className="text-input" value={form.subject} onChange={(event) => setForm({ ...form, subject: event.target.value })} maxLength={998} required />
            </Field>
            <Field label="Body">
              <textarea className="text-input" rows={6} value={form.body} onChange={(event) => setForm({ ...form, body: event.target.value })} maxLength={100000} required />
            </Field>
            <p className="field-hint">{mergeFieldHint || `Merge fields like {{first_name}} are filled in when the email is sent.`}</p>
            <MergeFieldCatalog groups={mergeFieldGroups} compact />
            <label className="field-hint">
              <input type="checkbox" checked={form.trackEngagement} onChange={(event) => setForm({ ...form, trackEngagement: event.target.checked })} /> Track opens/links (90 days, approximate). I confirm authorization.
            </label>
            <Button type="submit" disabled={isSending}>{isSending ? 'Sending...' : 'Send email'}</Button>
          </form>
        ) : null}
        {open && history.length > 0 ? (
          <div className="record-list" role="list" aria-label={`${entityType || 'Record'} email history`}>
            {history.map((message) => (
              <article className="record-row" key={message.id} role="listitem">
                <div>
                  <p>{message.subject}</p>
                  <p className="field-hint">{message.status === 'failed' ? 'Failed' : 'Sent'} · {message.sentByName || 'You'}</p>
                </div>
              </article>
            ))}
          </div>
        ) : null}
      </div>
    </Card>
  )
}
