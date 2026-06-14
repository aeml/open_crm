import { useEffect, useMemo, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { listEmailMessages } from '../lib/email_messages'
import { usePageTitle } from '../lib/use_page_title'

function formatTimestamp(value) {
  if (!value) {
    return ''
  }
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleString()
}

export function SettingsEmailLogRoute() {
  const { session } = useAuth()
  usePageTitle('Email Log')
  const role = session?.membership?.role || ''
  const canView = useMemo(() => ['owner', 'admin'].includes(role), [role])
  const [messages, setMessages] = useState([])
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)

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
                <div>
                  <p>No emails sent yet.</p>
                  <p className="field-hint">Emails your team sends to contacts will appear here.</p>
                </div>
              </article>
            ) : messages.map((message) => (
              <article className={message.status === 'failed' ? 'record-row record-row-alert' : 'record-row'} key={message.id} role="listitem">
                <div>
                  <h3>{message.subject}</h3>
                  <p className="field-hint">To {message.toEmail} · {message.sentByName || 'Unknown sender'}{message.status === 'failed' ? ' · Failed' : ''}</p>
                </div>
                <div>
                  <p>{formatTimestamp(message.createdAt)}</p>
                </div>
              </article>
            ))}
          </div>
        </div>
      </Card>
    </section>
  )
}
