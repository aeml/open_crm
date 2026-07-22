package app

import (
	"context"
	"log/slog"

	modulearchiveoperations "github.com/aeml/open_crm/apps/api/internal/modules/archiveoperations"
	moduleaudit "github.com/aeml/open_crm/apps/api/internal/modules/audit"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulebulkoperations "github.com/aeml/open_crm/apps/api/internal/modules/bulkoperations"
	modulecalendar "github.com/aeml/open_crm/apps/api/internal/modules/calendar"
	modulecalllogs "github.com/aeml/open_crm/apps/api/internal/modules/calllogs"
	moduleclientreviews "github.com/aeml/open_crm/apps/api/internal/modules/clientreviews"
	modulecollaboration "github.com/aeml/open_crm/apps/api/internal/modules/collaboration"
	modulecompanies "github.com/aeml/open_crm/apps/api/internal/modules/companies"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	modulecustomreports "github.com/aeml/open_crm/apps/api/internal/modules/customreports"
	moduledashboard "github.com/aeml/open_crm/apps/api/internal/modules/dashboard"
	moduledataquality "github.com/aeml/open_crm/apps/api/internal/modules/dataquality"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleduplicates "github.com/aeml/open_crm/apps/api/internal/modules/duplicateoperations"
	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	moduleemailfeedback "github.com/aeml/open_crm/apps/api/internal/modules/emailfeedback"
	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
	moduleemailsuppressions "github.com/aeml/open_crm/apps/api/internal/modules/emailsuppressions"
	moduleemailtemplates "github.com/aeml/open_crm/apps/api/internal/modules/emailtemplates"
	moduleexports "github.com/aeml/open_crm/apps/api/internal/modules/exports"
	moduleimports "github.com/aeml/open_crm/apps/api/internal/modules/imports"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
	moduleleadaudiences "github.com/aeml/open_crm/apps/api/internal/modules/leadaudiences"
	moduleleadforms "github.com/aeml/open_crm/apps/api/internal/modules/leadforms"
	moduleleadscoring "github.com/aeml/open_crm/apps/api/internal/modules/leadscoring"
	modulemailboxsync "github.com/aeml/open_crm/apps/api/internal/modules/mailboxsync"
	modulemarketingcampaigns "github.com/aeml/open_crm/apps/api/internal/modules/marketingcampaigns"
	modulenotifications "github.com/aeml/open_crm/apps/api/internal/modules/notifications"
	modulenurturecampaigns "github.com/aeml/open_crm/apps/api/internal/modules/nurturecampaigns"
	moduleonboarding "github.com/aeml/open_crm/apps/api/internal/modules/onboarding"
	moduleorgprofile "github.com/aeml/open_crm/apps/api/internal/modules/orgprofile"
	modulepasswordreset "github.com/aeml/open_crm/apps/api/internal/modules/passwordreset"
	moduleproductcatalog "github.com/aeml/open_crm/apps/api/internal/modules/productcatalog"
	modulesalesreports "github.com/aeml/open_crm/apps/api/internal/modules/salesreports"
	modulesavedviews "github.com/aeml/open_crm/apps/api/internal/modules/savedviews"
	modulesms "github.com/aeml/open_crm/apps/api/internal/modules/sms"
	moduletasks "github.com/aeml/open_crm/apps/api/internal/modules/tasks"
	moduletouchpoints "github.com/aeml/open_crm/apps/api/internal/modules/touchpoints"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
	moduleworkflowautomations "github.com/aeml/open_crm/apps/api/internal/modules/workflowautomations"
	moduleworkspaceexports "github.com/aeml/open_crm/apps/api/internal/modules/workspaceexports"
	platformtelemetry "github.com/aeml/open_crm/apps/api/internal/platform/telemetry"
)

type authService interface {
	Login(context.Context, string, string) (moduleauth.LoginResult, error)
	CurrentSession(context.Context, string) (moduleauth.SessionState, error)
	Logout(context.Context, string) error
	ListSessions(context.Context, int64, string) ([]moduleauth.SessionSummary, error)
	RevokeSession(context.Context, int64, int64, string) error
	RevokeOtherSessions(context.Context, int64, string) (int64, error)
}

type usersService interface {
	ListByOrganization(context.Context, int64) ([]moduleusers.UserSummary, error)
	CreateForOrganization(context.Context, int64, moduleusers.CreateUserInput) (moduleusers.UserSummary, error)
	ResendInvitation(context.Context, int64, int64, int64) (moduleusers.UserSummary, error)
	RecordInvitationDelivery(context.Context, int64, int64, string, string, string) (string, error)
	RevokeInvitation(context.Context, int64, int64, int64) (moduleusers.LifecycleResult, error)
	UpdateRole(context.Context, int64, int64, int64, string) (moduleusers.UserSummary, error)
	SetStatus(context.Context, int64, int64, int64, moduleusers.SetStatusInput) (moduleusers.LifecycleResult, error)
	CompleteSetup(context.Context, moduleusers.CompleteSetupInput) (moduleusers.SetupCompletion, error)
	UpdateProfile(context.Context, int64, moduleusers.UpdateProfileInput) (moduleusers.UserProfile, error)
	GetPreferences(context.Context, int64) (moduleusers.UserPreferences, error)
	UpdatePreferences(context.Context, int64, moduleusers.UserPreferences) (moduleusers.UserPreferences, error)
}

type auditService interface {
	ListByOrganization(context.Context, int64, moduleaudit.ListQuery) ([]moduleaudit.Event, error)
	ExportCSV(context.Context, int64, moduleaudit.ListQuery) (moduleaudit.File, error)
	Record(context.Context, int64, moduleaudit.RecordInput) error
}

type backgroundJobsService interface {
	List(context.Context, int64, modulejobs.ListQuery) ([]modulejobs.Job, error)
	Stats(context.Context, int64) (modulejobs.QueueStats, error)
	Replay(context.Context, int64, int64) (modulejobs.Job, error)
}

type sequenceDeliveryOperationsService interface {
	ResolveUncertainDeliveryJob(context.Context, int64, int64, string) (moduleemailsequences.DeliveryResolution, error)
}

type contactsService interface {
	ListByOrganization(context.Context, int64, modulecontacts.ListQuery) (modulecontacts.ListResult, error)
	GetByID(context.Context, int64, int64) (modulecontacts.Detail, error)
	Create(context.Context, int64, int64, modulecontacts.CreateInput) (modulecontacts.Detail, error)
	CreateLinkedCompanyPerson(context.Context, int64, int64, int64, modulecontacts.CreateInput) (modulecontacts.LinkedCompanyPersonResult, error)
	Update(context.Context, int64, int64, int64, modulecontacts.UpdateInput) (modulecontacts.Detail, error)
	Archive(context.Context, int64, int64, int64) error
}

type companiesService interface {
	ListByOrganization(context.Context, int64, modulecompanies.ListQuery) (modulecompanies.ListResult, error)
	GetByID(context.Context, int64, int64) (modulecompanies.Detail, error)
	Create(context.Context, int64, int64, modulecompanies.CreateInput) (modulecompanies.Detail, error)
	Update(context.Context, int64, int64, int64, modulecompanies.UpdateInput) (modulecompanies.Detail, error)
	Archive(context.Context, int64, int64, int64) error
}

type dealsService interface {
	ListPipelinesByOrganization(context.Context, int64) ([]moduledeals.Pipeline, error)
	CreatePipeline(context.Context, int64, int64, moduledeals.PipelineInput) (moduledeals.Pipeline, error)
	UpdatePipeline(context.Context, int64, int64, int64, moduledeals.PipelineUpdateInput) (moduledeals.Pipeline, error)
	CreateStage(context.Context, int64, int64, int64, moduledeals.StageDefinitionInput) (moduledeals.Pipeline, error)
	UpdateStageDefinition(context.Context, int64, int64, int64, int64, moduledeals.StageDefinitionInput) (moduledeals.Pipeline, error)
	ReorderStages(context.Context, int64, int64, int64, moduledeals.StageOrderInput) (moduledeals.Pipeline, error)
	ListStagesByOrganization(context.Context, int64) ([]moduledeals.Stage, error)
	ListByOrganization(context.Context, int64, moduledeals.ListQuery) (moduledeals.ListResult, error)
	GetByID(context.Context, int64, int64) (moduledeals.Detail, error)
	Create(context.Context, int64, int64, moduledeals.CreateInput) (moduledeals.Detail, error)
	Update(context.Context, int64, int64, int64, moduledeals.UpdateInput) (moduledeals.Detail, error)
	Archive(context.Context, int64, int64, int64) error
	UpdateStage(context.Context, int64, int64, int64, moduledeals.UpdateStageInput) (moduledeals.Detail, error)
	ReplaceLineItems(context.Context, int64, int64, int64, moduledeals.LineItemsInput) (moduledeals.Detail, error)
	FinalizeQuote(context.Context, int64, int64, int64, moduledeals.FinalizeQuoteInput) (moduledeals.QuoteVersion, error)
	ReissueExpiredQuote(context.Context, int64, int64, int64, int64, moduledeals.ReissueQuoteInput) (moduledeals.QuoteVersion, error)
	DecideQuoteApproval(context.Context, int64, int64, int64, int64, moduledeals.QuoteApprovalDecisionInput) (moduledeals.QuoteVersion, error)
	ListPendingQuoteApprovals(context.Context, int64) ([]moduledeals.PendingQuoteApproval, error)
	GetQuotePDF(context.Context, int64, int64, int64) (moduledeals.QuotePDFFile, error)
	ReplayQuoteDelivery(context.Context, int64, int64, int64, int64, moduledeals.QuoteDeliveryInput) (moduledeals.QuoteDeliveryIntent, bool, error)
	PrepareQuoteDelivery(context.Context, int64, int64, int64, int64, moduledeals.QuoteDeliveryInput) (moduledeals.QuoteDeliveryIntent, error)
	ClaimQuoteDelivery(context.Context, int64, int64, int64) (moduledeals.QuoteDeliveryIntent, bool, error)
	CompleteQuoteDelivery(context.Context, int64, int64, moduleuseremail.SendReceipt) (moduledeals.QuoteDelivery, error)
	FailQuoteDelivery(context.Context, int64, int64, error, bool) (moduledeals.QuoteDelivery, error)
	ResolveQuoteDelivery(context.Context, int64, int64, int64, string) (moduledeals.QuoteDeliveryResolution, error)
	GetPublicQuote(context.Context, string) (moduledeals.PublicQuote, error)
	GetPublicQuotePDF(context.Context, string) (moduledeals.QuotePDFFile, error)
	ConfirmPublicQuoteReceipt(context.Context, string) (moduledeals.PublicQuote, error)
	SignPublicQuote(context.Context, string, moduledeals.SignatureCompletionInput) (moduledeals.PublicQuote, error)
	DeclinePublicQuote(context.Context, string, moduledeals.SignatureDeclineInput) (moduledeals.PublicQuote, error)
	GetSignatureCertificate(context.Context, int64, int64, int64) (moduledeals.QuotePDFFile, error)
	GetPublicSignatureCertificate(context.Context, string) (moduledeals.QuotePDFFile, error)
	VoidSignatureRequest(context.Context, int64, int64, int64, int64) (moduledeals.Detail, error)
	ConvertSignedQuoteToWon(context.Context, int64, int64, int64, int64, moduledeals.SignatureConversionInput) (moduledeals.Detail, error)
}

type tasksService interface {
	ListByOrganization(context.Context, int64, moduletasks.ListQuery) (moduletasks.ListResult, error)
	GetByID(context.Context, int64, int64) (moduletasks.Detail, error)
	Archive(context.Context, int64, int64, int64) error
	Create(context.Context, int64, int64, moduletasks.CreateInput) (moduletasks.Detail, error)
	Update(context.Context, int64, int64, int64, moduletasks.UpdateInput) (moduletasks.Detail, error)
}

type dataExportsService interface {
	ContactsCSV(context.Context, int64, moduleexports.ContactsQuery) (moduleexports.File, error)
	CompaniesCSV(context.Context, int64, moduleexports.CompaniesQuery) (moduleexports.File, error)
	DealsCSV(context.Context, int64, moduleexports.DealsQuery) (moduleexports.File, error)
	TasksCSV(context.Context, int64, moduleexports.TasksQuery) (moduleexports.File, error)
}

type workspaceExportsService interface {
	Request(context.Context, int64, int64, string) (moduleworkspaceexports.Export, error)
	List(context.Context, int64) ([]moduleworkspaceexports.Export, error)
	Download(context.Context, int64, int64, int64) (moduleworkspaceexports.Download, error)
}

type orgProfileService interface {
	GetByOrganizationID(context.Context, int64) (moduleorgprofile.Detail, error)
	UpdateByOrganizationID(context.Context, int64, int64, moduleorgprofile.UpdateInput) (moduleorgprofile.Detail, error)
	UpsertExchangeRate(context.Context, int64, int64, moduleorgprofile.ExchangeRateInput) (moduleorgprofile.Detail, error)
}

type dashboardService interface {
	SummaryByOrganization(context.Context, int64, moduledashboard.ForecastQuery) (moduledashboard.Summary, error)
	UpsertSalesQuota(context.Context, int64, int64, int64, moduledashboard.QuotaInput) (moduledashboard.Summary, error)
}

type clientReviewsService interface {
	Get(context.Context, int64, string, int64) (moduleclientreviews.Schedule, error)
	Upsert(context.Context, int64, int64, string, int64, moduleclientreviews.Input) (moduleclientreviews.Schedule, error)
	Delete(context.Context, int64, int64, string, int64) error
}

type collaborationService interface {
	Followers(context.Context, int64, int64, string, int64) (modulecollaboration.Followers, error)
	SetFollowing(context.Context, int64, int64, string, int64, bool) (modulecollaboration.Followers, error)
	ActivityDigest(context.Context, int64, int64, modulecollaboration.DigestQuery) (modulecollaboration.Digest, error)
}

type callLogsService interface {
	ListByEntity(context.Context, int64, string, int64, int) ([]modulecalllogs.Log, error)
	StartOutbound(context.Context, int64, int64, modulecalllogs.StartInput) (modulecalllogs.StartResult, error)
	Complete(context.Context, int64, int64, int64, modulecalllogs.CompleteInput) (modulecalllogs.Log, error)
	RecordManual(context.Context, int64, int64, modulecalllogs.RecordInput) (modulecalllogs.Log, error)
	UpdateRecording(context.Context, int64, int64, int64, modulecalllogs.RecordingInput) (modulecalllogs.Log, error)
}

type smsService interface {
	ListByEntity(context.Context, int64, string, int64, int) ([]modulesms.Message, error)
	Send(context.Context, int64, int64, modulesms.SendInput) (modulesms.Message, error)
	RecordInbound(context.Context, int64, int64, modulesms.InboundInput) (modulesms.Message, error)
	Suppress(context.Context, int64, int64, modulesms.SuppressInput) (modulesms.Suppression, error)
}

type calendarService interface {
	ListByEntity(context.Context, int64, string, int64, int) ([]modulecalendar.Event, error)
	Schedule(context.Context, int64, int64, modulecalendar.ScheduleInput) (modulecalendar.Event, error)
	Cancel(context.Context, int64, int64, int64) (modulecalendar.Event, error)
	ListAvailability(context.Context, int64, int64) ([]modulecalendar.AvailabilityBlock, error)
	SetAvailability(context.Context, int64, int64, modulecalendar.AvailabilityInput) ([]modulecalendar.AvailabilityBlock, error)
	ListBookingLinks(context.Context, int64) ([]modulecalendar.BookingLink, error)
	CreateBookingLink(context.Context, int64, int64, modulecalendar.BookingLinkInput) (modulecalendar.BookingLink, error)
	UpdateBookingLink(context.Context, int64, int64, int64, modulecalendar.BookingLinkInput) (modulecalendar.BookingLink, error)
}

type importsService interface {
	Preview(context.Context, moduleimports.PreviewInput) (moduleimports.PreviewResult, error)
	Execute(context.Context, moduleimports.ExecuteInput) (moduleimports.Batch, error)
	List(context.Context, int64, int) ([]moduleimports.Batch, error)
	Rollback(context.Context, int64, int64, int64) (moduleimports.Batch, error)
	ErrorCSV(context.Context, int64, int64) (moduleimports.ErrorFile, error)
}

type bulkOperationsService interface {
	Execute(context.Context, modulebulkoperations.ExecuteInput) (modulebulkoperations.Operation, error)
	List(context.Context, int64, string, int) ([]modulebulkoperations.Operation, error)
	Rollback(context.Context, int64, int64, int64) (modulebulkoperations.Operation, error)
}

type archiveOperationsService interface {
	List(context.Context, int64, modulearchiveoperations.ListQuery) ([]modulearchiveoperations.Record, error)
	Restore(context.Context, int64, int64, string, int64) (modulearchiveoperations.Record, error)
}

type duplicateOperationsService interface {
	Review(context.Context, int64, string, int) (moduleduplicates.Review, error)
	Merge(context.Context, moduleduplicates.MergeInput) (moduleduplicates.MergeOperation, error)
}

type customFieldsService interface {
	List(context.Context, int64, string, bool) ([]modulecustomfields.Definition, error)
	Create(context.Context, int64, int64, modulecustomfields.CreateInput) (modulecustomfields.Definition, error)
	Update(context.Context, int64, int64, int64, modulecustomfields.UpdateInput) (modulecustomfields.Definition, error)
	Archive(context.Context, int64, int64, int64) error
}

type savedViewsService interface {
	ListByEntity(context.Context, int64, int64, string) ([]modulesavedviews.View, error)
	Create(context.Context, int64, int64, modulesavedviews.Input) (modulesavedviews.View, error)
	Update(context.Context, int64, int64, int64, modulesavedviews.Input) (modulesavedviews.View, error)
	Delete(context.Context, int64, int64, int64) error
}

type onboardingService interface {
	BootstrapOrganization(context.Context, moduleonboarding.BootstrapInput) (moduleonboarding.BootstrapResult, error)
	VerifyEmail(context.Context, string) (moduleauth.LoginResult, error)
	ResendVerification(context.Context, string) (moduleonboarding.ResendResult, error)
}

type passwordResetService interface {
	Request(context.Context, string) (modulepasswordreset.RequestResult, error)
	Complete(context.Context, modulepasswordreset.CompleteInput) error
}

type notificationsService interface {
	ListForUser(context.Context, int64, int64) ([]modulenotifications.Notification, error)
	MarkRead(context.Context, int64, int64, int64) error
	MarkAllRead(context.Context, int64, int64) error
	UnreadCount(context.Context, int64, int64) (int, error)
}

type emailService interface {
	ProviderName() string
	SendUserInvite(ctx context.Context, to, firstName, setupToken string, organizationID, userID int64, deliveryKey string) (string, error)
	Send(ctx context.Context, to, subject, body string) error
}

type emailFeedbackService interface {
	ProcessPostmark(context.Context, []byte) (moduleemailfeedback.Result, error)
}

type emailTemplatesService interface {
	ListByOrganization(context.Context, int64) ([]moduleemailtemplates.Template, error)
	Create(context.Context, int64, moduleemailtemplates.Input) (moduleemailtemplates.Template, error)
	Update(context.Context, int64, int64, moduleemailtemplates.Input) (moduleemailtemplates.Template, error)
	Delete(context.Context, int64, int64) error
	ListSnippetsByOrganization(context.Context, int64) ([]moduleemailtemplates.Snippet, error)
	CreateSnippet(context.Context, int64, moduleemailtemplates.SnippetInput) (moduleemailtemplates.Snippet, error)
	UpdateSnippet(context.Context, int64, int64, moduleemailtemplates.SnippetInput) (moduleemailtemplates.Snippet, error)
	DeleteSnippet(context.Context, int64, int64) error
}

type productCatalogService interface {
	ListByOrganization(context.Context, int64) ([]moduleproductcatalog.Item, error)
	Create(context.Context, int64, int64, moduleproductcatalog.Input) (moduleproductcatalog.Item, error)
	Update(context.Context, int64, int64, int64, moduleproductcatalog.Input) (moduleproductcatalog.Item, error)
	Archive(context.Context, int64, int64) error
}

type leadFormsService interface {
	ListByOrganization(context.Context, int64) ([]moduleleadforms.Form, error)
	Create(context.Context, int64, int64, moduleleadforms.Input) (moduleleadforms.Form, error)
	Update(context.Context, int64, int64, int64, moduleleadforms.Input) (moduleleadforms.Form, error)
	ListSubmissionReviews(context.Context, int64, moduleleadforms.SubmissionReviewQuery) (moduleleadforms.SubmissionReviewPage, error)
	ReviewSubmission(context.Context, int64, int64, int64, moduleleadforms.SubmissionReviewInput) (moduleleadforms.ReviewedSubmission, error)
	ListLandingPagesByOrganization(context.Context, int64) ([]moduleleadforms.LandingPage, error)
	CreateLandingPage(context.Context, int64, int64, moduleleadforms.LandingPageInput) (moduleleadforms.LandingPage, error)
	UpdateLandingPage(context.Context, int64, int64, int64, moduleleadforms.LandingPageInput) (moduleleadforms.LandingPage, error)
	GetPublicLandingPage(context.Context, string) (moduleleadforms.PublicLandingPage, error)
	ListChatWidgetsByOrganization(context.Context, int64) ([]moduleleadforms.ChatWidget, error)
	CreateChatWidget(context.Context, int64, int64, moduleleadforms.ChatWidgetInput) (moduleleadforms.ChatWidget, error)
	UpdateChatWidget(context.Context, int64, int64, int64, moduleleadforms.ChatWidgetInput) (moduleleadforms.ChatWidget, error)
	GetPublicChatWidget(context.Context, string) (moduleleadforms.PublicChatWidget, error)
	IssueSubmissionChallenge(context.Context, string) (moduleleadforms.SubmissionChallenge, error)
	SubmitByPublicID(context.Context, string, moduleleadforms.SubmissionInput) (moduleleadforms.SubmissionResult, error)
}

type leadAudiencesService interface {
	ListByOrganization(context.Context, int64) ([]moduleleadaudiences.Audience, error)
	Create(context.Context, int64, int64, moduleleadaudiences.Input) (moduleleadaudiences.Audience, error)
	Update(context.Context, int64, int64, int64, moduleleadaudiences.Input) (moduleleadaudiences.Audience, error)
	Preview(context.Context, int64, map[string]string) (moduleleadaudiences.Preview, error)
}

type emailSequencesService interface {
	ListByOrganization(context.Context, int64) ([]moduleemailsequences.Sequence, error)
	Create(context.Context, int64, int64, moduleemailsequences.Input) (moduleemailsequences.Sequence, error)
	Update(context.Context, int64, int64, moduleemailsequences.Input) (moduleemailsequences.Sequence, error)
	Delete(context.Context, int64, int64) error
	Approve(context.Context, int64, int64, int64) (moduleemailsequences.Sequence, error)
	Pause(context.Context, int64, int64) (moduleemailsequences.Sequence, error)
}

type marketingCampaignsService interface {
	ListByOrganization(context.Context, int64) ([]modulemarketingcampaigns.Campaign, error)
	Create(context.Context, int64, int64, modulemarketingcampaigns.Input) (modulemarketingcampaigns.Campaign, error)
	Update(context.Context, int64, int64, int64, modulemarketingcampaigns.Input) (modulemarketingcampaigns.Campaign, error)
}

type nurtureCampaignsService interface {
	ListByOrganization(context.Context, int64) ([]modulenurturecampaigns.Campaign, error)
	Create(context.Context, int64, int64, modulenurturecampaigns.Input) (modulenurturecampaigns.Campaign, error)
	Update(context.Context, int64, int64, int64, modulenurturecampaigns.Input) (modulenurturecampaigns.Campaign, error)
}

type leadScoringService interface {
	ListByOrganization(context.Context, int64) ([]moduleleadscoring.Rule, error)
	Create(context.Context, int64, int64, moduleleadscoring.Input) (moduleleadscoring.Rule, error)
	Update(context.Context, int64, int64, int64, moduleleadscoring.Input) (moduleleadscoring.Rule, error)
	EvaluateContact(context.Context, int64, int64, int64) (moduleleadscoring.Evaluation, error)
}

type workflowAutomationsService interface {
	ListByOrganization(context.Context, int64) ([]moduleworkflowautomations.Automation, error)
	ListRuns(context.Context, int64, moduleworkflowautomations.RunListQuery) ([]moduleworkflowautomations.Run, error)
	Create(context.Context, int64, int64, moduleworkflowautomations.Input) (moduleworkflowautomations.Automation, error)
	Update(context.Context, int64, int64, int64, moduleworkflowautomations.Input) (moduleworkflowautomations.Automation, error)
}

type customReportsService interface {
	ListByOrganization(context.Context, int64) ([]modulecustomreports.Definition, error)
	Create(context.Context, int64, int64, modulecustomreports.Input) (modulecustomreports.Definition, error)
	Update(context.Context, int64, int64, int64, modulecustomreports.Input) (modulecustomreports.Definition, error)
	Execute(context.Context, int64, int64, modulecustomreports.ExecuteQuery) (modulecustomreports.Execution, error)
	ExportCSV(context.Context, int64, int64, int64) (modulecustomreports.CSVFile, error)
}

type dataQualityService interface {
	Summary(context.Context, int64, moduledataquality.Query) (moduledataquality.Summary, error)
}

type salesReportsService interface {
	Activity(context.Context, int64, modulesalesreports.Query) (modulesalesreports.Report, error)
	Funnel(context.Context, int64, modulesalesreports.FunnelQuery) (modulesalesreports.FunnelReport, error)
}

type touchpointsService interface {
	Stale(context.Context, int64, int64, moduletouchpoints.Query) (moduletouchpoints.Report, error)
	ClientActivity(context.Context, int64, int64, moduletouchpoints.ClientActivityQuery) (moduletouchpoints.ClientActivityReport, error)
	Health(context.Context, int64, int64, moduletouchpoints.HealthQuery) (moduletouchpoints.HealthReport, error)
	Summary(context.Context, int64, int64, string, int64, int) (moduletouchpoints.Summary, error)
}

type emailSequenceEnrollmentsService interface {
	ListEnrollmentsByContact(context.Context, int64, int64) ([]moduleemailsequences.Enrollment, error)
	EnrollContact(context.Context, int64, moduleemailsequences.EnrollmentInput) (moduleemailsequences.Enrollment, error)
	CancelEnrollment(context.Context, int64, int64) error
}

type userEmailAccountService interface {
	Configured() bool
	GetForUser(context.Context, int64, int64) (moduleuseremail.Account, error)
	Upsert(context.Context, int64, int64, moduleuseremail.UpsertInput) (moduleuseremail.Account, error)
	SaveOAuthConnection(context.Context, int64, int64, moduleuseremail.OAuthConnectionInput) (moduleuseremail.Account, error)
	UpdateSyncState(context.Context, int64, int64, moduleuseremail.SyncStateInput) (moduleuseremail.Account, error)
	Delete(context.Context, int64, int64) error
	SendMessageAs(context.Context, int64, int64, moduleemail.Message) (moduleuseremail.SendReceipt, error)
	MemberExists(context.Context, int64, int64) (bool, error)
}

type mailboxSyncService interface {
	Configured() bool
	SyncUser(context.Context, int64, int64) (modulemailboxsync.Result, error)
}

type emailMessagesService interface {
	recordEmailDeliveryService
	Record(context.Context, int64, moduleemailmessages.RecordInput) error
	ReplayReply(context.Context, int64, moduleemailmessages.PrepareReplyInput) (moduleemailmessages.ReplyRequest, bool, error)
	PrepareReply(context.Context, int64, moduleemailmessages.PrepareReplyInput) (moduleemailmessages.ReplyRequest, error)
	ClaimReply(context.Context, int64, int64, int64) (moduleemailmessages.ReplyRequest, bool, error)
	CompleteReply(context.Context, int64, int64, moduleuseremail.SendReceipt) (moduleemailmessages.ReplyRequest, error)
	FailReply(context.Context, int64, int64, error, bool) (moduleemailmessages.ReplyRequest, error)
	ResolveReply(context.Context, int64, int64, int64, string) (moduleemailmessages.ReplyResolution, error)
	ListThread(context.Context, int64, int64, int64, bool) ([]moduleemailmessages.Message, []moduleemailmessages.ReplyRequest, error)
	GetByID(context.Context, int64, int64) (moduleemailmessages.Message, error)
	ListByOrganization(context.Context, int64, int) ([]moduleemailmessages.Message, error)
	ListByEntity(context.Context, int64, string, int64, int64, bool) ([]moduleemailmessages.Message, error)
	ListBySender(context.Context, int64, int64, int) ([]moduleemailmessages.Message, error)
	ListMailboxByUser(context.Context, int64, int64, int) ([]moduleemailmessages.Message, error)
	ListSharedInbox(context.Context, int64, int) ([]moduleemailmessages.Message, error)
	UpdateSharedInbox(context.Context, int64, int64, moduleemailmessages.SharedInboxUpdateInput) (moduleemailmessages.Message, error)
	MarkOpenedByToken(context.Context, string) error
	MarkClickedByToken(context.Context, string) (string, error)
}

type emailSuppressionsService interface {
	IsSuppressed(context.Context, int64, string) (bool, error)
	UnsubscribeToken(int64, string) (string, error)
	ValidateUnsubscribeToken(string) error
	UnsubscribeByToken(context.Context, string) (moduleemailsuppressions.Suppression, error)
}

type Dependencies struct {
	CheckReadiness                  func(context.Context) error
	Logger                          *slog.Logger
	RateLimitsService               rateLimitService
	AuthService                     authService
	UsersService                    usersService
	AuditService                    auditService
	BackgroundJobsService           backgroundJobsService
	SequenceDeliveryOperations      sequenceDeliveryOperationsService
	ContactsService                 contactsService
	CompaniesService                companiesService
	DealsService                    dealsService
	TasksService                    tasksService
	ExportsService                  dataExportsService
	WorkspaceExportsService         workspaceExportsService
	OrgProfileService               orgProfileService
	DashboardService                dashboardService
	ClientReviewsService            clientReviewsService
	NotesService                    notesService
	ActivityFeedService             activityFeedService
	CollaborationService            collaborationService
	CallLogsService                 callLogsService
	SMSService                      smsService
	CalendarService                 calendarService
	ImportsService                  importsService
	BulkOperationsService           bulkOperationsService
	ArchiveOperationsService        archiveOperationsService
	DuplicateOperationsService      duplicateOperationsService
	CustomFieldsService             customFieldsService
	SavedViewsService               savedViewsService
	OnboardingService               onboardingService
	PasswordResetService            passwordResetService
	NotificationsService            notificationsService
	BillingService                  billingService
	EmailService                    emailService
	EmailFeedbackService            emailFeedbackService
	EmailTemplatesService           emailTemplatesService
	ProductCatalogService           productCatalogService
	QuoteTemplatesService           quoteTemplatesService
	LeadFormsService                leadFormsService
	LeadAudiencesService            leadAudiencesService
	MarketingCampaignsService       marketingCampaignsService
	NurtureCampaignsService         nurtureCampaignsService
	LeadScoringService              leadScoringService
	WorkflowAutomationsService      workflowAutomationsService
	CustomReportsService            customReportsService
	DataQualityService              dataQualityService
	SalesReportsService             salesReportsService
	TouchpointsService              touchpointsService
	EmailSequencesService           emailSequencesService
	EmailSequenceEnrollmentsService emailSequenceEnrollmentsService
	UserEmailService                userEmailAccountService
	MailboxSyncService              mailboxSyncService
	EmailOAuthClient                emailOAuthClient
	EmailMessagesService            emailMessagesService
	EmailSuppressionsService        emailSuppressionsService
	Metrics                         *platformtelemetry.Collector
	OperationalMetrics              platformtelemetry.SnapshotSource
}
