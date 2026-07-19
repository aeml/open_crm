import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'

function formatMoney(value, currency = 'USD') {
  const amount = Number.parseFloat(value || '0')
  if (!Number.isFinite(amount)) {
    return '$0.00'
  }
  const normalizedCurrency = String(currency || 'USD').toUpperCase()
  const safeCurrency = /^[A-Z]{3}$/.test(normalizedCurrency) ? normalizedCurrency : 'USD'
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: safeCurrency }).format(amount)
}

function formatPercent(value) {
  const amount = Number.parseFloat(value || '0')
  return `${Number.isFinite(amount) ? amount.toFixed(1) : '0.0'}%`
}

function formatDate(value) {
  if (!value) {
    return 'Not set'
  }
  const [year, month, day] = value.split('-').map((part) => Number.parseInt(part, 10))
  const date = year && month && day ? new Date(year, month - 1, day) : new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString()
}

export function DashboardForecast({
  forecast,
  missingRateCurrencies,
  forecastPeriod,
  setForecastPeriod,
  isLoading,
  onApplyPeriod,
  canManageQuotas,
  quotaDrafts,
  setQuotaDrafts,
  savingQuotaUserId,
  onSaveQuota
}) {
  return (
    <Card>
      <div className="card-stack">
        <div>
          <p className="eyebrow">Forecast</p>
          <h2>Quota coverage</h2>
          <p>{`Current period: ${formatDate(forecast.periodStart)} to ${formatDate(forecast.periodEnd)}. Weighted forecast combines won revenue with stage-weighted open pipeline.`}</p>
          <p className="field-hint">Open deals without an expected close date are included. Won deals without one use their last update date.</p>
          {missingRateCurrencies.length > 0 ? (
            <p className="field-hint">Add exchange rates for {missingRateCurrencies.join(', ')} to include those currencies in converted rollups.</p>
          ) : null}
        </div>
        <form className="auth-form" onSubmit={onApplyPeriod}>
          <Field label="Forecast start"><input className="text-input" type="date" required value={forecastPeriod.start} onChange={(event) => setForecastPeriod((current) => ({ ...current, start: event.target.value }))} /></Field>
          <Field label="Forecast end"><input className="text-input" type="date" required value={forecastPeriod.end} onChange={(event) => setForecastPeriod((current) => ({ ...current, end: event.target.value }))} /></Field>
          <Button type="submit" disabled={isLoading}>{isLoading ? 'Loading...' : 'Apply forecast period'}</Button>
        </form>
        <div className="record-list" role="list" aria-label="Team forecast summary">
          <article className="record-row" role="listitem">
            <div><p>Team quota</p><p className="field-hint">Target revenue for this period.</p></div>
            <div><p>{formatMoney(forecast.teamQuota, forecast.currency)}</p></div>
          </article>
          <article className="record-row" role="listitem">
            <div><p>Won revenue</p><p className="field-hint">Closed-won revenue credited to this period.</p></div>
            <div><p>{formatMoney(forecast.wonAmount, forecast.currency)}</p><p className="field-hint">{formatPercent(forecast.attainmentPct)} attained</p></div>
          </article>
          <article className="record-row" role="listitem">
            <div><p>Weighted forecast</p><p className="field-hint">Won revenue plus probability-weighted open pipeline.</p></div>
            <div><p>{formatMoney(forecast.weightedForecastAmount, forecast.currency)}</p><p className="field-hint">{formatPercent(forecast.coveragePct)} coverage</p></div>
          </article>
        </div>
        <div>
          <h3>Stage assumptions</h3>
          <p className="field-hint">Each amount uses the probability configured under Pipelines. Stages without matching open deals are omitted.</p>
        </div>
        {forecast.stages.length === 0 ? <p className="field-hint">No open pipeline value falls in this forecast period.</p> : (
          <div className="record-list" role="list" aria-label="Forecast stage assumptions">
            {forecast.stages.map((stage) => (
              <article className="record-row" role="listitem" key={stage.stageId}>
                <div><p>{stage.pipelineName} · {stage.stageName} · {stage.probabilityPercent}%</p><p className="field-hint">{stage.openDealsCount} open {stage.openDealsCount === 1 ? 'deal' : 'deals'}</p></div>
                <div><p>{formatMoney(stage.weightedOpenAmount, forecast.currency)} weighted</p><p className="field-hint">of {formatMoney(stage.openPipelineAmount, forecast.currency)}</p></div>
              </article>
            ))}
          </div>
        )}
        {forecast.members.length === 0 ? (
          <p className="field-hint">Invite users and assign deals to start building a team forecast.</p>
        ) : (
          <div className="record-list" role="list" aria-label="Quota forecast by owner">
            {forecast.members.map((member) => (
              <article className="record-row" key={member.userId} role="listitem">
                <div>
                  <p>{member.userName}</p>
                  <p className="field-hint">Won {formatMoney(member.wonAmount, forecast.currency)} · Open {formatMoney(member.openPipelineAmount, forecast.currency)} · Weighted {formatMoney(member.weightedForecastAmount, forecast.currency)}</p>
                  <p className="field-hint">{formatPercent(member.attainmentPct)} attained · {formatPercent(member.coveragePct)} coverage</p>
                </div>
                {canManageQuotas && member.userId > 0 ? (
                  <div>
                    <input className="text-input" aria-label={`Quota for ${member.userName}`} value={quotaDrafts[member.userId] ?? member.quotaAmount ?? ''} onChange={(event) => setQuotaDrafts((current) => ({ ...current, [member.userId]: event.target.value }))} />
                    <Button className="button-secondary" type="button" aria-label={`Save quota for ${member.userName}`} disabled={savingQuotaUserId === member.userId} onClick={() => onSaveQuota(member)}>{savingQuotaUserId === member.userId ? 'Saving...' : 'Save quota'}</Button>
                  </div>
                ) : (
                  <div><p>{formatMoney(member.quotaAmount, forecast.currency)}</p><p className="field-hint">Quota</p></div>
                )}
              </article>
            ))}
          </div>
        )}
      </div>
    </Card>
  )
}
