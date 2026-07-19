import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => vi.unstubAllGlobals())

function session(role) {
  return { data: { user: { id: 7 }, organization: { id: 42, name: 'Acme' }, membership: { role } } }
}

describe('pipeline configuration', () => {
  it('renames/defaults pipelines, adds and reorders stages, and explains protected outcomes', async () => {
    let pipeline = { id: 9, name: 'Sales', position: 1, isDefault: false, stages: [
      { id: 31, pipelineId: 9, name: 'Lead', position: 1, isClosed: false, isWon: false },
      { id: 32, pipelineId: 9, name: 'Closed Won', position: 2, isClosed: true, isWon: true }
    ] }
    const calls = []
    const response = (payload, status = 200) => ({ ok: status >= 200 && status < 300, status, json: async () => payload })
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const method = options.method || 'GET'
      if (requestURL.pathname.endsWith('/auth/me')) return response(session('owner'))
      if (requestURL.pathname.endsWith('/api/deal-pipelines') && method === 'GET') return response({ data: { pipelines: [pipeline] } })
      if (requestURL.pathname.endsWith('/api/deal-pipelines/9') && method === 'PATCH') {
        calls.push({ method, path: requestURL.pathname, body: JSON.parse(options.body) })
        pipeline = { ...pipeline, name: 'Client acquisition', isDefault: true }
        return response({ data: { pipeline } })
      }
      if (requestURL.pathname.endsWith('/api/deal-pipelines/9/stages') && method === 'POST') {
        calls.push({ method, path: requestURL.pathname, body: JSON.parse(options.body) })
        pipeline = { ...pipeline, stages: [...pipeline.stages, { id: 33, pipelineId: 9, name: 'Contract', position: 3, isClosed: false, isWon: false }] }
        return response({ data: { pipeline } }, 201)
      }
      if (requestURL.pathname.endsWith('/api/deal-pipelines/9/stages/order') && method === 'PUT') {
        const body = JSON.parse(options.body)
        calls.push({ method, path: requestURL.pathname, body })
        pipeline = { ...pipeline, stages: body.stageIds.map((id, index) => ({ ...pipeline.stages.find((stage) => stage.id === id), position: index + 1 })) }
        return response({ data: { pipeline } })
      }
      if (requestURL.pathname.endsWith('/api/deal-pipelines/9/stages/31') && method === 'PATCH') {
        calls.push({ method, path: requestURL.pathname, body: JSON.parse(options.body) })
        return response({ error: { code: 'STAGE_IN_USE', message: 'Move existing deals out of this stage before changing whether it is open, won, or lost' } }, 409)
      }
      return response({ data: { unreadCount: 0 } })
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/pipelines')
    render(<AppRouter />)

    fireEvent.change(await screen.findByLabelText('Pipeline name for Sales'), { target: { value: 'Client acquisition' } })
    fireEvent.click(screen.getByLabelText('Make this the default pipeline'))
    fireEvent.click(screen.getByRole('button', { name: 'Save pipeline' }))
    expect(await screen.findByText('Client acquisition updated.')).toBeInTheDocument()
    expect(calls[0].body).toEqual({ name: 'Client acquisition', makeDefault: true })

    fireEvent.change(screen.getByLabelText('New stage name for Client acquisition'), { target: { value: 'Contract' } })
    fireEvent.change(screen.getByLabelText(/^New stage probability for Client acquisition/), { target: { value: '65' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add stage' }))
    expect(await screen.findByRole('button', { name: 'Save Contract' })).toBeInTheDocument()
    expect(calls.find((call) => call.method === 'POST').body).toEqual({ name: 'Contract', outcome: 'open', probabilityPercent: 65 })

    fireEvent.click(screen.getByRole('button', { name: 'Move Contract up' }))
    await waitFor(() => expect(calls.find((call) => call.method === 'PUT')?.body.stageIds).toEqual([31, 33, 32]))

    fireEvent.change(screen.getByLabelText('Outcome for Lead'), { target: { value: 'won' } })
    expect(screen.getByLabelText(/^Probability for Lead/)).toHaveValue(100)
    expect(screen.getByLabelText(/^Probability for Lead/)).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'Save Lead' }))
    expect(await screen.findByText(/move existing deals out of this stage/i)).toBeInTheDocument()
    expect(calls.find((call) => call.path.endsWith('/stages/31')).body.probabilityPercent).toBe(100)
  })

  it('keeps pipeline administration hidden from non-admin members', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url) => String(url).endsWith('/auth/me')
      ? { ok: true, status: 200, json: async () => session('member') }
      : { ok: true, status: 200, json: async () => ({ data: { unreadCount: 0 } }) }))
    window.history.pushState({}, '', '/settings/pipelines')
    render(<AppRouter />)
    expect(await screen.findByText('Admin access required')).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Pipeline configuration' })).not.toBeInTheDocument()
  })
})
