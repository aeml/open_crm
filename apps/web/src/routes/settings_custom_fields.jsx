import { useEffect, useMemo, useState } from 'react'
import { useAuth } from '../app/providers'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { archiveCustomField, createCustomField, listCustomFields, updateCustomField } from '../lib/custom_fields'
import { usePageTitle } from '../lib/use_page_title'

const emptyForm = { label: '', dataType: 'text', optionsText: '', required: false, showInList: false, position: 0 }

function optionsFromText(value) {
  return String(value || '').split(',').map((option) => option.trim()).filter(Boolean)
}

function editorValues(definition) {
  return { label: definition.label, optionsText: (definition.options || []).join(', '), required: definition.required, showInList: definition.showInList, position: definition.position }
}

export function SettingsCustomFieldsRoute() {
  const { session } = useAuth()
  usePageTitle('Custom fields')
  const canManage = useMemo(() => ['owner', 'admin'].includes(session?.membership?.role || ''), [session])
  const [entityType, setEntityType] = useState('contact')
  const [definitions, setDefinitions] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [editing, setEditing] = useState({})
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [isSaving, setIsSaving] = useState(false)

  async function load({ signal, type = entityType } = {}) {
    if (!canManage) return
    setIsLoading(true)
    try {
      const next = await listCustomFields(type, { signal })
      setDefinitions(next)
      setEditing(Object.fromEntries(next.map((definition) => [definition.id, editorValues(definition)])))
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) setError(loadError.message || 'Unable to load custom fields.')
    } finally {
      setIsLoading(false)
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    load({ signal: controller.signal })
    return () => controller.abort()
  }, [canManage, entityType])

  async function handleCreate(event) {
    event.preventDefault()
    setIsSaving(true)
    setError('')
    try {
      const created = await createCustomField({ entityType, label: form.label, dataType: form.dataType, options: form.dataType === 'select' ? optionsFromText(form.optionsText) : [], required: form.required, showInList: form.showInList, position: Number(form.position) || 0 })
      setNotice(`${created.label} created with stable key custom:${created.fieldKey}.`)
      setForm(emptyForm)
      await load()
    } catch (saveError) {
      setError(saveError.message || 'Unable to create custom field.')
    } finally {
      setIsSaving(false)
    }
  }

  async function handleUpdate(definition) {
    const value = editing[definition.id]
    setIsSaving(true)
    setError('')
    try {
      await updateCustomField(definition.id, { label: value.label, options: definition.dataType === 'select' ? optionsFromText(value.optionsText) : [], required: value.required, showInList: value.showInList, position: Number(value.position) || 0 })
      setNotice(`${value.label} updated.`)
      await load()
    } catch (saveError) {
      setError(saveError.message || 'Unable to update custom field.')
    } finally {
      setIsSaving(false)
    }
  }

  async function handleArchive(definition) {
    if (!window.confirm(`Archive “${definition.label}”? Existing values remain stored, but the field will leave normal forms, lists, filters, imports, and exports.`)) return
    setIsSaving(true)
    setError('')
    try {
      await archiveCustomField(definition.id)
      setNotice(`${definition.label} archived. Existing record values were retained.`)
      await load()
    } catch (saveError) {
      setError(saveError.message || 'Unable to archive custom field.')
    } finally {
      setIsSaving(false)
    }
  }

  if (!canManage) return <section className="dashboard-grid settings-grid"><Card><InlineError message="Admin access required" /></Card></section>

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <form className="card-stack" onSubmit={handleCreate}>
          <div><h2>Custom fields</h2><p>Add up to 25 lightweight fields per record type. Core CRM fields remain unchanged.</p></div>
          <Field label="Record type"><select className="text-input" value={entityType} onChange={(event) => { setEntityType(event.target.value); setForm(emptyForm); setNotice('') }}><option value="contact">Contacts</option><option value="company">Organization clients</option></select></Field>
          <Field label="Label"><input className="text-input" value={form.label} onChange={(event) => setForm((current) => ({ ...current, label: event.target.value }))} maxLength="100" required /></Field>
          <Field label="Type"><select className="text-input" value={form.dataType} onChange={(event) => setForm((current) => ({ ...current, dataType: event.target.value, optionsText: '' }))}><option value="text">Text</option><option value="number">Number</option><option value="date">Date</option><option value="boolean">Yes / no</option><option value="select">Single select</option></select></Field>
          {form.dataType === 'select' ? <Field label="Options" hint="Comma-separated; 1–25 options."><input className="text-input" value={form.optionsText} onChange={(event) => setForm((current) => ({ ...current, optionsText: event.target.value }))} required /></Field> : null}
          <Field label="Position"><input className="text-input" type="number" min="0" max="1000" value={form.position} onChange={(event) => setForm((current) => ({ ...current, position: event.target.value }))} /></Field>
          <label><input type="checkbox" checked={form.required} onChange={(event) => setForm((current) => ({ ...current, required: event.target.checked }))} /> Required when a record is created or edited</label>
          <label><input type="checkbox" checked={form.showInList} onChange={(event) => setForm((current) => ({ ...current, showInList: event.target.checked }))} /> Show in record lists</label>
          <Button type="submit" disabled={isSaving || definitions.length >= 25}>{isSaving ? 'Saving…' : 'Create field'}</Button>
          {definitions.length >= 25 ? <p className="field-hint">The 25-field limit is reached for this record type.</p> : null}
          {error ? <InlineError message={error} onRetry={() => load()} /> : null}
          {notice ? <div className="inline-note" role="status">{notice}</div> : null}
        </form>
      </Card>
      <Card>
        <div className="card-stack">
          <div><h2>Active definitions</h2><p>Keys and types are immutable so imports, exports, filters, and stored values remain stable.</p></div>
          {!isLoading && definitions.length === 0 ? <p className="empty-state">No custom fields for this record type.</p> : null}
          <div className="record-list" role="list" aria-label="Custom field definitions">
            {definitions.map((definition) => {
              const value = editing[definition.id] || editorValues(definition)
              const setValue = (patch) => setEditing((current) => ({ ...current, [definition.id]: { ...value, ...patch } }))
              return (
                <article className="record-row duplicate-candidate-row" role="listitem" key={definition.id}>
                  <div className="card-stack">
                    <div><strong>{definition.label}</strong><p className="field-hint">custom:{definition.fieldKey} · {definition.dataType}</p></div>
                    <Field label={`Label for ${definition.fieldKey}`}><input className="text-input" value={value.label} onChange={(event) => setValue({ label: event.target.value })} /></Field>
                    {definition.dataType === 'select' ? <Field label={`Options for ${definition.label}`}><input className="text-input" value={value.optionsText} onChange={(event) => setValue({ optionsText: event.target.value })} /></Field> : null}
                    <Field label={`Position for ${definition.label}`}><input className="text-input" type="number" min="0" max="1000" value={value.position} onChange={(event) => setValue({ position: event.target.value })} /></Field>
                    <label><input type="checkbox" checked={value.required} onChange={(event) => setValue({ required: event.target.checked })} /> Required</label>
                    <label><input type="checkbox" checked={value.showInList} onChange={(event) => setValue({ showInList: event.target.checked })} /> Show in lists</label>
                    <div className="button-row"><Button type="button" onClick={() => handleUpdate(definition)} disabled={isSaving}>Save changes</Button><Button className="button-danger" type="button" onClick={() => handleArchive(definition)} disabled={isSaving}>Archive field</Button></div>
                  </div>
                </article>
              )
            })}
          </div>
        </div>
      </Card>
    </section>
  )
}
