package db

import _ "embed"

//go:embed migrations/001_initial_schema.sql
var initialSchemaSQL string

//go:embed migrations/002_company_client_type.sql
var companyClientTypeSQL string

func MigrationFiles() []string {
	return []string{"001_initial_schema.sql", "002_company_client_type.sql"}
}

func MigrationSQL(name string) string {
	if name == "001_initial_schema.sql" {
		return initialSchemaSQL
	}
	if name == "002_company_client_type.sql" {
		return companyClientTypeSQL
	}
	return ""
}
