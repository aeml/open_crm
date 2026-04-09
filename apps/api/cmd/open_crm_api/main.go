package main

import (
	"context"
	"log"
	"net/http"

	"github.com/aeml/open_crm/apps/api/internal/app"
	"github.com/aeml/open_crm/apps/api/internal/config"
	"github.com/aeml/open_crm/apps/api/internal/db"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecompanies "github.com/aeml/open_crm/apps/api/internal/modules/companies"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	moduledashboard "github.com/aeml/open_crm/apps/api/internal/modules/dashboard"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleonboarding "github.com/aeml/open_crm/apps/api/internal/modules/onboarding"
	moduleorgprofile "github.com/aeml/open_crm/apps/api/internal/modules/orgprofile"
	moduletasks "github.com/aeml/open_crm/apps/api/internal/modules/tasks"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
)

func main() {
	env := config.Load()
	dbConfig, dbConfigErr := db.LoadConfigFromEnv()
	if dbConfigErr != nil {
		log.Printf("database config warning: %v", dbConfigErr)
	}

	var authService *moduleauth.Service
	var usersService *moduleusers.Service
	var contactsService *modulecontacts.Service
	var companiesService *modulecompanies.Service
	var dealsService *moduledeals.Service
	var tasksService *moduletasks.Service
	var dashboardService *moduledashboard.Service
	var onboardingService *moduleonboarding.Service
	var orgProfileService *moduleorgprofile.Service
	if dbConfigErr == nil {
		pool, err := db.NewPool(context.Background(), dbConfig)
		if err != nil {
			log.Printf("auth service disabled: %v", err)
		} else {
			defer pool.Close()
			authService = moduleauth.NewService(pool)
			usersService = moduleusers.NewService(pool)
			contactsService = modulecontacts.NewService(pool)
			companiesService = modulecompanies.NewService(pool)
			dealsService = moduledeals.NewService(pool)
			tasksService = moduletasks.NewService(pool)
			dashboardService = moduledashboard.NewService(pool)
			onboardingService = moduleonboarding.NewService(pool)
			orgProfileService = moduleorgprofile.NewService(pool)
		}
	}

	server := &http.Server{
		Addr: env.APIAddress(),
		Handler: app.NewServer(env, app.Dependencies{
			CheckReadiness: func(ctx context.Context) error {
				if dbConfigErr != nil {
					return dbConfigErr
				}
				return db.CheckReadiness(ctx, dbConfig)
			},
			AuthService:       authService,
			UsersService:      usersService,
			ContactsService:   contactsService,
			CompaniesService:  companiesService,
			DealsService:      dealsService,
			TasksService:      tasksService,
			DashboardService:  dashboardService,
			OnboardingService: onboardingService,
			OrgProfileService: orgProfileService,
		}),
	}

	log.Printf("open_crm api listening on %s", env.APIAddress())
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
