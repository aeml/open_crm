import { useEffect, useState } from 'react'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { isAbortError } from '../lib/api'
import { issuePublicLeadSubmissionChallenge, submitPublicLeadCaptureForm, waitForPublicLeadChallenge } from '../lib/lead_forms'

function initialValues(fields = []) {
  return Object.fromEntries(fields.map((field) => [field.key, '']))
}

function inputType(field) {
  return ['email', 'tel', 'hidden'].includes(field.fieldType) ? field.fieldType : 'text'
}

function attributionFromLocation(form) {
  if (typeof window === 'undefined') return { leadSource: form?.sourceLabel || '' }
  const params = new URL(window.location.href).searchParams
  const attribution = { leadSource: form?.sourceLabel || '' }
  for (const name of ['Source', 'Medium', 'Campaign', 'Term', 'Content']) {
    attribution[`utm${name}`] = params.get(`utm_${name.toLowerCase()}`) || params.get(`utm${name}`) || ''
  }
  return attribution
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

  function refresh(publicId) {
    return prepare(publicId).catch((refreshError) => {
      setError(refreshError.message || 'Unable to prepare the form.')
    })
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
      <label className="field-hint checkbox-row">
        <input type="checkbox" checked={consentGranted} onChange={(event) => setConsentGranted(event.target.checked)} required />
        <span>{challenge?.consentText || form.consentText || 'I agree to be contacted about this request.'}</span>
      </label>
      {error ? <p className="form-error" role="alert">{error}</p> : null}
      {status || (!challenge && !error) ? <p className={status ? 'inline-note' : 'field-hint'} role="status">{status || 'Preparing secure form...'}</p> : null}
      <Button type="submit" disabled={isSubmitting || !challenge}>{isSubmitting ? submittingLabel : submitLabel}</Button>
    </form>
  )
}
