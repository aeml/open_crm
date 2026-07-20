import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'

export const smsTemplates = [
  { name: 'Follow-up', body: 'Hi {{first_name}}, thanks for your time today. Reply STOP to opt out.' },
  { name: 'Appointment reminder', body: 'Hi {{first_name}}, quick reminder about our upcoming appointment. Reply STOP to opt out.' },
  { name: 'Callback request', body: 'Hi {{first_name}}, I tried reaching you. What is a good time for a quick call? Reply STOP to opt out.' }
]

function formatTime(value, fallback = 'Not scheduled') {
  if (!value) {
    return fallback
  }
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? fallback : date.toLocaleString()
}

export function ContactSMSCard({
  canWrite,
  contact,
  inboundForm,
  inboundOpen,
  isLoggingInbound,
  isOptingOut,
  isSending,
  messages,
  onApplyTemplate,
  onLogInbound,
  onOptOut,
  onSend,
  onSetForm,
  onSetInboundForm,
  onToggle,
  onToggleInbound,
  open,
  form,
  status
}) {
  const hasPhone = Boolean(contact?.phone)
  return (
    <Card>
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h3>SMS</h3>
            <p className="field-hint">Send compliant one-to-one texts and log inbound replies.</p>
          </div>
          <div className="button-row">
            <Button className="button-secondary" type="button" onClick={onToggle}>
              {open ? 'Hide SMS' : 'Show SMS'}
            </Button>
            {canWrite && hasPhone ? (
              <Button className="button-secondary" type="button" onClick={onToggleInbound}>
                {inboundOpen ? 'Cancel inbound SMS' : 'Log inbound SMS'}
              </Button>
            ) : null}
            {canWrite && hasPhone ? (
              <Button className="button-secondary" type="button" onClick={onOptOut} disabled={isOptingOut}>
                {isOptingOut ? 'Opting out...' : 'Mark SMS opt-out'}
              </Button>
            ) : null}
          </div>
        </div>
        {!hasPhone ? <p className="field-hint">Add a phone number to this contact before sending SMS.</p> : null}
        {status ? <p className="field-hint" role="status">{status}</p> : null}
        {canWrite && hasPhone ? (
          <form className="auth-form" onSubmit={onSend}>
            <Field label="SMS template">
              <select className="text-input" value={form.templateName} onChange={(event) => onApplyTemplate(event.target.value)}>
                <option value="">Start from scratch</option>
                {smsTemplates.map((template) => (
                  <option key={template.name} value={template.name}>{template.name}</option>
                ))}
              </select>
            </Field>
            <Field label="SMS body">
              <textarea className="text-input" rows={4} value={form.body} onChange={(event) => onSetForm((current) => ({ ...current, body: event.target.value }))} placeholder="Type a short text message" required />
            </Field>
            <p className="field-hint">Merge fields like {'{{first_name}}'} are filled in when the SMS is sent. Include opt-out language for outreach.</p>
            <Button type="submit" disabled={isSending}>{isSending ? 'Sending...' : 'Send text'}</Button>
          </form>
        ) : null}
        {inboundOpen ? (
          <form className="auth-form" onSubmit={onLogInbound}>
            <Field label="Inbound SMS body">
              <textarea className="text-input" rows={3} value={inboundForm.body} onChange={(event) => onSetInboundForm({ body: event.target.value })} placeholder="Paste the inbound text. STOP records an opt-out." required />
            </Field>
            <Button type="submit" disabled={isLoggingInbound}>{isLoggingInbound ? 'Logging...' : 'Save inbound SMS'}</Button>
          </form>
        ) : null}
        {open ? (
          <div className="record-list" role="list" aria-label="SMS history">
            {messages.length === 0 ? (
              <article className="record-row" role="listitem"><div><p>No SMS messages logged yet.</p></div></article>
            ) : messages.map((message) => (
              <article className="record-row" key={message.id} role="listitem">
                <div>
                  <p>{message.direction === 'inbound' ? 'Inbound SMS' : (message.status === 'suppressed' ? 'SMS suppressed' : 'Outbound SMS')}</p>
                  <p className="field-hint">{message.phoneNumber} · {message.status}{message.templateName ? ` · ${message.templateName}` : ''}</p>
                  <p className="field-hint">{message.body}</p>
                  {message.error ? <p className="field-hint">{message.error}</p> : null}
                </div>
                <div>
                  <p>{formatTime(message.sentAt || message.receivedAt || message.createdAt, 'Not sent')}</p>
                  <p className="field-hint">{message.createdByUserName || 'You'}</p>
                </div>
              </article>
            ))}
          </div>
        ) : null}
      </div>
    </Card>
  )
}

export function ContactMeetingsCard({
  canWrite,
  cancellingMeetingId,
  events,
  form,
  isScheduling,
  onCancel,
  onSchedule,
  onSetForm,
  onToggle,
  open,
  status
}) {
  return (
    <Card>
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h3>Meetings</h3>
            <p className="field-hint">Schedule meetings and keep a contact-level calendar history.</p>
          </div>
          <Button className="button-secondary" type="button" onClick={onToggle}>
            {open ? 'Hide meetings' : 'Show meetings'}
          </Button>
        </div>
        {status ? <p className="field-hint" role="status">{status}</p> : null}
        {canWrite ? (
          <form className="auth-form" onSubmit={onSchedule}>
            <Field label="Meeting title">
              <input className="text-input" value={form.title} onChange={(event) => onSetForm((current) => ({ ...current, title: event.target.value }))} placeholder="Discovery call" required />
            </Field>
            <Field label="Meeting start">
              <input className="text-input" type="datetime-local" value={form.startAt} onChange={(event) => onSetForm((current) => ({ ...current, startAt: event.target.value }))} required />
            </Field>
            <Field label="Meeting end">
              <input className="text-input" type="datetime-local" value={form.endAt} onChange={(event) => onSetForm((current) => ({ ...current, endAt: event.target.value }))} required />
            </Field>
            <Field label="Meeting location">
              <input className="text-input" value={form.location} onChange={(event) => onSetForm((current) => ({ ...current, location: event.target.value }))} placeholder="Zoom, Teams, office" />
            </Field>
            <Field label="Meeting notes">
              <textarea className="text-input" rows={3} value={form.description} onChange={(event) => onSetForm((current) => ({ ...current, description: event.target.value }))} />
            </Field>
            <Field label="Meeting visibility">
              <select className="text-input" value={form.visibility} onChange={(event) => onSetForm((current) => ({ ...current, visibility: event.target.value }))}>
                <option value="shared">Shared</option>
                <option value="private">Private</option>
              </select>
            </Field>
            <Button type="submit" disabled={isScheduling}>{isScheduling ? 'Scheduling...' : 'Schedule meeting'}</Button>
          </form>
        ) : null}
        {open ? (
          <div className="record-list" role="list" aria-label="Meeting history">
            {events.length === 0 ? (
              <article className="record-row" role="listitem"><div><p>No meetings scheduled yet.</p></div></article>
            ) : events.map((meeting) => (
              <article className="record-row" key={meeting.id} role="listitem">
                <div>
                  <p>{meeting.title}</p>
                  <p className="field-hint">{meeting.status} · {meeting.location || 'No location'} · {meeting.visibility}</p>
                  {meeting.description ? <p className="field-hint">{meeting.description}</p> : null}
                </div>
                <div>
                  <p>{formatTime(meeting.startAt)}</p>
                  <p className="field-hint">Ends {formatTime(meeting.endAt)}</p>
                  {canWrite && meeting.status === 'scheduled' ? (
                    <Button className="button-secondary" type="button" onClick={() => onCancel(meeting.id)} disabled={cancellingMeetingId === meeting.id}>
                      {cancellingMeetingId === meeting.id ? 'Cancelling...' : 'Cancel meeting'}
                    </Button>
                  ) : null}
                </div>
              </article>
            ))}
          </div>
        ) : null}
      </div>
    </Card>
  )
}

export function ContactEmailCard({ canWrite, form, history, isSending, onApplyTemplate, onSend, onSetForm, onToggle, open, status, templates }) {
  return (
    <Card>
      <div className="card-stack">
        <div className="section-header">
          <h3>Email</h3>
          {canWrite ? <Button className="button-secondary" type="button" onClick={onToggle}>{open ? 'Close' : 'Send email'}</Button> : null}
        </div>
        {status ? <p className="field-hint" role="status">{status}</p> : null}
        {canWrite && open ? (
          <form className="auth-form" onSubmit={onSend}>
            {templates.length > 0 ? (
              <Field label="Template">
                <select className="text-input" defaultValue="" onChange={(event) => onApplyTemplate(event.target.value)}>
                  <option value="">Start from scratch</option>
                  {templates.map((template) => <option key={template.id} value={template.id}>{template.name}</option>)}
                </select>
              </Field>
            ) : null}
            <Field label="Subject">
              <input className="text-input" value={form.subject} onChange={(event) => onSetForm({ ...form, subject: event.target.value })} required />
            </Field>
            <Field label="Body">
              <textarea className="text-input" rows={6} value={form.body} onChange={(event) => onSetForm({ ...form, body: event.target.value })} required />
            </Field>
            <p className="field-hint">Merge fields like {'{{first_name}}'} are filled in when the email is sent.</p>
            <Button type="submit" disabled={isSending}>{isSending ? 'Sending...' : 'Send email'}</Button>
          </form>
        ) : null}
        {open && history.length > 0 ? (
          <div className="record-list" role="list" aria-label="Email history">
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

export function ContactSequencesCard({
  canWrite,
  enrollments,
  form,
  isEnrolling,
  onCancel,
  onEnroll,
  onSetForm,
  onToggle,
  open,
  options,
  status
}) {
  return (
    <Card>
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h3>Sequences</h3>
            <p className="field-hint">Enroll this contact into a prepared cadence. Due steps are delivered by the durable background worker.</p>
          </div>
          {canWrite ? <Button className="button-secondary" type="button" onClick={onToggle}>{open ? 'Close' : 'Manage sequences'}</Button> : null}
        </div>
        {status ? <p className="field-hint" role="status">{status}</p> : null}
        {canWrite && open ? (
          <form className="auth-form" onSubmit={onEnroll}>
            {options.length > 0 ? (
              <Field label="Sequence">
                <select className="text-input" value={form.sequenceId} onChange={(event) => onSetForm({ sequenceId: event.target.value })} required>
                  {options.map((sequence) => <option key={sequence.id} value={sequence.id}>{sequence.name}</option>)}
                </select>
              </Field>
            ) : <p className="field-hint">Approve a sequence in Settings before enrolling contacts.</p>}
            <Button type="submit" disabled={isEnrolling || options.length === 0}>{isEnrolling ? 'Enrolling...' : 'Enroll contact'}</Button>
          </form>
        ) : null}
        {open ? (
          <div className="record-list" role="list" aria-label="Sequence enrollments">
            {enrollments.length === 0 ? (
              <article className="record-row" role="listitem"><div><p>No active sequence enrollments.</p></div></article>
            ) : enrollments.map((enrollment) => (
              <article className="record-row" key={enrollment.id} role="listitem">
                <div>
                  <p>{enrollment.sequenceName}</p>
                  <p className="field-hint">Step {enrollment.currentStepOrder} · next send {formatTime(enrollment.nextSendAt)}</p>
                </div>
                {canWrite ? <div><Button className="button-secondary" type="button" onClick={() => onCancel(enrollment.id)}>Cancel</Button></div> : null}
              </article>
            ))}
          </div>
        ) : null}
      </div>
    </Card>
  )
}
