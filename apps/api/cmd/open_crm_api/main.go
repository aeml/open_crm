package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/app"
	"github.com/aeml/open_crm/apps/api/internal/config"
	"github.com/aeml/open_crm/apps/api/internal/db"
	moduleaudit "github.com/aeml/open_crm/apps/api/internal/modules/audit"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	modulecalendar "github.com/aeml/open_crm/apps/api/internal/modules/calendar"
	modulecalllogs "github.com/aeml/open_crm/apps/api/internal/modules/calllogs"
	modulecompanies "github.com/aeml/open_crm/apps/api/internal/modules/companies"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	moduledashboard "github.com/aeml/open_crm/apps/api/internal/modules/dashboard"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
	moduleemailsuppressions "github.com/aeml/open_crm/apps/api/internal/modules/emailsuppressions"
	moduleemailtemplates "github.com/aeml/open_crm/apps/api/internal/modules/emailtemplates"
	moduleexports "github.com/aeml/open_crm/apps/api/internal/modules/exports"
	moduleimports "github.com/aeml/open_crm/apps/api/internal/modules/imports"
	modulemailboxsync "github.com/aeml/open_crm/apps/api/internal/modules/mailboxsync"
	modulenotes "github.com/aeml/open_crm/apps/api/internal/modules/notes"
	modulenotifications "github.com/aeml/open_crm/apps/api/internal/modules/notifications"
	moduleonboarding "github.com/aeml/open_crm/apps/api/internal/modules/onboarding"
	moduleorgprofile "github.com/aeml/open_crm/apps/api/internal/modules/orgprofile"
	moduleproductcatalog "github.com/aeml/open_crm/apps/api/internal/modules/productcatalog"
	modulesavedviews "github.com/aeml/open_crm/apps/api/internal/modules/savedviews"
	modulesequencerunner "github.com/aeml/open_crm/apps/api/internal/modules/sequencerunner"
	modulesms "github.com/aeml/open_crm/apps/api/internal/modules/sms"
	moduletasks "github.com/aeml/open_crm/apps/api/internal/modules/tasks"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
	platformlogger "github.com/aeml/open_crm/apps/api/internal/platform/logger"
	platformsecrets "github.com/aeml/open_crm/apps/api/internal/platform/secrets"
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
	var dashboardService *moduledashboard.Service
	importsService := moduleimports.NewService()
	emailProvider := moduleemail.NewProvider(moduleemail.ProviderConfig{
		Name:                  env.EmailProvider,
		Logger:                logger,
		PostmarkServerToken:   env.PostmarkServerToken,
		PostmarkFromEmail:     env.PostmarkFromEmail,
		PostmarkMessageStream: env.PostmarkMessageStream,
	})
	emailService := moduleemail.NewService(emailProvider, env.EmailFromName, env.EmailFromAddress, env.WebBaseURL)
	var notesService *modulenotes.Service
	var callLogsService *modulecalllogs.Service
	var smsService *modulesms.Service
	var calendarService *modulecalendar.Service
	var notificationsService *modulenotifications.Service
	var savedViewsService *modulesavedviews.Service
	var onboardingService *moduleonboarding.Service
	var orgProfileService *moduleorgprofile.Service
	var billingService *modulebilling.Service
	var emailTemplatesService *moduleemailtemplates.Service
	var productCatalogService *moduleproductcatalog.Service
	var emailSequencesService *moduleemailsequences.Service
	var emailSuppressionsService *moduleemailsuppressions.Service
	var userEmailService *moduleuseremail.Service
	var emailMessagesService *moduleemailmessages.Service
	var mailboxSyncService *modulemailboxsync.Service
	var sequenceRunnerService *modulesequencerunner.Service
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
			authService = moduleauth.NewService(pool)
			auditService = moduleaudit.NewService(pool)
			usersService = moduleusers.NewService(pool)
			contactsService = modulecontacts.NewService(pool)
			companiesService = modulecompanies.NewService(pool)
			dealsService = moduledeals.NewService(pool)
			tasksService = moduletasks.NewService(pool)
			exportsService = moduleexports.NewService(pool)
			dashboardService = moduledashboard.NewService(pool)
			notesService = modulenotes.NewService(pool)
			callLogsService = modulecalllogs.NewService(pool, modulecalllogs.NewProvider(env.TelephonyProvider, logger))
			smsService = modulesms.NewService(pool, modulesms.NewProvider(env.TelephonyProvider, logger))
			calendarService = modulecalendar.NewService(pool, modulecalendar.NewProvider(env.CalendarProvider, logger))
			notificationsService = modulenotifications.NewService(pool)
			savedViewsService = modulesavedviews.NewService(pool)
			onboardingService = moduleonboarding.NewService(pool)
			orgProfileService = moduleorgprofile.NewService(pool)
			billingService = modulebilling.NewService(pool, modulebilling.NewProvider(env.BillingProvider))
			emailTemplatesService = moduleemailtemplates.NewService(pool)
			productCatalogService = moduleproductcatalog.NewService(pool)
			emailSequencesService = moduleemailsequences.NewService(pool)
			emailSuppressionsService = moduleemailsuppressions.NewService(pool, env.CredentialEncryptionKey)
			userEmailService = moduleuseremail.NewService(pool, credentialCipher)
			emailMessagesService = moduleemailmessages.NewService(pool)
			mailboxSyncService = modulemailboxsync.NewServiceWithOAuthRefresh(userEmailService, emailMessagesService, nil, modulemailboxsync.NewOAuthTokenRefresher(modulemailboxsync.OAuthTokenRefresherConfig{
				GoogleClientID:        env.GoogleOAuthClientID,
				GoogleClientSecret:    env.GoogleOAuthClientSecret,
				MicrosoftClientID:     env.MicrosoftOAuthClientID,
				MicrosoftClientSecret: env.MicrosoftOAuthClientSecret,
			}))
			sequenceRunnerService = modulesequencerunner.NewServiceWithSuppressions(emailSequencesService, userEmailService, emailMessagesService, emailSuppressionsService, env.APIBaseURL)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if mailboxSyncService != nil && mailboxSyncService.Configured() {
		go mailboxSyncService.RunWorker(ctx, logger, 0, 0)
	}
	if sequenceRunnerService != nil && sequenceRunnerService.Configured() {
		go sequenceRunnerService.RunWorker(ctx, logger, 0, 0)
	}
	if calendarService != nil && calendarService.Configured() {
		go calendarService.RunReminderWorker(ctx, logger, 0, 0)
	}

	server := newHTTPServer(env, app.NewServer(env, app.Dependencies{
		CheckReadiness: func(ctx context.Context) error {
			if dbConfigErr != nil {
				return dbConfigErr
			}
			return db.CheckReadiness(ctx, dbConfig)
		},
		Logger:                          logger,
		AuthService:                     authService,
		AuditService:                    auditService,
		UsersService:                    usersService,
		ContactsService:                 contactsService,
		CompaniesService:                companiesService,
		DealsService:                    dealsService,
		TasksService:                    tasksService,
		ExportsService:                  exportsService,
		DashboardService:                dashboardService,
		NotesService:                    notesService,
		CallLogsService:                 callLogsService,
		SMSService:                      smsService,
		CalendarService:                 calendarService,
		ImportsService:                  importsService,
		SavedViewsService:               savedViewsService,
		OnboardingService:               onboardingService,
		OrgProfileService:               orgProfileService,
		NotificationsService:            notificationsService,
		BillingService:                  billingService,
		EmailService:                    emailService,
		EmailTemplatesService:           emailTemplatesService,
		ProductCatalogService:           productCatalogService,
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
