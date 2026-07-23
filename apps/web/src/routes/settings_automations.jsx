import { useEffect, useMemo, useRef, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { ControlledTextField, Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { listDealPipelines } from '../lib/deals'
import { listLeadCaptureForms } from '../lib/lead_forms'
import { listOrganizationUsers } from '../lib/users'
import { createIdempotencyKey } from '../lib/idempotency'
import { createWorkflowAutomation, decideWorkflowApproval, listWorkflowApprovals, listWorkflowAutomationRuns, listWorkflowAutomations, updateWorkflowAutomation } from '../lib/workflow_automations'
import { activeRunRefreshDelay } from '../lib/workflow_automation_polling'
import { usePageTitle } from '../lib/use_page_title'
import { SettingsAutomationRuns } from './settings_automation_runs'
import { SettingsWorkflowApprovals } from './settings_workflow_approvals'
import {
  approvalRoleOptions,
  conditionOperatorLabels,
  conditionOperators,
  conditionSummary,
  deactivationPayload,
  dealConditionDefinitions,
  emptyForm,
  equalsOperator,
  eventFromAutomation,
  existsOperator,
  formFromAutomation,
  isApprovalTaskRule,
	isDealOwnerAssignmentRule,
  isExecutableTaskRule,
  isNotificationTaskRule,
  leadFormEvent,
  maxActiveWorkflowActions,
  maxDealPlanTasks,
	ownerChangedEvent,
  payloadFromForm,
  stageChangedEvent,
  taskTimingSummary,
  taskActionsForAutomation,
  triggerOptions,
  triggerSummary
} from './settings_automation_task_model'

const activeActionSize = (automation) => automation?.isActive ? automation.actions?.length || 0 : 0

export function SettingsAutomationsRoute() {
  const { canAdminister: canManage } = useAuth()
  usePageTitle('Automations')
  const [automations, setAutomations] = useState([])
  const [definitionMeta, setDefinitionMeta] = useState({ page: 1, pageSize: 50, total: 0, activeActionCount: 0 })
  const [runs, setRuns] = useState([])
  const [approvals, setApprovals] = useState([])
  const [pipelines, setPipelines] = useState([])
  const [leadForms, setLeadForms] = useState([])
  const [users, setUsers] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  const [isLoadingMore, setIsLoadingMore] = useState(false)
  const [decidingApprovalId, setDecidingApprovalId] = useState(null)
  const loadVersion = useRef(0)
  const definitionLoadVersion = useRef(0)
  const mutationPending = useRef(false)
  const approvalAttempts = useRef(new Map())

  async function loadAutomations({ signal } = {}) {
    const version = loadVersion.current + 1
    const definitionVersion = definitionLoadVersion.current + 1
    loadVersion.current = version
    definitionLoadVersion.current = definitionVersion
    setIsLoading(true)
    try {
      const [nextDefinitions, nextRuns, nextApprovals, nextPipelines, nextLeadForms, nextUsers] = await Promise.all([
        listWorkflowAutomations({ signal }),
        listWorkflowAutomationRuns({ limit: 25, signal }),
        listWorkflowApprovals({ signal }),
        listDealPipelines({ signal }),
        listLeadCaptureForms({ signal }),
        listOrganizationUsers({ signal })
      ])
      if (signal?.aborted || loadVersion.current !== version || definitionLoadVersion.current !== definitionVersion) return
      setAutomations(nextDefinitions.automations)
      setDefinitionMeta(nextDefinitions.meta)
      setRuns(nextRuns)
      setApprovals(nextApprovals)
      setPipelines(nextPipelines)
      setLeadForms(nextLeadForms)
      setUsers(nextUsers)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError) && loadVersion.current === version) setError(loadError.message || 'Unable to load workflow automation rules.')
    } finally {
      if (!signal?.aborted && loadVersion.current === version) setIsLoading(false)
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadAutomations({ signal: controller.signal })
    return () => controller.abort()
  }, [])

  useEffect(() => {
    const refreshDelay = activeRunRefreshDelay(runs)
    if (refreshDelay === null) return undefined
    const controller = new AbortController()
    const timer = window.setTimeout(async () => {
      try {
        setRuns(await listWorkflowAutomationRuns({ limit: 25, signal: controller.signal }))
      } catch (pollError) {
        if (!isAbortError(pollError)) setError(pollError.message || 'Unable to refresh workflow automation runs.')
      }
    }, refreshDelay)
    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  }, [runs])

  async function loadMoreDefinitions() {
    if (isLoadingMore || automations.length >= definitionMeta.total) return
    const version = definitionLoadVersion.current + 1
    definitionLoadVersion.current = version
    setIsLoadingMore(true)
    try {
      const nextPage = await listWorkflowAutomations({ page: definitionMeta.page + 1, pageSize: definitionMeta.pageSize })
      if (definitionLoadVersion.current !== version) return
      setAutomations((current) => {
        const seen = new Set(current.map((automation) => automation.id))
        return [...current, ...nextPage.automations.filter((automation) => !seen.has(automation.id))]
      })
      setDefinitionMeta(nextPage.meta)
      setError('')
    } catch (loadError) {
      if (definitionLoadVersion.current === version) setError(loadError.message || 'Unable to load more stored definitions.')
    } finally {
      if (definitionLoadVersion.current === version) setIsLoadingMore(false)
    }
  }

  async function refreshDefinitionsAfterMutation(successMessage) {
    const version = definitionLoadVersion.current + 1
    definitionLoadVersion.current = version
    try {
      const firstPage = await listWorkflowAutomations()
      if (definitionLoadVersion.current !== version) return
      setAutomations(firstPage.automations)
      setDefinitionMeta(firstPage.meta)
      setError('')
      setStatus(successMessage)
    } catch (loadError) {
      if (definitionLoadVersion.current !== version) return
      setStatus(`${successMessage} The stored-definition list could not refresh; reload before another change.`)
      setError(loadError.message || 'Unable to refresh stored definitions.')
    }
  }

  const executableRules = useMemo(() => automations.filter(isExecutableTaskRule), [automations])
  const executableIDs = useMemo(() => new Set(executableRules.map((automation) => automation.id)), [executableRules])
  const visibleRuns = useMemo(() => runs.filter((run) => executableIDs.has(run.automationId)), [executableIDs, runs])
  const hiddenDefinitions = automations.length - executableRules.length
  const unsupportedActiveDefinitions = useMemo(() => automations.filter((automation) => automation.isActive && !isExecutableTaskRule(automation)), [automations])
  const activeActionCount = definitionMeta.activeActionCount
  const stages = useMemo(() => pipelines.flatMap((pipeline) => (pipeline.stages || []).map((stage) => ({ ...stage, pipelineName: pipeline.name }))), [pipelines])
  const stagesById = useMemo(() => new Map(stages.map((stage) => [stage.id, `${stage.pipelineName} · ${stage.name}`])), [stages])
  const formsById = useMemo(() => new Map(leadForms.map((leadForm) => [leadForm.id, leadForm.name])), [leadForms])
  const usersById = useMemo(() => new Map(users.map((user) => [user.id, `${user.firstName || ''} ${user.lastName || ''}`.trim() || user.email])), [users])

  function resetForm() {
    setEditingId(null)
    setForm(emptyForm())
  }

  function startEdit(automation) {
    setEditingId(automation.id)
    setForm(formFromAutomation(automation))
    setStatus('')
  }

  function addDealTask() {
    setForm((current) => ({
      ...current,
      additionalTasks: [...current.additionalTasks, { title: '', description: '', dueDays: '1' }]
    }))
  }

  function updateDealTask(index, field, value) {
    setForm((current) => ({
      ...current,
      additionalTasks: current.additionalTasks.map((task, taskIndex) => taskIndex === index ? { ...task, [field]: value } : task)
    }))
  }

  function removeDealTask(index) {
    setForm((current) => ({ ...current, additionalTasks: current.additionalTasks.filter((_, taskIndex) => taskIndex !== index) }))
  }

  async function handleSubmit(event) {
    event.preventDefault()
    if (!canManage || mutationPending.current) return
    let payload
    try {
      payload = payloadFromForm(form)
    } catch (validationError) {
      setError(validationError.message)
      return
    }
    mutationPending.current = true
    setIsSaving(true)
    setStatus('')
    try {
      if (editingId) {
        const previous = automations.find((automation) => automation.id === editingId)
        const updated = await updateWorkflowAutomation(editingId, payload)
        if (!updated || updated.id !== editingId) throw new Error('The server returned a different workflow automation rule. Refresh before retrying.')
        setAutomations((current) => current.map((automation) => (automation.id === editingId ? updated : automation)))
        setDefinitionMeta((current) => ({ ...current, activeActionCount: Math.max(0, current.activeActionCount - activeActionSize(previous) + activeActionSize(updated)) }))
        await refreshDefinitionsAfterMutation('Workflow automation rule updated.')
      } else {
        const created = await createWorkflowAutomation(payload)
        if (!created || !Number.isInteger(created.id) || created.id <= 0) throw new Error('The server did not return the created workflow automation rule. Refresh before retrying.')
        setAutomations((current) => [created, ...current])
        setDefinitionMeta((current) => ({ ...current, total: current.total + 1, activeActionCount: current.activeActionCount + activeActionSize(created) }))
        await refreshDefinitionsAfterMutation('Workflow automation rule created.')
      }
      resetForm()
    } catch (saveError) {
      setError(saveError.message || 'Unable to save workflow automation rule.')
    } finally {
      mutationPending.current = false
      setIsSaving(false)
    }
  }

  async function deactivateUnsupported(automation) {
    if (!canManage || mutationPending.current) return
    mutationPending.current = true
    setIsSaving(true)
    setStatus('')
    try {
      const updated = await updateWorkflowAutomation(automation.id, deactivationPayload(automation))
      if (!updated || updated.id !== automation.id || updated.isActive) throw new Error('The server did not confirm deactivation. Refresh before retrying.')
      setAutomations((current) => current.map((candidate) => candidate.id === automation.id ? updated : candidate))
      setDefinitionMeta((current) => ({ ...current, activeActionCount: Math.max(0, current.activeActionCount - activeActionSize(automation)) }))
      await refreshDefinitionsAfterMutation(`${automation.name} deactivated. Its stored foundation remains available for future review.`)
    } catch (saveError) {
      setError(saveError.message || 'Unable to deactivate the unsupported definition.')
    } finally {
      mutationPending.current = false
      setIsSaving(false)
    }
  }

  async function decideApproval(approval, decision, note) {
    if (decidingApprovalId) return
    const fingerprint = JSON.stringify({ decision, note })
    const retained = approvalAttempts.current.get(approval.id)
    const attempt = retained?.fingerprint === fingerprint
      ? retained
      : { fingerprint, key: createIdempotencyKey('workflow-approval') }
    approvalAttempts.current.set(approval.id, attempt)
    setDecidingApprovalId(approval.id)
    setStatus('')
    try {
      const decided = await decideWorkflowApproval(approval.id, { decision, note }, attempt.key)
      if (!decided || decided.id !== approval.id || decided.status !== decision) throw new Error('The server returned mismatched workflow approval evidence. Refresh before retrying.')
      setApprovals((current) => current.filter((candidate) => candidate.id !== approval.id))
      approvalAttempts.current.delete(approval.id)
      setError('')
      setStatus(decision === 'approved' ? 'Workflow approved and its captured tasks were created.' : 'Workflow rejected; no captured tasks were created.')
      try {
        setRuns(await listWorkflowAutomationRuns({ limit: 25 }))
      } catch {
        setError('Decision saved. Reload to refresh run history.')
      }
    } catch (decisionError) {
      setError(decisionError.message || 'Unable to decide workflow approval.')
    } finally {
      setDecidingApprovalId(null)
    }
  }

  const changeConditionValue = (event) => setForm({ ...form, conditionValue: event.target.value })
  const dealConditionValueProps = { className: 'text-input', 'aria-label': 'Deal condition value', required: true, value: form.conditionValue, onChange: changeConditionValue }
  const dealConditionChoices = form.conditionField === 'ownerUserId'
    ? users.filter((user) => user.status === 'active').map((user) => [user.id, usersById.get(user.id)])
    : form.conditionField === 'status' ? [['open', 'Open'], ['won', 'Won'], ['lost', 'Lost']] : null
	const assignmentOutcome = form.outcome === 'assign_owner'
	const visibleTriggerOptions = triggerOptions.filter((option) => assignmentOutcome
		? !['archived', leadFormEvent].includes(option.value)
		: option.value !== ownerChangedEvent)

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div>
            <p className="eyebrow">Follow-up operations</p>
							<h2>Workflow automation rules</h2>
							<p>Create replay-safe tasks or route deal ownership after reviewed events; lead work runs durably from retained evidence.</p>
          </div>
          {isLoading ? <p className="field-hint">Loading workflow automation rules...</p> : null}
          {status ? <p className="field-hint" role="status">{status}</p> : null}
          {error ? <InlineError message={error} onRetry={() => loadAutomations()} retryLabel="Retry workflow automations" /> : null}
					<p className="field-hint">{activeActionCount} of {maxActiveWorkflowActions} active workflow actions allocated. Each task, approval gate, teammate notification, and owner assignment uses one slot.</p>
          <p className="field-hint">Showing {automations.length} of {definitionMeta.total} stored definitions.</p>
          {hiddenDefinitions > 0 ? <p className="field-hint">{hiddenDefinitions} unsupported loaded {hiddenDefinitions === 1 ? 'definition' : 'definitions'} hidden.</p> : null}
          {unsupportedActiveDefinitions.length > 0 ? (
            <div className="record-list" role="list" aria-label="Active unsupported workflow definitions">
              <div><h3>Active unsupported definitions</h3><p className="field-hint">These definitions do not have a supported execution contract, but still consume active capacity. Deactivate them until they can be reviewed.</p></div>
              {unsupportedActiveDefinitions.map((automation) => (
                <article className="record-row record-row-alert" key={automation.id} role="listitem">
                  <div><p>{automation.name}</p><p className="field-hint">{(automation.actions || []).length} stored {(automation.actions || []).length === 1 ? 'action' : 'actions'} · unsupported active definition</p></div>
                  {canManage ? <Button className="button-secondary" type="button" disabled={isSaving} onClick={() => deactivateUnsupported(automation)}>Deactivate stored definition</Button> : <span className="chip">Admin action required</span>}
                </article>
              ))}
            </div>
          ) : null}
					<div className="record-list" role="list" aria-label="Workflow automation rules">
            {!isLoading && executableRules.length === 0 ? (
              <article className="record-row" role="listitem"><p>No executable workflow rules yet.</p></article>
            ) : executableRules.map((automation) => {
              const leadFollowUp = eventFromAutomation(automation) === leadFormEvent
              const approvalPlan = isApprovalTaskRule(automation)
              const notificationPlan = isNotificationTaskRule(automation)
								const assignmentPlan = isDealOwnerAssignmentRule(automation)
              const taskActions = taskActionsForAutomation(automation)
              const approvalAction = approvalPlan ? automation.actions[0] : null
              const notificationAction = notificationPlan ? automation.actions.at(-1) : null
								const assignmentAction = assignmentPlan ? automation.actions[0] : null
              return (
                <article className={automation.isActive ? 'record-row' : 'record-row record-row-alert'} key={automation.id} role="listitem">
                  <div>
                    <h3>{automation.name}</h3>
                    <p>{triggerSummary(automation, stagesById, formsById)}</p>
                    {automation.targetEntityType === 'deal' && automation.conditions?.length ? <p className="field-hint">{conditionSummary(automation, usersById)}</p> : null}
										<p className="field-hint">{leadFollowUp ? 'One durable task from retained submission evidence.' : assignmentPlan ? 'One transactional owner assignment · nested owner-change rules are causally bounded.' : approvalPlan ? `${taskActions.length}-task playbook · waits for a retained human decision.` : notificationPlan ? `${taskActions.length}-task playbook · then notifies eligible teammates in the same transaction.` : `${taskActions.length}-task playbook · all tasks commit with the deal event.`}</p>
                    {approvalAction ? <p className="field-hint">Approval: {approvalAction.config.approvalName} · {approvalRoleOptions.find((option) => option.value === approvalAction.config.approverRole)?.label}</p> : null}
                    {notificationAction ? <p className="field-hint">Notification: {approvalRoleOptions.find((option) => option.value === notificationAction.config.recipientRole)?.label} · “{notificationAction.config.message}”</p> : null}
											{assignmentAction ? <p className="field-hint">Assign deal to {usersById.get(Number(assignmentAction.config.userId)) || 'unavailable teammate'}.</p> : null}
											{taskActions.length ? <ol className="field-hint" aria-label={`${automation.name} task plan`}>
                      {taskActions.map((action, index) => <li key={`${index}-${action.config.title}`}>Create “{action.config.title}” · {taskTimingSummary(action, leadFollowUp)}{leadFollowUp ? ` · assign to ${usersById.get(Number(action.config.assignedToUserId)) || 'unavailable teammate'}` : ''}</li>)}
											</ol> : null}
                  </div>
                  <div><span className="chip">{automation.isActive ? 'Active' : 'Inactive'}</span>{canManage ? <Button className="button-secondary" type="button" onClick={() => startEdit(automation)}>Edit</Button> : null}</div>
                </article>
              )
            })}
          </div>
          {automations.length < definitionMeta.total ? <Button className="button-secondary" type="button" disabled={isLoadingMore || isSaving} onClick={loadMoreDefinitions}>{isLoadingMore ? 'Loading stored definitions...' : 'Load more stored definitions'}</Button> : null}
          <SettingsWorkflowApprovals approvals={approvals} decidingApprovalId={decidingApprovalId} isLoading={isLoading} onDecide={decideApproval} />
          <SettingsAutomationRuns canManage={canManage} isLoading={isLoading} runs={visibleRuns} />
        </div>
      </Card>

      {canManage ? (
        <Card>
          <form className="auth-form card-stack" onSubmit={handleSubmit}>
							<div><h2>{editingId ? 'Edit workflow rule' : 'New workflow rule'}</h2><p className="field-hint">A deal event can create an ordered 1–5 task playbook or assign one active owner. Lead forms create one durable task.</p></div>
            <ControlledTextField form={form} label="Rule name" maxLength="120" name="name" placeholder="Proposal follow-up" required setForm={setForm} />
            <Field label="When"><select className="text-input" value={form.event} onChange={(event) => {
              const nextEvent = event.target.value
								setForm((current) => ({ ...current, event: nextEvent, outcome: nextEvent === leadFormEvent ? 'tasks' : current.outcome, stageId: '', conditionField: '', conditionOperator: equalsOperator, conditionValue: '', additionalTasks: nextEvent === leadFormEvent ? [] : current.additionalTasks, requiresApproval: nextEvent === leadFormEvent ? false : current.requiresApproval, notifyAfterTasks: nextEvent === leadFormEvent ? false : current.notifyAfterTasks }))
							}}>{visibleTriggerOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
							{form.event !== leadFormEvent ? <Field label="Outcome"><select className="text-input" value={form.outcome} onChange={(event) => {
								const outcome = event.target.value
								setForm((current) => ({
									...current,
									outcome,
									event: (outcome === 'assign_owner' && current.event === 'archived') || (outcome === 'tasks' && current.event === ownerChangedEvent) ? 'created' : current.event,
									requiresApproval: outcome === 'assign_owner' ? false : current.requiresApproval,
									notifyAfterTasks: outcome === 'assign_owner' ? false : current.notifyAfterTasks
								}))
							}}><option value="tasks">Create follow-up tasks</option><option value="assign_owner">Assign deal owner</option></select></Field> : null}
            {form.event === stageChangedEvent ? (
              <Field label="Destination stage" hint="Choose any stage to run after every real stage change."><select className="text-input" value={form.stageId} onChange={(event) => setForm({ ...form, stageId: event.target.value })}><option value="">Any stage</option>{stages.map((stage) => <option key={stage.id} value={stage.id}>{stage.pipelineName} · {stage.name}</option>)}</select></Field>
            ) : null}
							{form.event !== leadFormEvent ? (
              <>
                <Field label="Optional deal condition"><select className="text-input" value={form.conditionField} onChange={(event) => {
                  const field = event.target.value
                  setForm({ ...form, conditionField: field, conditionOperator: conditionOperators(field)[0] || equalsOperator, conditionValue: '' })
                }}><option value="">No condition</option>{Object.entries(dealConditionDefinitions).map(([value, [label]]) => <option key={value} value={value}>{label}</option>)}</select></Field>
                {form.conditionField ? (
                  <Field label="Deal condition">
                    <div className="filter-row">
                      <select className="text-input" aria-label="Deal condition operator" value={form.conditionOperator} onChange={(event) => setForm({ ...form, conditionOperator: event.target.value, conditionValue: event.target.value === existsOperator ? '' : form.conditionValue })}>{conditionOperators(form.conditionField).map((value) => <option key={value} value={value}>{conditionOperatorLabels[value]}</option>)}</select>
                      {form.conditionOperator !== existsOperator ? dealConditionChoices ? (
                        <select {...dealConditionValueProps}><option value="">Choose a value</option>{dealConditionChoices.map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select>
                      ) : (
                        <input {...dealConditionValueProps} type={form.conditionField === 'valueAmount' ? 'number' : 'text'} min={form.conditionField === 'valueAmount' ? '0' : undefined} step={form.conditionField === 'valueAmount' ? '0.01' : undefined} maxLength={form.conditionField === 'valueCurrency' ? 3 : undefined} />
                      ) : null}
                    </div>
                  </Field>
                ) : null}
              </>
            ) : null}
							{form.event !== leadFormEvent && !assignmentOutcome ? (
              <fieldset className="card-stack">
                <legend>Human approval gate</legend>
                <label className="field-hint"><input type="checkbox" checked={form.requiresApproval} onChange={(event) => setForm({ ...form, requiresApproval: event.target.checked, notifyAfterTasks: event.target.checked ? false : form.notifyAfterTasks })} /> Require a decision before creating any tasks</label>
                {form.requiresApproval ? (
                  <>
                    <ControlledTextField form={form} label="Approval name" maxLength="200" name="approvalName" required setForm={setForm} />
                    <Field label="Who can approve"><select className="text-input" value={form.approverRole} onChange={(event) => setForm({ ...form, approverRole: event.target.value })}>{approvalRoleOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
                    <ControlledTextField form={form} label="Reviewer guidance" maxLength="2000" multiline name="approvalMessage" required rows={3} setForm={setForm} />
                    <p className="field-hint">The deal event, decision, and exact task plan remain inspectable. Rejection, definition changes, deactivation, or requester deactivation create no tasks.</p>
                  </>
                ) : null}
              </fieldset>
            ) : null}
							{form.event !== leadFormEvent && !assignmentOutcome ? (
              <fieldset className="card-stack">
                <legend>Teammate notification</legend>
                <label className="field-hint"><input type="checkbox" checked={form.notifyAfterTasks} onChange={(event) => setForm({ ...form, notifyAfterTasks: event.target.checked, requiresApproval: event.target.checked ? false : form.requiresApproval })} /> Notify eligible teammates after every task commits</label>
                {form.notifyAfterTasks ? <><Field label="Notify"><select className="text-input" value={form.notificationRecipientRole} onChange={(event) => setForm({ ...form, notificationRecipientRole: event.target.value })}>{approvalRoleOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field><ControlledTextField form={form} label="Notification message" maxLength="500" multiline name="notificationMessage" required rows={3} setForm={setForm} /><p className="field-hint">At most 50 active recipients are allowed. If membership changes beyond that boundary, the whole deal event fails without partial tasks or notifications.</p></> : null}
              </fieldset>
            ) : null}
            {form.event === leadFormEvent ? (
              <>
                <Field label="Lead form" hint="Choose one active form or all forms."><select className="text-input" value={form.formId} onChange={(event) => setForm({ ...form, formId: event.target.value })}><option value="">Any active lead form</option>{leadForms.filter((leadForm) => leadForm.isActive).map((leadForm) => <option key={leadForm.id} value={leadForm.id}>{leadForm.name}</option>)}</select></Field>
                <Field label="Optional attribution condition"><select className="text-input" value={form.conditionField} onChange={(event) => setForm({ ...form, conditionField: event.target.value, conditionValue: '' })}><option value="">No condition</option><option value="leadSource">Lead source</option><option value="utmSource">UTM source</option><option value="utmMedium">UTM medium</option><option value="utmCampaign">UTM campaign</option><option value="sourceUrl">Source URL</option></select></Field>
                {form.conditionField ? <Field label="Condition"><div className="filter-row"><select className="text-input" aria-label="Condition operator" value={form.conditionOperator} onChange={(event) => setForm({ ...form, conditionOperator: event.target.value })}><option value={equalsOperator}>Equals</option><option value="notEquals">Does not equal</option><option value="contains">Contains</option><option value={existsOperator}>Exists</option></select>{form.conditionOperator !== existsOperator ? <input className="text-input" aria-label="Condition value" maxLength="500" required value={form.conditionValue} onChange={changeConditionValue} /> : null}</div></Field> : null}
                <Field label="Assign task to"><select className="text-input" required value={form.assignedToUserId} onChange={(event) => setForm({ ...form, assignedToUserId: event.target.value })}><option value="">Choose a teammate</option>{users.map((user) => <option key={user.id} value={user.id}>{usersById.get(user.id)}</option>)}</select></Field>
                <ControlledTextField form={form} hint="0 runs immediately; maximum 365." label="Create task after days" max="365" min="0" name="waitDays" required setForm={setForm} step="1" type="number" />
              </>
            ) : null}
							{assignmentOutcome ? <Field label="Assign deal owner to"><select className="text-input" required value={form.dealOwnerUserId} onChange={(event) => setForm({ ...form, dealOwnerUserId: event.target.value })}><option value="">Choose an active teammate</option>{users.filter((user) => user.status === 'active').map((user) => <option key={user.id} value={user.id}>{usersById.get(user.id)}</option>)}</select></Field> : null}
							{!assignmentOutcome ? <><ControlledTextField form={form} label="Task title" maxLength="200" name="title" placeholder="Prepare proposal" required setForm={setForm} />
            <ControlledTextField form={form} label="Task description" maxLength="2000" multiline name="description" rows={3} setForm={setForm} />
							<ControlledTextField form={form} hint={form.event === leadFormEvent ? 'From task creation; 0 is immediate; maximum 365.' : '0 is immediate; maximum 365.'} label="Due in days" max="365" min="0" name="dueDays" required setForm={setForm} step="1" type="number" /></> : null}
							{form.event !== leadFormEvent && !assignmentOutcome ? form.additionalTasks.map((task, index) => (
              <fieldset className="card-stack" key={index}>
                <legend>Task {index + 2}</legend>
                <Field label={`Task ${index + 2} title`}><input className="text-input" maxLength="200" required value={task.title} onChange={(event) => updateDealTask(index, 'title', event.target.value)} /></Field>
                <Field label={`Task ${index + 2} description`}><textarea className="text-input" rows={3} maxLength="2000" value={task.description} onChange={(event) => updateDealTask(index, 'description', event.target.value)} /></Field>
                <Field label={`Task ${index + 2} due in days`} hint="0 is immediate; maximum 365."><input className="text-input" type="number" min="0" max="365" step="1" required value={task.dueDays} onChange={(event) => updateDealTask(index, 'dueDays', event.target.value)} /></Field>
                <Button className="button-secondary" type="button" onClick={() => removeDealTask(index)}>Remove task {index + 2}</Button>
              </fieldset>
            )) : null}
							{form.event !== leadFormEvent && !assignmentOutcome && form.additionalTasks.length < maxDealPlanTasks - 1 ? <Button className="button-secondary" type="button" onClick={addDealTask}>Add another task</Button> : null}
            <label className="field-hint"><input type="checkbox" checked={form.isActive} onChange={(event) => setForm({ ...form, isActive: event.target.checked })} /> Active rule</label>
							<div className="button-row"><Button type="submit" disabled={isSaving}>{isSaving ? 'Saving...' : editingId ? 'Save workflow rule' : 'Create workflow rule'}</Button>{editingId ? <Button className="button-secondary" type="button" onClick={resetForm}>Cancel</Button> : null}</div>
          </form>
        </Card>
      ) : null}
    </section>
  )
}
