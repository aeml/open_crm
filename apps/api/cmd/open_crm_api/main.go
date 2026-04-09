package main

import (
	"log"
	"net/http"

	"github.com/aeml/open_crm/apps/api/internal/app"
	"github.com/aeml/open_crm/apps/api/internal/config"
)

func main() {
	env := config.Load()
	server := &http.Server{
		Addr:    env.APIAddress(),
		Handler: app.NewServer(env),
	}

	log.Printf("open_crm api listening on %s", env.APIAddress())
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
