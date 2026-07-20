import { useEffect, useRef } from 'react'

function selectionFor(dealId) {
  return { dealId, controller: null, pending: new Map() }
}

// Deal requests can finish after A -> B -> A navigation. Identity, rather than
// the numeric ID alone, distinguishes each visit and lets callers reject work
// from an older visit to the same deal.
export function useDealSelection(selectedDealId) {
  const activeSelectionRef = useRef(selectionFor(selectedDealId))

  useEffect(() => () => {
    activeSelectionRef.current.controller?.abort()
    activeSelectionRef.current = selectionFor(null)
  }, [])

  function begin(dealId) {
    activeSelectionRef.current.controller?.abort()
    const selection = selectionFor(dealId)
    if (dealId) {
      selection.controller = new AbortController()
    }
    activeSelectionRef.current = selection
    return selection
  }

  function clear() {
    begin(null)
  }

  function isCurrent(selection) {
    return activeSelectionRef.current === selection
  }

  function isDealActive(dealId) {
    return activeSelectionRef.current.dealId === dealId
  }

  function canApply(operation) {
    return isCurrent(operation.selection) || !isDealActive(operation.dealId)
  }

  function start(key, dealId = selectedDealId, { allowEmpty = false, group = key } = {}) {
    const selection = activeSelectionRef.current
    if ((!allowEmpty && !dealId) || selection.dealId !== dealId || [...selection.pending.values()].includes(group)) {
      return null
    }
    selection.pending.set(key, group)
    return { dealId, key, selection }
  }

  function finish(operation) {
    operation?.selection.pending.delete(operation.key)
  }

  return { begin, canApply, clear, finish, isCurrent, isDealActive, start }
}

export function requireDealResponse(data, dealId, message = 'Unable to update deal.') {
  if (!data?.deal?.id || data.deal.id !== dealId) {
    throw new Error(message)
  }
  return data
}
