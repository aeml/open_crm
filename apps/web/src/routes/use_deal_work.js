import { useRecordWork } from './use_record_work'

export function useDealWork({ selectedDealId, ...options }) {
  return useRecordWork({ ...options, entityType: 'deal', selectedEntityId: selectedDealId })
}
