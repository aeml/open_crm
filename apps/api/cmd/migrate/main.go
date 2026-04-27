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

	result, err := db.RunMigrations(context.Background(), cfg)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("applied %d migration(s), skipped %d already-applied migration(s)\n", result.AppliedCount(), result.SkippedCount())
}
