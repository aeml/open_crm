import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { InlineError } from '../components/ui/inline_error'
import { EmailThread } from '../components/email_thread'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { emailMessageTimestamp, emailRecordLabel, emailRecordPath, formatEmailTimestamp, listSharedInboxEmailMessages, updateSharedInboxEmailMessage } from '../lib/email_messages'
import { usePageTitle } from '../lib/use_page_title'

function assignmentLabel(message) {
  return message?.sharedInboxAssignedToUserName || 'Unassigned'
}

const inboxLoadError = 'Unable to load shared inbox.'

export function TeamInboxRoute() {
  const { session, canWrite } = useAuth()
  usePageTitle('Team Inbox')
  const [messages, setMessages] = useState([])
  const [selectedMessageId, setSelectedMessageId] = useState(null)
  const [pageMeta, setPageMeta] = useState({ hasMore: false })
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isLoadingOlder, setIsLoadingOlder] = useState(false)
  const [isUpdating, setIsUpdating] = useState(false)
  const operationPending = useRef(false)

  async function load({ signal } = {}) {
    const userRequested = !signal
    if (userRequested && operationPending.current) return
    if (userRequested) operationPending.current = true
    setIsLoading(true)
    try {
      const page = await listSharedInboxEmailMessages({ signal })
      if (signal?.aborted) return
      setMessages(page.messages)
      setPageMeta(page.meta)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || inboxLoadError)
      }
    } finally {
      if (userRequested) operationPending.current = false
      if (!signal?.aborted) setIsLoading(false)
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    load({ signal: controller.signal })
    return () => controller.abort()
  }, [])

  function mergeUpdatedMessage(updated) {
    setMessages((current) => current.map((message) => message.id === updated.id ? { ...message, ...updated } : message))
  }

  async function updateMessage(message, input) {
    if (operationPending.current) return
    operationPending.current = true
    setIsUpdating(true)
    setError('')
    try {
      const updated = await updateSharedInboxEmailMessage(message.id, { ...input, expectedUpdatedAt: message.sharedInboxUpdatedAt })
      if (updated?.id !== message.id || updated.direction !== 'inbound' || updated.visibility !== 'shared') {
        throw new Error('Unable to update message. Refresh and try again.')
      }
      mergeUpdatedMessage(updated)
    } catch (updateError) {
      if (!isAbortError(updateError)) {
        setError(updateError.message || 'Unable to update message.')
      }
    } finally {
      operationPending.current = false
      setIsUpdating(false)
    }
  }

  async function loadOlderMessages() {
    const cursor = pageMeta.nextCursor
    if (!pageMeta.hasMore || !cursor || operationPending.current) return
    operationPending.current = true
    setIsLoadingOlder(true)
    setError('')
    try {
      const page = await listSharedInboxEmailMessages({ cursor, limit: pageMeta.limit || 50 })
      setMessages((current) => {
        const ids = new Set(current.map((message) => message.id))
        return [...current, ...page.messages.filter((message) => !ids.has(message.id))]
      })
      setPageMeta((current) => current.nextCursor === cursor ? page.meta : current)
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || inboxLoadError)
      }
    } finally {
      operationPending.current = false
      setIsLoadingOlder(false)
    }
  }

  function assignToMe(message) {
    const userId = session?.user?.id || 0
    if (userId > 0) {
      updateMessage(message, { assignedToUserId: userId, status: 'open' })
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
            <Button className="button-secondary" type="button" onClick={() => load()} disabled={isLoading || isLoadingOlder || isUpdating}>Refresh</Button>
          </div>
          {isLoading ? <p className="field-hint" role="status">Loading shared inbox...</p> : null}
          {error ? <InlineError message={error} /> : null}
          <div className="record-list" role="list" aria-label="Team inbox messages" aria-busy={isLoading || isLoadingOlder}>
            {!isLoading && messages.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No shared inbox messages yet.</p>
                  <p className="field-hint">Share inbound mailbox messages from your Mailbox to collaborate on replies.</p>
                </div>
              </article>
            ) : messages.map((message) => {
              const path = emailRecordPath(message)
              const label = emailRecordLabel(message)
              const closed = message.sharedInboxStatus === 'closed'
              return (
                <article className={closed ? 'record-row' : 'record-row record-row-alert'} key={message.id} role="listitem">
                  <div>
                    <h3>{message.subject}</h3>
                    <p className="field-hint">From {message.fromEmail || 'unknown sender'} · {closed ? 'Closed' : 'Open'} · {assignmentLabel(message)}</p>
                    <p className="field-hint">{formatEmailTimestamp(emailMessageTimestamp(message))}</p>
                  </div>
                  <div className="button-row">
                    <Button className="button-secondary" type="button" onClick={() => setSelectedMessageId(message.id)}>View details</Button>
                    {!closed && canWrite ? <Button className="button-secondary" type="button" onClick={() => assignToMe(message)} disabled={isLoading || isLoadingOlder || isUpdating}>Assign to me</Button> : null}
                    {canWrite ? <Button className="button-secondary" type="button" onClick={() => updateMessage(message, { status: closed ? 'open' : 'closed' })} disabled={isLoading || isLoadingOlder || isUpdating}>{closed ? 'Reopen' : 'Close'}</Button> : null}
                    {path ? <Link className="button button-ghost" to={path}>Open {label}</Link> : null}
                  </div>
                </article>
              )
            })}
          </div>
          {pageMeta.hasMore && pageMeta.nextCursor ? (
            <div className="button-row">
              <Button className="button-secondary" type="button" onClick={loadOlderMessages} disabled={isLoading || isLoadingOlder || isUpdating}>
                {isLoadingOlder ? 'Loading...' : 'Load older messages'}
              </Button>
            </div>
          ) : null}
          {selectedMessageId ? <EmailThread messageId={selectedMessageId} canWrite={canWrite} currentUserId={session?.user?.id || 0} canManageReplies={['owner', 'admin'].includes(session?.membership?.role || '')} /> : null}
        </div>
      </Card>
    </section>
  )
}
