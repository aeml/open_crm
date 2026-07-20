import { requireRecordResponse, useRecordSelection } from './use_record_selection'

export function useDealSelection(selectedDealId) {
  return useRecordSelection(selectedDealId)
}

export function requireDealResponse(data, dealId, message = 'Unable to update deal.') {
  return requireRecordResponse(data, 'deal', dealId, message)
}
