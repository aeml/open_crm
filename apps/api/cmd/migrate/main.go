package main

import (
	"context"
	"fmt"
	"log"

	"github.com/aeml/open_crm/apps/api/internal/db"
)

const usageText = "Run database migrations using DATABASE_URL from the environment."

func main() {
	cfg, err := db.LoadConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	if err := db.RunMigrations(context.Background(), cfg); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("applied %d migration(s)\n", len(db.MigrationFiles()))
}
