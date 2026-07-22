import { useEffect, useRef, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { createEmailSequence, deleteEmailSequence, listEmailSequencePage, transitionEmailSequence, updateEmailSequence } from '../lib/email_sequences'
import { usePageTitle } from '../lib/use_page_title'
import { EmailSequenceEnrollmentHistory } from './email_sequence_enrollment_history'

const emptyStep = { delayDays: 0, subject: '', body: '' }
const emptyForm = { name: '', description: '', steps: [emptyStep], expectedRevision: 0 }
const pageSize = 50
const emptyMeta = { page: 1, pageSize, total: 0 }
const maxSequenceSteps = 20

function formFromSequence(sequence) {
  const steps = sequence.steps.length
    ? sequence.steps.map((step) => ({ delayDays: step.delayDays, subject: step.subject, body: step.body }))
    : [emptyStep]
  return {
    name: sequence.name,
    description: sequence.description,
    steps,
    expectedRevision: sequence.revision
  }
}

function payloadFromForm(form) {
  const payload = {
    name: form.name,
    description: form.description,
    status: 'draft',
    steps: form.steps.map((step) => ({
      delayDays: +step.delayDays || 0,
      subject: step.subject,
      body: step.body
    }))
  }
  if (form.expectedRevision > 0) payload.expectedRevision = form.expectedRevision
  return payload
}

export function SettingsEmailSequencesRoute() {
  const { canWrite: canManage, canAdminister } = useAuth()
  usePageTitle('Email Sequences')
  const [sequences, setSequences] = useState([])
  const [sequenceMeta, setSequenceMeta] = useState(emptyMeta)
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  const [pendingSequenceId, setPendingSequenceId] = useState(null)
  const [statusMessage, setStatusMessage] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [appliedSearch, setAppliedSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [pageNumber, setPageNumber] = useState(1)
  const latestLoad = useRef(0)
  const operationPending = useRef(false)

  async function loadSequences({ signal, requestedPage = pageNumber, search = appliedSearch, sequenceStatus = statusFilter } = {}) {
    const loadId = latestLoad.current + 1
    latestLoad.current = loadId
    setIsLoading(true)
    try {
      const page = await listEmailSequencePage({ search, status: sequenceStatus, page: requestedPage, pageSize, signal })
      if (signal?.aborted || loadId !== latestLoad.current) return null
      setSequences(page.sequences)
      setSequenceMeta(page.meta)
      setError('')
      return page
    } catch (loadError) {
      if (!isAbortError(loadError) && loadId === latestLoad.current) {
        setError(loadError.message || 'Unable to load email sequences.')
      }
    } finally {
      if (!signal?.aborted && loadId === latestLoad.current) {
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
  }, [appliedSearch, pageNumber, statusFilter])

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
    if (form.steps.length >= maxSequenceSteps) return
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
    if (operationPending.current) return
    operationPending.current = true
    setIsSaving(true)
    setStatusMessage('')
    try {
      const payload = payloadFromForm(form)
      const operation = editingId ? 'updated' : 'created'
      if (editingId) {
        await updateEmailSequence(editingId, payload)
      } else {
        await createEmailSequence(payload)
      }
      resetForm()
      setStatusMessage(`Email sequence ${operation}.`)
      setError('')
      if (pageNumber === 1) await loadSequences({ requestedPage: 1 })
      else setPageNumber(1)
    } catch (saveError) {
      setError(saveError.message || 'Unable to save email sequence.')
    } finally {
      setIsSaving(false)
      operationPending.current = false
    }
  }

  async function handleDelete(sequence) {
    if (operationPending.current) return
    operationPending.current = true
    setPendingSequenceId(sequence.id)
    setStatusMessage('')
    try {
      await deleteEmailSequence(sequence.id, sequence.revision)
      if (editingId === sequence.id) {
        resetForm()
      }
      setStatusMessage('Email sequence deleted.')
      setError('')
      const page = await loadSequences()
      if (page && page.sequences.length === 0 && page.meta.total > 0 && pageNumber > 1) setPageNumber((current) => current - 1)
    } catch (deleteError) {
      setError(deleteError.message || 'Unable to delete email sequence.')
    } finally {
      setPendingSequenceId(null)
      operationPending.current = false
    }
  }

  async function handleTransition(sequence, action) {
    if (operationPending.current) return
    operationPending.current = true
    setPendingSequenceId(sequence.id)
    setStatusMessage('')
    try {
      await transitionEmailSequence(sequence.id, action, sequence.revision)
      setStatusMessage(action === 'approve' ? 'Email sequence approved and activated.' : 'Email sequence paused.')
      setError('')
      const page = await loadSequences()
      if (page && page.sequences.length === 0 && page.meta.total > 0 && pageNumber > 1) setPageNumber((current) => current - 1)
    } catch (transitionError) {
      setError(transitionError.message)
    } finally {
      setPendingSequenceId(null)
      operationPending.current = false
    }
  }

  function handleSearch(event) {
    event.preventDefault()
    const nextSearch = searchInput.trim()
    if (nextSearch === appliedSearch && pageNumber === 1) loadSequences({ requestedPage: 1, search: nextSearch })
    else {
      setPageNumber(1)
      setAppliedSearch(nextSearch)
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <h2>Email sequences</h2>
          {isLoading ? <p className="field-hint">Loading sequences...</p> : null}
          {error ? <InlineError message={error} /> : null}
          {statusMessage ? <p className="field-hint" role="status">{statusMessage}</p> : null}
          <form className="filters-grid" onSubmit={handleSearch}>
            <Field label="Search email sequences">
              <input className="text-input" maxLength={100} value={searchInput} disabled={isSaving || pendingSequenceId !== null} onChange={(event) => setSearchInput(event.target.value)} placeholder="Sequence name" />
            </Field>
            <Field label="Email sequence status">
              <select className="text-input" value={statusFilter} disabled={isSaving || pendingSequenceId !== null} onChange={(event) => { setPageNumber(1); setStatusFilter(event.target.value) }}>
                <option value="all">Draft, active, and paused</option>
                <option value="draft">Draft</option>
                <option value="active">Active</option>
                <option value="paused">Paused</option>
              </select>
            </Field>
            <Button className="button-secondary" type="submit" disabled={isLoading || isSaving || pendingSequenceId !== null}>Apply search</Button>
          </form>
          <div className="record-list" role="list" aria-label="Email sequences">
            {!isLoading && sequences.length === 0 ? (
              <article className="record-row" role="listitem">
                <p>{appliedSearch || statusFilter !== 'all' ? 'No email sequences match these filters.' : 'No email sequences yet.'}</p>
              </article>
            ) : sequences.map((sequence) => (
              <article className="record-row sequence-row" key={sequence.id} role="listitem">
                <div>
                  <h3>{sequence.name}</h3>
                  <p className="field-hint">{sequence.status} · revision {sequence.revision} · {sequence.approvedAt && sequence.approvedRevision === sequence.revision ? 'approved' : 'approval required'} · steps {sequence.steps.length}</p>
                  {sequence.outcomes?.enrolled ? (
                    <p className="field-hint">{sequence.outcomes.enrolled} enrolled · {sequence.outcomes.providerAccepted} accepted · {sequence.outcomes.bouncedMessages || 0} bounced · {sequence.outcomes.complaints || 0} complaints · {sequence.outcomes.replied} replied · {sequence.outcomes.cadenceFinished} finished · {sequence.outcomes.suppressedExits} suppressed · {sequence.outcomes.needsReview} review</p>
                  ) : null}
                  {sequence.description ? <p className="field-hint">{sequence.description}</p> : null}
                </div>
                {canManage ? (
                  <div>
                    {sequence.status !== 'active' ? <Button className="button-secondary" type="button" disabled={isSaving || pendingSequenceId !== null} onClick={() => startEdit(sequence)}>Edit</Button> : null}
                    {canAdminister && sequence.status !== 'active' ? (
                      <Button type="button" disabled={isSaving || pendingSequenceId !== null} onClick={() => handleTransition(sequence, 'approve')}>
                        {pendingSequenceId === sequence.id ? 'Applying…' : sequence.status === 'paused' ? 'Approve & resume' : 'Approve & activate'}
                      </Button>
                    ) : null}
                    {sequence.status === 'active' ? (
                      <Button className="button-secondary" type="button" disabled={isSaving || pendingSequenceId !== null} onClick={() => handleTransition(sequence, 'pause')}>{pendingSequenceId === sequence.id ? 'Pausing…' : 'Pause sending'}</Button>
                    ) : <Button className="button-secondary" type="button" disabled={isSaving || pendingSequenceId !== null} onClick={() => handleDelete(sequence)}>{pendingSequenceId === sequence.id ? 'Deleting…' : 'Delete'}</Button>}
                  </div>
                ) : null}
                {sequence.outcomes?.enrolled ? <EmailSequenceEnrollmentHistory sequence={sequence} /> : null}
              </article>
            ))}
          </div>
          <p className="field-hint" role="status">Showing {sequences.length} of {sequenceMeta.total} email sequences{appliedSearch ? ` matching “${appliedSearch}”` : ''}. Up to 100 sequences may be active for enrollment and delivery.</p>
          <div className="button-row">
            <Button className="button-secondary" type="button" disabled={isLoading || pageNumber <= 1 || isSaving || pendingSequenceId !== null} onClick={() => setPageNumber((current) => current - 1)}>Previous page</Button>
            <Button className="button-secondary" type="button" disabled={isLoading || pageNumber * sequenceMeta.pageSize >= sequenceMeta.total || isSaving || pendingSequenceId !== null} onClick={() => setPageNumber((current) => current + 1)}>Next page</Button>
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
              <input className="text-input" maxLength={120} value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} required />
            </Field>
            <Field label="Description">
              <textarea className="text-input" maxLength={1000} rows={3} value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} />
            </Field>
            <div className="card-stack">
              <div className="section-header">
                <div>
                  <h3>Steps</h3>
                  <p className="field-hint">Pause stops new attempts; a claimed send may finish.</p>
                </div>
                <Button className="button-secondary" type="button" disabled={form.steps.length >= maxSequenceSteps || isSaving || pendingSequenceId !== null} onClick={addStep}>Add step</Button>
              </div>
              {form.steps.map((step, index) => (
                <Card key={index}>
                  <div className="card-stack">
                    <div className="section-header">
                      <h3>Step {index + 1}</h3>
                      <Button className="button-secondary" type="button" onClick={() => removeStep(index)} disabled={form.steps.length === 1 || isSaving || pendingSequenceId !== null}>Remove</Button>
                    </div>
                    <Field label={`Step ${index + 1} delay days`}>
                      <input className="text-input" min="0" max="365" type="number" value={step.delayDays} onChange={(event) => updateStep(index, { delayDays: event.target.value })} required />
                    </Field>
                    <Field label={`Step ${index + 1} subject`}>
                      <input className="text-input" maxLength={500} value={step.subject} onChange={(event) => updateStep(index, { subject: event.target.value })} required />
                    </Field>
                    <Field label={`Step ${index + 1} body`}>
                      <textarea className="text-input" maxLength={10000} rows={5} value={step.body} onChange={(event) => updateStep(index, { body: event.target.value })} required />
                    </Field>
                  </div>
                </Card>
              ))}
            </div>
            <div>
              <Button type="submit" disabled={isSaving || pendingSequenceId !== null}>{isSaving ? 'Saving...' : editingId ? 'Save changes' : 'Create sequence'}</Button>
              {editingId ? <Button className="button-secondary" type="button" disabled={isSaving || pendingSequenceId !== null} onClick={resetForm}>Cancel</Button> : null}
            </div>
          </form>
        </Card>
      ) : null}
    </section>
  )
}
