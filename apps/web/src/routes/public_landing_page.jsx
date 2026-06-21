import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { submitPublicLeadCaptureForm } from '../lib/lead_forms'
import { getPublicLeadLandingPage } from '../lib/landing_pages'
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

export function PublicLandingPageRoute() {
  const { slug = '' } = useParams()
  const [landingPage, setLandingPage] = useState(null)
  const [form, setForm] = useState(null)
  const [values, setValues] = useState({})
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSubmitting, setIsSubmitting] = useState(false)
  usePageTitle(landingPage?.title || 'Landing Page')

  async function loadLandingPage({ signal } = {}) {
    setIsLoading(true)
    try {
      const result = await getPublicLeadLandingPage(slug, { signal })
      setLandingPage(result?.page || null)
      setForm(result?.form || null)
      setValues(initialValues(result?.form?.fields || []))
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load landing page.')
      }
    } finally {
      if (!signal?.aborted) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadLandingPage({ signal: controller.signal })
    return () => {
      controller.abort()
    }
  }, [slug])

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
  const theme = landingPage?.theme || 'light'

  return (
    <main className={`landing-page landing-page-${theme}`}>
      <section className="landing-page-panel">
        {isLoading ? <p className="field-hint">Loading landing page...</p> : null}
        {error && !landingPage ? <InlineError message={error} onRetry={() => loadLandingPage()} retryLabel="Retry page" /> : null}
        {landingPage && form ? (
          <div className="landing-page-grid">
            <div className="landing-page-copy">
              <p className="eyebrow">{form.sourceLabel || 'Lead capture'}</p>
              <h1>{landingPage.title}</h1>
              {landingPage.subtitle ? <p className="landing-page-subtitle">{landingPage.subtitle}</p> : null}
              {landingPage.body ? <p className="message-body">{landingPage.body}</p> : null}
            </div>
            <form className="auth-form landing-page-form" onSubmit={handleSubmit}>
              <div>
                <h2>{form.title || landingPage.ctaLabel}</h2>
                {form.description ? <p className="field-hint">{form.description}</p> : null}
              </div>
              {fields.map((field) => (
                field.fieldType === 'textarea' ? (
                  <Field key={field.key} label={field.label}>
                    <textarea className="text-input" rows={4} value={values[field.key] || ''} onChange={(event) => setValues({ ...values, [field.key]: event.target.value })} required={field.required} />
                  </Field>
                ) : (
                  <Field key={field.key} label={field.label}>
                    <input className="text-input" type={inputType(field)} value={values[field.key] || ''} onChange={(event) => setValues({ ...values, [field.key]: event.target.value })} required={field.required} />
                  </Field>
                )
              ))}
              {error ? <p className="form-error" role="alert">{error}</p> : null}
              {status ? <p className="inline-note" role="status">{status}</p> : null}
              <Button type="submit" disabled={isSubmitting}>{isSubmitting ? 'Submitting...' : landingPage.ctaLabel || 'Submit'}</Button>
            </form>
          </div>
        ) : null}
      </section>
    </main>
  )
}
