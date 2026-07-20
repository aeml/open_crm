package app

import "net/http"

func registerCRMRoutes(mux *http.ServeMux, dependencies Dependencies, rateLimiter rateLimitService) {
	mux.HandleFunc("GET /api/contacts", func(w http.ResponseWriter, r *http.Request) {
		handleListContacts(dependencies.AuthService, dependencies.ContactsService, w, r)
	})
	mux.HandleFunc("GET /api/export/contacts", func(w http.ResponseWriter, r *http.Request) {
		handleExportContacts(dependencies.AuthService, dependencies.ExportsService, w, r)
	})
	mux.HandleFunc("POST /api/contacts", func(w http.ResponseWriter, r *http.Request) {
		handleCreateContact(dependencies.AuthService, dependencies.ContactsService, dependencies.BillingService, w, r)
	})
	mux.HandleFunc("GET /api/contacts/{contactID}", func(w http.ResponseWriter, r *http.Request) {
		handleGetContact(dependencies.AuthService, dependencies.ContactsService, w, r)
	})
	mux.HandleFunc("PATCH /api/contacts/{contactID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateContact(dependencies.AuthService, dependencies.ContactsService, w, r)
	})
	mux.HandleFunc("DELETE /api/contacts/{contactID}", func(w http.ResponseWriter, r *http.Request) {
		handleArchiveContact(dependencies.AuthService, dependencies.ContactsService, w, r)
	})
	mux.HandleFunc("POST /api/contacts/{contactID}/email", func(w http.ResponseWriter, r *http.Request) {
		handleSendContactEmail(dependencies.AuthService, dependencies.ContactsService, dependencies.UserEmailService, dependencies.NotesService, dependencies.EmailMessagesService, dependencies.EmailSuppressionsService, w, r)
	})
	mux.HandleFunc("POST /api/contacts/{contactID}/sms", func(w http.ResponseWriter, r *http.Request) {
		handleSendContactSMS(dependencies.AuthService, dependencies.ContactsService, dependencies.SMSService, w, r)
	})
	mux.HandleFunc("GET /api/email-unsubscribe/{token}", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(rateLimiter, dependencies.Metrics, "public.email-unsubscribe", publicWriteRateLimit, publicRateWindow, "Too many unsubscribe requests", w, r) {
			return
		}
		handleEmailUnsubscribe(dependencies.EmailSuppressionsService, w, r)
	})
	mux.HandleFunc("GET /api/email-messages/open/{trackingToken}", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(rateLimiter, dependencies.Metrics, "public.email-open", trackingRateLimit, publicRateWindow, "Too many email tracking requests", w, r) {
			return
		}
		handleTrackEmailOpen(dependencies.EmailMessagesService, w, r)
	})
	mux.HandleFunc("GET /api/email-messages/click/{clickToken}", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(rateLimiter, dependencies.Metrics, "public.email-click", trackingRateLimit, publicRateWindow, "Too many email tracking requests", w, r) {
			return
		}
		handleTrackEmailClick(dependencies.EmailMessagesService, w, r)
	})
	mux.HandleFunc("GET /api/email-messages", func(w http.ResponseWriter, r *http.Request) {
		handleListEmailMessages(dependencies.AuthService, dependencies.EmailMessagesService, w, r)
	})
	mux.HandleFunc("GET /api/email-messages/{messageID}", func(w http.ResponseWriter, r *http.Request) {
		handleGetEmailMessage(dependencies.AuthService, dependencies.EmailMessagesService, w, r)
	})
	mux.HandleFunc("GET /api/me/email-messages", func(w http.ResponseWriter, r *http.Request) {
		handleListMyEmailMessages(dependencies.AuthService, dependencies.EmailMessagesService, w, r)
	})
	mux.HandleFunc("GET /api/shared-inbox/email-messages", func(w http.ResponseWriter, r *http.Request) {
		handleListSharedInboxMessages(dependencies.AuthService, dependencies.EmailMessagesService, w, r)
	})
	mux.HandleFunc("PATCH /api/email-messages/{messageID}/shared-inbox", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateSharedInboxMessage(dependencies.AuthService, dependencies.EmailMessagesService, w, r)
	})
	mux.HandleFunc("GET /api/companies", func(w http.ResponseWriter, r *http.Request) {
		handleListCompanies(dependencies.AuthService, dependencies.CompaniesService, w, r)
	})
	mux.HandleFunc("GET /api/export/companies", func(w http.ResponseWriter, r *http.Request) {
		handleExportCompanies(dependencies.AuthService, dependencies.ExportsService, w, r)
	})
	mux.HandleFunc("POST /api/companies", func(w http.ResponseWriter, r *http.Request) {
		handleCreateCompany(dependencies.AuthService, dependencies.CompaniesService, w, r)
	})
	mux.HandleFunc("GET /api/companies/{companyID}", func(w http.ResponseWriter, r *http.Request) {
		handleGetCompany(dependencies.AuthService, dependencies.CompaniesService, w, r)
	})
	mux.HandleFunc("PATCH /api/companies/{companyID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateCompany(dependencies.AuthService, dependencies.CompaniesService, w, r)
	})
	mux.HandleFunc("DELETE /api/companies/{companyID}", func(w http.ResponseWriter, r *http.Request) {
		handleArchiveCompany(dependencies.AuthService, dependencies.CompaniesService, w, r)
	})
	mux.HandleFunc("POST /api/companies/{companyID}/email", func(w http.ResponseWriter, r *http.Request) {
		handleSendCompanyEmail(dependencies.AuthService, dependencies.CompaniesService, dependencies.UserEmailService, dependencies.NotesService, dependencies.EmailMessagesService, dependencies.EmailSuppressionsService, w, r)
	})
	mux.HandleFunc("POST /api/companies/{companyID}/linked-contacts", func(w http.ResponseWriter, r *http.Request) {
		handleCreateLinkedCompanyPerson(dependencies.AuthService, dependencies.ContactsService, dependencies.BillingService, w, r)
	})
	mux.HandleFunc("GET /api/deal-pipelines", func(w http.ResponseWriter, r *http.Request) {
		handleListDealPipelines(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("POST /api/deal-pipelines", func(w http.ResponseWriter, r *http.Request) {
		handleCreateDealPipeline(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("PATCH /api/deal-pipelines/{pipelineID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateDealPipeline(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("POST /api/deal-pipelines/{pipelineID}/stages", func(w http.ResponseWriter, r *http.Request) {
		handleCreateDealStage(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("PATCH /api/deal-pipelines/{pipelineID}/stages/{stageID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateDealStageDefinition(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("PUT /api/deal-pipelines/{pipelineID}/stages/order", func(w http.ResponseWriter, r *http.Request) {
		handleReorderDealStages(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("GET /api/deal-stages", func(w http.ResponseWriter, r *http.Request) {
		handleListDealStages(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("GET /api/deals", func(w http.ResponseWriter, r *http.Request) {
		handleListDeals(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("GET /api/export/deals", func(w http.ResponseWriter, r *http.Request) {
		handleExportDeals(dependencies.AuthService, dependencies.ExportsService, w, r)
	})
	mux.HandleFunc("GET /api/deals/{dealID}", func(w http.ResponseWriter, r *http.Request) {
		handleGetDeal(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("GET /api/deals/{dealID}/quote.pdf", func(w http.ResponseWriter, r *http.Request) {
		handleDownloadDealQuotePDF(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("POST /api/deals", func(w http.ResponseWriter, r *http.Request) {
		handleCreateDeal(dependencies.AuthService, dependencies.DealsService, dependencies.NotificationsService, dependencies.BillingService, w, r)
	})
	mux.HandleFunc("PATCH /api/deals/{dealID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateDeal(dependencies.AuthService, dependencies.DealsService, dependencies.NotificationsService, w, r)
	})
	mux.HandleFunc("DELETE /api/deals/{dealID}", func(w http.ResponseWriter, r *http.Request) {
		handleArchiveDeal(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("POST /api/deals/{dealID}/email", func(w http.ResponseWriter, r *http.Request) {
		handleSendDealEmail(dependencies.AuthService, dependencies.DealsService, dependencies.ContactsService, dependencies.UserEmailService, dependencies.NotesService, dependencies.EmailMessagesService, dependencies.EmailSuppressionsService, w, r)
	})
	mux.HandleFunc("PATCH /api/deals/{dealID}/stage", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateDealStage(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("PUT /api/deals/{dealID}/line-items", func(w http.ResponseWriter, r *http.Request) {
		handleReplaceDealLineItems(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("POST /api/deals/{dealID}/signature-requests", func(w http.ResponseWriter, r *http.Request) {
		handleCreateDealSignatureRequest(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("PATCH /api/deals/{dealID}/signature-requests/{requestID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateDealSignatureRequestStatus(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("GET /api/notes", func(w http.ResponseWriter, r *http.Request) {
		handleListNotes(dependencies.AuthService, dependencies.NotesService, w, r)
	})
	mux.HandleFunc("POST /api/notes", func(w http.ResponseWriter, r *http.Request) {
		handleCreateNote(dependencies.AuthService, dependencies.NotesService, w, r)
	})
	mux.HandleFunc("GET /api/record-followers", func(w http.ResponseWriter, r *http.Request) {
		handleGetRecordFollowers(dependencies.AuthService, dependencies.CollaborationService, w, r)
	})
	mux.HandleFunc("PUT /api/record-followers/me", func(w http.ResponseWriter, r *http.Request) {
		handleSetRecordFollowing(dependencies.AuthService, dependencies.CollaborationService, true, w, r)
	})
	mux.HandleFunc("DELETE /api/record-followers/me", func(w http.ResponseWriter, r *http.Request) {
		handleSetRecordFollowing(dependencies.AuthService, dependencies.CollaborationService, false, w, r)
	})
	mux.HandleFunc("GET /api/collaboration/activity-digest", func(w http.ResponseWriter, r *http.Request) {
		handleActivityDigest(dependencies.AuthService, dependencies.CollaborationService, w, r)
	})
	mux.HandleFunc("POST /api/imports/preview", func(w http.ResponseWriter, r *http.Request) {
		handlePreviewImport(dependencies.AuthService, dependencies.ImportsService, w, r)
	})
	mux.HandleFunc("POST /api/imports", func(w http.ResponseWriter, r *http.Request) {
		handleExecuteImport(dependencies.AuthService, dependencies.ImportsService, w, r)
	})
	mux.HandleFunc("GET /api/imports", func(w http.ResponseWriter, r *http.Request) {
		handleListImports(dependencies.AuthService, dependencies.ImportsService, w, r)
	})
	mux.HandleFunc("GET /api/imports/{batchID}/errors.csv", func(w http.ResponseWriter, r *http.Request) {
		handleImportErrorsCSV(dependencies.AuthService, dependencies.ImportsService, w, r)
	})
	mux.HandleFunc("POST /api/imports/{batchID}/rollback", func(w http.ResponseWriter, r *http.Request) {
		handleRollbackImport(dependencies.AuthService, dependencies.ImportsService, w, r)
	})
	mux.HandleFunc("POST /api/data-operations/bulk", func(w http.ResponseWriter, r *http.Request) {
		handleExecuteBulkOperation(dependencies.AuthService, dependencies.BulkOperationsService, w, r)
	})
	mux.HandleFunc("GET /api/data-operations/bulk", func(w http.ResponseWriter, r *http.Request) {
		handleListBulkOperations(dependencies.AuthService, dependencies.BulkOperationsService, w, r)
	})
	mux.HandleFunc("POST /api/data-operations/bulk/{operationID}/rollback", func(w http.ResponseWriter, r *http.Request) {
		handleRollbackBulkOperation(dependencies.AuthService, dependencies.BulkOperationsService, w, r)
	})
	mux.HandleFunc("GET /api/data-operations/archive", func(w http.ResponseWriter, r *http.Request) {
		handleListArchivedRecords(dependencies.AuthService, dependencies.ArchiveOperationsService, w, r)
	})
	mux.HandleFunc("POST /api/data-operations/archive/{entityType}/{entityID}/restore", func(w http.ResponseWriter, r *http.Request) {
		handleRestoreArchivedRecord(dependencies.AuthService, dependencies.ArchiveOperationsService, w, r)
	})
	mux.HandleFunc("GET /api/data-operations/duplicates", func(w http.ResponseWriter, r *http.Request) {
		handleReviewDuplicates(dependencies.AuthService, dependencies.DuplicateOperationsService, w, r)
	})
	mux.HandleFunc("POST /api/data-operations/duplicates/merge", func(w http.ResponseWriter, r *http.Request) {
		handleMergeDuplicate(dependencies.AuthService, dependencies.DuplicateOperationsService, w, r)
	})
	mux.HandleFunc("GET /api/custom-fields", func(w http.ResponseWriter, r *http.Request) {
		handleListCustomFields(dependencies.AuthService, dependencies.CustomFieldsService, w, r)
	})
	mux.HandleFunc("POST /api/custom-fields", func(w http.ResponseWriter, r *http.Request) {
		handleCreateCustomField(dependencies.AuthService, dependencies.CustomFieldsService, w, r)
	})
	mux.HandleFunc("PATCH /api/custom-fields/{definitionID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateCustomField(dependencies.AuthService, dependencies.CustomFieldsService, w, r)
	})
	mux.HandleFunc("DELETE /api/custom-fields/{definitionID}", func(w http.ResponseWriter, r *http.Request) {
		handleArchiveCustomField(dependencies.AuthService, dependencies.CustomFieldsService, w, r)
	})
	mux.HandleFunc("GET /api/saved-views", func(w http.ResponseWriter, r *http.Request) {
		handleListSavedViews(dependencies.AuthService, dependencies.SavedViewsService, w, r)
	})
	mux.HandleFunc("POST /api/saved-views", func(w http.ResponseWriter, r *http.Request) {
		handleCreateSavedView(dependencies.AuthService, dependencies.SavedViewsService, w, r)
	})
	mux.HandleFunc("PATCH /api/saved-views/{viewID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateSavedView(dependencies.AuthService, dependencies.SavedViewsService, w, r)
	})
	mux.HandleFunc("DELETE /api/saved-views/{viewID}", func(w http.ResponseWriter, r *http.Request) {
		handleDeleteSavedView(dependencies.AuthService, dependencies.SavedViewsService, w, r)
	})
	mux.HandleFunc("GET /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		handleListTasks(dependencies.AuthService, dependencies.TasksService, w, r)
	})
	mux.HandleFunc("GET /api/export/tasks", func(w http.ResponseWriter, r *http.Request) {
		handleExportTasks(dependencies.AuthService, dependencies.ExportsService, w, r)
	})
	mux.HandleFunc("GET /api/tasks/{taskID}", func(w http.ResponseWriter, r *http.Request) {
		handleGetTask(dependencies.AuthService, dependencies.TasksService, w, r)
	})
	mux.HandleFunc("POST /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		handleCreateTask(dependencies.AuthService, dependencies.TasksService, w, r)
	})
	mux.HandleFunc("PATCH /api/tasks/{taskID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateTask(dependencies.AuthService, dependencies.TasksService, w, r)
	})
	mux.HandleFunc("DELETE /api/tasks/{taskID}", func(w http.ResponseWriter, r *http.Request) {
		handleArchiveTask(dependencies.AuthService, dependencies.TasksService, w, r)
	})
	mux.HandleFunc("GET /api/dashboard/summary", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardSummary(dependencies.AuthService, dependencies.DashboardService, w, r)
	})
	mux.HandleFunc("PUT /api/dashboard/sales-quotas/{userID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpsertDashboardSalesQuota(dependencies.AuthService, dependencies.DashboardService, w, r)
	})
	mux.HandleFunc("GET /api/organization/profile", func(w http.ResponseWriter, r *http.Request) {
		handleGetOrganizationProfile(dependencies.AuthService, dependencies.OrgProfileService, w, r)
	})
	mux.HandleFunc("PATCH /api/organization/profile", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateOrganizationProfile(dependencies.AuthService, dependencies.OrgProfileService, dependencies.AuditService, w, r)
	})
	mux.HandleFunc("PUT /api/organization/exchange-rates/{quoteCurrency}", func(w http.ResponseWriter, r *http.Request) {
		handleUpsertOrganizationExchangeRate(dependencies.AuthService, dependencies.OrgProfileService, dependencies.AuditService, w, r)
	})
	mux.HandleFunc("GET /api/notifications", func(w http.ResponseWriter, r *http.Request) {
		handleListNotifications(dependencies.AuthService, dependencies.NotificationsService, w, r)
	})
	mux.HandleFunc("GET /api/notifications/unread-count", func(w http.ResponseWriter, r *http.Request) {
		handleGetNotificationUnreadCount(dependencies.AuthService, dependencies.NotificationsService, w, r)
	})
	mux.HandleFunc("PATCH /api/notifications/{notificationID}/read", func(w http.ResponseWriter, r *http.Request) {
		handleMarkNotificationRead(dependencies.AuthService, dependencies.NotificationsService, w, r)
	})
	mux.HandleFunc("POST /api/notifications/read-all", func(w http.ResponseWriter, r *http.Request) {
		handleMarkAllNotificationsRead(dependencies.AuthService, dependencies.NotificationsService, w, r)
	})
}
