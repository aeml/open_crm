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
			AuthService:      authService,
			UsersService:     usersService,
			ContactsService:  contactsService,
			CompaniesService: companiesService,
		}),
	}

	log.Printf("open_crm api listening on %s", env.APIAddress())
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
