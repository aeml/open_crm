import { afterEach, describe, expect, it, vi } from 'vitest'
import { listWorkflowAutomationRuns } from './workflow_automations'

afterEach(() => vi.unstubAllGlobals())

function actionResponse(action) {
  vi.stubGlobal('fetch', vi.fn(async () => ({
    ok: true,
    json: async () => ({ data: { runs: [{ id: 9, actions: [action] }] } })
  })))
}

const expectedCloseAction = {
  id: 12,
  position: 1,
  type: 'update_field',
  label: 'Set expected close date',
  status: 'succeeded',
  attempts: 1,
  scheduledAt: '2026-07-23T12:00:00Z',
  updatedField: 'expectedCloseDate',
  previousValue: '2026-07-31',
  currentValue: '2026-08-22',
  fieldValueChanged: true
}

describe('workflow automation run evidence', () => {
  it('accepts the exact typed expected-close result', async () => {
    actionResponse(expectedCloseAction)

    await expect(listWorkflowAutomationRuns()).resolves.toMatchObject([{
      actions: [{ updatedField: 'expectedCloseDate', currentValue: '2026-08-22', fieldValueChanged: true }]
    }])
  })

  it.each([
    ['missing field identity', { updatedField: undefined }],
    ['missing current date', { currentValue: undefined }],
    ['invalid previous date', { previousValue: 'July 31' }],
    ['field evidence on another action', { type: 'create_task' }]
  ])('rejects malformed typed evidence: %s', async (_label, changes) => {
    actionResponse({ ...expectedCloseAction, ...changes })

    await expect(listWorkflowAutomationRuns()).rejects.toThrow('invalid workflow action evidence')
  })
})
