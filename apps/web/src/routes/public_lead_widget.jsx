import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { submitPublicLeadCaptureForm } from '../lib/lead_forms'
import { getPublicLeadChatWidget } from '../lib/lead_chat_widgets'
import { usePageTitle } from '../lib/use_page_title'

function initialValues(fields = []) {
  return fields.reduce((values, field) => ({ ...values, [field.key]: '' }), {})
}

function inputType(field) {
  if (field.fieldType === 'email') return 'email'
  if (field.fieldType === 'tel') return 'tel'
  if (field.fieldType === 'hidden') return 'hidden'
  return 'text'
}

function attributionFromLocation(form) {
  if (typeof window === 'undefined') {
    return { leadSource: form?.sourceLabel || '' }
  }
  const params = new URL(window.location.href).searchParams
  return {
    leadSource: form?.sourceLabel || '',
    utmSource: params.get('utm_source') || params.get('utmSource') || '',
    utmMedium: params.get('utm_medium') || params.get('utmMedium') || '',
    utmCampaign: params.get('utm_campaign') || params.get('utmCampaign') || '',
    utmTerm: params.get('utm_term') || params.get('utmTerm') || '',
    utmContent: params.get('utm_content') || params.get('utmContent') || ''
  }
}

export function PublicLeadWidgetRoute() {
  const { publicId = '' } = useParams()
  const [widget, setWidget] = useState(null)
  const [form, setForm] = useState(null)
  const [values, setValues] = useState({})
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSubmitting, setIsSubmitting] = useState(false)
  usePageTitle(widget?.title || 'Website Widget')

  async function loadWidget({ signal } = {}) {
    setIsLoading(true)
    try {
      const result = await getPublicLeadChatWidget(publicId, { signal })
      setWidget(result?.widget || null)
      setForm(result?.form || null)
      setValues(initialValues(result?.form?.fields || []))
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

  async function handleSubmit(event) {
    event.preventDefault()
    if (!form?.publicId) return

    setIsSubmitting(true)
    setStatus('')
    setError('')
    try {
      const result = await submitPublicLeadCaptureForm(form.publicId, {
        values,
        sourceUrl: typeof window === 'undefined' ? '' : window.location.href,
        attribution: attributionFromLocation(form)
      })
      setStatus(result?.successMessage || form.successMessage || 'Thanks. We will be in touch soon.')
      setValues(initialValues(form.fields || []))
    } catch (submitError) {
      setError(submitError.message || 'Unable to submit the form.')
    } finally {
      setIsSubmitting(false)
    }
  }

  const fields = form?.fields || []
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
          <form className="auth-form" onSubmit={handleSubmit}>
            {fields.map((field) => (
              field.fieldType === 'textarea' ? (
                <Field key={field.key} label={field.label}>
                  <textarea className="text-input" rows={3} value={values[field.key] || ''} onChange={(event) => setValues({ ...values, [field.key]: event.target.value })} required={field.required} />
                </Field>
              ) : (
                <Field key={field.key} label={field.label}>
                  <input className="text-input" type={inputType(field)} value={values[field.key] || ''} onChange={(event) => setValues({ ...values, [field.key]: event.target.value })} required={field.required} />
                </Field>
              )
            ))}
            {error ? <p className="form-error" role="alert">{error}</p> : null}
            {status ? <p className="inline-note" role="status">{status}</p> : null}
            <Button type="submit" disabled={isSubmitting}>{isSubmitting ? 'Sending...' : widget.ctaLabel || 'Send'}</Button>
          </form>
        </section>
      ) : null}
    </main>
  )
}
