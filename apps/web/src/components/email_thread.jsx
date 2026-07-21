import { useEffect, useRef, useState } from 'react'
import { Button } from './ui/button'
import { Card } from './ui/card'
import { InlineError } from './ui/inline_error'
import { Field } from './ui/field'
import { formatEmailTimestamp, getEmailThread, resolveEmailReply, sendEmailReply } from '../lib/email_messages'
import { isAbortError } from '../lib/api'

function newReplyKey() {
  return `email-reply-${globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random()}`}`
}

function participant(message) {
  return message.direction === 'inbound'
    ? `From ${message.fromEmail || 'unknown sender'}`
    : `From ${message.sentByName || message.fromEmail || 'team member'} to ${message.toEmail || 'unknown recipient'}`
}

function replyStateLabel(reply) {
  if (reply.status === 'uncertain') return 'Outcome uncertain — check the sender’s Sent folder before resolving.'
  if (reply.status === 'failed') return 'Not sent'
  if (reply.status === 'sending') return 'Sending'
  return 'Prepared'
}

export function EmailThread({ messageId, canWrite, currentUserId, canManageReplies }) {
  const [thread, setThread] = useState({ messages: [], replies: [] })
  const [body, setBody] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSending, setIsSending] = useState(false)
  const replyKey = useRef(newReplyKey())

  async function load({ signal } = {}) {
    setIsLoading(true)
    try {
      setThread(await getEmailThread(messageId, { signal }))
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) setError(loadError.message || 'Unable to load email thread.')
    } finally {
      if (!signal?.aborted) setIsLoading(false)
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    load({ signal: controller.signal })
    return () => controller.abort()
  }, [messageId])

  const replyTarget = [...thread.messages].reverse().find((message) => message.direction === 'inbound')
  const currentUserHasUnresolvedReply = thread.replies.some((reply) => reply.actorUserId === currentUserId && ['prepared', 'sending', 'uncertain'].includes(reply.status))

  async function handleReply(event) {
    event.preventDefault()
    if (!replyTarget || !body.trim()) return
    setIsSending(true)
    setError('')
    try {
      const reply = await sendEmailReply(replyTarget.id, body, replyKey.current)
      if (reply?.status === 'accepted') {
        setBody('')
        replyKey.current = newReplyKey()
      } else if (reply?.status === 'failed') {
        replyKey.current = newReplyKey()
      }
      await load()
    } catch (sendError) {
      if (!isAbortError(sendError)) {
        await load()
        setError(sendError.message || 'Unable to send email reply.')
      }
    } finally {
      setIsSending(false)
    }
  }

  async function resolve(reply, resolution) {
    const warning = resolution === 'retry'
      ? 'Retry this reply? The earlier provider outcome is unknown, so this can send a duplicate.'
      : resolution === 'confirmed_sent'
        ? 'Confirm this reply was sent after checking the sender’s Sent folder? This records it in the CRM without another provider call.'
        : 'Mark this reply not sent? No provider call will be made.'
    if (!window.confirm(warning)) return
    setIsSending(true)
    setError('')
    try {
      await resolveEmailReply(reply.id, resolution)
      await load()
    } catch (resolveError) {
      if (!isAbortError(resolveError)) setError(resolveError.message || 'Unable to resolve email reply.')
    } finally {
      setIsSending(false)
    }
  }

  return (
    <Card>
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h3>Conversation</h3>
            <p className="field-hint">Replies use your own connected mailbox. Open CRM never sends as the original mailbox owner.</p>
          </div>
          <Button className="button-secondary" type="button" onClick={() => load()} disabled={isLoading}>Refresh thread</Button>
        </div>
        {isLoading ? <p className="field-hint">Loading conversation...</p> : null}
        {error ? <InlineError message={error} /> : null}
        {!isLoading && thread.messages.length === 0 ? <p className="field-hint">No visible messages in this conversation.</p> : null}
        {thread.messages.map((message) => (
          <article className="record-row" key={message.id}>
            <div className="card-stack">
              <div>
                <h4>{message.subject}</h4>
                <p className="field-hint">{participant(message)} · {formatEmailTimestamp(message.receivedAt || message.createdAt)}</p>
              </div>
              {message.error ? <InlineError message={message.error} /> : null}
              <pre className="field-hint message-body">{message.body}</pre>
            </div>
          </article>
        ))}
        {thread.replies.map((reply) => {
          const mayResolve = reply.actorUserId === currentUserId || canManageReplies
          return (
            <article className="record-row record-row-alert" key={`reply-${reply.id}`}>
              <div className="card-stack">
                <h4>{reply.subject}</h4>
                <p className="field-hint">From {reply.senderEmail} to {reply.recipientEmail} · {replyStateLabel(reply)}</p>
                <pre className="field-hint message-body">{reply.body}</pre>
                {reply.lastError ? <InlineError message={reply.lastError} /> : null}
                {reply.status === 'uncertain' && mayResolve ? (
                  <div className="button-row">
                    {reply.actorUserId === currentUserId ? <Button className="button-secondary" type="button" onClick={() => resolve(reply, 'retry')} disabled={isSending}>Retry explicitly</Button> : null}
                    <Button className="button-secondary" type="button" onClick={() => resolve(reply, 'confirmed_sent')} disabled={isSending}>Confirm sent</Button>
                    <Button className="button-secondary" type="button" onClick={() => resolve(reply, 'not_sent')} disabled={isSending}>Mark not sent</Button>
                  </div>
                ) : null}
              </div>
            </article>
          )
        })}
        {currentUserHasUnresolvedReply ? <p className="inline-note">Resolve your uncertain or in-progress reply before composing another message in this conversation.</p> : null}
        {canWrite && replyTarget && !currentUserHasUnresolvedReply ? (
          <form className="form-grid" onSubmit={handleReply}>
            <Field label={`Reply to ${replyTarget.fromEmail || 'sender'}`}>
              <textarea className="text-input" rows={5} value={body} onChange={(event) => setBody(event.target.value)} maxLength={100000} required />
            </Field>
            <p className="field-hint">Thread headers are preserved. Engagement tracking is off for replies.</p>
            <Button type="submit" disabled={isSending}>{isSending ? 'Sending...' : 'Send reply'}</Button>
          </form>
        ) : null}
      </div>
    </Card>
  )
}
