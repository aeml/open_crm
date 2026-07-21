package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/app"
	"github.com/aeml/open_crm/apps/api/internal/config"
	"github.com/aeml/open_crm/apps/api/internal/db"
	modulearchiveoperations "github.com/aeml/open_crm/apps/api/internal/modules/archiveoperations"
	moduleaudit "github.com/aeml/open_crm/apps/api/internal/modules/audit"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
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
	modulenotes "github.com/aeml/open_crm/apps/api/internal/modules/notes"
	modulenotifications "github.com/aeml/open_crm/apps/api/internal/modules/notifications"
	modulenurturecampaigns "github.com/aeml/open_crm/apps/api/internal/modules/nurturecampaigns"
	moduleonboarding "github.com/aeml/open_crm/apps/api/internal/modules/onboarding"
	moduleorgprofile "github.com/aeml/open_crm/apps/api/internal/modules/orgprofile"
	modulepasswordreset "github.com/aeml/open_crm/apps/api/internal/modules/passwordreset"
	moduleproductcatalog "github.com/aeml/open_crm/apps/api/internal/modules/productcatalog"
	moduleratelimits "github.com/aeml/open_crm/apps/api/internal/modules/ratelimits"
	modulesalesreports "github.com/aeml/open_crm/apps/api/internal/modules/salesreports"
	modulesavedviews "github.com/aeml/open_crm/apps/api/internal/modules/savedviews"
	modulesequencerunner "github.com/aeml/open_crm/apps/api/internal/modules/sequencerunner"
	modulesms "github.com/aeml/open_crm/apps/api/internal/modules/sms"
	moduletaskreminders "github.com/aeml/open_crm/apps/api/internal/modules/taskreminders"
	moduletasks "github.com/aeml/open_crm/apps/api/internal/modules/tasks"
	moduletouchpoints "github.com/aeml/open_crm/apps/api/internal/modules/touchpoints"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
	moduleworkflowautomations "github.com/aeml/open_crm/apps/api/internal/modules/workflowautomations"
	moduleworkspaceexports "github.com/aeml/open_crm/apps/api/internal/modules/workspaceexports"
	platformlogger "github.com/aeml/open_crm/apps/api/internal/platform/logger"
	platformsecrets "github.com/aeml/open_crm/apps/api/internal/platform/secrets"
	platformtelemetry "github.com/aeml/open_crm/apps/api/internal/platform/telemetry"
)

const (
	serverReadHeaderTimeout = 5 * time.Second
	serverReadTimeout       = 15 * time.Second
	serverWriteTimeout      = 30 * time.Second
	serverIdleTimeout       = 60 * time.Second
	serverShutdownTimeout   = 10 * time.Second
)

func main() {
	env := config.Load()
	logger := platformlogger.New(env.GOEnv)
	metrics := platformtelemetry.NewCollector()
	dbConfig, dbConfigErr := db.LoadConfigFromEnv()
	if dbConfigErr != nil {
		log.Printf("database config warning: %v", dbConfigErr)
	}

	var authService *moduleauth.Service
	var auditService *moduleaudit.Service
	var usersService *moduleusers.Service
	var contactsService *modulecontacts.Service
	var companiesService *modulecompanies.Service
	var dealsService *moduledeals.Service
	var tasksService *moduletasks.Service
	var exportsService *moduleexports.Service
	var workspaceExportsService *moduleworkspaceexports.Service
	var dashboardService *moduledashboard.Service
	var clientReviewsService *moduleclientreviews.Service
	var importsService *moduleimports.Service
	var bulkOperationsService *modulebulkoperations.Service
	var archiveOperationsService *modulearchiveoperations.Service
	var duplicateOperationsService *moduleduplicates.Service
	var customFieldsService *modulecustomfields.Service
	emailProvider := moduleemail.WithObserver(moduleemail.NewProvider(moduleemail.ProviderConfig{
		Name:                  env.EmailProvider,
		Logger:                logger,
		PostmarkServerToken:   env.PostmarkServerToken,
		PostmarkFromEmail:     env.PostmarkFromEmail,
		PostmarkMessageStream: env.PostmarkMessageStream,
	}), metrics)
	emailService := moduleemail.NewService(emailProvider, env.EmailFromName, env.EmailFromAddress, env.WebBaseURL)
	var notesService *modulenotes.Service
	var collaborationService *modulecollaboration.Service
	var callLogsService *modulecalllogs.Service
	var smsService *modulesms.Service
	var calendarService *modulecalendar.Service
	var notificationsService *modulenotifications.Service
	var savedViewsService *modulesavedviews.Service
	var onboardingService *moduleonboarding.Service
	var passwordResetService *modulepasswordreset.Service
	var orgProfileService *moduleorgprofile.Service
	var billingService *modulebilling.Service
	var emailTemplatesService *moduleemailtemplates.Service
	var productCatalogService *moduleproductcatalog.Service
	var leadFormsService *moduleleadforms.Service
	var leadAudiencesService *moduleleadaudiences.Service
	var marketingCampaignsService *modulemarketingcampaigns.Service
	var nurtureCampaignsService *modulenurturecampaigns.Service
	var leadScoringService *moduleleadscoring.Service
	var workflowAutomationsService *moduleworkflowautomations.Service
	var customReportsService *modulecustomreports.Service
	var dataQualityService *moduledataquality.Service
	var salesReportsService *modulesalesreports.Service
	var touchpointsService *moduletouchpoints.Service
	var emailSequencesService *moduleemailsequences.Service
	var emailSuppressionsService *moduleemailsuppressions.Service
	var emailFeedbackService *moduleemailfeedback.Service
	var userEmailService *moduleuseremail.Service
	var emailMessagesService *moduleemailmessages.Service
	var mailboxSyncService *modulemailboxsync.Service
	var sequenceRunnerService *modulesequencerunner.Service
	var jobsService *modulejobs.Service
	var taskRemindersService *moduletaskreminders.Service
	var rateLimitsService *moduleratelimits.Service
	var databasePool *db.Pool
	credentialCipher, cipherErr := platformsecrets.NewCipherFromBase64(env.CredentialEncryptionKey)
	if cipherErr != nil {
		log.Printf("credential encryption disabled: %v", cipherErr)
	}
	if dbConfigErr == nil {
		pool, err := db.NewPool(context.Background(), dbConfig)
		if err != nil {
			log.Printf("auth service disabled: %v", err)
		} else {
			defer pool.Close()
			databasePool = pool
			rateLimitsService = moduleratelimits.NewService(pool)
			billingService = modulebilling.NewService(pool, modulebilling.WithObserver(modulebilling.NewProvider(env.BillingProvider, modulebilling.ProviderConfig{
				SecretKey:       env.StripeSecretKey,
				WebhookSecret:   env.StripeWebhookSecret,
				PriceStarter:    env.StripePriceStarter,
				PricePro:        env.StripePricePro,
				PriceEnterprise: env.StripePriceEnterprise,
				WebBaseURL:      env.WebBaseURL,
			}), metrics))
			authService = moduleauth.NewService(pool)
			auditService = moduleaudit.NewService(pool)
			usersService = moduleusers.NewServiceWithCapacity(pool, billingService)
			contactsService = modulecontacts.NewServiceWithCapacity(pool, billingService)
			companiesService = modulecompanies.NewService(pool)
			dealsService = moduledeals.NewServiceWithCapacity(pool, billingService)
			tasksService = moduletasks.NewService(pool)
			taskRemindersService = moduletaskreminders.NewService(pool)
			exportsService = moduleexports.NewService(pool)
			workspaceExportsService = moduleworkspaceexports.NewService(pool)
			dashboardService = moduledashboard.NewService(pool)
			clientReviewsService = moduleclientreviews.NewService(pool)
			importsService = moduleimports.NewServiceWithCapacity(pool, billingService)
			bulkOperationsService = modulebulkoperations.NewServiceWithCapacity(pool, billingService)
			archiveOperationsService = modulearchiveoperations.NewServiceWithCapacity(pool, billingService)
			duplicateOperationsService = moduleduplicates.NewService(pool)
			customFieldsService = modulecustomfields.NewService(pool)
			notesService = modulenotes.NewService(pool)
			collaborationService = modulecollaboration.NewService(pool)
			callLogsService = modulecalllogs.NewService(pool, modulecalllogs.NewProvider(env.TelephonyProvider, logger))
			smsService = modulesms.NewService(pool, modulesms.NewProvider(env.TelephonyProvider, logger))
			calendarService = modulecalendar.NewService(pool, modulecalendar.NewProvider(env.CalendarProvider, logger))
			notificationsService = modulenotifications.NewService(pool)
			savedViewsService = modulesavedviews.NewService(pool)
			onboardingService = moduleonboarding.NewService(pool, emailService)
			allowLocalResetLinks := strings.EqualFold(env.GOEnv, "development") || strings.EqualFold(env.GOEnv, "test")
			passwordResetService = modulepasswordreset.NewService(pool, emailService, modulepasswordreset.WithLocalResetLinks(allowLocalResetLinks))
			orgProfileService = moduleorgprofile.NewService(pool)
			emailTemplatesService = moduleemailtemplates.NewService(pool)
			productCatalogService = moduleproductcatalog.NewService(pool)
			leadFormsService = moduleleadforms.NewServiceWithCapacity(pool, billingService, billingService.Hosted())
			leadAudiencesService = moduleleadaudiences.NewService(pool)
			marketingCampaignsService = modulemarketingcampaigns.NewService(pool)
			nurtureCampaignsService = modulenurturecampaigns.NewService(pool)
			leadScoringService = moduleleadscoring.NewService(pool)
			workflowAutomationsService = moduleworkflowautomations.NewService(pool)
			customReportsService = modulecustomreports.NewService(pool)
			dataQualityService = moduledataquality.NewService(pool)
			salesReportsService = modulesalesreports.NewService(pool)
			touchpointsService = moduletouchpoints.NewService(pool)
			emailSequencesService = moduleemailsequences.NewService(pool)
			emailSuppressionsService = moduleemailsuppressions.NewService(pool, env.CredentialEncryptionKey)
			emailFeedbackService = moduleemailfeedback.NewService(pool, env.PostmarkMessageStream)
			oauthTokenRefresher := modulemailboxsync.NewOAuthTokenRefresher(modulemailboxsync.OAuthTokenRefresherConfig{
				GoogleClientID:        env.GoogleOAuthClientID,
				GoogleClientSecret:    env.GoogleOAuthClientSecret,
				MicrosoftClientID:     env.MicrosoftOAuthClientID,
				MicrosoftClientSecret: env.MicrosoftOAuthClientSecret,
			})
			userEmailService = moduleuseremail.NewServiceWithProviders(pool, credentialCipher, metrics, oauthTokenRefresher, modulemailboxsync.NewOAuthSender(modulemailboxsync.OAuthSenderConfig{}))
			emailMessagesService = moduleemailmessages.NewService(pool)
			mailboxSyncService = modulemailboxsync.NewServiceWithOAuthRefresh(userEmailService, emailMessagesService, nil, oauthTokenRefresher)
			sequenceRunnerService, err = buildSequenceRunner(env, billingService.Hosted(), emailSequencesService, userEmailService, emailMessagesService, emailSuppressionsService, rateLimitsService)
			if err != nil {
				log.Fatalf("configure sequence runner: %v", err)
			}
			jobsService = modulejobs.NewService(pool)
		}
	}
	if rateLimitsService == nil {
		// The production HTTP boundary must fail closed when PostgreSQL could
		// not be configured; NewServer's local limiter is test-only fallback.
		rateLimitsService = moduleratelimits.NewService(databasePool)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if jobsService != nil {
		go jobsService.RunRetentionScheduler(ctx, logger, modulejobs.DefaultRetentionPolicy(), 0)
		jobHandlers := map[string]modulejobs.Handler{}
		if workspaceExportsService != nil {
			jobHandlers[moduleworkspaceexports.JobType] = func(ctx context.Context, job modulejobs.Job) (map[string]any, error) {
				result, err := workspaceExportsService.HandleJob(ctx, job)
				if moduleworkspaceexports.IsPermanentFailure(err) {
					return nil, modulejobs.Permanent(err)
				}
				return result, err
			}
			go workspaceExportsService.RunCleanupScheduler(ctx, logger, 0)
		}
		if calendarService != nil && calendarService.Configured() {
			jobHandlers[modulecalendar.ReminderJobType] = func(ctx context.Context, job modulejobs.Job) (map[string]any, error) {
				result, err := calendarService.DeliverReminderJob(ctx, job.OrganizationID, job.Payload)
				if errors.Is(err, modulecalendar.ErrInvalidInput) {
					return nil, modulejobs.Permanent(err)
				}
				return result, err
			}
		}
		if taskRemindersService != nil {
			jobHandlers[moduletaskreminders.JobType] = func(ctx context.Context, job modulejobs.Job) (map[string]any, error) {
				result, err := taskRemindersService.DeliverJob(ctx, job.OrganizationID, job.Payload)
				if errors.Is(err, moduletaskreminders.ErrInvalidInput) {
					return nil, modulejobs.Permanent(err)
				}
				return result, err
			}
		}
		if mailboxSyncService != nil && mailboxSyncService.Configured() {
			jobHandlers[modulemailboxsync.MailboxSyncJobType] = func(ctx context.Context, job modulejobs.Job) (map[string]any, error) {
				result, err := mailboxSyncService.HandleJob(ctx, job)
				if errors.Is(err, modulemailboxsync.ErrInvalidJobPayload) {
					return nil, modulejobs.Permanent(err)
				}
				return result, err
			}
			go mailboxSyncService.RunJobScheduler(ctx, jobsService, logger, 0, 0)
		}
		if billingService != nil && billingService.ReconciliationConfigured() {
			jobHandlers[modulebilling.ReconciliationJobType] = func(ctx context.Context, job modulejobs.Job) (map[string]any, error) {
				result, err := billingService.HandleReconciliationJob(ctx, job)
				if errors.Is(err, modulebilling.ErrInvalidReconciliationJob) {
					return nil, modulejobs.Permanent(err)
				}
				return result, err
			}
			go billingService.RunReconciliationScheduler(ctx, jobsService, logger, 0, 0)
		}
		if billingService != nil {
			jobHandlers[modulebilling.UsageSnapshotJobType] = func(ctx context.Context, job modulejobs.Job) (map[string]any, error) {
				result, err := billingService.HandleUsageSnapshotJob(ctx, job)
				if errors.Is(err, modulebilling.ErrInvalidUsageSnapshotJob) {
					return nil, modulejobs.Permanent(err)
				}
				return result, err
			}
			go billingService.RunUsageSnapshotScheduler(ctx, jobsService, logger, 0, 0)
		}
		if sequenceRunnerService != nil && sequenceRunnerService.Configured() {
			jobHandlers[moduleemailsequences.SequenceSendJobType] = sequenceRunnerService.HandleJob
		}
		if billingService != nil {
			for jobType, handler := range jobHandlers {
				if jobType != modulebilling.ReconciliationJobType && jobType != modulebilling.UsageSnapshotJobType && jobType != moduleworkspaceexports.JobType {
					jobHandlers[jobType] = modulebilling.GuardJobHandler(billingService, handler)
				}
			}
		}
		if len(jobHandlers) > 0 {
			jobWorker := modulejobs.NewWorker(jobsService, jobHandlers, backgroundWorkerID(), logger, metrics)
			go jobWorker.Run(ctx)
		}
	}
	if notificationsService != nil {
		go notificationsService.RunRetentionScheduler(ctx, logger, modulenotifications.DefaultRetentionPolicy(), 0, metrics)
	}
	if emailFeedbackService != nil {
		go emailFeedbackService.RunRetentionScheduler(ctx, logger, 0)
	}
	if emailMessagesService != nil {
		go emailMessagesService.RunTrackingRetentionScheduler(ctx, logger, 0, metrics)
		go emailMessagesService.RunReplyRecoveryScheduler(ctx, logger, 0, metrics)
	}

	checkReadiness := func(ctx context.Context) error {
		if dbConfigErr != nil {
			return dbConfigErr
		}
		if databasePool == nil {
			return fmt.Errorf("database pool unavailable")
		}
		return databasePool.Ping(ctx)
	}
	operationalMetrics := func(ctx context.Context) platformtelemetry.RuntimeSnapshot {
		snapshot := platformtelemetry.RuntimeSnapshot{CollectionSuccess: true}
		if err := checkReadiness(ctx); err == nil {
			snapshot.DatabaseUp = true
		} else {
			snapshot.CollectionSuccess = false
		}
		if jobsService == nil {
			snapshot.CollectionSuccess = false
		} else if stats, err := jobsService.OperationalStats(ctx); err != nil {
			snapshot.CollectionSuccess = false
		} else {
			snapshot.JobsAvailable = true
			snapshot.JobsPending = stats.Pending
			snapshot.JobsRunning = stats.Running
			snapshot.JobsRetryable = stats.Retryable
			snapshot.JobsDead = stats.Dead
			if !stats.OldestReadyAt.IsZero() {
				snapshot.OldestReadyLag = time.Since(stats.OldestReadyAt)
			}
		}
		if notificationsService == nil {
			snapshot.CollectionSuccess = false
		} else if stats, err := notificationsService.OperationalStats(ctx); err != nil {
			snapshot.CollectionSuccess = false
		} else {
			snapshot.NotificationsAvailable = true
			snapshot.NotificationsUnread = stats.Unread
			snapshot.NotificationsCreated24h = stats.Created24h
			snapshot.NotificationRecipients24h = stats.Recipients24h
			snapshot.NotificationMaxPerRecipient24h = stats.MaxPerRecipient24h
			snapshot.OldestUnreadAge = stats.OldestUnreadAge
			snapshot.NotificationEvents24h = stats.Events24h
		}
		if passwordResetService == nil {
			snapshot.CollectionSuccess = false
		} else if stats, err := passwordResetService.OperationalStats(ctx); err != nil {
			snapshot.CollectionSuccess = false
		} else {
			snapshot.PasswordResetsAvailable = true
			snapshot.PasswordResetsOutstanding = stats.Outstanding
			snapshot.PasswordResetStalePending = stats.StalePending
			snapshot.PasswordResetFailed24h = stats.FailedLast24h
		}
		if emailFeedbackService == nil {
			snapshot.CollectionSuccess = false
		} else if stats, err := emailFeedbackService.OperationalStats(ctx); err != nil {
			snapshot.CollectionSuccess = false
		} else {
			snapshot.SystemEmailFeedbackAvailable = true
			snapshot.SystemEmailBounces24h = stats.Bounces24h
			snapshot.SystemEmailComplaints24h = stats.Complaints24h
			snapshot.SystemEmailUnapplied24h = stats.Unapplied24h
			snapshot.CustomerEmailFeedbackAvailable = true
			snapshot.CustomerEmailBounces24h = stats.CustomerBounces24h
			snapshot.CustomerEmailComplaints24h = stats.CustomerComplaints24h
			snapshot.CustomerEmailUnapplied24h = stats.CustomerUnapplied24h
		}
		if emailMessagesService == nil {
			snapshot.CollectionSuccess = false
		} else if stats, err := emailMessagesService.ReplyOperationalStats(ctx); err != nil {
			snapshot.CollectionSuccess = false
		} else {
			snapshot.EmailRepliesAvailable = true
			snapshot.EmailRepliesSending = stats.Sending
			snapshot.EmailRepliesStaleSending = stats.StaleSending
			snapshot.EmailRepliesUncertain = stats.Uncertain
		}
		snapshot.Backup = platformtelemetry.ReadBackupStatus(env.BackupStatusPath)
		return snapshot
	}

	server := newHTTPServer(env, app.NewServer(env, app.Dependencies{
		CheckReadiness:                  checkReadiness,
		Metrics:                         metrics,
		OperationalMetrics:              operationalMetrics,
		Logger:                          logger,
		RateLimitsService:               rateLimitsService,
		AuthService:                     authService,
		AuditService:                    auditService,
		BackgroundJobsService:           jobsService,
		SequenceDeliveryOperations:      emailSequencesService,
		UsersService:                    usersService,
		ContactsService:                 contactsService,
		CompaniesService:                companiesService,
		DealsService:                    dealsService,
		TasksService:                    tasksService,
		ExportsService:                  exportsService,
		WorkspaceExportsService:         workspaceExportsService,
		DashboardService:                dashboardService,
		ClientReviewsService:            clientReviewsService,
		NotesService:                    notesService,
		CollaborationService:            collaborationService,
		CallLogsService:                 callLogsService,
		SMSService:                      smsService,
		CalendarService:                 calendarService,
		ImportsService:                  importsService,
		BulkOperationsService:           bulkOperationsService,
		ArchiveOperationsService:        archiveOperationsService,
		DuplicateOperationsService:      duplicateOperationsService,
		CustomFieldsService:             customFieldsService,
		SavedViewsService:               savedViewsService,
		OnboardingService:               onboardingService,
		PasswordResetService:            passwordResetService,
		OrgProfileService:               orgProfileService,
		NotificationsService:            notificationsService,
		BillingService:                  billingService,
		EmailService:                    emailService,
		EmailFeedbackService:            emailFeedbackService,
		EmailTemplatesService:           emailTemplatesService,
		ProductCatalogService:           productCatalogService,
		LeadFormsService:                leadFormsService,
		LeadAudiencesService:            leadAudiencesService,
		MarketingCampaignsService:       marketingCampaignsService,
		NurtureCampaignsService:         nurtureCampaignsService,
		LeadScoringService:              leadScoringService,
		WorkflowAutomationsService:      workflowAutomationsService,
		CustomReportsService:            customReportsService,
		DataQualityService:              dataQualityService,
		SalesReportsService:             salesReportsService,
		TouchpointsService:              touchpointsService,
		EmailSequencesService:           emailSequencesService,
		EmailSequenceEnrollmentsService: emailSequencesService,
		EmailSuppressionsService:        emailSuppressionsService,
		UserEmailService:                userEmailService,
		MailboxSyncService:              mailboxSyncService,
		EmailMessagesService:            emailMessagesService,
	}))

	log.Printf("open_crm api listening on %s", env.APIAddress())
	if err := serveWithShutdown(ctx, server); err != nil {
		log.Fatal(err)
	}
}

func backgroundWorkerID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "open-crm"
	}
	return fmt.Sprintf("%s:%d", hostname, os.Getpid())
}

func newHTTPServer(env config.Env, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              env.APIAddress(),
		Handler:           handler,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      serverWriteTimeout,
		IdleTimeout:       serverIdleTimeout,
	}
}

func serveWithShutdown(ctx context.Context, server *http.Server) error {
	serverErr := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}

	return <-serverErr
}
