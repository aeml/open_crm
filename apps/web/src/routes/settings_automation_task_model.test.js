import { describe, expect, it } from 'vitest'
import { deactivationPayload, emptyForm, formFromAutomation, isExecutableTaskRule, payloadFromForm } from './settings_automation_task_model'

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

  it('uses a safety-only deactivation intent without resubmitting an unknown definition', () => {
    expect(deactivationPayload()).toEqual({ deactivateOnly: true })
  })
})
