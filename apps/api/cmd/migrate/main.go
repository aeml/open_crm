package main

import (
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

	fmt.Printf("migrations ready for %s with %d file(s)\n", cfg.DatabaseURL, len(db.MigrationFiles()))
}
