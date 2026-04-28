package logger

import (
	"log/slog"
	"testing"
)

func TestNewReturnsLogger(t *testing.T) {
	if got := New("production"); got == nil {
		t.Fatal("expected production logger")
	}
	if got := New("development"); got == nil {
		t.Fatal("expected development logger")
	}
	var _ *slog.Logger = New("")
}
