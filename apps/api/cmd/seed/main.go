package main

import (
	"context"
	"fmt"
	"log"

	"github.com/aeml/open_crm/apps/api/internal/db"
)

const usageText = "Seed the database with a default CRM workspace using DATABASE_URL from the environment."

func main() {
	cfg, err := db.LoadConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	if err := db.SeedDatabase(context.Background(), cfg); err != nil {
		log.Fatal(err)
	}

	fmt.Println("seed complete for Acme, Inc. (owner@acme.test, admin@acme.test, member@acme.test, viewer@acme.test)")
}
