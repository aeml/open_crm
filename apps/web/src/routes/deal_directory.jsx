import { BulkActions } from '../components/ui/bulk_actions'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { EmptyState } from '../components/ui/empty_state'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { SavedViews } from '../components/ui/saved_views'
import { dealsExportURL } from '../lib/deals'
import { emptyDealsDescription, emptyDealsMessage, formatMoney, stageLabel } from './deal_view'

export function DealDirectory({
  canWrite,
  closeFrom,
  closeTo,
  currentUserId,
  deals,
  error,
  filteredStages,
  hasFilters,
  isLoading,
  labels,
  meta,
  onApplyCloseDates,
  onApplyOwnerFilter,
  onApplySavedView,
  onBulkChanged,
  onClearFilters,
  onCloseFromChange,
  onCloseToChange,
  onOpenDeal,
  onPipelineChange,
  onReload,
  onSearchChange,
  onSelectionChange,
  onStageChange,
  onToggleSelection,
  ownerFilter,
  pipelines,
  pipelineFilter,
  search,
  selectedDealIds,
  stageFilter,
  users
}) {
  const exportURL = dealsExportURL({
    search,
    pipelineId: pipelineFilter === 'all' ? 0 : Number.parseInt(pipelineFilter, 10) || 0,
    stageId: stageFilter === 'all' ? 0 : Number.parseInt(stageFilter, 10) || 0,
    ownerUserId: ownerFilter === 'all' ? 0 : Number.parseInt(ownerFilter, 10) || 0,
    closeFrom,
    closeTo
  })

  return (
    <Card>
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h2>{labels.collection}</h2>
            <p>Real pipeline, real stages, no fake dashboard filler.</p>
          </div>
          <a className="button button-secondary" href={exportURL}>Export CSV</a>
        </div>
        <div className="record-list" role="list" aria-label="Pipeline summary">
          <article className="record-row" role="listitem">
            <div><p>{labels.summaryOpen}</p></div>
            <div><p>{meta.openCount}</p></div>
          </article>
          <article className="record-row" role="listitem">
            <div><p>{labels.summaryWon}</p></div>
            <div><p>{meta.wonCount}</p></div>
          </article>
          <article className="record-row" role="listitem">
            <div><p>Pipeline value</p></div>
            <div>
              <p>{formatMoney(meta.pipelineValue, meta.currency)}</p>
              {(meta.missingRateCurrencies || []).length > 0 ? <p className="field-hint">Missing rates: {meta.missingRateCurrencies.join(', ')}</p> : null}
            </div>
          </article>
        </div>
        <Field label={labels.searchLabel}>
          <input className="text-input" type="search" value={search} onChange={onSearchChange} />
        </Field>
        <SavedViews entityType="deals" canManage={canWrite} currentFilters={{ q: search, pipeline: pipelineFilter, stage: stageFilter, owner: ownerFilter, closeFrom, closeTo }} onApply={onApplySavedView} defaultName={`${labels.singular} view`} />
        <Field label="Pipeline filter">
          <select className="text-input" value={pipelineFilter} onChange={onPipelineChange}>
            <option value="all">All pipelines</option>
            {pipelines.map((pipeline) => (
              <option key={pipeline.id} value={pipeline.id}>{pipeline.name}</option>
            ))}
          </select>
        </Field>
        <Field label="Stage filter">
          <select className="text-input" value={stageFilter} onChange={onStageChange}>
            <option value="all">All stages</option>
            {filteredStages.map((stage) => (
              <option key={stage.id} value={stage.id}>{stageLabel(stage, pipelineFilter)}</option>
            ))}
          </select>
        </Field>
        <Field label="Owner filter">
          <div className="button-row">
            <select className="text-input" value={ownerFilter} onChange={(event) => onApplyOwnerFilter(event.target.value)}>
              <option value="all">All owners</option>
              <option value="unassigned">Unassigned</option>
              {users.map((user) => (
                <option key={user.id} value={user.id}>{`${user.firstName} ${user.lastName}`.trim() || user.email}</option>
              ))}
            </select>
            {currentUserId ? (
              <Button className={ownerFilter === currentUserId ? '' : 'button-secondary'} type="button" onClick={() => onApplyOwnerFilter(currentUserId)}>
                Mine
              </Button>
            ) : null}
            <Button className={ownerFilter === 'unassigned' ? '' : 'button-secondary'} type="button" onClick={() => onApplyOwnerFilter('unassigned')}>
              Unassigned
            </Button>
          </div>
        </Field>
        <form className="auth-form" onSubmit={onApplyCloseDates}>
          <Field label="Expected close from"><input className="text-input" type="date" value={closeFrom} onChange={(event) => onCloseFromChange(event.target.value)} /></Field>
          <Field label="Expected close to"><input className="text-input" type="date" value={closeTo} onChange={(event) => onCloseToChange(event.target.value)} /></Field>
          <Button className="button-secondary" type="submit">Apply close dates</Button>
        </form>
        {isLoading ? <p className="field-hint">Loading {labels.showingLabel}...</p> : null}
        {error ? <InlineError message={error} onRetry={onReload} retryLabel={`Retry ${labels.showingLabel}`} /> : null}
        {canWrite ? <BulkActions entityType="deal" selectedIds={selectedDealIds} visibleIds={deals.map((deal) => deal.id)} onSelectionChange={onSelectionChange} onChanged={onBulkChanged} statuses={[]} userOptions={users} /> : null}
        <div className="record-list" role="list" aria-label={labels.listAria}>
          {!isLoading && deals.length === 0 ? (
            <EmptyState
              title={emptyDealsMessage(search, pipelineFilter, stageFilter, ownerFilter, labels, closeFrom, closeTo)}
              description={emptyDealsDescription(search, pipelineFilter, stageFilter, ownerFilter, labels, closeFrom, closeTo)}
              actionLabel={hasFilters ? 'Clear filters' : ''}
              onAction={onClearFilters}
            />
          ) : deals.map((deal) => (
            <article className="record-row" key={deal.id} role="listitem">
              <div>
                {canWrite ? <input className="record-select-checkbox" type="checkbox" aria-label={`Select ${deal.name}`} checked={selectedDealIds.includes(deal.id)} onChange={() => onToggleSelection(deal.id)} /> : null}
                <button className="button button-ghost contact-link" type="button" onClick={() => onOpenDeal(deal)}>
                  {deal.name}
                </button>
                <p>{deal.pipelineName ? `${deal.pipelineName} · ${deal.stageName}` : deal.stageName}</p>
              </div>
              <div>
                <p>{formatMoney(deal.valueAmount, deal.valueCurrency)}</p>
                <p>{deal.companyName || labels.companyEmpty}</p>
                <p className="field-hint">{deal.ownerUserName || 'Unassigned'}</p>
              </div>
            </article>
          ))}
        </div>
        <p className="field-hint">Showing {deals.length} of {meta.total} {labels.showingLabel}.</p>
      </div>
    </Card>
  )
}
