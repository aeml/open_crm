package app

import (
	"context"
	"log/slog"

	platformtelemetry "github.com/aeml/open_crm/apps/api/internal/platform/telemetry"
)

// Dependencies is the explicit runtime container consumed by NewServer.
// Service contracts remain in dependencies.go and focused domain files.
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
