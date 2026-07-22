package app

import "net/http"

func registerFoundationRoutes(mux *http.ServeMux, dependencies Dependencies, rateLimiter rateLimitService) {
	mux.HandleFunc("GET /api/quote-templates", func(w http.ResponseWriter, r *http.Request) {
		handleListQuoteTemplates(dependencies.AuthService, dependencies.QuoteTemplatesService, w, r)
	})
	mux.HandleFunc("GET /api/quote-templates/policy", func(w http.ResponseWriter, r *http.Request) {
		handleGetQuoteTemplatePolicy(dependencies.AuthService, dependencies.QuoteTemplatesService, w, r)
	})
	mux.HandleFunc("GET /api/quote-templates/merge-tokens", func(w http.ResponseWriter, r *http.Request) {
		handleListQuoteTemplateMergeTokens(dependencies.AuthService, w, r)
	})
	mux.HandleFunc("POST /api/quote-templates", func(w http.ResponseWriter, r *http.Request) {
		handleCreateQuoteTemplate(dependencies.AuthService, dependencies.QuoteTemplatesService, w, r)
	})
	mux.HandleFunc("PATCH /api/quote-templates/{templateID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateQuoteTemplate(dependencies.AuthService, dependencies.QuoteTemplatesService, w, r)
	})
	mux.HandleFunc("DELETE /api/quote-templates/{templateID}", func(w http.ResponseWriter, r *http.Request) {
		handleArchiveQuoteTemplate(dependencies.AuthService, dependencies.QuoteTemplatesService, w, r)
	})
	mux.HandleFunc("PUT /api/quote-templates/policy", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateQuoteTemplatePolicy(dependencies.AuthService, dependencies.QuoteTemplatesService, w, r)
	})
	mux.HandleFunc("GET /api/email-templates", func(w http.ResponseWriter, r *http.Request) {
		handleListEmailTemplates(dependencies.AuthService, dependencies.EmailTemplatesService, w, r)
	})
	mux.HandleFunc("GET /api/email-templates/merge-fields", func(w http.ResponseWriter, r *http.Request) {
		handleListEmailTemplateMergeFields(dependencies.AuthService, dependencies.CustomFieldsService, w, r)
	})
	mux.HandleFunc("POST /api/email-templates", func(w http.ResponseWriter, r *http.Request) {
		handleCreateEmailTemplate(dependencies.AuthService, dependencies.EmailTemplatesService, w, r)
	})
	mux.HandleFunc("PATCH /api/email-templates/{templateID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateEmailTemplate(dependencies.AuthService, dependencies.EmailTemplatesService, w, r)
	})
	mux.HandleFunc("DELETE /api/email-templates/{templateID}", func(w http.ResponseWriter, r *http.Request) {
		handleDeleteEmailTemplate(dependencies.AuthService, dependencies.EmailTemplatesService, w, r)
	})
	mux.HandleFunc("GET /api/email-snippets", func(w http.ResponseWriter, r *http.Request) {
		handleListEmailSnippets(dependencies.AuthService, dependencies.EmailTemplatesService, w, r)
	})
	mux.HandleFunc("POST /api/email-snippets", func(w http.ResponseWriter, r *http.Request) {
		handleCreateEmailSnippet(dependencies.AuthService, dependencies.EmailTemplatesService, w, r)
	})
	mux.HandleFunc("PATCH /api/email-snippets/{snippetID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateEmailSnippet(dependencies.AuthService, dependencies.EmailTemplatesService, w, r)
	})
	mux.HandleFunc("DELETE /api/email-snippets/{snippetID}", func(w http.ResponseWriter, r *http.Request) {
		handleDeleteEmailSnippet(dependencies.AuthService, dependencies.EmailTemplatesService, w, r)
	})
	mux.HandleFunc("GET /api/product-catalog-items", func(w http.ResponseWriter, r *http.Request) {
		handleListProductCatalogItems(dependencies.AuthService, dependencies.ProductCatalogService, w, r)
	})
	mux.HandleFunc("POST /api/product-catalog-items", func(w http.ResponseWriter, r *http.Request) {
		handleCreateProductCatalogItem(dependencies.AuthService, dependencies.ProductCatalogService, w, r)
	})
	mux.HandleFunc("PATCH /api/product-catalog-items/{itemID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateProductCatalogItem(dependencies.AuthService, dependencies.ProductCatalogService, w, r)
	})
	mux.HandleFunc("DELETE /api/product-catalog-items/{itemID}", func(w http.ResponseWriter, r *http.Request) {
		handleArchiveProductCatalogItem(dependencies.AuthService, dependencies.ProductCatalogService, w, r)
	})
	mux.HandleFunc("GET /api/lead-capture-forms", func(w http.ResponseWriter, r *http.Request) {
		handleListLeadCaptureForms(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("POST /api/lead-capture-forms", func(w http.ResponseWriter, r *http.Request) {
		handleCreateLeadCaptureForm(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("PATCH /api/lead-capture-forms/{formID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateLeadCaptureForm(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("GET /api/lead-capture-submissions", func(w http.ResponseWriter, r *http.Request) {
		handleListLeadSubmissionReviews(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("POST /api/lead-capture-submissions/{submissionID}/review", func(w http.ResponseWriter, r *http.Request) {
		handleReviewLeadSubmission(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("GET /api/lead-landing-pages", func(w http.ResponseWriter, r *http.Request) {
		handleListLeadLandingPages(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("POST /api/lead-landing-pages", func(w http.ResponseWriter, r *http.Request) {
		handleCreateLeadLandingPage(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("PATCH /api/lead-landing-pages/{pageID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateLeadLandingPage(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("GET /api/public/landing-pages/{slug}", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(rateLimiter, dependencies.Metrics, "public.landing-page", publicReadRateLimit, publicRateWindow, "Too many public page requests", w, r) {
			return
		}
		handleGetPublicLeadLandingPage(dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("POST /api/public/lead-capture-forms/{publicID}/submissions", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(rateLimiter, dependencies.Metrics, "public.lead-submission", publicWriteRateLimit, publicRateWindow, "Too many lead submissions", w, r) {
			return
		}
		handleSubmitPublicLeadCaptureForm(dependencies.LeadFormsService, dependencies.Metrics, w, r)
	})
	mux.HandleFunc("POST /api/public/lead-capture-forms/{publicID}/challenge", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(rateLimiter, dependencies.Metrics, "public.lead-challenge", publicWriteRateLimit, publicRateWindow, "Too many lead form challenges", w, r) {
			return
		}
		handleIssuePublicLeadSubmissionChallenge(dependencies.LeadFormsService, dependencies.Metrics, w, r)
	})
	mux.HandleFunc("GET /api/lead-chat-widgets", func(w http.ResponseWriter, r *http.Request) {
		handleListLeadChatWidgets(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("POST /api/lead-chat-widgets", func(w http.ResponseWriter, r *http.Request) {
		handleCreateLeadChatWidget(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("PATCH /api/lead-chat-widgets/{widgetID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateLeadChatWidget(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("GET /api/public/lead-chat-widgets/{publicID}", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(rateLimiter, dependencies.Metrics, "public.lead-widget", publicReadRateLimit, publicRateWindow, "Too many public widget requests", w, r) {
			return
		}
		handleGetPublicLeadChatWidget(dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("GET /api/lead-audiences", func(w http.ResponseWriter, r *http.Request) {
		handleListLeadAudiences(dependencies.AuthService, dependencies.LeadAudiencesService, w, r)
	})
	mux.HandleFunc("POST /api/lead-audiences", func(w http.ResponseWriter, r *http.Request) {
		handleCreateLeadAudience(dependencies.AuthService, dependencies.LeadAudiencesService, w, r)
	})
	mux.HandleFunc("POST /api/lead-audiences/preview", func(w http.ResponseWriter, r *http.Request) {
		handlePreviewLeadAudience(dependencies.AuthService, dependencies.LeadAudiencesService, w, r)
	})
	mux.HandleFunc("PATCH /api/lead-audiences/{audienceID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateLeadAudience(dependencies.AuthService, dependencies.LeadAudiencesService, w, r)
	})
	mux.HandleFunc("GET /api/marketing-email-campaigns", func(w http.ResponseWriter, r *http.Request) {
		handleListMarketingCampaigns(dependencies.AuthService, dependencies.MarketingCampaignsService, w, r)
	})
	mux.HandleFunc("POST /api/marketing-email-campaigns", func(w http.ResponseWriter, r *http.Request) {
		handleCreateMarketingCampaign(dependencies.AuthService, dependencies.MarketingCampaignsService, w, r)
	})
	mux.HandleFunc("PATCH /api/marketing-email-campaigns/{campaignID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateMarketingCampaign(dependencies.AuthService, dependencies.MarketingCampaignsService, w, r)
	})
	mux.HandleFunc("GET /api/lead-nurture-campaigns", func(w http.ResponseWriter, r *http.Request) {
		handleListNurtureCampaigns(dependencies.AuthService, dependencies.NurtureCampaignsService, w, r)
	})
	mux.HandleFunc("POST /api/lead-nurture-campaigns", func(w http.ResponseWriter, r *http.Request) {
		handleCreateNurtureCampaign(dependencies.AuthService, dependencies.NurtureCampaignsService, w, r)
	})
	mux.HandleFunc("PATCH /api/lead-nurture-campaigns/{campaignID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateNurtureCampaign(dependencies.AuthService, dependencies.NurtureCampaignsService, w, r)
	})
	mux.HandleFunc("GET /api/lead-scoring-rules", func(w http.ResponseWriter, r *http.Request) {
		handleListLeadScoringRules(dependencies.AuthService, dependencies.LeadScoringService, w, r)
	})
	mux.HandleFunc("POST /api/lead-scoring-rules", func(w http.ResponseWriter, r *http.Request) {
		handleCreateLeadScoringRule(dependencies.AuthService, dependencies.LeadScoringService, w, r)
	})
	mux.HandleFunc("PATCH /api/lead-scoring-rules/{ruleID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateLeadScoringRule(dependencies.AuthService, dependencies.LeadScoringService, w, r)
	})
	mux.HandleFunc("POST /api/contacts/{contactID}/lead-score", func(w http.ResponseWriter, r *http.Request) {
		handleEvaluateContactLeadScore(dependencies.AuthService, dependencies.LeadScoringService, w, r)
	})
	mux.HandleFunc("GET /api/workflow-automations", func(w http.ResponseWriter, r *http.Request) {
		handleListWorkflowAutomations(dependencies.AuthService, dependencies.WorkflowAutomationsService, w, r)
	})
	mux.HandleFunc("GET /api/workflow-automation-runs", func(w http.ResponseWriter, r *http.Request) {
		handleListWorkflowAutomationRuns(dependencies.AuthService, dependencies.WorkflowAutomationsService, w, r)
	})
	mux.HandleFunc("GET /api/workflow-approvals", func(w http.ResponseWriter, r *http.Request) {
		handleListWorkflowApprovals(dependencies.AuthService, dependencies.WorkflowAutomationsService, w, r)
	})
	mux.HandleFunc("POST /api/workflow-approvals/{approvalID}/decision", func(w http.ResponseWriter, r *http.Request) {
		handleDecideWorkflowApproval(dependencies.AuthService, dependencies.WorkflowAutomationsService, w, r)
	})
	mux.HandleFunc("POST /api/workflow-automations", func(w http.ResponseWriter, r *http.Request) {
		handleCreateWorkflowAutomation(dependencies.AuthService, dependencies.WorkflowAutomationsService, w, r)
	})
	mux.HandleFunc("PATCH /api/workflow-automations/{automationID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateWorkflowAutomation(dependencies.AuthService, dependencies.WorkflowAutomationsService, w, r)
	})
	mux.HandleFunc("GET /api/report-definitions", func(w http.ResponseWriter, r *http.Request) {
		handleListCustomReportDefinitions(dependencies.AuthService, dependencies.CustomReportsService, w, r)
	})
	mux.HandleFunc("POST /api/report-definitions", func(w http.ResponseWriter, r *http.Request) {
		handleCreateCustomReportDefinition(dependencies.AuthService, dependencies.CustomReportsService, w, r)
	})
	mux.HandleFunc("PATCH /api/report-definitions/{definitionID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateCustomReportDefinition(dependencies.AuthService, dependencies.CustomReportsService, w, r)
	})
	mux.HandleFunc("GET /api/report-definitions/{definitionID}/results", func(w http.ResponseWriter, r *http.Request) {
		handleExecuteCustomReport(dependencies.AuthService, dependencies.CustomReportsService, w, r)
	})
	mux.HandleFunc("GET /api/report-definitions/{definitionID}/export.csv", func(w http.ResponseWriter, r *http.Request) {
		handleExportCustomReport(dependencies.AuthService, dependencies.CustomReportsService, w, r)
	})
	mux.HandleFunc("GET /api/data-quality/summary", func(w http.ResponseWriter, r *http.Request) {
		handleDataQualitySummary(dependencies.AuthService, dependencies.DataQualityService, w, r)
	})
	mux.HandleFunc("GET /api/reports/sales-activity", func(w http.ResponseWriter, r *http.Request) {
		handleSalesActivityReport(dependencies.AuthService, dependencies.SalesReportsService, w, r)
	})
	mux.HandleFunc("GET /api/reports/pipeline-funnel", func(w http.ResponseWriter, r *http.Request) {
		handlePipelineFunnelReport(dependencies.AuthService, dependencies.SalesReportsService, w, r)
	})
	mux.HandleFunc("GET /api/reports/follow-up", func(w http.ResponseWriter, r *http.Request) {
		handleStaleTouchpoints(dependencies.AuthService, dependencies.TouchpointsService, w, r)
	})
	mux.HandleFunc("GET /api/reports/client-activity", func(w http.ResponseWriter, r *http.Request) {
		handleClientActivity(dependencies.AuthService, dependencies.TouchpointsService, w, r)
	})
	mux.HandleFunc("GET /api/reports/client-health", func(w http.ResponseWriter, r *http.Request) {
		handleClientHealth(dependencies.AuthService, dependencies.TouchpointsService, w, r)
	})
	mux.HandleFunc("GET /api/touchpoints/{entityType}/{entityID}", func(w http.ResponseWriter, r *http.Request) {
		handleTouchpointSummary(dependencies.AuthService, dependencies.TouchpointsService, w, r)
	})
	mux.HandleFunc("GET /api/client-reviews/{entityType}/{entityID}", func(w http.ResponseWriter, r *http.Request) {
		handleGetClientReview(dependencies.AuthService, dependencies.ClientReviewsService, w, r)
	})
	mux.HandleFunc("PUT /api/client-reviews/{entityType}/{entityID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpsertClientReview(dependencies.AuthService, dependencies.ClientReviewsService, w, r)
	})
	mux.HandleFunc("DELETE /api/client-reviews/{entityType}/{entityID}", func(w http.ResponseWriter, r *http.Request) {
		handleDeleteClientReview(dependencies.AuthService, dependencies.ClientReviewsService, w, r)
	})
	mux.HandleFunc("GET /api/email-sequences", func(w http.ResponseWriter, r *http.Request) {
		handleListEmailSequences(dependencies.AuthService, dependencies.EmailSequencesService, w, r)
	})
	mux.HandleFunc("POST /api/email-sequences", func(w http.ResponseWriter, r *http.Request) {
		handleCreateEmailSequence(dependencies.AuthService, dependencies.EmailSequencesService, w, r)
	})
	mux.HandleFunc("PATCH /api/email-sequences/{sequenceID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateEmailSequence(dependencies.AuthService, dependencies.EmailSequencesService, w, r)
	})
	mux.HandleFunc("DELETE /api/email-sequences/{sequenceID}", func(w http.ResponseWriter, r *http.Request) {
		handleDeleteEmailSequence(dependencies.AuthService, dependencies.EmailSequencesService, w, r)
	})
	mux.HandleFunc("POST /api/email-sequences/{sequenceID}/approve", func(w http.ResponseWriter, r *http.Request) {
		handleApproveEmailSequence(dependencies.AuthService, dependencies.EmailSequencesService, w, r)
	})
	mux.HandleFunc("POST /api/email-sequences/{sequenceID}/pause", func(w http.ResponseWriter, r *http.Request) {
		handlePauseEmailSequence(dependencies.AuthService, dependencies.EmailSequencesService, w, r)
	})
	mux.HandleFunc("GET /api/email-sequence-enrollments", func(w http.ResponseWriter, r *http.Request) {
		handleListEmailSequenceEnrollments(dependencies.AuthService, dependencies.EmailSequenceEnrollmentsService, w, r)
	})
	mux.HandleFunc("POST /api/email-sequence-enrollments", func(w http.ResponseWriter, r *http.Request) {
		handleCreateEmailSequenceEnrollment(dependencies.AuthService, dependencies.EmailSequenceEnrollmentsService, w, r)
	})
	mux.HandleFunc("DELETE /api/email-sequence-enrollments/{enrollmentID}", func(w http.ResponseWriter, r *http.Request) {
		handleCancelEmailSequenceEnrollment(dependencies.AuthService, dependencies.EmailSequenceEnrollmentsService, w, r)
	})
	mux.HandleFunc("GET /api/calls", func(w http.ResponseWriter, r *http.Request) {
		handleListCallLogs(dependencies.AuthService, dependencies.CallLogsService, w, r)
	})
	mux.HandleFunc("POST /api/calls/start", func(w http.ResponseWriter, r *http.Request) {
		handleStartCall(dependencies.AuthService, dependencies.CallLogsService, w, r)
	})
	mux.HandleFunc("POST /api/calls/log", func(w http.ResponseWriter, r *http.Request) {
		handleRecordCall(dependencies.AuthService, dependencies.CallLogsService, w, r)
	})
	mux.HandleFunc("PATCH /api/calls/{callID}/complete", func(w http.ResponseWriter, r *http.Request) {
		handleCompleteCall(dependencies.AuthService, dependencies.CallLogsService, w, r)
	})
	mux.HandleFunc("PATCH /api/calls/{callID}/recording", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateCallRecording(dependencies.AuthService, dependencies.CallLogsService, w, r)
	})
	mux.HandleFunc("GET /api/sms-messages", func(w http.ResponseWriter, r *http.Request) {
		handleListSMSMessages(dependencies.AuthService, dependencies.SMSService, w, r)
	})
	mux.HandleFunc("POST /api/sms-messages/log", func(w http.ResponseWriter, r *http.Request) {
		handleRecordInboundSMS(dependencies.AuthService, dependencies.SMSService, w, r)
	})
	mux.HandleFunc("POST /api/sms/opt-outs", func(w http.ResponseWriter, r *http.Request) {
		handleSMSOptOut(dependencies.AuthService, dependencies.SMSService, w, r)
	})
	mux.HandleFunc("GET /api/calendar-events", func(w http.ResponseWriter, r *http.Request) {
		handleListCalendarEvents(dependencies.AuthService, dependencies.CalendarService, w, r)
	})
	mux.HandleFunc("POST /api/calendar-events", func(w http.ResponseWriter, r *http.Request) {
		handleScheduleCalendarEvent(dependencies.AuthService, dependencies.CalendarService, w, r)
	})
	mux.HandleFunc("PATCH /api/calendar-events/{eventID}/cancel", func(w http.ResponseWriter, r *http.Request) {
		handleCancelCalendarEvent(dependencies.AuthService, dependencies.CalendarService, w, r)
	})
	mux.HandleFunc("GET /api/me/calendar-availability", func(w http.ResponseWriter, r *http.Request) {
		handleListCalendarAvailability(dependencies.AuthService, dependencies.CalendarService, w, r)
	})
	mux.HandleFunc("PUT /api/me/calendar-availability", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateCalendarAvailability(dependencies.AuthService, dependencies.CalendarService, w, r)
	})
	mux.HandleFunc("GET /api/calendar-booking-links", func(w http.ResponseWriter, r *http.Request) {
		handleListCalendarBookingLinks(dependencies.AuthService, dependencies.CalendarService, w, r)
	})
	mux.HandleFunc("POST /api/calendar-booking-links", func(w http.ResponseWriter, r *http.Request) {
		handleCreateCalendarBookingLink(dependencies.AuthService, dependencies.CalendarService, w, r)
	})
	mux.HandleFunc("PATCH /api/calendar-booking-links/{bookingLinkID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateCalendarBookingLink(dependencies.AuthService, dependencies.CalendarService, w, r)
	})
}
