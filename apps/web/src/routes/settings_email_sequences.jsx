import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { createEmailSequence, deleteEmailSequence, listEmailSequences, updateEmailSequence } from '../lib/email_sequences'
import { usePageTitle } from '../lib/use_page_title'

const emptyStep = { delayDays: 0, subject: '', body: '' }
const emptyForm = { name: '', description: '', status: 'draft', steps: [emptyStep] }

function sequenceStepCount(sequence) {
  const count = Array.isArray(sequence?.steps) ? sequence.steps.length : 0
  return `${count} ${count === 1 ? 'step' : 'steps'}`
}

function formFromSequence(sequence) {
  const steps = Array.isArray(sequence.steps) && sequence.steps.length > 0
    ? sequence.steps.map((step) => ({ delayDays: step.delayDays || 0, subject: step.subject || '', body: step.body || '' }))
    : [emptyStep]
  return {
    name: sequence.name || '',
    description: sequence.description || '',
    status: sequence.status || 'draft',
    steps
  }
}

function payloadFromForm(form) {
  return {
    ...form,
    steps: form.steps.map((step) => ({
      delayDays: Number.parseInt(String(step.delayDays || 0), 10) || 0,
      subject: step.subject,
      body: step.body
    }))
  }
}

export function SettingsEmailSequencesRoute() {
  const { session } = useAuth()
  usePageTitle('Email Sequences')
  const role = session?.membership?.role || ''
  const canManage = role !== 'viewer'
  const [sequences, setSequences] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)

  async function loadSequences({ signal } = {}) {
    setIsLoading(true)
    try {
      const next = await listEmailSequences({ signal })
      setSequences(next)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load email sequences.')
      }
    } finally {
      if (!signal?.aborted) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadSequences({ signal: controller.signal })
    return () => {
      controller.abort()
    }
  }, [])

  function resetForm() {
    setForm({ name: '', description: '', status: 'draft', steps: [{ ...emptyStep }] })
    setEditingId(null)
  }

  function startEdit(sequence) {
    setEditingId(sequence.id)
    setForm(formFromSequence(sequence))
  }

  function updateStep(index, patch) {
    setForm((current) => ({
      ...current,
      steps: current.steps.map((step, stepIndex) => (stepIndex === index ? { ...step, ...patch } : step))
    }))
  }

  function addStep() {
    setForm((current) => ({ ...current, steps: [...current.steps, { ...emptyStep }] }))
  }

  function removeStep(index) {
    setForm((current) => ({
      ...current,
      steps: current.steps.length === 1 ? current.steps : current.steps.filter((_, stepIndex) => stepIndex !== index)
    }))
  }

  async function handleSubmit(event) {
    event.preventDefault()
    setIsSaving(true)
    try {
      const payload = payloadFromForm(form)
      if (editingId) {
        const updated = await updateEmailSequence(editingId, payload)
        setSequences((current) => current.map((sequence) => (sequence.id === editingId ? updated : sequence)))
      } else {
        const created = await createEmailSequence(payload)
        setSequences((current) => [...current, created])
      }
      resetForm()
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to save email sequence.')
    } finally {
      setIsSaving(false)
    }
  }

  async function handleDelete(sequenceId) {
    try {
      await deleteEmailSequence(sequenceId)
      setSequences((current) => current.filter((sequence) => sequence.id !== sequenceId))
      if (editingId === sequenceId) {
        resetForm()
      }
      setError('')
    } catch (deleteError) {
      setError(deleteError.message || 'Unable to delete email sequence.')
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Email sequences</h2>
              <p>Reusable cadence definitions for {session?.organization?.name || 'your team'}. Enrollment and automated sending will be added after this foundation.</p>
            </div>
          </div>
          {isLoading ? <p className="field-hint">Loading sequences...</p> : null}
          {error ? <InlineError message={error} /> : null}
          <div className="record-list" role="list" aria-label="Email sequences">
            {!isLoading && sequences.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No email sequences yet.</p>
                  <p className="field-hint">Create a draft cadence before adding enrollment and scheduling.</p>
                </div>
              </article>
            ) : sequences.map((sequence) => (
              <article className="record-row" key={sequence.id} role="listitem">
                <div>
                  <h3>{sequence.name}</h3>
                  <p className="field-hint">{sequence.status || 'draft'} · {sequenceStepCount(sequence)}</p>
                  {sequence.description ? <p className="field-hint">{sequence.description}</p> : null}
                </div>
                {canManage ? (
                  <div>
                    <Button className="button-secondary" type="button" onClick={() => startEdit(sequence)}>Edit</Button>
                    <Button className="button-secondary" type="button" onClick={() => handleDelete(sequence.id)}>Delete</Button>
                  </div>
                ) : null}
              </article>
            ))}
          </div>
        </div>
      </Card>

      {canManage ? (
        <Card>
          <form className="auth-form card-stack" onSubmit={handleSubmit}>
            <div>
              <h2>{editingId ? 'Edit sequence' : 'New sequence'}</h2>
              <p className="field-hint">Status is metadata until enrollment, scheduler, and reply detection ship.</p>
            </div>
            <Field label="Sequence name">
              <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} required />
            </Field>
            <Field label="Description">
              <textarea className="text-input" rows={3} value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} />
            </Field>
            <Field label="Status">
              <select className="text-input" value={form.status} onChange={(event) => setForm({ ...form, status: event.target.value })}>
                <option value="draft">Draft</option>
                <option value="active">Active</option>
                <option value="paused">Paused</option>
              </select>
            </Field>
            <div className="card-stack">
              <div className="section-header">
                <div>
                  <h3>Steps</h3>
                  <p className="field-hint">Each step stores the future send delay and email content.</p>
                </div>
                <Button className="button-secondary" type="button" onClick={addStep}>Add step</Button>
              </div>
              {form.steps.map((step, index) => (
                <Card key={index}>
                  <div className="card-stack">
                    <div className="section-header">
                      <h3>Step {index + 1}</h3>
                      <Button className="button-secondary" type="button" onClick={() => removeStep(index)} disabled={form.steps.length === 1}>Remove</Button>
                    </div>
                    <Field label={`Step ${index + 1} delay days`}>
                      <input className="text-input" min="0" type="number" value={step.delayDays} onChange={(event) => updateStep(index, { delayDays: event.target.value })} required />
                    </Field>
                    <Field label={`Step ${index + 1} subject`}>
                      <input className="text-input" value={step.subject} onChange={(event) => updateStep(index, { subject: event.target.value })} required />
                    </Field>
                    <Field label={`Step ${index + 1} body`}>
                      <textarea className="text-input" rows={5} value={step.body} onChange={(event) => updateStep(index, { body: event.target.value })} required />
                    </Field>
                  </div>
                </Card>
              ))}
            </div>
            <div>
              <Button type="submit" disabled={isSaving}>{isSaving ? 'Saving...' : editingId ? 'Save changes' : 'Create sequence'}</Button>
              {editingId ? <Button className="button-secondary" type="button" onClick={resetForm}>Cancel</Button> : null}
            </div>
          </form>
        </Card>
      ) : null}
    </section>
  )
}
