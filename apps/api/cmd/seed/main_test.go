package main

import "testing"

func TestSeedCommandHasUsageText(t *testing.T) {
	if usageText == "" {
		t.Fatal("expected seed command usage text to be defined")
	}
}
