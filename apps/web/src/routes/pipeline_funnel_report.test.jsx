import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { PipelineFunnelReport } from './pipeline_funnel_report'

afterEach(() => vi.unstubAllGlobals())

const pipeline = {
  id: 5, name: 'Sales', isDefault: true,
  stages: [
    { id: 8, name: 'Discovery', position: 1, isClosed: false },
    { id: 9, name: 'Proposal', position: 2, isClosed: false },
    { id: 10, name: 'Closed won', position: 3, isClosed: true, isWon: true }
  ]
}

function response(data) {
  return { ok: true, status: 200, json: async () => ({ data }) }
}

describe('pipeline funnel report', () => {
  it('shows exact cohort reach and velocity and reruns retained-owner filters', async () => {
    const requests = []
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      requests.push(requestURL)
      if (requestURL.pathname.endsWith('/api/deal-pipelines')) return response({ pipelines: [pipeline] })
      if (requestURL.pathname.endsWith('/api/users')) return response({ users: [{ id: 7, firstName: 'Dana', lastName: 'Owner', status: 'disabled' }] })
      return response({
        pipelineId: Number(requestURL.searchParams.get('pipelineId')),
        pipelineName: 'Sales',
        entryStageId: Number(requestURL.searchParams.get('entryStageId')),
        entryStageName: 'Discovery',
        fromDate: requestURL.searchParams.get('from'),
        toDate: requestURL.searchParams.get('to'),
        asOfDate: requestURL.searchParams.get('asOf'),
        ownerUserId: Number(requestURL.searchParams.get('ownerUserId') || 0),
        coverageStartedAt: '2026-01-01T00:00:00Z',
        historyComplete: true,
        totals: { cohortDeals: 4, openDeals: 1, wonDeals: 2, lostDeals: 1, closedDeals: 3, movedOutOpenDeals: 0, winRatePercent: '66.7', medianDaysToWin: '12.5' },
        stages: [
          { stageId: 8, stageName: 'Discovery', stageOutcome: 'open', reachedDeals: 4, reachRatePercent: '100.0', currentlyInStageDeals: 1, exitedDeals: 3, forwardOrWonDeals: 2, forwardExitRatePercent: '66.7', lostExitDeals: 1, medianDaysToReach: '0.0', medianDaysInCompletedVisit: '2.5' },
          { stageId: 9, stageName: 'Proposal', stageOutcome: 'open', reachedDeals: 2, reachRatePercent: '50.0', currentlyInStageDeals: 0, exitedDeals: 2, forwardOrWonDeals: 2, forwardExitRatePercent: '100.0', lostExitDeals: 0, medianDaysToReach: '3.5', medianDaysInCompletedVisit: '9.0' }
        ],
        semantics: ['Skipped stages are not inferred.']
      })
    }))
    render(<PipelineFunnelReport />)

    const table = await screen.findByRole('table', { name: /exact stage reach and elapsed-time metrics/i })
    const discovery = within(table).getByRole('row', { name: /Discovery/ })
    expect(discovery).toHaveTextContent('4 · 100.0%')
    expect(discovery).toHaveTextContent('2.5 days')
    expect(screen.getByRole('list', { name: 'Pipeline cohort totals' })).toHaveTextContent(/Closed win rate66\.7%/i)
    expect(screen.getByText('Skipped stages are not inferred.')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Owner at creation'), { target: { value: '7' } })
    fireEvent.click(screen.getByRole('button', { name: 'Run pipeline report' }))
    await waitFor(() => expect(requests.some((request) => request.pathname.endsWith('/api/reports/pipeline-funnel') && request.searchParams.get('ownerUserId') === '7')).toBe(true))
    expect(screen.getByRole('option', { name: 'Dana Owner (disabled)' })).toBeInTheDocument()
  })

  it('fails closed when the response does not match the requested cohort', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      if (requestURL.pathname.endsWith('/api/deal-pipelines')) return response({ pipelines: [pipeline] })
      if (requestURL.pathname.endsWith('/api/users')) return response({ users: [] })
      return response({ pipelineId: 999, entryStageId: 8, fromDate: '2000-01-01', toDate: '2000-01-02', asOfDate: '2000-01-03', ownerUserId: 0, totals: {}, stages: [], semantics: [] })
    }))
    render(<PipelineFunnelReport />)
    expect(await screen.findByText('The pipeline report returned a different cohort or observation window.')).toBeInTheDocument()
    expect(screen.queryByRole('table', { name: /exact stage reach/i })).not.toBeInTheDocument()
  })
})
