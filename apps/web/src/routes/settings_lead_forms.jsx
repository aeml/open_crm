import { useEffect, useRef, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { listCustomFields } from '../lib/custom_fields'
import { createLeadCaptureForm, listLeadCaptureFormPage, listLeadCaptureForms, publicLeadCaptureFormChallengeURL, publicLeadCaptureFormSubmitURL, updateLeadCaptureForm } from '../lib/lead_forms'
import { usePageTitle } from '../lib/use_page_title'
import { LeadSubmissionReview } from './lead_submission_review'

const defaultFields = [
  { key: 'firstName', label: 'First name', fieldType: 'text', required: true, mapTo: 'firstName' },
  { key: 'lastName', label: 'Last name', fieldType: 'text', required: true, mapTo: 'lastName' },
  { key: 'email', label: 'Email', fieldType: 'email', required: true, mapTo: 'email' },
  { key: 'phone', label: 'Phone', fieldType: 'tel', required: false, mapTo: 'phone' },
  { key: 'message', label: 'How can we help?', fieldType: 'textarea', required: false, mapTo: '' }
]

function fieldTypeForCustomField(definition, requestedType = '') {
  if (definition?.dataType === 'text') return requestedType === 'textarea' ? 'textarea' : 'text'
  if (definition?.dataType === 'boolean') return 'boolean'
  return definition?.dataType || 'text'
}

function customFieldFormField(definition) {
  return {
    key: `custom_${definition.fieldKey}`,
    label: definition.label,
    fieldType: fieldTypeForCustomField(definition),
    required: definition.required === true,
    mapTo: `custom:${definition.fieldKey}`,
    options: definition.options || []
  }
}

function withRequiredCustomFields(fields, customFields) {
  const mappings = new Set((fields || []).map((field) => field.mapTo))
  return [
    ...(fields || []).map((field) => ({ ...field, options: field.options || [] })),
    ...(customFields || [])
      .filter((definition) => definition.required && !mappings.has(`custom:${definition.fieldKey}`))
      .map(customFieldFormField)
  ]
}

function emptyForm(customFields = []) {
  return {
    name: '',
    slug: '',
    title: '',
    description: '',
    successMessage: 'Thanks. We will be in touch soon.',
    sourceLabel: 'Lead capture form',
    consentText: 'I agree to be contacted about this request.',
    isActive: true,
    revision: 0,
    fields: withRequiredCustomFields(defaultFields, customFields)
  }
}

function formFromLeadForm(form, customFields = []) {
  return {
    name: form.name || '',
    slug: form.slug || '',
    title: form.title || form.name || '',
    description: form.description || '',
    successMessage: form.successMessage || 'Thanks. We will be in touch soon.',
    sourceLabel: form.sourceLabel || 'Lead capture form',
    consentText: form.consentText || 'I agree to be contacted about this request.',
    isActive: form.isActive !== false,
    revision: Number(form.revision || 0),
    fields: withRequiredCustomFields(form.fields && form.fields.length > 0 ? form.fields : defaultFields, customFields)
  }
}

function leadFormPayload(form) {
  return {
    name: form.name,
    slug: form.slug,
    title: form.title,
    description: form.description,
    successMessage: form.successMessage,
    sourceLabel: form.sourceLabel,
    consentText: form.consentText,
    isActive: form.isActive,
    revision: form.revision,
    fields: form.fields
  }
}

function fieldInputType(field) {
  if (field.fieldType === 'email') return 'email'
  if (field.fieldType === 'tel') return 'tel'
  if (field.fieldType === 'hidden') return 'hidden'
	if (field.fieldType === 'number') return 'number'
	if (field.fieldType === 'date') return 'date'
  return 'text'
}

function escapeHTML(value) {
  return String(value || '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
}

function embedSnippet(form) {
  const action = publicLeadCaptureFormSubmitURL(form.publicId || '')
  const challengeURL = publicLeadCaptureFormChallengeURL(form.publicId || '')
  const controls = (form.fields || defaultFields).map((field) => {
    if (field.fieldType === 'textarea') {
      return `  <label>${escapeHTML(field.label)}\n    <textarea name="${escapeHTML(field.key)}"${field.required ? ' required' : ''}></textarea>\n  </label>`
    }
	if (field.fieldType === 'select') {
	  const options = (field.options || []).map((option) => `      <option value="${escapeHTML(option)}">${escapeHTML(option)}</option>`)
	  return [`  <label>${escapeHTML(field.label)}`, `    <select name="${escapeHTML(field.key)}"${field.required ? ' required' : ''}>`, '      <option value="">Select...</option>', ...options, '    </select>', '  </label>'].join('\n')
	}
	if (field.fieldType === 'boolean' || field.fieldType === 'checkbox') {
	  return `  <label>${escapeHTML(field.label)}\n    <select name="${escapeHTML(field.key)}"${field.required ? ' required' : ''}><option value="">Select...</option><option value="true">Yes</option><option value="false">No</option></select>\n  </label>`
	}
    return `  <label>${escapeHTML(field.label)}\n    <input name="${escapeHTML(field.key)}" type="${fieldInputType(field)}"${field.required ? ' required' : ''}>\n  </label>`
  })

  return [
    `<form method="post" action="${action}">`,
    ...controls,
    '  <input type="hidden" name="sourceUrl" value="https://example.com/contact?utm_source=google&utm_medium=cpc&utm_campaign=spring-demo">',
    `  <input type="hidden" name="leadSource" value="${form.sourceLabel || 'Lead capture form'}">`,
    '  <input type="hidden" name="utm_source" value="">',
    '  <input type="hidden" name="utm_medium" value="">',
    '  <input type="hidden" name="utm_campaign" value="">',
    '  <input type="hidden" name="utm_term" value="">',
    '  <input type="hidden" name="utm_content" value="">',
    `  <label><input type="checkbox" name="consentGranted" value="true" required> <span data-open-crm-consent>${escapeHTML(form.consentText || 'I agree to be contacted about this request.')}</span></label>`,
    '  <input type="hidden" name="challengeToken">',
    '  <p data-open-crm-status role="status">Preparing secure form...</p>',
    '  <button type="submit" disabled>Submit</button>',
    '</form>',
    '<script>',
    '(() => {',
    '  const form = document.currentScript.previousElementSibling',
    '  const button = form.querySelector(\'button[type="submit"]\')',
    '  const status = form.querySelector(\'[data-open-crm-status]\')',
    `  fetch(${JSON.stringify(challengeURL)}, { method: 'POST', headers: { Accept: 'application/json' } })`,
    '    .then((response) => response.ok ? response.json() : Promise.reject(new Error(\'challenge failed\')))',
    '    .then((payload) => {',
    '      const challenge = payload.data.challenge',
	`      if (Number(challenge.formRevision) !== ${Number(form.revision || 0)}) throw new Error('form changed')`,
    '      form.elements.challengeToken.value = challenge.token',
    '      form.querySelector(\'[data-open-crm-consent]\').textContent = challenge.consentText',
    '      const delay = Math.max(0, Date.parse(challenge.notBefore) - Date.now())',
    '      setTimeout(() => { button.disabled = false; status.textContent = \'\' }, delay)',
    '    })',
    '    .catch(() => { status.textContent = \'This form is temporarily unavailable.\' })',
    '})()',
    '</script>'
  ].join('\n')
}

function mappedFieldLabel(field) {
	return field.mapTo?.startsWith('custom:') ? `Maps to contact custom field ${field.mapTo.slice(7)}` : field.mapTo ? `Maps to contact ${field.mapTo}` : 'Stored only on the lead form submission'
}

const coreDestinations = [
  ['', 'Submission only'], ['firstName', 'Contact first name'], ['lastName', 'Contact last name'],
  ['email', 'Contact email'], ['phone', 'Contact phone'], ['addressLine1', 'Contact address line 1'],
  ['addressLine2', 'Contact address line 2'], ['city', 'Contact city'], ['state', 'Contact state'],
  ['postalCode', 'Contact postal code'], ['country', 'Contact country'], ['jobTitle', 'Contact job title']
]

const leadFormPageSize = 50
const emptyLeadFormMeta = { page: 1, pageSize: leadFormPageSize, total: 0 }

function mappedFieldType(mapping, customFields, currentType) {
  const definition = customFields.find((field) => `custom:${field.fieldKey}` === mapping)
  if (definition) return fieldTypeForCustomField(definition, currentType)
  if (mapping === 'email') return 'email'
  if (mapping === 'phone') return 'tel'
  return currentType === 'textarea' && mapping === '' ? 'textarea' : 'text'
}

export function SettingsLeadFormsRoute() {
  const { session, canAdminister: canManage } = useAuth()
  usePageTitle('Lead Forms')
  const [forms, setForms] = useState([])
	const [reviewForms, setReviewForms] = useState([])
	const [formMeta, setFormMeta] = useState(emptyLeadFormMeta)
	const [customFields, setCustomFields] = useState([])
  const [form, setForm] = useState(() => emptyForm())
  const [editingId, setEditingId] = useState(null)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
	const [statusFilter, setStatusFilter] = useState('all')
	const [pageNumber, setPageNumber] = useState(1)
	const latestLoad = useRef(0)
	const dependenciesLoaded = useRef(false)
	const operationPending = useRef(false)

  async function loadForms({ signal, requestedPage = pageNumber, formStatus = statusFilter, refreshDependencies = false } = {}) {
	const loadID = latestLoad.current + 1
	latestLoad.current = loadID
    setIsLoading(true)
    try {
	  const loadDependencies = refreshDependencies || !dependenciesLoaded.current
	  const [catalog, allForms, nextCustomFields] = await Promise.all([
		listLeadCaptureFormPage({ status: formStatus, page: requestedPage, pageSize: leadFormPageSize, signal }),
		loadDependencies ? listLeadCaptureForms({ signal }) : Promise.resolve(null),
		loadDependencies ? listCustomFields('contact', { signal }) : Promise.resolve(null)
	  ])
	  if (signal?.aborted || loadID !== latestLoad.current) return null
	  setForms(catalog.forms)
	  setFormMeta(catalog.meta)
	  if (loadDependencies) {
		setReviewForms(allForms)
		setCustomFields(nextCustomFields)
		setForm((current) => current.name || editingId
		  ? { ...current, fields: withRequiredCustomFields(current.fields, nextCustomFields) }
		  : emptyForm(nextCustomFields))
		dependenciesLoaded.current = true
	  }
      setError('')
	  return catalog
    } catch (loadError) {
	  if (!isAbortError(loadError) && loadID === latestLoad.current) {
        setError(loadError.message || 'Unable to load lead forms.')
      }
    } finally {
	  if (!signal?.aborted && loadID === latestLoad.current) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadForms({ signal: controller.signal })
    return () => {
      controller.abort()
    }
  }, [pageNumber, statusFilter])

  function resetForm() {
    setEditingId(null)
	setForm(emptyForm(customFields))
  }

  function startEdit(nextForm) {
    setEditingId(nextForm.id)
	setForm(formFromLeadForm(nextForm, customFields))
    setStatus('')
  }

  function updateField(index, patch) {
    setForm((current) => ({
      ...current,
      fields: current.fields.map((field, fieldIndex) => (fieldIndex === index ? { ...field, ...patch } : field))
    }))
  }

  function updateFieldMapping(index, mapTo) {
	const definition = customFields.find((field) => `custom:${field.fieldKey}` === mapTo)
	updateField(index, {
	  mapTo,
	  fieldType: mappedFieldType(mapTo, customFields, form.fields[index].fieldType),
	  required: definition?.required || mapTo === 'firstName' || mapTo === 'lastName' ? true : form.fields[index].required,
	  options: definition?.options || []
	})
  }

  function addField() {
	setForm((current) => {
	  const keys = new Set(current.fields.map((field) => field.key))
	  let suffix = current.fields.length + 1
	  while (keys.has(`field${suffix}`)) suffix += 1
	  return { ...current, fields: [...current.fields, { key: `field${suffix}`, label: 'New field', fieldType: 'text', required: false, mapTo: '', options: [] }] }
	})
  }

  function removeField(index) {
	setForm((current) => ({ ...current, fields: current.fields.filter((_, fieldIndex) => fieldIndex !== index) }))
  }

  async function handleSubmit(event) {
    event.preventDefault()
	if (!canManage || operationPending.current) return

	operationPending.current = true
    setIsSaving(true)
    setStatus('')
    try {
      const payload = leadFormPayload(form)
      if (editingId) {
		await updateLeadCaptureForm(editingId, payload)
        setStatus('Lead form updated.')
      } else {
		await createLeadCaptureForm(payload)
        setStatus('Lead form created.')
      }
      resetForm()
      setError('')
	  dependenciesLoaded.current = false
	  if (pageNumber === 1) await loadForms({ requestedPage: 1, refreshDependencies: true })
	  else setPageNumber(1)
    } catch (saveError) {
      setError(saveError.message || 'Unable to save lead form.')
    } finally {
      setIsSaving(false)
	  operationPending.current = false
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Lead forms</h2>
              <p>Create embeddable forms that capture website inquiries as CRM lead contacts for {session?.organization?.name || 'your team'}.</p>
            </div>
          </div>
          {isLoading ? <p className="field-hint">Loading lead forms...</p> : null}
          {status ? <p className="field-hint" role="status">{status}</p> : null}
          {error ? <InlineError message={error} onRetry={() => loadForms()} retryLabel="Retry lead forms" /> : null}
		  <Field label="Lead form status">
			<select className="text-input" value={statusFilter} disabled={isLoading || isSaving} onChange={(event) => { setPageNumber(1); setStatusFilter(event.target.value) }}>
			  <option value="all">Active and inactive</option>
			  <option value="active">Active</option>
			  <option value="inactive">Inactive</option>
			</select>
		  </Field>
          <div className="record-list" role="list" aria-label="Lead forms">
            {!isLoading && forms.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No lead forms yet.</p>
                  <p className="field-hint">Create a website form to start converting inbound demand into lead contacts.</p>
                </div>
              </article>
            ) : forms.map((item) => (
              <article className={item.isActive ? 'record-row' : 'record-row record-row-alert'} key={item.id} role="listitem">
                <div>
                  <h3>{item.name}</h3>
                  <p className="field-hint">/{item.slug} · {item.submissionCount || 0} submissions · public id {item.publicId}</p>
                  {item.description ? <p className="field-hint">{item.description}</p> : null}
                  {item.publicId ? <textarea className="text-input" readOnly rows={8} aria-label={`Embed code for ${item.name}`} value={embedSnippet(item)} /> : null}
                </div>
                <div>
                  <span className="chip">{item.isActive ? 'Active' : 'Inactive'}</span>
                  {canManage ? <Button className="button-secondary" type="button" onClick={() => startEdit(item)}>Edit</Button> : null}
                </div>
              </article>
            ))}
          </div>
		  <p className="field-hint" role="status">Showing {forms.length} of {formMeta.total} lead forms.</p>
		  <div className="button-row">
			<Button className="button-secondary" type="button" disabled={isLoading || isSaving || pageNumber <= 1} onClick={() => setPageNumber((current) => current - 1)}>Previous form page</Button>
			<Button className="button-secondary" type="button" disabled={isLoading || isSaving || pageNumber * formMeta.pageSize >= formMeta.total} onClick={() => setPageNumber((current) => current + 1)}>Next form page</Button>
		  </div>
        </div>
      </Card>

	  {canManage ? <Card><LeadSubmissionReview forms={reviewForms} /></Card> : null}

      {canManage ? (
        <Card>
          <form className="auth-form card-stack" onSubmit={handleSubmit}>
            <div>
              <h2>{editingId ? 'Edit lead form' : 'New lead form'}</h2>
			  <p className="field-hint">Map each visitor field to a standard or organization-defined contact field. Submission-only values remain available in lead review without changing the contact.</p>
            </div>
            <Field label="Name">
              <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="Website contact form" required />
            </Field>
            <Field label="Slug" hint="Optional. Used for admin readability; the public embed uses a generated public id.">
              <input className="text-input" value={form.slug} onChange={(event) => setForm({ ...form, slug: event.target.value })} placeholder="website-contact" />
            </Field>
            <Field label="Title">
              <input className="text-input" value={form.title} onChange={(event) => setForm({ ...form, title: event.target.value })} placeholder="Talk to our team" />
            </Field>
            <Field label="Description">
              <textarea className="text-input" rows={3} value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} placeholder="Tell visitors what happens after they submit." />
            </Field>
            <Field label="Success message">
              <input className="text-input" value={form.successMessage} onChange={(event) => setForm({ ...form, successMessage: event.target.value })} required />
            </Field>
            <Field label="Source label">
              <input className="text-input" value={form.sourceLabel} onChange={(event) => setForm({ ...form, sourceLabel: event.target.value })} required />
            </Field>
            <Field label="Consent statement" hint="Visitors must actively confirm this statement. The exact text is retained with each accepted submission.">
              <textarea className="text-input" rows={3} maxLength={1000} value={form.consentText} onChange={(event) => setForm({ ...form, consentText: event.target.value })} required />
            </Field>
            <label className="field-hint">
              <input type="checkbox" checked={form.isActive} onChange={(event) => setForm({ ...form, isActive: event.target.checked })} /> Active lead form
            </label>
            <div className="record-list" role="list" aria-label="Mapped lead form fields">
              {form.fields.map((field, index) => (
                <article className="record-row" key={field.key} role="listitem">
                  <div>
                    <Field label={`${field.key} label`} hint={mappedFieldLabel(field)}>
                      <input className="text-input" value={field.label} onChange={(event) => updateField(index, { label: event.target.value })} required />
                    </Field>
					<Field label={`${field.key} destination`}>
					  <select className="text-input" value={field.mapTo || ''} onChange={(event) => updateFieldMapping(index, event.target.value)}>
						{coreDestinations.map(([value, label]) => <option key={value || 'submission'} value={value}>{label}</option>)}
						{customFields.map((definition) => <option key={definition.id} value={`custom:${definition.fieldKey}`}>{definition.label} ({definition.dataType})</option>)}
						{field.mapTo?.startsWith('custom:') && !customFields.some((definition) => `custom:${definition.fieldKey}` === field.mapTo) ? <option value={field.mapTo}>Unavailable custom field ({field.mapTo.slice(7)})</option> : null}
					  </select>
					</Field>
					{!field.mapTo || !field.mapTo.startsWith('custom:') ? (
					  <Field label={`${field.key} control`}>
						<select className="text-input" value={field.fieldType} onChange={(event) => updateField(index, { fieldType: event.target.value, options: [] })}>
						  <option value="text">Single-line text</option>
						  <option value="textarea">Multi-line text</option>
						  <option value="email">Email</option>
						  <option value="tel">Telephone</option>
						  <option value="hidden">Hidden</option>
						</select>
					  </Field>
					) : null}
                  </div>
                  <div>
                    <label className="field-hint">
					  <input type="checkbox" checked={field.required} disabled={field.mapTo === 'firstName' || field.mapTo === 'lastName' || customFields.some((definition) => definition.required && `custom:${definition.fieldKey}` === field.mapTo)} onChange={(event) => updateField(index, { required: event.target.checked })} /> Required
                    </label>
					<Button className="button-secondary" type="button" onClick={() => removeField(index)} disabled={form.fields.length <= 1}>Remove</Button>
                  </div>
                </article>
              ))}
            </div>
			<Button className="button-secondary" type="button" onClick={addField} disabled={form.fields.length >= 25}>Add field</Button>
            <div>
              <Button type="submit" disabled={isSaving}>{isSaving ? 'Saving...' : editingId ? 'Save lead form' : 'Create lead form'}</Button>
              {editingId ? <Button className="button-secondary" type="button" onClick={resetForm}>Cancel</Button> : null}
            </div>
          </form>
        </Card>
      ) : null}
    </section>
  )
}
