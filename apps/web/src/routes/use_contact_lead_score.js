import { useLayoutEffect, useRef, useState } from 'react'
import { isAbortError } from '../lib/api'
import { evaluateContactLeadScore } from '../lib/lead_scoring'

// Lead-score evaluation is record-scoped. A fresh selection object for every
// contact change rejects late responses even after A -> B -> A navigation, and
// its in-flight flag prevents repeated activation from starting duplicate work.
export function useContactLeadScore({ selectedContactId, onScored, onError }) {
  const activeSelectionRef = useRef({ contactId: selectedContactId, inFlight: false })
  if (activeSelectionRef.current.contactId !== selectedContactId) {
    activeSelectionRef.current = { contactId: selectedContactId, inFlight: false }
  }
  const [leadScoreStatus, setLeadScoreStatus] = useState('')
  const [isEvaluatingLeadScore, setIsEvaluatingLeadScore] = useState(false)

  useLayoutEffect(() => {
    setLeadScoreStatus('')
    setIsEvaluatingLeadScore(false)
  }, [selectedContactId])

  async function handleEvaluateLeadScore() {
    const selection = activeSelectionRef.current
    const contactId = selection.contactId
    if (!contactId || selection.inFlight) {
      return
    }

    selection.inFlight = true
    setIsEvaluatingLeadScore(true)
    setLeadScoreStatus('')
    try {
      const evaluation = await evaluateContactLeadScore(contactId)
      if (activeSelectionRef.current !== selection) {
        return
      }
      const scoredContact = evaluation?.contact
      if (!scoredContact?.id || scoredContact.id !== contactId) {
        throw new Error('Unable to evaluate lead score.')
      }
      onScored(scoredContact, contactId)
      const matchedCount = evaluation?.matchedRules?.length || 0
      const gradeText = evaluation?.grade ? ` (${evaluation.grade})` : ''
      const assignmentText = evaluation?.assignedToUserName ? ` Routed to ${evaluation.assignedToUserName}.` : ''
      setLeadScoreStatus(`Lead scored ${evaluation?.score ?? 0}${gradeText}; ${matchedCount} rule${matchedCount === 1 ? '' : 's'} matched.${assignmentText}`)
      onError('')
    } catch (scoreError) {
      if (!isAbortError(scoreError) && activeSelectionRef.current === selection) {
        onError(scoreError.message || 'Unable to evaluate lead score.')
      }
    } finally {
      selection.inFlight = false
      if (activeSelectionRef.current === selection) {
        setIsEvaluatingLeadScore(false)
      }
    }
  }

  return { handleEvaluateLeadScore, isEvaluatingLeadScore, leadScoreStatus }
}
