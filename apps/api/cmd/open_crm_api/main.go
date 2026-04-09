package main

import (
	"context"
	"log"
	"net/http"

	"github.com/aeml/open_crm/apps/api/internal/app"
	"github.com/aeml/open_crm/apps/api/internal/config"
	"github.com/aeml/open_crm/apps/api/internal/db"
)

func main() {
	env := config.Load()
	dbConfig, dbConfigErr := db.LoadConfigFromEnv()
	if dbConfigErr != nil {
		log.Printf("database config warning: %v", dbConfigErr)
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
		}),
	}

	log.Printf("open_crm api listening on %s", env.APIAddress())
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
