import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { getEmailMessage, listMyEmailMessages } from '../lib/email_messages'
import { usePageTitle } from '../lib/use_page_title'

function formatTimestamp(value) {
  if (!value) {
    return ''
  }
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleString()
}

function recordPath(message) {
  if (!message?.entityType || !message?.entityId) {
    return ''
  }
  if (message.entityType === 'contact') {
    return `/contacts/${message.entityId}`
  }
  if (message.entityType === 'company') {
    return `/companies/${message.entityId}`
  }
  if (message.entityType === 'deal') {
    return `/deals/${message.entityId}`
  }
  return ''
}

function recordLabel(message) {
  if (!message?.entityType || !message?.entityId) {
    return ''
  }
  return `${message.entityType} #${message.entityId}`
}

export function MailboxRoute() {
  usePageTitle('Mailbox')
  const [messages, setMessages] = useState([])
  const [selectedMessage, setSelectedMessage] = useState(null)
  const [error, setError] = useState('')
  const [detailError, setDetailError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isDetailLoading, setIsDetailLoading] = useState(false)

  async function load({ signal } = {}) {
    setIsLoading(true)
    try {
      const next = await listMyEmailMessages({ signal })
      setMessages(next)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load your sent email.')
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

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Mailbox</h2>
              <p>Your CRM-sent emails. Incoming sync will appear here once mailbox sync is enabled.</p>
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
                  <p>No sent CRM emails yet.</p>
                  <p className="field-hint">Emails you send from contacts, companies, and deals will appear here.</p>
                </div>
              </article>
            ) : messages.map((message) => {
              const path = recordPath(message)
              const label = recordLabel(message)
              return (
                <article className={message.status === 'failed' ? 'record-row record-row-alert' : 'record-row'} key={message.id} role="listitem">
                  <div>
                    <h3>{message.subject}</h3>
                    <p className="field-hint">To {message.toEmail}{message.status === 'failed' ? ' · Failed' : ''}</p>
                  </div>
                  <div>
                    <p>{formatTimestamp(message.createdAt)}</p>
                    <Button className="button-secondary" type="button" onClick={() => handleSelectMessage(message.id)}>View details</Button>
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
                  <p className="field-hint">To {selectedMessage.toEmail} · {formatTimestamp(selectedMessage.createdAt)}</p>
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
