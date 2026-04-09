package db

import (
	"context"
	"testing"
)

func TestNewPoolRejectsMissingDatabaseURL(t *testing.T) {
	_, err := NewPool(context.Background(), Config{})
	if err == nil {
		t.Fatal("expected missing database url to fail")
	}
}
