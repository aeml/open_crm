import { useEffect, useState } from 'react'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { isAbortError } from '../lib/api'
import { issuePublicLeadSubmissionChallenge, submitPublicLeadCaptureForm, waitForPublicLeadChallenge } from '../lib/lead_forms'

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
  if (typeof window === 'undefined') return { leadSource: form?.sourceLabel || '' }
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

export function PublicLeadCaptureForm({ className = 'auth-form landing-page-form', form, submitLabel = 'Submit', submittingLabel = 'Submitting...', textareaRows = 4 }) {
  const [values, setValues] = useState(() => initialValues(form?.fields))
  const [challenge, setChallenge] = useState(null)
  const [consentGranted, setConsentGranted] = useState(false)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  async function prepare(publicId, { signal } = {}) {
    setChallenge(null)
    setConsentGranted(false)
    const nextChallenge = await issuePublicLeadSubmissionChallenge(publicId || '', { signal })
    setChallenge(nextChallenge || null)
  }

  async function refresh(publicId) {
    try {
      await prepare(publicId)
    } catch (refreshError) {
      setChallenge(null)
      setError(refreshError.message || 'Unable to prepare the form.')
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    setValues(initialValues(form?.fields))
    setError('')
    prepare(form?.publicId || '', { signal: controller.signal }).catch((prepareError) => {
      if (!isAbortError(prepareError)) setError(prepareError.message || 'Unable to prepare the form.')
    })
    return () => controller.abort()
  }, [form?.publicId])

  async function handleSubmit(event) {
    event.preventDefault()
    if (!form?.publicId || !challenge?.token) return

    setIsSubmitting(true)
    setStatus('')
    setError('')
    try {
      await waitForPublicLeadChallenge(challenge)
      const result = await submitPublicLeadCaptureForm(form.publicId, {
        values,
        sourceUrl: typeof window === 'undefined' ? '' : window.location.href,
        attribution: attributionFromLocation(form),
        challengeToken: challenge.token,
        consentGranted
      })
      setStatus(result?.successMessage || form.successMessage || 'Thanks. We will be in touch soon.')
      setValues(initialValues(form.fields))
      setConsentGranted(false)
      setChallenge(null)
      await refresh(form.publicId)
    } catch (submitError) {
      if (submitError?.payload?.error?.code === 'SUBMISSION_CHALLENGE_INVALID') await refresh(form.publicId)
      setError(submitError.message || 'Unable to submit the form.')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <form className={className} onSubmit={handleSubmit}>
      <div>
        <h2>{form.title || submitLabel}</h2>
        {form.description ? <p className="field-hint">{form.description}</p> : null}
      </div>
      {(form.fields || []).map((field) => (
        field.fieldType === 'textarea' ? (
          <Field key={field.key} label={field.label}>
            <textarea className="text-input" rows={textareaRows} value={values[field.key] || ''} onChange={(event) => setValues({ ...values, [field.key]: event.target.value })} required={field.required} />
          </Field>
        ) : (
          <Field key={field.key} label={field.label}>
            <input className="text-input" type={inputType(field)} value={values[field.key] || ''} onChange={(event) => setValues({ ...values, [field.key]: event.target.value })} required={field.required} />
          </Field>
        )
      ))}
      <label className="field-hint lead-consent-control">
        <input type="checkbox" checked={consentGranted} onChange={(event) => setConsentGranted(event.target.checked)} required />
        <span>{challenge?.consentText || form.consentText || 'I agree to be contacted about this request.'}</span>
      </label>
      {error ? <p className="form-error" role="alert">{error}</p> : null}
      {status ? <p className="inline-note" role="status">{status}</p> : null}
      {!challenge && !error && !status ? <p className="field-hint" role="status">Preparing secure form...</p> : null}
      <Button type="submit" disabled={isSubmitting || !challenge}>{isSubmitting ? submittingLabel : submitLabel}</Button>
    </form>
  )
}
