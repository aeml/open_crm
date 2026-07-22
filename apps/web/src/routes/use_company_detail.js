import { useEffect, useRef, useState } from 'react'
import { isAbortError } from '../lib/api'
import { getCompany } from '../lib/companies'
import { listDeals } from '../lib/deals'
import { companyFormValues } from './company_view'
import { requireRecordResponse, useRecordSelection } from './use_record_selection'
import { useRecordWork } from './use_record_work'

const emptyCompanyForm = {
  name: '',
  clientType: 'organization',
  addressLine1: '',
  addressLine2: '',
  city: '',
  state: '',
  postalCode: '',
  country: '',
  industry: '',
  email: '',
  phone: '',
  website: '',
  status: 'prospect',
  linkedContactIDs: '',
  customFields: {}
}

export function useCompanyDetail({ companyCustomDefinitions, customDefinitionsLoaded, navigateToCompany, routeCompanyId, setError, userOptions }) {
  const [mode, setMode] = useState('list')
  const [selectedCompanyId, setSelectedCompanyId] = useState(null)
  const [detail, setDetail] = useState(null)
  const [form, setForm] = useState(emptyCompanyForm)
  const [isDetailLoading, setIsDetailLoading] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const [isArchiving, setIsArchiving] = useState(false)
  const seededRouteRef = useRef(null)
  const selection = useRecordSelection(selectedCompanyId)
  const work = useRecordWork({
    defaultAssignedToUserId: userOptions[0]?.id ? String(userOptions[0].id) : '',
    entityType: 'company',
    selectedEntityId: selectedCompanyId,
    selection,
    onError: setError
  })

  function fillForm(data) {
    setForm(companyFormValues(data.company, data.linkedContacts || [], companyCustomDefinitions))
  }

  function clear() {
    seededRouteRef.current = null
    selection.clear()
    setSelectedCompanyId(null)
    setDetail(null)
    setForm(emptyCompanyForm)
    work.reset()
    setMode('list')
    setIsDetailLoading(false)
    setIsSaving(false)
    setIsArchiving(false)
  }

  function startCreate() {
    seededRouteRef.current = null
    selection.clear()
    setMode('create')
    setForm(emptyCompanyForm)
    setDetail(null)
    setSelectedCompanyId(null)
    work.reset()
    setIsDetailLoading(false)
    setIsSaving(false)
    setIsArchiving(false)
  }

  function open(companyId) {
    selection.begin(companyId)
    setSelectedCompanyId(companyId)
    setDetail(null)
    setForm(emptyCompanyForm)
    work.reset()
    setMode('detail')
    setIsDetailLoading(true)
    navigateToCompany(companyId)
  }

  function seedCreated(data) {
    const companyId = data.company.id
    const activeSelection = selection.begin(companyId)
    const detailData = { ...data, deals: data.deals || [] }
    seededRouteRef.current = { companyId, selection: activeSelection }
    setSelectedCompanyId(companyId)
    setDetail(detailData)
    fillForm(detailData)
    work.load({ notes: data.notes || [], tasks: data.tasks || [], activities: data.activities || [], activityMeta: data.activityMeta })
    setMode('detail')
    setIsDetailLoading(false)
    setIsSaving(false)
    setIsArchiving(false)
    navigateToCompany(companyId)
  }

  useEffect(() => {
    async function openRouteCompany() {
      if (!customDefinitionsLoaded) return
      if (!Number.isInteger(routeCompanyId) || routeCompanyId <= 0) {
        if (selectedCompanyId || mode === 'detail') clear()
        else {
          seededRouteRef.current = null
          setIsDetailLoading(false)
          setIsSaving(false)
          setIsArchiving(false)
        }
        return
      }

      const seededRoute = seededRouteRef.current
      if (seededRoute?.companyId === routeCompanyId && selection.isCurrent(seededRoute.selection)) {
        setIsDetailLoading(false)
        return
      }
      seededRouteRef.current = null
      const activeSelection = selection.begin(routeCompanyId)
      const signal = activeSelection.controller.signal
      setSelectedCompanyId(routeCompanyId)
      setDetail(null)
      setForm(emptyCompanyForm)
      work.reset()
      setMode('detail')
      setIsSaving(false)
      setIsArchiving(false)

      try {
        setIsDetailLoading(true)
        const [data, loadedWork, dealData] = await Promise.all([
          getCompany(routeCompanyId, { signal }),
          work.fetchWork(routeCompanyId, { signal }),
          listDeals({ companyId: routeCompanyId }, { signal })
        ])
        if (!selection.isCurrent(activeSelection)) return
        requireRecordResponse(data, 'company', routeCompanyId, 'Unable to load company.')
        const detailData = { ...data, deals: dealData.deals || [] }
        setDetail(detailData)
        fillForm(detailData)
        work.load({ ...loadedWork, activities: data.activities || [], activityMeta: data.activityMeta })
        setError('')
      } catch (loadError) {
        if (!isAbortError(loadError) && selection.isCurrent(activeSelection)) {
          setError(loadError.message || 'Unable to load company.')
        }
      } finally {
        if (selection.isCurrent(activeSelection)) setIsDetailLoading(false)
      }
    }

    openRouteCompany()
  }, [customDefinitionsLoaded, routeCompanyId])

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
    seedCreated,
    selectedCompanyId,
    selection,
    setDetail,
    setForm,
    setIsArchiving,
    setIsSaving,
    startCreate,
    work
  }
}
