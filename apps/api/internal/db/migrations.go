package db

import _ "embed"

//go:embed migrations/001_initial_schema.sql
var initialSchemaSQL string

func MigrationFiles() []string {
	return []string{"001_initial_schema.sql"}
}

func MigrationSQL(name string) string {
	if name == "001_initial_schema.sql" {
		return initialSchemaSQL
	}
	return ""
}
