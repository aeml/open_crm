package db

import _ "embed"

//go:embed migrations/001_initial_schema.sql
var initialSchemaSQL string

//go:embed migrations/002_company_client_type.sql
var companyClientTypeSQL string

//go:embed migrations/003_contact_client_flag.sql
var contactClientFlagSQL string

//go:embed migrations/004_client_address.sql
var clientAddressSQL string

//go:embed migrations/005_client_structured_address.sql
var clientStructuredAddressSQL string

//go:embed migrations/006_remove_company_domain.sql
var removeCompanyDomainSQL string

//go:embed migrations/007_task_archive.sql
var taskArchiveSQL string

func MigrationFiles() []string {
	return []string{"001_initial_schema.sql", "002_company_client_type.sql", "003_contact_client_flag.sql", "004_client_address.sql", "005_client_structured_address.sql", "006_remove_company_domain.sql", "007_task_archive.sql"}
}

func MigrationSQL(name string) string {
	if name == "001_initial_schema.sql" {
		return initialSchemaSQL
	}
	if name == "002_company_client_type.sql" {
		return companyClientTypeSQL
	}
	if name == "003_contact_client_flag.sql" {
		return contactClientFlagSQL
	}
	if name == "004_client_address.sql" {
		return clientAddressSQL
	}
	if name == "005_client_structured_address.sql" {
		return clientStructuredAddressSQL
	}
	if name == "006_remove_company_domain.sql" {
		return removeCompanyDomainSQL
	}
	if name == "007_task_archive.sql" {
		return taskArchiveSQL
	}
	return ""
}
