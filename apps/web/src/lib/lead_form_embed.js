import { escapeEmbedHTML } from './api'
import { publicLeadCaptureFormChallengeURL, publicLeadCaptureFormSubmitURL } from './lead_forms'

const fallbackFields = [
  { key: 'firstName', label: 'First name', fieldType: 'text', required: true },
  { key: 'lastName', label: 'Last name', fieldType: 'text', required: true },
  { key: 'email', label: 'Email', fieldType: 'email', required: true }
]
const attributionFields = ['utm_source', 'utm_medium', 'utm_campaign', 'utm_term', 'utm_content']

function inputType(field) {
  return ['email', 'tel', 'hidden', 'number', 'date'].includes(field.fieldType) ? field.fieldType : 'text'
}

function fieldControl(field) {
  const label = escapeEmbedHTML(field.label)
  const name = escapeEmbedHTML(field.key)
  const required = field.required ? ' required' : ''
  if (field.fieldType === 'textarea') {
    return `  <label>${label}\n    <textarea name="${name}"${required}></textarea>\n  </label>`
  }
  if (field.fieldType === 'select') {
    const options = (field.options || []).map((option) => `      <option value="${escapeEmbedHTML(option)}">${escapeEmbedHTML(option)}</option>`)
    return [`  <label>${label}`, `    <select name="${name}"${required}>`, '      <option value="">Select...</option>', ...options, '    </select>', '  </label>'].join('\n')
  }
  if (field.fieldType === 'boolean' || field.fieldType === 'checkbox') {
    return `  <label>${label}\n    <select name="${name}"${required}><option value="">Select...</option><option value="true">Yes</option><option value="false">No</option></select>\n  </label>`
  }
  return `  <label>${label}\n    <input name="${name}" type="${inputType(field)}"${required}>\n  </label>`
}

function scriptLiteral(value) {
  return JSON.stringify(String(value ?? ''))
    .replaceAll('<', '\\u003c')
    .replaceAll('>', '\\u003e')
    .replaceAll('&', '\\u0026')
}

function leadFormEmbedRuntime(challengeURL, expectedRevision, fallbackSuccess, attributionFields) {
  const form = document.currentScript && document.currentScript.previousElementSibling
  if (!form) return
  const button = form.querySelector('button[type="submit"]')
  const status = form.querySelector('[data-open-crm-status]')
  const consent = form.querySelector('[data-open-crm-consent]')
  let challengeToken = ''
  let prepareGeneration = 0
  const setStatus = (message, state = '') => {
    status.textContent = message
    if (state) status.dataset.state = state
    else delete status.dataset.state
  }
  const decode = async (response) => {
    const payload = await response.json().catch(() => ({}))
    if (!response.ok) {
      const error = new Error(payload?.error?.message || 'The form request failed. Retry safely.')
      error.code = payload?.error?.code || ''
      throw error
    }
    return payload
  }
  const hydrateAttribution = () => {
    const currentURL = new URL(window.location.href)
    form.elements.sourceUrl.value = currentURL.href
    for (const field of attributionFields) {
      form.elements[field].value = currentURL.searchParams.get(field) || ''
    }
  }
  const prepare = async ({ preserveStatus = false } = {}) => {
    const generation = ++prepareGeneration
    challengeToken = ''
    form.elements.challengeToken.value = ''
    button.disabled = true
    if (!preserveStatus) setStatus('Preparing form...')
    try {
      const payload = await fetch(challengeURL, { method: 'POST', headers: { Accept: 'application/json' }, credentials: 'omit' }).then(decode)
      const challenge = payload?.data?.challenge
      if (!challenge?.token || Number(challenge.formRevision) !== expectedRevision) throw new Error('This form changed. Refresh the page before submitting.')
      if (generation !== prepareGeneration) return
      challengeToken = String(challenge.token)
      form.elements.challengeToken.value = challengeToken
      consent.textContent = challenge.consentText || consent.textContent
      const notBefore = Date.parse(challenge.notBefore || '')
      const delay = Number.isFinite(notBefore) ? Math.max(0, Math.min(5000, notBefore - Date.now())) : 0
      window.setTimeout(() => {
        if (generation !== prepareGeneration) return
        button.disabled = false
        if (!preserveStatus) setStatus('')
      }, delay)
    } catch (error) {
      if (generation === prepareGeneration) setStatus(error.message || 'Form temporarily unavailable.', 'error')
    }
  }
  form.addEventListener('submit', async (event) => {
    event.preventDefault()
    if (!challengeToken || button.disabled) return
    button.disabled = true
    setStatus('Submitting...')
    const body = new URLSearchParams()
    for (const [name, value] of new FormData(form).entries()) body.append(name, String(value))
    try {
      const payload = await fetch(form.action, { method: 'POST', headers: { Accept: 'application/json' }, body, credentials: 'omit' }).then(decode)
      form.reset()
      hydrateAttribution()
      setStatus(payload?.data?.successMessage || fallbackSuccess, 'success')
      await prepare({ preserveStatus: true })
    } catch (error) {
      if (error.code === 'SUBMISSION_CHALLENGE_INVALID') {
        setStatus('This form changed. Preparing a fresh submission...', 'error')
        await prepare()
        return
      }
      setStatus(error.message || 'Unable to submit the form. Retry safely.', 'error')
      button.disabled = false
    }
  })
  hydrateAttribution()
  prepare()
}

export function leadFormEmbedSnippet(form) {
  const action = publicLeadCaptureFormSubmitURL(form?.publicId || '')
  const challengeURL = publicLeadCaptureFormChallengeURL(form?.publicId || '')
  const expectedRevision = Number.isInteger(Number(form?.revision)) ? Number(form.revision) : 0
  const successMessage = String(form?.successMessage || 'Thanks. We will be in touch soon.')
  const controls = (form?.fields?.length ? form.fields : fallbackFields).map(fieldControl)

  return [
    `<form method="post" action="${escapeEmbedHTML(action)}" data-open-crm-lead-form>`,
    ...controls,
    '  <input type="hidden" name="sourceUrl">',
    `  <input type="hidden" name="leadSource" value="${escapeEmbedHTML(form?.sourceLabel || 'Lead capture form')}">`,
    ...attributionFields.map((field) => `  <input type="hidden" name="${field}">`),
    `  <label><input type="checkbox" name="consentGranted" value="true" required> <span data-open-crm-consent>${escapeEmbedHTML(form?.consentText || 'I agree to be contacted about this request.')}</span></label>`,
    '  <input type="hidden" name="challengeToken">',
    '  <p data-open-crm-status role="status" aria-live="polite">Preparing form...</p>',
    '  <button type="submit" disabled>Submit</button>',
    '</form>',
    `<script>(${String(leadFormEmbedRuntime)})(${scriptLiteral(challengeURL)},${expectedRevision},${scriptLiteral(successMessage)},${JSON.stringify(attributionFields)})</script>`
  ].join('\n')
}
