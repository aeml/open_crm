import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { getPublicLeadChatWidget } from '../lib/lead_chat_widgets'
import { usePageTitle } from '../lib/use_page_title'
import { PublicLeadCaptureForm } from './public_lead_capture_form'

export function PublicLeadWidgetRoute() {
  const { publicId = '' } = useParams()
  const [widget, setWidget] = useState(null)
  const [form, setForm] = useState(null)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  usePageTitle(widget?.title || 'Website Widget')

  async function loadWidget({ signal } = {}) {
    setIsLoading(true)
    try {
      const result = await getPublicLeadChatWidget(publicId, { signal })
      setWidget(result?.widget || null)
      setForm(result?.form || null)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load website widget.')
      }
    } finally {
      if (!signal?.aborted) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadWidget({ signal: controller.signal })
    return () => {
      controller.abort()
    }
  }, [publicId])

  const theme = widget?.theme || 'light'
  const position = widget?.position || 'bottom-right'

  return (
    <main className={`chat-widget-page chat-widget-${theme} chat-widget-${position}`}>
      {isLoading ? <p className="field-hint">Loading widget...</p> : null}
      {error && !widget ? <InlineError message={error} onRetry={() => loadWidget()} retryLabel="Retry widget" /> : null}
      {widget && form ? (
        <section className="chat-widget-card" aria-label={widget.title || 'Lead chat widget'}>
          <div className="chat-widget-header">
            <span className="chat-widget-avatar" aria-hidden="true">OC</span>
            <div>
              <p className="eyebrow">{widget.promptLabel || 'Chat with us'}</p>
              <h1>{widget.title}</h1>
            </div>
          </div>
          {widget.welcomeMessage ? <p className="chat-bubble">{widget.welcomeMessage}</p> : null}
          <PublicLeadCaptureForm className="auth-form" form={form} submitLabel={widget.ctaLabel || 'Send'} submittingLabel="Sending..." textareaRows={3} />
        </section>
      ) : null}
    </main>
  )
}
