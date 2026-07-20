import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { createEmailSequence, deleteEmailSequence, listEmailSequences, transitionEmailSequence, updateEmailSequence } from '../lib/email_sequences'
import { usePageTitle } from '../lib/use_page_title'

const emptyStep = { delayDays: 0, subject: '', body: '' }
const emptyForm = { name: '', description: '', steps: [emptyStep] }

function formFromSequence(sequence) {
  const steps = sequence.steps.length
    ? sequence.steps.map((step) => ({ delayDays: step.delayDays, subject: step.subject, body: step.body }))
    : [emptyStep]
  return {
    name: sequence.name,
    description: sequence.description,
    steps
  }
}

function payloadFromForm(form) {
  return {
    ...form,
    status: 'draft',
    steps: form.steps.map((step) => ({
      delayDays: +step.delayDays || 0,
      subject: step.subject,
      body: step.body
    }))
  }
}

export function SettingsEmailSequencesRoute() {
  const { canWrite: canManage, canAdminister } = useAuth()
  usePageTitle('Email Sequences')
  const [sequences, setSequences] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)

  async function loadSequences(signal) {
    try {
      const next = await listEmailSequences({ signal })
      setSequences(next)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load email sequences.')
      }
    } finally {
      if (!signal.aborted) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadSequences(controller.signal)
    return () => {
      controller.abort()
    }
  }, [])

  function resetForm() {
    setForm(emptyForm)
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
    setForm((current) => ({ ...current, steps: [...current.steps, emptyStep] }))
  }

  function removeStep(index) {
    setForm((current) => ({
      ...current,
      steps: current.steps.filter((_, stepIndex) => stepIndex !== index)
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

  async function handleTransition(sequenceId, action) {
    try {
      const updated = await transitionEmailSequence(sequenceId, action)
      setSequences((current) => current.map((sequence) => (sequence.id === sequenceId ? updated : sequence)))
      setError('')
    } catch (transitionError) {
      setError(transitionError.message)
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <h2>Email sequences</h2>
          {isLoading ? <p className="field-hint">Loading sequences...</p> : null}
          {error ? <InlineError message={error} /> : null}
          <div className="record-list" role="list" aria-label="Email sequences">
            {!isLoading && sequences.length === 0 ? (
              <article className="record-row" role="listitem">
                <p>No email sequences yet.</p>
              </article>
            ) : sequences.map((sequence) => (
              <article className="record-row" key={sequence.id} role="listitem">
                <div>
                  <h3>{sequence.name}</h3>
                  <p className="field-hint">{sequence.status} · revision {sequence.revision} · {sequence.approvedAt && sequence.approvedRevision === sequence.revision ? 'approved' : 'approval required'} · steps {sequence.steps.length}</p>
                  {sequence.outcomes?.enrolled ? (
                    <p className="field-hint">{sequence.outcomes.enrolled} enrolled · {sequence.outcomes.providerAccepted} accepted · {sequence.outcomes.replied} replied · {sequence.outcomes.cadenceFinished} finished · {sequence.outcomes.suppressedExits} suppressed · {sequence.outcomes.needsReview} review</p>
                  ) : null}
                  {sequence.description ? <p className="field-hint">{sequence.description}</p> : null}
                </div>
                {canManage ? (
                  <div>
                    {sequence.status !== 'active' ? <Button className="button-secondary" type="button" onClick={() => startEdit(sequence)}>Edit</Button> : null}
                    {canAdminister && sequence.status !== 'active' ? (
                      <Button type="button" onClick={() => handleTransition(sequence.id, 'approve')}>
                        {sequence.status === 'paused' ? 'Approve & resume' : 'Approve & activate'}
                      </Button>
                    ) : null}
                    {sequence.status === 'active' ? (
                      <Button className="button-secondary" type="button" onClick={() => handleTransition(sequence.id, 'pause')}>Pause sending</Button>
                    ) : <Button className="button-secondary" type="button" onClick={() => handleDelete(sequence.id)}>Delete</Button>}
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
              <p className="field-hint">Saves as a draft for admin approval.</p>
            </div>
            <Field label="Sequence name">
              <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} required />
            </Field>
            <Field label="Description">
              <textarea className="text-input" rows={3} value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} />
            </Field>
            <div className="card-stack">
              <div className="section-header">
                <div>
                  <h3>Steps</h3>
                  <p className="field-hint">Pause stops new attempts; a claimed send may finish.</p>
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
