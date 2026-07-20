import { BulkActions, bulkStatusOptions } from '../components/ui/bulk_actions'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { CustomFieldFilter } from '../components/ui/custom_field_filter'
import { CustomFieldValue } from '../components/ui/custom_fields_form'
import { EmptyState } from '../components/ui/empty_state'
import { Field } from '../components/ui/field'
import { SavedViews } from '../components/ui/saved_views'
import { companiesExportURL } from '../lib/companies'
import { clientTypeLabel, formatAddress } from './company_view'

export function CompanyDirectory({
  bulkEntityType,
  canWrite,
  companies,
  currentUserId,
  customDefinitions,
  customFilter,
  duplicateCandidate,
  duplicateSearch,
  error,
  hasFilter,
  isLoading,
  meta,
  onAddClient,
  onApplyCustomFilter,
  onApplyOwnerFilter,
  onApplySavedView,
  onBulkChanged,
  onClearFilters,
  onDuplicateSearch,
  onOpenClient,
  onOpenDuplicate,
  onReload,
  onSearchChange,
  onSelectionChange,
  onToggleSelection,
  ownerFilter,
  ownerOptions,
  search,
  selectedClientIds,
  userOptions
}) {
  return (
    <Card>
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h2>Clients</h2>
            <p>See client ownership, linked people, and live pipeline in one place.</p>
          </div>
          <div className="button-row">
            <a className="button button-secondary" href={companiesExportURL({ search, customField: customFilter })}>
              Export CSV
            </a>
            {canWrite ? <Button onClick={onAddClient}>Add client</Button> : null}
          </div>
        </div>
        <p className="field-hint">CSV exports include up to 10,000 matching clients. Apply filters first for larger sets.</p>
        <Field label="Search clients">
          <input className="text-input" type="search" value={search} onChange={onSearchChange} />
        </Field>
        <SavedViews entityType="companies" canManage={canWrite} currentFilters={{ q: search, owner: ownerFilter, customField: customFilter.fieldKey, customOperator: customFilter.operator, customValue: customFilter.value }} onApply={onApplySavedView} defaultName="Client view" />
        <Field label="Owner filter">
          <div className="button-row">
            <select className="text-input" value={ownerFilter} onChange={(event) => onApplyOwnerFilter(event.target.value)}>
              <option value="all">All owners</option>
              <option value="unassigned">Unassigned</option>
              {ownerOptions.map((user) => (
                <option key={user.id} value={user.id}>{`${user.firstName} ${user.lastName}`.trim() || user.email}{user.status === 'disabled' ? ' (disabled)' : ''}</option>
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
        <CustomFieldFilter definitions={customDefinitions} value={customFilter} onApply={onApplyCustomFilter} onClear={() => onApplyCustomFilter({ fieldKey: '', operator: '', value: '' })} />
        {isLoading ? <p className="field-hint">Loading clients...</p> : null}
        {error ? (
          <div className="card-stack">
            <p className="form-error">{error}</p>
            <div>
              <Button className="button-secondary" type="button" onClick={onReload}>
                Retry clients
              </Button>
            </div>
            {duplicateCandidate ? (
              <div>
                <Button className="button-secondary" onClick={onOpenDuplicate}>
                  Open matching {duplicateCandidate.entityType === 'contact' ? 'contact' : 'client'}
                </Button>
              </div>
            ) : null}
            {duplicateSearch ? (
              <div>
                <Button className="button-secondary" onClick={onDuplicateSearch}>
                  Search existing clients for {duplicateSearch}
                </Button>
              </div>
            ) : null}
          </div>
        ) : null}
        {canWrite ? <BulkActions entityType={bulkEntityType} selectedIds={selectedClientIds} visibleIds={companies.filter((company) => company.entityType === bulkEntityType).map((company) => company.entityId)} onSelectionChange={onSelectionChange} onChanged={onBulkChanged} statuses={bulkStatusOptions[bulkEntityType]} userOptions={userOptions} /> : null}
        <div className="record-list" role="list" aria-label="Clients list">
          {!isLoading && companies.length === 0 ? (
            <EmptyState
              title={hasFilter ? 'No clients match the current filters.' : 'No clients yet.'}
              description={hasFilter ? 'Try a different client, website, or contact name, or change the owner filter.' : 'Create an organization or individual client so your contacts, pipeline records, notes, and tasks have a home.'}
              actionLabel={hasFilter ? 'Clear filters' : (canWrite ? 'Create first client' : '')}
              onAction={hasFilter ? onClearFilters : onAddClient}
            />
          ) : companies.map((company) => (
            <article className="record-row" key={company.id} role="listitem">
              <div>
                {canWrite ? <input type="checkbox" aria-label={`Select ${company.name}`} checked={bulkEntityType === company.entityType && selectedClientIds.includes(company.entityId)} onChange={() => onToggleSelection(company)} /> : null}
                <button className="button button-ghost contact-link" type="button" onClick={() => onOpenClient(company)}>
                  {company.name}
                </button>
                <p>{company.industry || `${clientTypeLabel(company.clientType)} client`}</p>
              </div>
              <div>
                <p>{company.email || company.website || formatAddress(company) || clientTypeLabel(company.clientType)}</p>
                <p>{company.status}</p>
                <p className="field-hint">{company.ownerUserName || 'Unassigned'}</p>
                {company.entityType === 'company' ? customDefinitions.filter((definition) => definition.showInList).map((definition) => (
                  <p className="field-hint" key={definition.id}><CustomFieldValue definition={definition} value={company.customFields?.[definition.fieldKey]} /></p>
                )) : null}
              </div>
            </article>
          ))}
        </div>
        <p className="field-hint">Showing {companies.length} of {meta.total} clients.</p>
      </div>
    </Card>
  )
}
