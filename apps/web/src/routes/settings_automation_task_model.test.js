import { describe, expect, it } from 'vitest'
import { deactivationPayload, emptyForm, formFromAutomation, isApprovalTaskRule, isDealOwnerAssignmentRule, isDealSequenceEnrollmentRule, isExecutableTaskRule, isNotificationTaskRule, payloadFromForm, taskActionsForAutomation } from './settings_automation_task_model'

function dealAutomation(actions, triggerConfig = {}) {
  return {
    id: 1,
    name: 'Proposal playbook',
    triggerType: 'stage_changed',
    targetEntityType: 'deal',
    triggerConfig,
    conditionLogic: 'all',
    conditions: [],
    actions,
    isActive: true
  }
}

const firstTask = { type: 'create_task', config: { title: 'Prepare proposal' }, delayMinutes: 1440 }
const secondTask = { type: 'create_task', config: { title: 'Schedule decision review' }, delayMinutes: 4320 }

describe('settings automation task model', () => {
  it('requires an explicit reviewed contract before exposing multiple deal tasks', () => {
    expect(isExecutableTaskRule(dealAutomation([firstTask]))).toBe(true)
    expect(isExecutableTaskRule(dealAutomation([firstTask, secondTask]))).toBe(false)
    expect(isExecutableTaskRule(dealAutomation([firstTask, secondTask], { taskPlanContract: 'deal_task_plan_v1' }))).toBe(true)
    expect(isExecutableTaskRule(dealAutomation([firstTask], { taskPlanContract: 'future_contract' }))).toBe(false)
    expect(isExecutableTaskRule(dealAutomation([{ ...firstTask, config: { ...firstTask.config, assignedToUserId: 7 } }], { taskPlanContract: 'deal_task_plan_v1' }))).toBe(false)
  })

  it('builds and restores a bounded ordered deal task playbook', () => {
    const form = {
      ...emptyForm(),
      name: 'Proposal playbook',
      event: 'stage_changed',
      stageId: '12',
      title: 'Prepare proposal',
      description: 'Confirm scope.',
      dueDays: '1',
      additionalTasks: [{ title: 'Schedule decision review', description: '', dueDays: '3' }]
    }
    const payload = payloadFromForm(form)
    expect(payload.description).toBe('Creates 2 assigned follow-up tasks from a deal event.')
    expect(payload.triggerConfig).toEqual({ stageId: 12, taskPlanContract: 'deal_task_plan_v1' })
    expect(payload.actions).toEqual([firstTask, secondTask].map((action, index) => index === 0 ? { ...action, config: { ...action.config, description: 'Confirm scope.' } } : action))

    const restored = formFromAutomation({ ...dealAutomation(payload.actions, payload.triggerConfig), ...payload })
    expect(restored.title).toBe('Prepare proposal')
    expect(restored.additionalTasks).toEqual([{ title: 'Schedule decision review', description: '', dueDays: '3' }])
  })

  it('keeps lead-form automation to one durable task and rejects a sixth deal task', () => {
    const leadPayload = payloadFromForm({
      ...emptyForm(), name: 'Lead follow-up', event: 'lead_form_submitted', assignedToUserId: '7',
      title: 'Call lead', waitDays: '2', dueDays: '1', additionalTasks: [{ title: 'Must not run', description: '', dueDays: '2' }]
    })
    expect(leadPayload.actions).toHaveLength(1)
    expect(leadPayload.triggerConfig).not.toHaveProperty('taskPlanContract')
    expect(leadPayload.triggerConfig).toEqual({ taskContract: 'lead_follow_up_task_v1' })

    const tooMany = { ...emptyForm(), name: 'Too many', title: 'One', additionalTasks: Array.from({ length: 5 }, (_, index) => ({ title: `Task ${index + 2}`, description: '', dueDays: '1' })) }
    expect(() => payloadFromForm(tooMany)).toThrow('at most 5 tasks')
  })

  it('builds, exposes, and restores one reviewed approval gate before captured deal tasks', () => {
    const payload = payloadFromForm({
      ...emptyForm(),
      name: 'Approved proposal playbook',
      requiresApproval: true,
      approvalName: 'Proposal readiness',
      approverRole: 'record_owner',
      approvalMessage: 'Confirm scope before creating tasks.',
      title: 'Prepare proposal',
      dueDays: '1',
      additionalTasks: [{ title: 'Schedule decision review', description: '', dueDays: '3' }]
    })
    expect(payload.triggerConfig).toEqual({ taskPlanContract: 'deal_approval_task_plan_v1' })
    expect(payload.description).toBe('Requests a human decision before creating 2 follow-up tasks from a deal event.')
    expect(payload.actions).toEqual([
      { type: 'request_approval', config: { approvalName: 'Proposal readiness', approverRole: 'record_owner', message: 'Confirm scope before creating tasks.' } },
      firstTask,
      secondTask
    ])
    const automation = { ...dealAutomation(payload.actions, payload.triggerConfig), ...payload }
    expect(isExecutableTaskRule(automation)).toBe(true)
    expect(isApprovalTaskRule(automation)).toBe(true)
    expect(taskActionsForAutomation(automation)).toEqual([firstTask, secondTask])
    expect(formFromAutomation(automation)).toMatchObject({
      requiresApproval: true,
      approvalName: 'Proposal readiness',
      approverRole: 'record_owner',
      approvalMessage: 'Confirm scope before creating tasks.',
      title: 'Prepare proposal',
      additionalTasks: [{ title: 'Schedule decision review', description: '', dueDays: '3' }]
    })
    expect(isExecutableTaskRule({ ...automation, actions: [firstTask, payload.actions[0], secondTask] })).toBe(false)
    expect(isExecutableTaskRule({ ...automation, actions: [{ ...payload.actions[0], config: { ...payload.actions[0].config, futurePolicy: true } }, firstTask] })).toBe(false)
  })

  it('builds, exposes, and restores one bounded teammate notification after deal tasks', () => {
    const payload = payloadFromForm({
      ...emptyForm(),
      name: 'Notify proposal team',
      notifyAfterTasks: true,
      notificationRecipientRole: 'admin',
      notificationMessage: 'Proposal preparation has started.',
      title: 'Prepare proposal',
      dueDays: '1',
      additionalTasks: [{ title: 'Schedule decision review', description: '', dueDays: '3' }]
    })
    expect(payload.triggerConfig).toEqual({ taskPlanContract: 'deal_task_notify_plan_v1' })
    expect(payload.description).toBe('Creates 2 follow-up tasks and then notifies eligible teammates in the same deal transaction.')
    expect(payload.actions).toEqual([
      firstTask,
      secondTask,
      { type: 'notify', config: { recipientRole: 'admin', message: 'Proposal preparation has started.' } }
    ])
    const automation = { ...dealAutomation(payload.actions, payload.triggerConfig), ...payload }
    expect(isExecutableTaskRule(automation)).toBe(true)
    expect(isNotificationTaskRule(automation)).toBe(true)
    expect(taskActionsForAutomation(automation)).toEqual([firstTask, secondTask])
    expect(formFromAutomation(automation)).toMatchObject({
      notifyAfterTasks: true,
      notificationRecipientRole: 'admin',
      notificationMessage: 'Proposal preparation has started.',
      title: 'Prepare proposal',
      additionalTasks: [{ title: 'Schedule decision review', description: '', dueDays: '3' }]
    })
    expect(isExecutableTaskRule({ ...automation, actions: [payload.actions[2], firstTask] })).toBe(false)
    expect(() => payloadFromForm({ ...emptyForm(), name: 'Ambiguous plan', title: 'Task', requiresApproval: true, notifyAfterTasks: true })).toThrow('either a human approval gate or a teammate notification')
  })

  it('builds, exposes, and restores one exact deal-owner assignment action', () => {
    const payload = payloadFromForm({
      ...emptyForm(),
      name: 'Route changed deals',
      event: 'owner_changed',
      outcome: 'assign_owner',
      dealOwnerUserId: '8',
      conditionField: 'status',
      conditionOperator: 'equals',
      conditionValue: 'open'
    })
    expect(payload).toMatchObject({
      description: 'Assigns the deal to one active teammate and emits one causally bounded owner-change event.',
      triggerType: 'record_updated',
      targetEntityType: 'deal',
      triggerConfig: { event: 'owner_changed', conditionContract: 'deal_snapshot_v1', actionPlanContract: 'deal_assign_owner_v1' },
      conditionLogic: 'all',
      conditions: [{ field: 'status', operator: 'equals', value: 'open' }],
      actions: [{ type: 'assign_owner', config: { userId: 8 } }]
    })
    const automation = { ...dealAutomation(payload.actions, payload.triggerConfig), ...payload }
    expect(isExecutableTaskRule(automation)).toBe(true)
    expect(isDealOwnerAssignmentRule(automation)).toBe(true)
    expect(taskActionsForAutomation(automation)).toEqual([])
    expect(formFromAutomation(automation)).toMatchObject({
      event: 'owner_changed',
      outcome: 'assign_owner',
      dealOwnerUserId: '8',
      conditionField: 'status',
      conditionValue: 'open'
    })

    expect(isExecutableTaskRule({ ...automation, triggerConfig: { ...automation.triggerConfig, futureMode: true } })).toBe(false)
    expect(isExecutableTaskRule({ ...automation, triggerConfig: { ...automation.triggerConfig, taskPlanContract: 'deal_task_plan_v1' } })).toBe(false)
    expect(isExecutableTaskRule({ ...automation, actions: [{ ...payload.actions[0], config: { userId: 8, futureMode: true } }] })).toBe(false)
    expect(() => payloadFromForm({ ...emptyForm(), name: 'Archived assignment', event: 'archived', outcome: 'assign_owner', dealOwnerUserId: '8' })).toThrow('supports creation, stage changes, or direct owner changes')
    expect(() => payloadFromForm({ ...emptyForm(), name: 'Missing target', outcome: 'assign_owner' })).toThrow('Choose an active teammate')
  })

	it('builds, exposes, and restores one exact primary-contact sequence enrollment', () => {
		const payload = payloadFromForm({
			...emptyForm(),
			name: 'Start proposal cadence',
			event: 'stage_changed',
			stageId: '12',
			outcome: 'add_to_sequence',
			sequenceId: '31',
			conditionField: 'status',
			conditionOperator: 'equals',
			conditionValue: 'open'
		})
		expect(payload).toMatchObject({
			description: 'Enrolls the current primary contact in one approved sequence using the active deal owner as sender.',
			triggerType: 'stage_changed',
			triggerConfig: { stageId: 12, conditionContract: 'deal_snapshot_v1', actionPlanContract: 'deal_add_to_sequence_v1' },
			conditions: [{ field: 'status', operator: 'equals', value: 'open' }],
			actions: [{ type: 'add_to_sequence', config: { sequenceId: 31 } }]
		})
		const automation = { ...dealAutomation(payload.actions, payload.triggerConfig), ...payload }
		expect(isExecutableTaskRule(automation)).toBe(true)
		expect(isDealSequenceEnrollmentRule(automation)).toBe(true)
		expect(taskActionsForAutomation(automation)).toEqual([])
		expect(formFromAutomation(automation)).toMatchObject({ outcome: 'add_to_sequence', sequenceId: '31' })

		expect(isExecutableTaskRule({ ...automation, actions: [{ ...payload.actions[0], config: { sequenceId: 31, future: true } }] })).toBe(false)
		expect(() => payloadFromForm({ ...emptyForm(), name: 'Archived cadence', event: 'archived', outcome: 'add_to_sequence', sequenceId: '31' })).toThrow('supports deal creation or stage changes')
		expect(() => payloadFromForm({ ...emptyForm(), name: 'Missing cadence', outcome: 'add_to_sequence' })).toThrow('Choose an approved active email sequence')
	})

  it('uses a safety-only deactivation intent without resubmitting an unknown definition', () => {
    expect(deactivationPayload()).toEqual({ deactivateOnly: true })
  })
})
