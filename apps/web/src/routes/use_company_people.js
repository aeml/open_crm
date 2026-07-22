import { useLayoutEffect, useRef, useState } from 'react'
import { isAbortError } from '../lib/api'
import { createCompanyLinkedPerson, linkCompanyContact, unlinkCompanyContact } from '../lib/companies'
import { customFieldPayload } from '../lib/custom_fields'
import { emptyLinkedPersonForm, isIndividualClient, linkedPersonFormValues } from './company_view'

// The form and mutation are company-scoped. A selection object changes even
// when navigation leaves and later returns to the same company ID, so a late
// response can never update whichever client is currently visible.
export function useCompanyPeople({ selectedCompanyId, selectedCompany, customDefinitions, onCreated, onError, onRelationshipsChanged }) {
  const activeSelectionRef = useRef({ companyId: selectedCompanyId })
  const relationshipMutationRef = useRef(false)
  if (activeSelectionRef.current.companyId !== selectedCompanyId) {
    activeSelectionRef.current = { companyId: selectedCompanyId }
  }
  const [form, setForm] = useState(emptyLinkedPersonForm)
  const [showForm, setShowForm] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const [showLinkForm, setShowLinkForm] = useState(false)
  const [linkForm, setLinkForm] = useState({ contactId: '', relationshipTitle: '', isPrimary: false })
  const [isLinking, setIsLinking] = useState(false)

  useLayoutEffect(() => {
    setForm(selectedCompany ? linkedPersonFormValues(selectedCompany) : emptyLinkedPersonForm)
    setShowForm(false)
    setIsSaving(false)
    setShowLinkForm(false)
    setLinkForm({ contactId: '', relationshipTitle: '', isPrimary: false })
    setIsLinking(false)
  }, [selectedCompanyId, selectedCompany])

  function handleToggleForm() {
    setForm(linkedPersonFormValues(selectedCompany))
    setShowForm((current) => !current)
    setShowLinkForm(false)
  }

  function handleToggleLinkForm() {
    setLinkForm({ contactId: '', relationshipTitle: '', isPrimary: false })
    setShowLinkForm((current) => !current)
    setShowForm(false)
  }

  async function handleSubmit(event) {
    event.preventDefault()
    const selection = activeSelectionRef.current
    const companyId = selection.companyId
    if (!companyId || !selectedCompany || isIndividualClient(selectedCompany.clientType)) {
      return
    }

    setIsSaving(true)
    try {
      const result = await createCompanyLinkedPerson(companyId, {
        firstName: form.firstName,
        lastName: form.lastName,
        email: form.email,
        phone: form.phone,
        jobTitle: form.jobTitle,
        status: form.status,
        customFields: customFieldPayload(customDefinitions, form.customFields)
      })
      if (activeSelectionRef.current !== selection) {
        return
      }
      if (!result?.contact?.id || !result?.link) {
        throw new Error('Unable to add linked person.')
      }
      onCreated(result)
      setForm(linkedPersonFormValues(selectedCompany))
      setShowForm(false)
    } catch (saveError) {
      if (!isAbortError(saveError) && activeSelectionRef.current === selection) {
        onError(saveError, form.email || `${form.firstName} ${form.lastName}`)
      }
    } finally {
      if (activeSelectionRef.current === selection) {
        setIsSaving(false)
      }
    }
  }

  function beginRelationshipMutation() {
    if (relationshipMutationRef.current) return false
    relationshipMutationRef.current = true
    setIsLinking(true)
    return true
  }

  function finishRelationshipMutation(selection) {
    relationshipMutationRef.current = false
    if (activeSelectionRef.current === selection) setIsLinking(false)
  }

  async function handleLinkSubmit(event) {
    event.preventDefault()
    const selection = activeSelectionRef.current
    const companyId = selection.companyId
    const contactId = Number.parseInt(linkForm.contactId, 10)
    if (!companyId || !Number.isInteger(contactId) || contactId <= 0 || !beginRelationshipMutation()) return
    try {
      await linkCompanyContact(companyId, contactId, {
        relationshipTitle: linkForm.relationshipTitle,
        isPrimary: linkForm.isPrimary
      })
      if (activeSelectionRef.current !== selection) return
      await onRelationshipsChanged?.()
      setShowLinkForm(false)
      setLinkForm({ contactId: '', relationshipTitle: '', isPrimary: false })
    } catch (saveError) {
      if (!isAbortError(saveError) && activeSelectionRef.current === selection) onError(saveError, '')
    } finally {
      finishRelationshipMutation(selection)
    }
  }

  async function handleMakePrimary(contact) {
    const selection = activeSelectionRef.current
    if (!selection.companyId || !contact?.id || contact.isPrimary || !beginRelationshipMutation()) return
    try {
      await linkCompanyContact(selection.companyId, contact.id, { relationshipTitle: contact.relationshipTitle || '', isPrimary: true })
      if (activeSelectionRef.current === selection) await onRelationshipsChanged?.()
    } catch (saveError) {
      if (!isAbortError(saveError) && activeSelectionRef.current === selection) onError(saveError, '')
    } finally {
      finishRelationshipMutation(selection)
    }
  }

  async function handleUnlink(contact) {
    const selection = activeSelectionRef.current
    if (!selection.companyId || !contact?.id) return
    if (!window.confirm(`Unlink ${contact.firstName || ''} ${contact.lastName || ''} from this client?`)) return
    if (!beginRelationshipMutation()) return
    try {
      await unlinkCompanyContact(selection.companyId, contact.id)
      if (activeSelectionRef.current === selection) await onRelationshipsChanged?.()
    } catch (saveError) {
      if (!isAbortError(saveError) && activeSelectionRef.current === selection) onError(saveError, '')
    } finally {
      finishRelationshipMutation(selection)
    }
  }

  return {
    form, handleLinkSubmit, handleMakePrimary, handleSubmit, handleToggleForm, handleToggleLinkForm,
    handleUnlink, isLinking, isSaving, linkForm, setForm, setLinkForm, showForm, showLinkForm
  }
}
