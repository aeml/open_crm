import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'

function formatCallTime(value) {
  if (!value) {
    return 'Unknown time'
  }
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'Unknown time' : date.toLocaleString()
}

function formatRecordingConsent(value) {
  if (value === 'granted') return 'Consent granted'
  if (value === 'denied') return 'Consent denied'
  if (value === 'not_required') return 'Consent not required'
  return 'Consent unknown'
}

function formatRecordingRetention(value) {
  if (!value) {
    return 'No retention date'
  }
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'No retention date' : `Retain until ${date.toLocaleDateString()}`
}

export function ContactCallsCard({
  activeCall,
  callForm,
  calls,
  canWrite,
  contact,
  dialURL,
  inboundForm,
  inboundOpen,
  isCompleting,
  isLogging,
  isStarting,
  isUpdatingRecording,
  onComplete,
  onDeleteRecording,
  onRecordInbound,
  onSetCallForm,
  onSetInboundForm,
  onSetRecordingForm,
  onStart,
  onToggle,
  onToggleInbound,
  onToggleRecording,
  onUpdateRecording,
  open,
  recordingCallId,
  recordingForm,
  status
}) {
  const hasPhone = Boolean(contact?.phone)
  return (
    <Card>
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h3>Calls</h3>
            <p className="field-hint">Start an outbound call and log the outcome on this contact.</p>
          </div>
          <div className="button-row">
            <Button className="button-secondary" type="button" onClick={onToggle}>
              {open ? 'Hide calls' : 'Show calls'}
            </Button>
            {canWrite && hasPhone ? (
              <Button className="button-secondary" type="button" onClick={onStart} disabled={isStarting}>
                {isStarting ? 'Starting...' : 'Start call'}
              </Button>
            ) : null}
            {canWrite ? (
              <Button className="button-secondary" type="button" onClick={onToggleInbound}>
                {inboundOpen ? 'Cancel inbound log' : 'Add inbound call'}
              </Button>
            ) : null}
          </div>
        </div>
        {!hasPhone ? <p className="field-hint">Add a phone number to this contact before starting a call.</p> : null}
        {status ? <p className="field-hint" role="status">{status}</p> : null}
        {dialURL ? <a className="button button-ghost" href={dialURL}>Open dialer</a> : null}
        {activeCall ? (
          <form className="auth-form" onSubmit={onComplete}>
            <Field label="Disposition">
              <input className="text-input" value={callForm.disposition} onChange={(event) => onSetCallForm((current) => ({ ...current, disposition: event.target.value }))} placeholder="Connected, left voicemail, no answer" />
            </Field>
            <Field label="Call notes">
              <textarea className="text-input" rows={4} value={callForm.notes} onChange={(event) => onSetCallForm((current) => ({ ...current, notes: event.target.value }))} />
            </Field>
            <Button type="submit" disabled={isCompleting}>{isCompleting ? 'Logging...' : 'Log call outcome'}</Button>
          </form>
        ) : null}
        {inboundOpen ? (
          <form className="auth-form" onSubmit={onRecordInbound}>
            <Field label="Inbound phone number">
              <input className="text-input" value={inboundForm.phoneNumber} onChange={(event) => onSetInboundForm((current) => ({ ...current, phoneNumber: event.target.value }))} required />
            </Field>
            <Field label="Inbound disposition">
              <input className="text-input" value={inboundForm.disposition} onChange={(event) => onSetInboundForm((current) => ({ ...current, disposition: event.target.value }))} placeholder="Connected, voicemail, missed" />
            </Field>
            <Field label="Inbound notes">
              <textarea className="text-input" rows={4} value={inboundForm.notes} onChange={(event) => onSetInboundForm((current) => ({ ...current, notes: event.target.value }))} />
            </Field>
            <Button type="submit" disabled={isLogging}>{isLogging ? 'Saving...' : 'Save inbound call'}</Button>
          </form>
        ) : null}
        {open ? (
          <div className="record-list" role="list" aria-label="Call history">
            {calls.length === 0 ? (
              <article className="record-row" role="listitem"><div><p>No calls logged yet.</p></div></article>
            ) : calls.map((call) => (
              <article className="record-row" key={call.id} role="listitem">
                <div>
                  <p>{call.disposition || (call.status === 'initiated' ? 'Call started' : 'Call logged')}</p>
                  <p className="field-hint">{call.phoneNumber} · {call.status}</p>
                  {call.notes ? <p className="field-hint">{call.notes}</p> : null}
                  <p className="field-hint">
                    Recording: {call.recordingStatus === 'available' && call.recordingUrl ? <a href={call.recordingUrl} target="_blank" rel="noreferrer">available</a> : (call.recordingStatus === 'deleted' ? 'deleted' : 'not recorded')} · {formatRecordingConsent(call.recordingConsent)}
                  </p>
                  {call.recordingStatus === 'available' ? <p className="field-hint">{formatRecordingRetention(call.recordingRetentionUntil)}</p> : null}
                </div>
                <div>
                  <p>{formatCallTime(call.completedAt || call.startedAt || call.createdAt)}</p>
                  <p className="field-hint">{call.createdByUserName || 'You'}</p>
                  {canWrite ? (
                    <Button className="button-ghost" type="button" onClick={() => onToggleRecording(call)}>
                      {recordingCallId === call.id ? 'Cancel recording controls' : 'Edit recording controls'}
                    </Button>
                  ) : null}
                </div>
                {recordingCallId === call.id ? (
                  <form className="auth-form" onSubmit={onUpdateRecording}>
                    <Field label="Recording URL">
                      <input className="text-input" value={recordingForm.recordingUrl} onChange={(event) => onSetRecordingForm((current) => ({ ...current, recordingUrl: event.target.value }))} placeholder="https://recordings.example/call.mp3" />
                    </Field>
                    <Field label="Recording consent">
                      <select className="text-input" value={recordingForm.recordingConsent} onChange={(event) => onSetRecordingForm((current) => ({ ...current, recordingConsent: event.target.value }))}>
                        <option value="unknown">Unknown</option>
                        <option value="granted">Granted</option>
                        <option value="denied">Denied</option>
                        <option value="not_required">Not required</option>
                      </select>
                    </Field>
                    <Field label="Retention policy">
                      <select className="text-input" value={recordingForm.retentionDays} onChange={(event) => onSetRecordingForm((current) => ({ ...current, retentionDays: event.target.value }))}>
                        <option value="30">Delete after 30 days</option>
                        <option value="90">Delete after 90 days</option>
                        <option value="365">Delete after 1 year</option>
                        <option value="1095">Delete after 3 years</option>
                      </select>
                    </Field>
                    <div className="button-row">
                      <Button type="submit" disabled={isUpdatingRecording}>{isUpdatingRecording ? 'Saving...' : 'Save recording controls'}</Button>
                      {call.recordingStatus === 'available' ? (
                        <Button className="button-secondary" type="button" onClick={onDeleteRecording} disabled={isUpdatingRecording}>Delete recording metadata</Button>
                      ) : null}
                    </div>
                  </form>
                ) : null}
              </article>
            ))}
          </div>
        ) : null}
      </div>
    </Card>
  )
}
