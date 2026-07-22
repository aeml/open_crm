import { useEffect, useState } from 'react'
import { isAbortError } from '../lib/api'
import { getContact } from '../lib/contacts'
import { listDeals } from '../lib/deals'
import { contactFormValues, emptyContactForm } from './contact_view'
import { requireRecordResponse, useRecordSelection } from './use_record_selection'
import { requireRecordWork, useRecordWork } from './use_record_work'

export function useContactDetail({ customDefinitions, customDefinitionsLoaded, navigateToContact, routeContactId, setError, userOptions }) {
  const [mode, setMode] = useState('list')
  const [selectedContactId, setSelectedContactId] = useState(null)
  const [detail, setDetail] = useState(null)
  const [form, setForm] = useState(emptyContactForm)
  const [isDetailLoading, setIsDetailLoading] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const [isArchiving, setIsArchiving] = useState(false)
  const selection = useRecordSelection(selectedContactId)
  const work = useRecordWork({
    defaultAssignedToUserId: userOptions[0]?.id ? String(userOptions[0].id) : '',
    entityType: 'contact',
    selectedEntityId: selectedContactId,
    selection,
    onError: setError
  })

  function fillForm(data) {
    setForm(contactFormValues(data.contact, customDefinitions))
  }

  function clear() {
    selection.clear()
    setSelectedContactId(null)
    setDetail(null)
    setForm(emptyContactForm)
    work.reset()
    setMode('list')
    setIsDetailLoading(false)
    setIsSaving(false)
    setIsArchiving(false)
  }

  function startCreate() {
    selection.clear()
    setMode('create')
    setForm(emptyContactForm)
    setDetail(null)
    setSelectedContactId(null)
    work.reset()
    setIsDetailLoading(false)
    setIsSaving(false)
    setIsArchiving(false)
  }

  function open(contactId) {
    selection.begin(contactId)
    setSelectedContactId(contactId)
    setDetail(null)
    setForm(emptyContactForm)
    work.reset()
    setMode('detail')
    setIsDetailLoading(true)
    navigateToContact(contactId)
  }

  useEffect(() => {
    async function openRouteContact() {
      if (!customDefinitionsLoaded) return
      if (!Number.isInteger(routeContactId) || routeContactId <= 0) {
        if (selectedContactId || mode === 'detail') clear()
        else {
          setIsDetailLoading(false)
          setIsSaving(false)
          setIsArchiving(false)
        }
        return
      }

      const activeSelection = selection.begin(routeContactId)
      const signal = activeSelection.controller.signal
      setSelectedContactId(routeContactId)
      setDetail(null)
      setForm(emptyContactForm)
      work.reset()
      setMode('detail')
      setIsSaving(false)
      setIsArchiving(false)

      try {
        setIsDetailLoading(true)
        const [data, tasks, dealData] = await Promise.all([
          getContact(routeContactId, { signal }),
          work.fetchTasks(routeContactId, { signal }),
          listDeals({ primaryContactId: routeContactId }, { signal })
        ])
        if (!selection.isCurrent(activeSelection)) return
        requireRecordResponse(data, 'contact', routeContactId, 'Unable to load contact.')
        const loadedWork = { ...requireRecordWork({ notes: data.notes || [], tasks }, 'contact', routeContactId), noteMeta: data.noteMeta }
        const detailData = { ...data, deals: dealData.deals || [] }
        setDetail(detailData)
        fillForm(detailData)
        work.load({ ...loadedWork, activities: data.activities || [], activityMeta: data.activityMeta })
        setError('')
      } catch (loadError) {
        if (!isAbortError(loadError) && selection.isCurrent(activeSelection)) {
          setError(loadError.message || 'Unable to load contact.')
        }
      } finally {
        if (selection.isCurrent(activeSelection)) setIsDetailLoading(false)
      }
    }

    openRouteContact()
  }, [customDefinitionsLoaded, routeContactId])

  return {
    clear,
    detail,
    fillForm,
    form,
    isArchiving,
    isDetailLoading,
    isSaving,
    mode,
    open,
    selectedContactId,
    selection,
    setDetail,
    setForm,
    setIsArchiving,
    setIsSaving,
    startCreate,
    work
  }
}
