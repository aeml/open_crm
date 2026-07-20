import { useEffect, useRef } from 'react'

function selectionFor(entityId) {
  return { entityId, dealId: entityId, controller: null, pending: new Map() }
}

// A selection object identifies one visit, including repeated A -> B -> A
// navigation. Requests keep that identity so an older visit can never write
// into a newer view of the same record.
export function useRecordSelection(selectedEntityId) {
  const activeSelectionRef = useRef(selectionFor(selectedEntityId))

  useEffect(() => () => {
    activeSelectionRef.current.controller?.abort()
    activeSelectionRef.current = selectionFor(null)
  }, [])

  function begin(entityId) {
    activeSelectionRef.current.controller?.abort()
    const selection = selectionFor(entityId)
    if (entityId) selection.controller = new AbortController()
    activeSelectionRef.current = selection
    return selection
  }

  function clear() {
    begin(null)
  }

  function isCurrent(selection) {
    return activeSelectionRef.current === selection
  }

  function isEntityActive(entityId) {
    return activeSelectionRef.current.entityId === entityId
  }

  function canApply(operation) {
    return isCurrent(operation.selection) || !isEntityActive(operation.entityId ?? operation.dealId)
  }

  function start(key, entityId = selectedEntityId, { allowEmpty = false, group = key } = {}) {
    const selection = activeSelectionRef.current
    if ((!allowEmpty && !entityId) || selection.entityId !== entityId || [...selection.pending.values()].includes(group)) return null
    selection.pending.set(key, group)
    return { dealId: entityId, entityId, key, selection }
  }

  function finish(operation) {
    operation?.selection.pending.delete(operation.key)
  }

  return {
    begin,
    canApply,
    clear,
    finish,
    isCurrent,
    isDealActive: isEntityActive,
    isEntityActive,
    start
  }
}

export function requireRecordResponse(data, entityKey, entityId, message) {
  if (!data?.[entityKey]?.id || data[entityKey].id !== entityId) throw new Error(message)
  return data
}
