package main

import "testing"

func TestMigrateCommandHasUsageText(t *testing.T) {
	if usageText == "" {
		t.Fatal("expected migrate command usage text to be defined")
	}
}
