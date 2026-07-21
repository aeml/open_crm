import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { emailEngagementSummary, formatEmailTimestamp, getEmailMessage, listEmailMessages } from '../lib/email_messages'
import { usePageTitle } from '../lib/use_page_title'

export function SettingsEmailLogRoute() {
  const { session } = useAuth()
  usePageTitle('Email Log')
  const role = session?.membership?.role || ''
  const canView = ['owner', 'admin'].includes(role)
  const [messages, setMessages] = useState([])
  const [selectedMessage, setSelectedMessage] = useState(null)
  const [error, setError] = useState('')
  const [detailError, setDetailError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isDetailLoading, setIsDetailLoading] = useState(false)

  async function load({ signal } = {}) {
    if (!canView) {
      setError('Admin access required')
      setIsLoading(false)
      return
    }
    setIsLoading(true)
    try {
      const next = await listEmailMessages({ signal })
      setMessages(next)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load email log.')
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
    return () => {
      controller.abort()
    }
  }, [canView])

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
              <h2>Email log</h2>
              <p>Customer emails sent through {session?.organization?.name || 'your workspace'} by your team.</p>
            </div>
            <div>
              <Button className="button-secondary" type="button" onClick={() => load()} disabled={!canView || isLoading}>Refresh</Button>
            </div>
          </div>
          {isLoading ? <p className="field-hint">Loading email log...</p> : null}
          {error ? <InlineError message={error} /> : null}
          <div className="record-list" role="list" aria-label="Sent emails">
            {!isLoading && messages.length === 0 ? (
              <article className="record-row" role="listitem">
                <p>No emails sent yet.</p>
              </article>
            ) : messages.map((message) => (
              <article className={message.status === 'failed' || message.deliveryOutcome ? 'record-row record-row-alert' : 'record-row'} key={message.id} role="listitem">
                <div>
                  <h3>{message.subject}</h3>
                  <p className="field-hint">To {message.toEmail} · {message.sentByName || 'Unknown sender'}{message.status === 'failed' ? ' · Failed' : message.deliveryOutcome ? ` · ${message.deliveryOutcome}` : ''}</p>
                  <p className="field-hint">{emailEngagementSummary(message)}</p>
                </div>
                <div>
                  <p>{formatEmailTimestamp(message.createdAt)}</p>
                  <Button className="button-secondary" type="button" onClick={() => handleSelectMessage(message.id)}>View details</Button>
                </div>
              </article>
            ))}
          </div>
          {isDetailLoading ? <p className="field-hint">Loading message details...</p> : null}
          {detailError ? <InlineError message={detailError} /> : null}
          {selectedMessage ? (
            <Card>
              <div className="card-stack">
                <div>
                  <h3>{selectedMessage.subject}</h3>
                  <p className="field-hint">To {selectedMessage.toEmail} · {selectedMessage.sentByName || 'Unknown sender'} · {formatEmailTimestamp(selectedMessage.createdAt)}{selectedMessage.deliveryOutcome ? ` · ${selectedMessage.deliveryOutcome}` : ''}</p>
                  <p className="field-hint">{emailEngagementSummary(selectedMessage)}</p>
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
