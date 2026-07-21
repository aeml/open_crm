import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { emailMessageTimestamp, emailRecordLabel, emailRecordPath, formatEmailTimestamp, getEmailMessage, listMyEmailMessages, updateSharedInboxEmailMessage } from '../lib/email_messages'
import { usePageTitle } from '../lib/use_page_title'

function engagementStatus(message, field, verb, empty) {
  const count = +message?.[field] || 0
  return count ? `${verb} ${count} ${count === 1 ? 'time' : 'times'}` : empty
}

function isInbound(message) {
  return message?.direction === 'inbound'
}

function participantLabel(message) {
  if (isInbound(message)) {
    return `From ${message.fromEmail || 'unknown sender'}`
  }
  return `To ${message.toEmail || 'unknown recipient'}`
}

export function MailboxRoute() {
  const { session, canWrite } = useAuth()
  usePageTitle('Mailbox')
  const [messages, setMessages] = useState([])
  const [selectedMessage, setSelectedMessage] = useState(null)
  const [error, setError] = useState('')
  const [detailError, setDetailError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isDetailLoading, setIsDetailLoading] = useState(false)
  const [isSharing, setIsSharing] = useState(false)

  async function load({ signal } = {}) {
    setIsLoading(true)
    try {
      const next = await listMyEmailMessages({ signal })
      setMessages(next)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load your mailbox.')
      }
    } finally {
      if (!signal?.aborted) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    load({ signal: controller.signal })
    return () => controller.abort()
  }, [])

  async function handleSelectMessage(messageId) {
    setIsDetailLoading(true)
    setDetailError('')
    try {
      const message = await getEmailMessage(messageId)
      setSelectedMessage(message)
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setDetailError(loadError.message || 'Unable to load email message.')
      }
    } finally {
      setIsDetailLoading(false)
    }
  }

  async function handlePrivacyChange(message, visibility) {
    const sharing = visibility === 'shared'
    const confirmed = window.confirm(sharing
      ? 'Share this complete message with everyone in this workspace?'
      : 'Make private? Team access ends now; audit history remains.')
    if (!confirmed) return
    setIsSharing(true)
    setError('')
    try {
      const userId = session?.user?.id || 0
      const input = {
        visibility,
        expectedUpdatedAt: message.sharedInboxUpdatedAt
      }
      if (sharing) {
        input.status = 'open'
        input.assignedToUserId = userId || undefined
      }
      const updated = await updateSharedInboxEmailMessage(message.id, input)
      setMessages((current) => current.map((message) => (message.id === updated.id ? { ...message, ...updated } : message)))
      setSelectedMessage((current) => (current?.id === updated.id ? { ...current, ...updated } : current))
    } catch (shareError) {
      if (!isAbortError(shareError)) {
        setError(shareError.message || 'Unable to change message privacy.')
      }
    } finally {
      setIsSharing(false)
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Mailbox</h2>
              <p>Your CRM-sent emails and synced incoming mailbox messages.</p>
            </div>
            <div className="button-row">
              <Link className="button button-secondary" to="/settings/email-account">Email settings</Link>
              <Button className="button-secondary" type="button" onClick={() => load()} disabled={isLoading}>Refresh</Button>
            </div>
          </div>
          {isLoading ? <p className="field-hint">Loading mailbox...</p> : null}
          {error ? <InlineError message={error} /> : null}
          <div className="record-list" role="list" aria-label="Mailbox messages">
            {!isLoading && messages.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No mailbox messages yet.</p>
                  <p className="field-hint">Emails you send from CRM records and synced inbound messages will appear here.</p>
                </div>
              </article>
            ) : messages.map((message) => {
              const path = emailRecordPath(message)
              const label = emailRecordLabel(message)
              return (
                <article className={message.status === 'failed' ? 'record-row record-row-alert' : 'record-row'} key={message.id} role="listitem">
                  <div>
                    <h3>{message.subject}</h3>
                    <p className="field-hint">{participantLabel(message)}{message.status === 'failed' ? ' · Failed' : ''}{isInbound(message) ? ' · Received' : ''}</p>
                    {!isInbound(message) ? <p className="field-hint">{engagementStatus(message, 'openCount', 'Opened', 'Not opened yet')}</p> : null}
                    {!isInbound(message) ? <p className="field-hint">{engagementStatus(message, 'clickCount', 'Clicked', 'No clicks yet')}</p> : null}
                  </div>
                  <div>
                    <p>{formatEmailTimestamp(emailMessageTimestamp(message))}</p>
                    <Button className="button-secondary" type="button" onClick={() => handleSelectMessage(message.id)}>View details</Button>
                    {isInbound(message) && canWrite ? (
                      message.visibility === 'shared'
                        ? <Button className="button-secondary" type="button" onClick={() => handlePrivacyChange(message, 'private')} disabled={isSharing}>Make private</Button>
                        : <Button className="button-secondary" type="button" onClick={() => handlePrivacyChange(message, 'shared')} disabled={isSharing}>Share with team</Button>
                    ) : null}
                    {path ? <Link className="button button-ghost" to={path}>Open {label}</Link> : null}
                  </div>
                </article>
              )
            })}
          </div>
          {isDetailLoading ? <p className="field-hint">Loading message details...</p> : null}
          {detailError ? <InlineError message={detailError} /> : null}
          {selectedMessage ? (
            <Card>
              <div className="card-stack">
                <div>
                  <h3>{selectedMessage.subject}</h3>
                  <p className="field-hint">{participantLabel(selectedMessage)} · {formatEmailTimestamp(emailMessageTimestamp(selectedMessage))}</p>
                  {!isInbound(selectedMessage) ? <p className="field-hint">{engagementStatus(selectedMessage, 'openCount', 'Opened', 'Not opened yet')}</p> : null}
                  {!isInbound(selectedMessage) ? <p className="field-hint">{engagementStatus(selectedMessage, 'clickCount', 'Clicked', 'No clicks yet')}</p> : null}
                </div>
                {selectedMessage.error ? <InlineError message={selectedMessage.error} /> : null}
                <pre className="field-hint message-body">{selectedMessage.body}</pre>
              </div>
            </Card>
          ) : null}
        </div>
      </Card>
    </section>
  )
}
