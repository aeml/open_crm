import { afterEach, describe, expect, it, vi } from 'vitest'
import { getSharedReportDashboard, getSharedReportDashboardResults, listReportDefinitions, updateSharedReportDashboard } from './reports'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('saved report definition API', () => {
  it.each([
    ['page identity', { page: 2, pageSize: 50, total: 51 }],
    ['page-size identity', { page: 1, pageSize: 49, total: 51 }],
    ['exact total', { page: 1, pageSize: 50, total: 0 }]
  ])('rejects invalid server metadata for %s', async (_label, meta) => {
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: true,
      json: async () => ({ data: { definitions: [{ id: 1, name: 'Saved report' }], meta } })
    })))

    await expect(listReportDefinitions()).rejects.toThrow('invalid report definition page')
  })
})

describe('shared report dashboard API', () => {
  const definition = {
    id: 9,
    name: 'Pipeline by stage',
    description: '',
    sourceType: 'deals',
    visualizationType: 'bar',
    visualizationContract: 'grouped_bar_v1',
    columns: [],
    filters: [],
    groupBy: 'stageName',
    aggregation: { function: 'count', field: '' },
    isActive: true
  }

  it('loads and saves an exact ordered dashboard contract', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { dashboard: { id: 4, revision: 2, widgets: [{ id: 7, reportDefinitionId: 9, position: 0, width: 'half', definition }] } } })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { dashboard: { id: 4, revision: 3, widgets: [{ id: 8, reportDefinitionId: 9, position: 0, width: 'full', definition }] } } })
      })
    vi.stubGlobal('fetch', fetchMock)

    const loaded = await getSharedReportDashboard()
    expect(loaded.revision).toBe(2)
    expect(loaded.widgets[0].definition.name).toBe('Pipeline by stage')
    const saved = await updateSharedReportDashboard({ revision: 2, widgets: [{ reportDefinitionId: 9, width: 'full' }] })
    expect(saved.revision).toBe(3)
    expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({ revision: 2, widgets: [{ reportDefinitionId: 9, width: 'full' }] })
  })

  it.each([
    ['out-of-order widget', { id: 4, revision: 2, widgets: [{ id: 7, reportDefinitionId: 9, position: 1, width: 'half', definition }] }],
    ['duplicate widget', { id: 4, revision: 2, widgets: [{ id: 7, reportDefinitionId: 9, position: 0, width: 'half', definition }, { id: 8, reportDefinitionId: 9, position: 1, width: 'full', definition }] }],
    ['unsupported width', { id: 4, revision: 2, widgets: [{ id: 7, reportDefinitionId: 9, position: 0, width: 'third', definition }] }]
  ])('rejects an invalid configuration response: %s', async (_label, dashboard) => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ data: { dashboard } }) })))
    await expect(getSharedReportDashboard()).rejects.toThrow('invalid shared dashboard')
  })

  it('validates the shared timestamp, result identity, and 12-row ceiling', async () => {
    const generatedAt = '2026-07-23T02:00:00Z'
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: true,
      json: async () => ({
        data: {
          revision: 3,
          generatedAt,
          widgets: [{
            position: 0,
            width: 'full',
            definition,
            result: {
              definitionId: 9,
              definitionName: definition.name,
              sourceType: 'deals',
              visualizationType: 'bar',
              visualizationContract: 'grouped_bar_v1',
              columns: [{ key: 'stageName', label: 'Stage', dataType: 'text' }, { key: 'recordCount', label: 'Record count', dataType: 'integer' }],
              rows: [{ values: { stageName: 'Discovery', recordCount: '4' } }],
              page: 1,
              pageSize: 12,
              hasMore: true,
              generatedAt
            }
          }]
        }
      })
    })))

    const execution = await getSharedReportDashboardResults()
    expect(execution.widgets[0].result.rows).toHaveLength(1)
    expect(execution.widgets[0].result.hasMore).toBe(true)
  })

  it('rejects a result generated outside the shared snapshot', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: true,
      json: async () => ({
        data: {
          revision: 3,
          generatedAt: '2026-07-23T02:00:00Z',
          widgets: [{
            position: 0,
            width: 'half',
            definition,
            result: { definitionId: 9, visualizationType: 'bar', visualizationContract: 'grouped_bar_v1', columns: [{}, {}], rows: [], page: 1, pageSize: 12, generatedAt: '2026-07-23T02:01:00Z' }
          }]
        }
      })
    })))

    await expect(getSharedReportDashboardResults()).rejects.toThrow('invalid shared dashboard results')
  })
})
