import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useAuth } from '../app/providers'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { decideDealQuoteApproval } from '../lib/deals'
import { createIdempotencyKey } from '../lib/idempotency'
import {
  archiveQuoteTemplate,
  createQuoteTemplate,
  getQuoteTemplatePolicy,
  listPendingQuoteApprovals,
  listQuoteTemplatePage,
  listQuoteTemplateMergeTokens,
  updateQuoteTemplate,
  updateQuoteTemplatePolicy
} from '../lib/quote_templates'
import { usePageTitle } from '../lib/use_page_title'
import { formatMoney, formatSignatureTime } from './deal_view'

const emptyForm = {
  name: '',
  terms: 'Payment due within 30 days of invoice.',
  defaultValidityDays: '30',
  deliverySubjectTemplate: 'Finalized quote {{quote_number}}',
  deliveryMessageTemplate: 'Hi {{recipient_name}},\n\nPlease review {{quote_number}}.',
  requestSignature: false,
  requiresApproval: false,
  isActive: true,
  expectedRevision: 0
}
const pageSize = 50
const emptyMeta = { page: 1, pageSize, total: 0 }

function formFromTemplate(template) {
  return {
    name: template.name,
    terms: template.terms,
    defaultValidityDays: String(template.defaultValidityDays),
    deliverySubjectTemplate: template.deliverySubjectTemplate,
    deliveryMessageTemplate: template.deliveryMessageTemplate,
    requestSignature: template.requestSignature,
    requiresApproval: template.requiresApproval,
    isActive: template.isActive,
    expectedRevision: template.revision
  }
}

function templateInput(form) {
  return {
    ...form,
    defaultValidityDays: Number.parseInt(form.defaultValidityDays, 10),
    expectedRevision: Number.parseInt(form.expectedRevision, 10) || 0
  }
}

function PendingApprovalRow({ currentUserId, isDeciding, item, onDecide }) {
  const [note, setNote] = useState('')
  const isRequester = String(item.requestedByUserId) === String(currentUserId)
  return (
    <article className="record-row" role="listitem">
      <div>
        <h3>{item.quoteNumber} · {item.dealName}</h3>
        <p className="field-hint">{item.recipientName} · {formatMoney(item.total, item.currency)} · requested {formatSignatureTime(item.requestedAt)} by {item.requestedByUserName}</p>
        <p className="field-hint">SHA-256 {item.pdfSha256}</p>
        <Link className="button button-secondary" to={`/deals/${item.dealId}`}>Review deal and PDF</Link>
        {isRequester ? <p className="field-hint">A different active owner or admin must decide this request.</p> : null}
        {!isRequester ? (
          <Field label={`Decision note for ${item.quoteNumber}`}>
            <textarea className="text-input" maxLength={1000} rows="3" value={note} onChange={(event) => setNote(event.target.value)} />
          </Field>
        ) : null}
      </div>
      {!isRequester ? (
        <div>
          <Button type="button" disabled={isDeciding} onClick={() => onDecide(item, 'approved', note)}>{isDeciding ? 'Recording…' : 'Approve exact PDF'}</Button>
          <Button className="button-danger" type="button" disabled={isDeciding || !note.trim()} onClick={() => onDecide(item, 'rejected', note)}>Reject with note</Button>
        </div>
      ) : null}
    </article>
  )
}

export function SettingsQuoteTemplatesRoute() {
  const { session } = useAuth()
  const canManage = ['owner', 'admin'].includes(session?.membership?.role || '')
  const currentUserId = session?.user?.id || ''
  usePageTitle('Quote Templates')
  const [templates, setTemplates] = useState([])
  const [templateMeta, setTemplateMeta] = useState(emptyMeta)
  const [policy, setPolicy] = useState({ approvalRequired: false, activeApprovers: 0 })
  const [mergeTokens, setMergeTokens] = useState([])
  const [pendingApprovals, setPendingApprovals] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [decidingId, setDecidingId] = useState(null)
  const [pendingArchiveId, setPendingArchiveId] = useState(null)
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  const [isSavingPolicy, setIsSavingPolicy] = useState(false)
  const [error, setError] = useState('')
  const [statusMessage, setStatusMessage] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [appliedSearch, setAppliedSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [pageNumber, setPageNumber] = useState(1)
  const decisionAttempts = useRef(new Map())
  const latestLoad = useRef(0)
  const operationPending = useRef(false)

  async function load({ signal, requestedPage = pageNumber, search = appliedSearch, templateStatus = statusFilter } = {}) {
    const loadID = latestLoad.current + 1
    latestLoad.current = loadID
    setIsLoading(true)
    try {
      const [catalog, nextPolicy, tokens, approvals] = await Promise.all([
        listQuoteTemplatePage({ search, status: templateStatus, page: requestedPage, pageSize, signal }),
        getQuoteTemplatePolicy({ signal }),
        listQuoteTemplateMergeTokens({ signal }),
        canManage ? listPendingQuoteApprovals({ signal }) : Promise.resolve([])
      ])
      if (signal?.aborted || loadID !== latestLoad.current) return null
      setTemplates(catalog.templates)
      setTemplateMeta(catalog.meta)
      setPolicy(nextPolicy)
      setMergeTokens(tokens)
      setPendingApprovals(approvals)
      setError('')
      return catalog
    } catch (loadError) {
      if (!isAbortError(loadError) && loadID === latestLoad.current) setError(loadError.message || 'Unable to load quote templates.')
    } finally {
      if (!signal?.aborted && loadID === latestLoad.current) setIsLoading(false)
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    load({ signal: controller.signal })
    return () => controller.abort()
  }, [appliedSearch, canManage, pageNumber, statusFilter])

  function resetForm() {
    setForm(emptyForm)
    setEditingId(null)
  }

  function startEdit(template) {
    setEditingId(template.id)
    setForm(formFromTemplate(template))
  }

  async function handleSubmit(event) {
    event.preventDefault()
    if (operationPending.current) return
    operationPending.current = true
    setIsSaving(true)
    setStatusMessage('')
    try {
      const operation = editingId ? 'updated' : 'created'
      editingId
        ? await updateQuoteTemplate(editingId, templateInput(form))
        : await createQuoteTemplate(templateInput(form))
      resetForm()
      setStatusMessage(`Quote template ${operation}.`)
      setError('')
      if (pageNumber === 1) await load({ requestedPage: 1 })
      else setPageNumber(1)
    } catch (saveError) {
      setError(saveError.message || 'Unable to save quote template.')
    } finally {
      setIsSaving(false)
      operationPending.current = false
    }
  }

  async function handleArchive(template) {
    if (operationPending.current) return
    operationPending.current = true
    setPendingArchiveId(template.id)
    setStatusMessage('')
    try {
      await archiveQuoteTemplate(template.id, template.revision)
      if (editingId === template.id) resetForm()
      setStatusMessage('Quote template archived.')
      setError('')
      const catalog = await load()
      if (catalog && catalog.templates.length === 0 && catalog.meta.total > 0 && pageNumber > 1) setPageNumber((current) => current - 1)
    } catch (archiveError) {
      setError(archiveError.message || 'Unable to archive quote template.')
    } finally {
      setPendingArchiveId(null)
      operationPending.current = false
    }
  }

  function handleSearch(event) {
    event.preventDefault()
    const nextSearch = searchInput.trim()
    if (nextSearch === appliedSearch && pageNumber === 1) load({ requestedPage: 1, search: nextSearch })
    else {
      setPageNumber(1)
      setAppliedSearch(nextSearch)
    }
  }

  async function handlePolicyChange() {
    const nextRequired = !policy.approvalRequired
    setIsSavingPolicy(true)
    try {
      setPolicy(await updateQuoteTemplatePolicy(nextRequired))
      setError('')
    } catch (policyError) {
      setError(policyError.message || 'Unable to update quote approval policy.')
    } finally {
      setIsSavingPolicy(false)
    }
  }

  function decisionKey(item, payload) {
    const name = String(item.quoteId)
    const fingerprint = JSON.stringify(payload)
    if (decisionAttempts.current.get(name)?.fingerprint !== fingerprint) {
      decisionAttempts.current.set(name, { fingerprint, key: createIdempotencyKey('quote-approval') })
    }
    return decisionAttempts.current.get(name).key
  }

  async function handleDecision(item, decision, note) {
    const payload = { decision, note: note.trim() }
    setDecidingId(item.quoteId)
    try {
      await decideDealQuoteApproval(item.dealId, item.quoteId, payload, decisionKey(item, payload))
      setPendingApprovals((current) => current.filter((entry) => entry.quoteId !== item.quoteId))
      decisionAttempts.current.delete(String(item.quoteId))
      setError('')
    } catch (decisionError) {
      setError(decisionError.message || 'Unable to decide quote approval.')
    } finally {
      setDecidingId(null)
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Quote preparation policy</h2>
              <p>Reusable terms and delivery defaults become immutable snapshots when a quote is finalized.</p>
            </div>
          </div>
          {isLoading ? <p className="field-hint" role="status">Loading quote preparation settings…</p> : null}
          {error ? <InlineError message={error} /> : null}
          {!isLoading ? <p><strong>{policy.approvalRequired ? 'Independent approval required workspace-wide' : 'Independent approval optional'}</strong></p> : null}
          {!isLoading ? <p className="field-hint">{policy.activeApprovers} active owner/admin reviewer(s). Finalizers cannot approve their own immutable PDF.</p> : null}
          {canManage && !isLoading ? (
            <Button type="button" disabled={isSavingPolicy || (!policy.approvalRequired && policy.activeApprovers < 2)} onClick={handlePolicyChange}>
              {isSavingPolicy ? 'Saving…' : policy.approvalRequired ? 'Make approval optional' : 'Require approval for every quote'}
            </Button>
          ) : null}
          {!policy.approvalRequired && policy.activeApprovers < 2 ? <p className="field-hint">Add another active owner or admin before enabling workspace-wide approval.</p> : null}
          {!canManage && !isLoading ? <p className="field-hint">Only owners and admins can change templates or approval policy.</p> : null}
        </div>
      </Card>

      <Card>
        <div className="card-stack">
          <h2>Quote templates</h2>
          {statusMessage ? <p className="field-hint" role="status">{statusMessage}</p> : null}
          <form className="filters-grid" onSubmit={handleSearch}>
            <Field label="Search quote templates">
              <input className="text-input" maxLength={100} value={searchInput} disabled={isSaving || pendingArchiveId !== null} onChange={(event) => setSearchInput(event.target.value)} placeholder="Template name" />
            </Field>
            <Field label="Quote template status">
              <select className="text-input" value={statusFilter} disabled={isSaving || pendingArchiveId !== null} onChange={(event) => { setPageNumber(1); setStatusFilter(event.target.value) }}>
                <option value="all">Active and archived</option>
                <option value="active">Active</option>
                <option value="inactive">Archived</option>
              </select>
            </Field>
            <Button className="button-secondary" type="submit" disabled={isLoading || isSaving || pendingArchiveId !== null}>Apply search</Button>
          </form>
          <div className="record-list" role="list" aria-label="Quote templates">
            {!isLoading && templates.length === 0 ? <article className="record-row" role="listitem"><div><p>{appliedSearch || statusFilter !== 'all' ? 'No quote templates match these filters.' : 'No quote templates yet.'}</p><p className="field-hint">{appliedSearch || statusFilter !== 'all' ? 'Change the search or status filter and try again.' : 'Quotes can still use custom terms.'}</p></div></article> : null}
            {templates.map((template) => (
              <article className="record-row" key={template.id} role="listitem">
                <div>
                  <h3>{template.name}</h3>
                  <p className="field-hint">Revision {template.revision} · {template.defaultValidityDays} days · {template.requestSignature ? 'signature requested by default' : 'review only by default'} · {template.requiresApproval ? 'approval required' : 'approval optional'} · {template.isActive ? 'active' : 'archived'}</p>
                  <p className="field-hint">Updated {formatSignatureTime(template.updatedAt)} by {template.updatedByUserName}</p>
                </div>
                {canManage && template.isActive ? <div><Button className="button-secondary" type="button" disabled={isSaving || pendingArchiveId !== null} onClick={() => startEdit(template)}>Edit</Button><Button className="button-danger" type="button" disabled={isSaving || pendingArchiveId !== null} onClick={() => handleArchive(template)}>{pendingArchiveId === template.id ? 'Archiving…' : 'Archive'}</Button></div> : null}
              </article>
            ))}
          </div>
          <p className="field-hint" role="status">Showing {templates.length} of {templateMeta.total} quote templates{appliedSearch ? ` matching “${appliedSearch}”` : ''}. Up to 100 templates may be active for quote preparation.</p>
          <div className="button-row">
            <Button className="button-secondary" type="button" disabled={isLoading || pageNumber <= 1 || isSaving || pendingArchiveId !== null} onClick={() => setPageNumber((current) => current - 1)}>Previous page</Button>
            <Button className="button-secondary" type="button" disabled={isLoading || pageNumber * templateMeta.pageSize >= templateMeta.total || isSaving || pendingArchiveId !== null} onClick={() => setPageNumber((current) => current + 1)}>Next page</Button>
          </div>
          {mergeTokens.length > 0 ? <p className="field-hint">Available delivery merge fields: {mergeTokens.join(', ')}</p> : null}
        </div>
      </Card>

      {canManage ? (
        <Card>
          <form className="auth-form" aria-label={editingId ? 'Edit quote template' : 'Create quote template'} onSubmit={handleSubmit}>
            <h2>{editingId ? 'Edit quote template' : 'Create quote template'}</h2>
            <Field label="Template name"><input className="text-input" maxLength={120} value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} required /></Field>
            <Field label="Default validity days"><input className="text-input" type="number" min="1" max="366" value={form.defaultValidityDays} onChange={(event) => setForm({ ...form, defaultValidityDays: event.target.value })} required /></Field>
            <Field label="Quote terms"><textarea className="text-input" maxLength={10000} rows="5" value={form.terms} onChange={(event) => setForm({ ...form, terms: event.target.value })} required /></Field>
            <Field label="Delivery subject"><input className="text-input" maxLength={500} value={form.deliverySubjectTemplate} onChange={(event) => setForm({ ...form, deliverySubjectTemplate: event.target.value })} required /></Field>
            <Field label="Delivery message"><textarea className="text-input" maxLength={10000} rows="5" value={form.deliveryMessageTemplate} onChange={(event) => setForm({ ...form, deliveryMessageTemplate: event.target.value })} required /></Field>
            <label className="checkbox-row"><input type="checkbox" checked={form.requestSignature} onChange={(event) => setForm({ ...form, requestSignature: event.target.checked })} /> Request electronic signature by default</label>
            <label className="checkbox-row"><input type="checkbox" checked={form.requiresApproval} onChange={(event) => setForm({ ...form, requiresApproval: event.target.checked })} /> Require independent approval for this template</label>
            <div className="button-row"><Button type="submit" disabled={isSaving || pendingArchiveId !== null}>{isSaving ? 'Saving…' : editingId ? 'Save new revision' : 'Create template'}</Button>{editingId ? <Button className="button-secondary" type="button" disabled={isSaving || pendingArchiveId !== null} onClick={resetForm}>Cancel</Button> : null}</div>
          </form>
        </Card>
      ) : null}

      {canManage ? (
        <Card>
          <div className="card-stack">
            <h2>Pending quote approvals</h2>
            <p className="field-hint">Review the retained PDF and digest. Approval permits delivery; it does not sign or close the deal.</p>
            <div className="record-list" role="list" aria-label="Pending quote approvals">
              {!isLoading && pendingApprovals.length === 0 ? <article className="record-row" role="listitem"><div><p>No quotes are waiting for approval.</p></div></article> : null}
              {pendingApprovals.map((item) => <PendingApprovalRow currentUserId={currentUserId} isDeciding={decidingId === item.quoteId} item={item} key={item.approvalId} onDecide={handleDecision} />)}
            </div>
          </div>
        </Card>
      ) : null}
    </section>
  )
}
