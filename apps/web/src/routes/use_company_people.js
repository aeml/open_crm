import { useLayoutEffect, useRef, useState } from 'react'
import { isAbortError } from '../lib/api'
import { createCompanyLinkedPerson } from '../lib/companies'
import { customFieldPayload } from '../lib/custom_fields'
import { emptyLinkedPersonForm, isIndividualClient, linkedPersonFormValues } from './company_view'

// The form and mutation are company-scoped. A selection object changes even
// when navigation leaves and later returns to the same company ID, so a late
// response can never update whichever client is currently visible.
export function useCompanyPeople({ selectedCompanyId, selectedCompany, customDefinitions, onCreated, onError }) {
  const activeSelectionRef = useRef({ companyId: selectedCompanyId })
  if (activeSelectionRef.current.companyId !== selectedCompanyId) {
    activeSelectionRef.current = { companyId: selectedCompanyId }
  }
  const [form, setForm] = useState(emptyLinkedPersonForm)
  const [showForm, setShowForm] = useState(false)
  const [isSaving, setIsSaving] = useState(false)

  useLayoutEffect(() => {
    setForm(selectedCompany ? linkedPersonFormValues(selectedCompany) : emptyLinkedPersonForm)
    setShowForm(false)
    setIsSaving(false)
  }, [selectedCompanyId, selectedCompany])

  function handleToggleForm() {
    setForm(linkedPersonFormValues(selectedCompany))
    setShowForm((current) => !current)
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

  return { form, setForm, showForm, isSaving, handleSubmit, handleToggleForm }
}
