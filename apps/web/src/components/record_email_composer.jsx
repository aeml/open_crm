import { useEffect, useState } from 'react'
import { Card } from './ui/card'
import { Button } from './ui/button'
import { Field } from './ui/field'
import { InlineError } from './ui/inline_error'
import { isAbortError } from '../lib/api'
import { listEmailMessages } from '../lib/email_messages'
import { listEmailTemplates } from '../lib/email_templates'

const emptyForm = { subject: '', body: '' }

export function RecordEmailComposer({ entityType, entityId, canWrite, recipientOptions = [], sendEmail, emptyMessage, mergeFieldHint }) {
  const [open, setOpen] = useState(false)
  const [templates, setTemplates] = useState([])
  const [history, setHistory] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [selectedRecipientId, setSelectedRecipientId] = useState('')
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [isSending, setIsSending] = useState(false)

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
    const messages = await listEmailMessages({ entityType, entityId })
    setHistory(messages)
  }

  async function handleToggle() {
    const nextOpen = !open
    setOpen(nextOpen)
    setStatus('')
    setError('')
    if (!nextOpen) {
      return
    }
    try {
      if (templates.length === 0) {
        setTemplates(await listEmailTemplates())
      }
      await loadHistory()
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load email tools.')
      }
    }
  }

  function applyTemplate(templateId) {
    const template = templates.find((item) => String(item.id) === String(templateId))
    if (template) {
      setForm({ subject: template.subject, body: template.body })
    }
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
      const result = await sendEmail(entityId, { ...form, contactId })
      setStatus(`Email sent to ${result?.to || 'recipient'}.`)
      setForm(emptyForm)
      setError('')
      try {
        await loadHistory()
      } catch (historyError) {
        if (!isAbortError(historyError)) {
          // History refresh is best-effort after a successful send.
        }
      }
    } catch (sendError) {
      setError(sendError.message || 'Unable to send email.')
    } finally {
      setIsSending(false)
    }
  }

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
        {canWrite && open && recipientOptions.length > 0 ? (
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
            <Field label="Subject">
              <input className="text-input" value={form.subject} onChange={(event) => setForm({ ...form, subject: event.target.value })} required />
            </Field>
            <Field label="Body">
              <textarea className="text-input" rows={6} value={form.body} onChange={(event) => setForm({ ...form, body: event.target.value })} required />
            </Field>
            <p className="field-hint">{mergeFieldHint || `Merge fields like {{first_name}} are filled in when the email is sent.`}</p>
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
