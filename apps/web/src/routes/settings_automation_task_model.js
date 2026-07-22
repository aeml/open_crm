export const leadFormEvent = 'lead_form_submitted'
export const stageChangedEvent = 'stage_changed'
export const triggerOptions = [
  { value: 'created', label: 'Deal created' },
  { value: stageChangedEvent, label: 'Deal moved to a stage' },
  { value: 'archived', label: 'Deal archived' },
  { value: leadFormEvent, label: 'Lead form submitted' }
]

const dealConditionContract = 'deal_snapshot_v1'
const dealTaskPlanContract = 'deal_task_plan_v1'
const dealApprovalTaskPlanContract = 'deal_approval_task_plan_v1'
const dealTaskNotifyPlanContract = 'deal_task_notify_plan_v1'
const leadFollowUpTaskContract = 'lead_follow_up_task_v1'
export const maxDealPlanTasks = 5
export const maxActiveWorkflowActions = 50
export const approvalRoleOptions = [
  { value: 'owner', label: 'Workspace owners' },
  { value: 'admin', label: 'Workspace owners and admins' },
  { value: 'record_owner', label: 'Current deal owner' }
]
export const conditionOperatorLabels = { greaterThan: 'is greater than', lessThan: 'is less than', equals: 'equals', notEquals: 'does not equal', exists: 'is set' }
export const equalsOperator = 'equals'
export const existsOperator = 'exists'
const equalityOperators = [equalsOperator, 'notEquals', existsOperator]
export const dealConditionDefinitions = {
  valueAmount: ['Value amount', ['greaterThan', 'lessThan', existsOperator], (value) => isFinite(value) && +value >= 0],
  valueCurrency: ['Currency', equalityOperators, (value) => /^[A-Za-z]{3}$/.test(value)],
  ownerUserId: ['Owner', equalityOperators, (value) => Number.isInteger(+value) && +value > 0],
  status: ['Status', [equalsOperator, 'notEquals'], (value) => /^(open|won|lost)$/i.test(value)]
}

export function conditionOperators(field) {
  return dealConditionDefinitions[field]?.[1] || equalityOperators
}

function executableDealCondition(condition) {
  if (!condition || !Object.hasOwn(dealConditionDefinitions, condition.field)) return false
  const definition = dealConditionDefinitions[condition.field]
  if (!definition[1].includes(condition.operator)) return false
  if (condition.operator === existsOperator) return true
  const value = String(condition.value || '').trim()
  return Boolean(value) && definition[2](value)
}

export function emptyForm() {
  return {
    name: '', event: 'created', stageId: '', formId: '', conditionField: '', conditionOperator: equalsOperator,
    conditionValue: '', assignedToUserId: '', title: '', description: '', waitDays: '0', dueDays: '1',
    additionalTasks: [], requiresApproval: false, approvalName: 'Review task playbook', approverRole: 'admin',
    approvalMessage: 'Review this deal before creating the follow-up tasks.', notifyAfterTasks: false,
    notificationRecipientRole: 'record_owner', notificationMessage: 'The deal follow-up task plan is ready.', isActive: true
  }
}

function validWholeDays(value) {
  return Number.isInteger(value) && value >= 0 && value <= 365
}

function dayCount(value) {
  return `${value} day${value === 1 ? '' : 's'}`
}

export function eventFromAutomation(automation) {
  if (automation.triggerType === 'record_created') return 'created'
  if (automation.triggerType === stageChangedEvent) return stageChangedEvent
  if (automation.triggerType === 'record_updated' && automation.triggerConfig?.event === 'archived') return 'archived'
  if (automation.triggerType === 'form_submitted' && automation.targetEntityType === 'lead_form') return leadFormEvent
  return ''
}

export function isExecutableTaskRule(automation) {
  const actions = automation.actions || []
  const event = eventFromAutomation(automation)
  const stageID = Number(automation.triggerConfig?.stageId || 0)
  const taskPlanContract = automation.triggerConfig?.taskPlanContract
  if (automation.targetEntityType === 'deal') {
    const approvalPlan = taskPlanContract === dealApprovalTaskPlanContract
    const notificationPlan = taskPlanContract === dealTaskNotifyPlanContract
    const taskActions = approvalPlan ? actions.slice(1) : notificationPlan ? actions.slice(0, -1) : actions
    const validTaskPlan = approvalPlan
      ? validApprovalAction(actions[0]) && validDealTaskActions(taskActions, true)
      : notificationPlan
        ? validDealTaskActions(taskActions, true) && validNotificationAction(actions.at(-1))
      : validDealTaskActions(taskActions, Boolean(taskPlanContract)) && (taskActions.length === 1
          ? (!taskPlanContract || taskPlanContract === dealTaskPlanContract)
          : taskPlanContract === dealTaskPlanContract)
    const conditions = automation.conditions || []
    const condition = conditions[0]
    const validCondition = conditions.length === 0 || (conditions.length === 1 && automation.conditionLogic === 'all' && automation.triggerConfig?.conditionContract === dealConditionContract && executableDealCondition(condition))
    return Boolean(event) && event !== leadFormEvent && validTaskPlan && validCondition &&
      (event !== stageChangedEvent || !automation.triggerConfig?.stageId || (Number.isInteger(stageID) && stageID > 0))
  }
  const action = actions[0]
  const config = action?.config || {}
  if (!validDealTaskActions(actions, false)) return false
  const formID = Number(automation.triggerConfig?.formId || 0)
  const leadContract = automation.triggerConfig?.taskContract
  const assigneeID = Number(config.assignedToUserId || 0)
  const hasDueDays = Object.hasOwn(config, 'dueDays')
  const dueDays = Number(config.dueDays)
  const allowedFields = new Set(['sourceUrl', 'leadSource', 'utmSource', 'utmMedium', 'utmCampaign'])
  const allowedOperators = new Set(['equals', 'notEquals', 'contains', 'exists'])
  const conditions = automation.conditions || []
  return actions.length === 1 && event === leadFormEvent && conditions.length <= 1 &&
    (!leadContract || leadContract === leadFollowUpTaskContract) &&
    (!automation.triggerConfig?.formId || (Number.isInteger(formID) && formID > 0)) &&
    Number.isInteger(assigneeID) && assigneeID > 0 &&
    Object.keys(config).every((key) => ['title', 'description', 'assignedToUserId', 'dueDays'].includes(key)) &&
    (!hasDueDays || validWholeDays(dueDays)) &&
    conditions.every((condition) => allowedFields.has(condition.field) && allowedOperators.has(condition.operator) && (condition.operator === 'exists' || Boolean(String(condition.value || '').trim())))
}

function validDealTaskActions(actions, exactReviewedConfig) {
  return actions.length > 0 && actions.length <= maxDealPlanTasks && actions.every((candidate) => {
    const title = String(candidate?.config?.title || '')
    const description = String(candidate?.config?.description || '')
    const delayMinutes = Number(candidate?.delayMinutes || 0)
    const exactConfig = !exactReviewedConfig || Object.keys(candidate?.config || {}).every((key) => ['title', 'description'].includes(key))
    return candidate?.type === 'create_task' && exactConfig && !candidate.scheduledAt && Boolean(title) && title.length <= 200 && description.length <= 2000 &&
      Number.isInteger(delayMinutes) && delayMinutes >= 0 && delayMinutes <= 525600 && delayMinutes % 1440 === 0
  })
}

function validApprovalAction(action) {
  const config = action?.config || {}
  return action?.type === 'request_approval' && !action.scheduledAt && Number(action.delayMinutes || 0) === 0 &&
    Object.keys(config).every((key) => ['approvalName', 'approverRole', 'message'].includes(key)) &&
    Boolean(String(config.approvalName || '').trim()) && String(config.approvalName).length <= 200 &&
    approvalRoleOptions.some((option) => option.value === config.approverRole) &&
    Boolean(String(config.message || '').trim()) && String(config.message).length <= 2000
}

function validNotificationAction(action) {
  const config = action?.config || {}
  return action?.type === 'notify' && !action.scheduledAt && Number(action.delayMinutes || 0) === 0 &&
    Object.keys(config).every((key) => ['recipientRole', 'message'].includes(key)) &&
    approvalRoleOptions.some((option) => option.value === config.recipientRole) &&
    Boolean(String(config.message || '').trim()) && String(config.message).length <= 500
}

export function isApprovalTaskRule(automation) {
  return automation?.targetEntityType === 'deal' && automation?.triggerConfig?.taskPlanContract === dealApprovalTaskPlanContract && isExecutableTaskRule(automation)
}

export function isNotificationTaskRule(automation) {
  return automation?.targetEntityType === 'deal' && automation?.triggerConfig?.taskPlanContract === dealTaskNotifyPlanContract && isExecutableTaskRule(automation)
}

export function taskActionsForAutomation(automation) {
  if (isApprovalTaskRule(automation)) return automation.actions.slice(1)
  if (isNotificationTaskRule(automation)) return automation.actions.slice(0, -1)
  return automation.actions || []
}

export function formFromAutomation(automation) {
  const approvalPlan = isApprovalTaskRule(automation)
  const notificationPlan = isNotificationTaskRule(automation)
  const approvalAction = approvalPlan ? automation.actions[0] : null
  const notificationAction = notificationPlan ? automation.actions.at(-1) : null
  const taskActions = approvalPlan ? automation.actions.slice(1) : notificationPlan ? automation.actions.slice(0, -1) : automation.actions
  const action = taskActions[0]
  const leadFollowUp = eventFromAutomation(automation) === leadFormEvent
  const hasDueDays = leadFollowUp && Object.hasOwn(action.config || {}, 'dueDays')
  return {
    name: automation.name || '',
    event: eventFromAutomation(automation),
    stageId: automation.triggerConfig?.stageId ? String(automation.triggerConfig.stageId) : '',
    formId: automation.triggerConfig?.formId ? String(automation.triggerConfig.formId) : '',
    conditionField: automation.conditions?.[0]?.field || '',
    conditionOperator: automation.conditions?.[0]?.operator || equalsOperator,
    conditionValue: automation.conditions?.[0]?.value || '',
    assignedToUserId: action.config?.assignedToUserId ? String(action.config.assignedToUserId) : '',
    title: action.config?.title || '',
    description: action.config?.description || '',
    waitDays: String(hasDueDays ? (action.delayMinutes || 0) / 1440 : 0),
    dueDays: String(hasDueDays ? action.config.dueDays : (action.delayMinutes || 0) / 1440),
    additionalTasks: leadFollowUp ? [] : taskActions.slice(1).map((candidate) => ({
      title: candidate.config?.title || '',
      description: candidate.config?.description || '',
      dueDays: String((candidate.delayMinutes || 0) / 1440)
    })),
    requiresApproval: approvalPlan,
    approvalName: approvalAction?.config?.approvalName || 'Review task playbook',
    approverRole: approvalAction?.config?.approverRole || 'admin',
    approvalMessage: approvalAction?.config?.message || 'Review this deal before creating the follow-up tasks.',
    notifyAfterTasks: notificationPlan,
    notificationRecipientRole: notificationAction?.config?.recipientRole || 'record_owner',
    notificationMessage: notificationAction?.config?.message || 'The deal follow-up task plan is ready.',
    isActive: automation.isActive === true
  }
}

export function payloadFromForm(form) {
  const dueDays = Number(form.dueDays)
  if (!validWholeDays(dueDays)) throw new Error('Due days must be a whole number from 0 to 365.')
  const leadFollowUp = form.event === leadFormEvent
  const taskForms = leadFollowUp ? [{ title: form.title, description: form.description, dueDays: form.dueDays }] : [
    { title: form.title, description: form.description, dueDays: form.dueDays },
    ...(form.additionalTasks || [])
  ]
  if (taskForms.length > maxDealPlanTasks) throw new Error(`Deal task plans support at most ${maxDealPlanTasks} tasks.`)
  for (const [index, task] of taskForms.entries()) {
    const title = String(task.title || '').trim()
    const description = String(task.description || '').trim()
    if (!title) throw new Error(`Task ${index + 1} needs a title.`)
    if (title.length > 200 || description.length > 2000) throw new Error(`Task ${index + 1} exceeds its text limit.`)
    if (!validWholeDays(Number(task.dueDays))) throw new Error(`Task ${index + 1} due days must be a whole number from 0 to 365.`)
  }
  const waitDays = leadFollowUp ? Number(form.waitDays) : 0
  if (!validWholeDays(waitDays)) throw new Error('Create-after days must be a whole number from 0 to 365.')
  const triggerType = form.event === 'created' ? 'record_created' : form.event === stageChangedEvent ? stageChangedEvent : leadFollowUp ? 'form_submitted' : 'record_updated'
  const triggerConfig = form.event === 'archived'
    ? { event: 'archived' }
    : form.event === stageChangedEvent && form.stageId
      ? { stageId: Number(form.stageId) }
      : leadFollowUp && form.formId
        ? { taskContract: leadFollowUpTaskContract, formId: Number(form.formId) }
        : {}
  if (leadFollowUp && !form.formId) triggerConfig.taskContract = leadFollowUpTaskContract
  const dealCondition = !leadFollowUp && form.conditionField
  if (dealCondition) triggerConfig.conditionContract = dealConditionContract
  if (!leadFollowUp) triggerConfig.taskPlanContract = dealTaskPlanContract
  if (!leadFollowUp && form.requiresApproval) {
    if (!String(form.approvalName || '').trim()) throw new Error('Approval needs a name.')
    if (String(form.approvalName).trim().length > 200) throw new Error('Approval name exceeds 200 characters.')
    if (!approvalRoleOptions.some((option) => option.value === form.approverRole)) throw new Error('Choose a supported approval role.')
    if (!String(form.approvalMessage || '').trim()) throw new Error('Approval needs reviewer guidance.')
    if (String(form.approvalMessage).trim().length > 2000) throw new Error('Approval guidance exceeds 2,000 characters.')
    triggerConfig.taskPlanContract = dealApprovalTaskPlanContract
  }
  if (!leadFollowUp && form.notifyAfterTasks) {
    if (form.requiresApproval) throw new Error('Choose either a human approval gate or a teammate notification for this bounded plan.')
    if (!approvalRoleOptions.some((option) => option.value === form.notificationRecipientRole)) throw new Error('Choose a supported notification recipient role.')
    if (!String(form.notificationMessage || '').trim()) throw new Error('Teammate notification needs a message.')
    if (String(form.notificationMessage).trim().length > 500) throw new Error('Teammate notification exceeds 500 characters.')
    triggerConfig.taskPlanContract = dealTaskNotifyPlanContract
  }
  const config = { title: form.title.trim() }
  if (form.description.trim()) config.description = form.description.trim()
  if (leadFollowUp) {
    const assignedToUserId = Number(form.assignedToUserId)
    if (!Number.isInteger(assignedToUserId) || assignedToUserId <= 0) throw new Error('Choose an active teammate for lead follow-up tasks.')
    config.assignedToUserId = assignedToUserId
    config.dueDays = dueDays
  }
  const conditions = []
  if (form.conditionField) {
    if (form.conditionOperator !== existsOperator && !form.conditionValue.trim()) throw new Error('Enter a condition value or remove the attribution condition.')
    const condition = { field: form.conditionField, operator: form.conditionOperator, value: form.conditionOperator === existsOperator ? '' : form.conditionValue.trim() }
    if (dealCondition && !executableDealCondition(condition)) throw new Error('Invalid deal condition.')
    if (dealCondition && condition.field === 'valueCurrency') condition.value = condition.value.toUpperCase()
    conditions.push(condition)
  }
  let actions = leadFollowUp
    ? [{ type: 'create_task', config, delayMinutes: waitDays * 1440 }]
    : taskForms.map((task) => {
      const taskConfig = { title: task.title.trim() }
      const description = String(task.description || '').trim()
      if (description) taskConfig.description = description
      return { type: 'create_task', config: taskConfig, delayMinutes: Number(task.dueDays) * 1440 }
    })
  if (!leadFollowUp && form.requiresApproval) {
    actions = [{
      type: 'request_approval',
      config: {
        approvalName: String(form.approvalName).trim(),
        approverRole: form.approverRole,
        message: String(form.approvalMessage).trim()
      }
    }, ...actions]
  }
  if (!leadFollowUp && form.notifyAfterTasks) {
    actions = [...actions, {
      type: 'notify',
      config: {
        recipientRole: form.notificationRecipientRole,
        message: String(form.notificationMessage).trim()
      }
    }]
  }
  return {
    name: form.name.trim(),
    description: leadFollowUp
      ? 'Creates one durable assigned follow-up task from an accepted lead form submission.'
      : form.requiresApproval
        ? `Requests a human decision before creating ${taskForms.length} follow-up ${taskForms.length === 1 ? 'task' : 'tasks'} from a deal event.`
        : form.notifyAfterTasks
          ? `Creates ${taskForms.length} follow-up ${taskForms.length === 1 ? 'task' : 'tasks'} and then notifies eligible teammates in the same deal transaction.`
        : `Creates ${taskForms.length} assigned follow-up ${taskForms.length === 1 ? 'task' : 'tasks'} from a deal event.`,
    triggerType,
    targetEntityType: leadFollowUp ? 'lead_form' : 'deal',
    triggerConfig,
    conditionLogic: 'all',
    conditions,
    actions,
    isActive: form.isActive,
    position: 0
  }
}

export function deactivationPayload() {
  return { deactivateOnly: true }
}

export function conditionSummary(automation, usersById) {
  const condition = automation.conditions?.[0]
  const field = dealConditionDefinitions[condition.field][0]
  const operator = conditionOperatorLabels[condition.operator]
  const value = condition.field === 'ownerUserId' ? usersById.get(Number(condition.value)) || 'unavailable teammate' : condition.value
  const summary = `Only if ${field.toLowerCase()} ${operator}`
  return condition.operator === existsOperator ? summary : `${summary} ${value}`
}

export function triggerSummary(automation, stagesById, formsById) {
  const event = eventFromAutomation(automation)
  if (event === stageChangedEvent) {
    const stageID = Number(automation.triggerConfig?.stageId || 0)
    return stageID ? `When moved to ${stagesById.get(stageID) || `stage #${stageID}`}` : 'After every real stage change'
  }
  if (event === leadFormEvent) {
    const formID = Number(automation.triggerConfig?.formId || 0)
    return formID ? `When ${formsById.get(formID) || `lead form #${formID}`} is submitted` : 'When any active lead form is submitted'
  }
  return event === 'archived' ? 'When a deal is archived' : 'When a deal is created'
}

export function taskTimingSummary(action, leadFollowUp) {
  const delay = Number(action?.delayMinutes || 0)
  if (leadFollowUp && Object.hasOwn(action?.config || {}, 'dueDays')) {
    const waitDays = delay / 1440
    const dueDays = Number(action.config.dueDays)
    const createText = waitDays === 0 ? 'create immediately' : `create after ${dayCount(waitDays)}`
    const dueText = dueDays === 0 ? 'due on creation' : `due ${dayCount(dueDays)} later`
    return `${createText} · ${dueText}`
  }
  if (delay === 0) return 'due immediately'
  if (delay % 1440 === 0) return `due in ${dayCount(delay / 1440)}`
  return `due in ${delay} minutes`
}

export function formatRunTime(value) {
  const date = new Date(value)
  return value && !Number.isNaN(date.getTime()) ? date.toLocaleString() : 'Not recorded'
}
