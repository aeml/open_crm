import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { getEmailMessage, listSharedInboxEmailMessages, updateSharedInboxEmailMessage } from '../lib/email_messages'
import { usePageTitle } from '../lib/use_page_title'

function formatTimestamp(value) {
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleString()
}

function messageTimestamp(message) {
  return message?.receivedAt || message?.createdAt
}

function recordPath(message) {
  if (!message?.entityType || !message?.entityId) return ''
  if (message.entityType === 'contact') return `/contacts/${message.entityId}`
  if (message.entityType === 'company') return `/companies/${message.entityId}`
  if (message.entityType === 'deal') return `/deals/${message.entityId}`
  return ''
}

function recordLabel(message) {
  if (!message?.entityType || !message?.entityId) return ''
  return `${message.entityType} #${message.entityId}`
}

function assignmentLabel(message) {
  return message?.sharedInboxAssignedToUserName || 'Unassigned'
}

export function TeamInboxRoute() {
  const { session, workspaceWritable } = useAuth()
  usePageTitle('Team Inbox')
  const [messages, setMessages] = useState([])
  const [selectedMessage, setSelectedMessage] = useState(null)
  const [error, setError] = useState('')
  const [detailError, setDetailError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isUpdating, setIsUpdating] = useState(false)

  async function load({ signal } = {}) {
    setIsLoading(true)
    try {
      setMessages(await listSharedInboxEmailMessages({ signal }))
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load shared inbox.')
      }
    } finally {
      if (!signal?.aborted) setIsLoading(false)
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    load({ signal: controller.signal })
    return () => controller.abort()
  }, [])

  function mergeUpdatedMessage(updated) {
    setMessages((current) => current.map((message) => (message.id === updated.id ? { ...message, ...updated } : message)))
    setSelectedMessage((current) => (current?.id === updated.id ? { ...current, ...updated } : current))
  }

  async function handleSelectMessage(messageId) {
    setDetailError('')
    try {
      setSelectedMessage(await getEmailMessage(messageId))
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setDetailError(loadError.message || 'Unable to load email message.')
      }
    }
  }

  async function updateMessage(messageId, input) {
    setIsUpdating(true)
    setError('')
    try {
      mergeUpdatedMessage(await updateSharedInboxEmailMessage(messageId, input))
    } catch (updateError) {
      if (!isAbortError(updateError)) {
        setError(updateError.message || 'Unable to update shared inbox message.')
      }
    } finally {
      setIsUpdating(false)
    }
  }

  function assignToMe(messageId) {
    const userId = session?.user?.id || 0
    if (userId > 0) {
      updateMessage(messageId, { assignedToUserId: userId, status: 'open' })
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Team inbox</h2>
              <p>Shared inbound customer emails that need team follow-up.</p>
            </div>
            <Button className="button-secondary" type="button" onClick={() => load()} disabled={isLoading}>Refresh</Button>
          </div>
          {isLoading ? <p className="field-hint">Loading shared inbox...</p> : null}
          {error ? <InlineError message={error} /> : null}
          <div className="record-list" role="list" aria-label="Team inbox messages">
            {!isLoading && messages.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No shared inbox messages yet.</p>
                  <p className="field-hint">Share inbound mailbox messages from your Mailbox to collaborate on replies.</p>
                </div>
              </article>
            ) : messages.map((message) => {
              const path = recordPath(message)
              const label = recordLabel(message)
              const closed = message.sharedInboxStatus === 'closed'
              return (
                <article className={closed ? 'record-row' : 'record-row record-row-alert'} key={message.id} role="listitem">
                  <div>
                    <h3>{message.subject}</h3>
                    <p className="field-hint">From {message.fromEmail || 'unknown sender'} · {closed ? 'Closed' : 'Open'} · {assignmentLabel(message)}</p>
                    <p className="field-hint">{formatTimestamp(messageTimestamp(message))}</p>
                  </div>
                  <div className="button-row">
                    <Button className="button-secondary" type="button" onClick={() => handleSelectMessage(message.id)}>View details</Button>
                    {!closed && workspaceWritable ? <Button className="button-secondary" type="button" onClick={() => assignToMe(message.id)} disabled={isUpdating}>Assign to me</Button> : null}
                    {workspaceWritable ? <Button className="button-secondary" type="button" onClick={() => updateMessage(message.id, { status: closed ? 'open' : 'closed' })} disabled={isUpdating}>{closed ? 'Reopen' : 'Close'}</Button> : null}
                    {path ? <Link className="button button-ghost" to={path}>Open {label}</Link> : null}
                  </div>
                </article>
              )
            })}
          </div>
          {detailError ? <InlineError message={detailError} /> : null}
          {selectedMessage ? (
            <Card>
              <div className="card-stack">
                <div>
                  <h3>{selectedMessage.subject}</h3>
                  <p className="field-hint">From {selectedMessage.fromEmail || 'unknown sender'} · {formatTimestamp(messageTimestamp(selectedMessage))}</p>
                  <p className="field-hint">{selectedMessage.sharedInboxStatus === 'closed' ? 'Closed' : 'Open'} · {assignmentLabel(selectedMessage)}</p>
                </div>
                <pre className="field-hint message-body">{selectedMessage.body}</pre>
              </div>
            </Card>
          ) : null}
        </div>
      </Card>
    </section>
  )
}
