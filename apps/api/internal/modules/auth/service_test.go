package auth

import (
	"os"
	"strings"
	"testing"
)

func TestNormalizeLoginEmailTrimsAndLowercases(t *testing.T) {
	got := normalizeLoginEmail("  Owner@Acme.Test ")
	if got != "owner@acme.test" {
		t.Fatalf("expected normalized email owner@acme.test, got %q", got)
	}
}

func TestCredentialLookupSQLUsesIndexedEmailEquality(t *testing.T) {
	sql := credentialLookupSQL
	if strings.Contains(strings.ToLower(sql), "lower(") {
		t.Fatalf("expected credential lookup SQL to avoid lower(), got %q", sql)
	}
	if !strings.Contains(sql, "WHERE u.email = $1") {
		t.Fatalf("expected credential lookup SQL to use direct email equality, got %q", sql)
	}
	if strings.Contains(sql, "organization_memberships") || strings.Contains(sql, "JOIN organizations") {
		t.Fatalf("expected credential lookup SQL to avoid organization joins, got %q", sql)
	}
	if !strings.Contains(sql, "SELECT u.id, u.email, u.password_hash, u.email_verified_at") {
		t.Fatalf("expected credential lookup SQL to fetch only login credentials, got %q", sql)
	}
}

func TestSessionStateLookupSQLLoadsContextAfterPasswordValidation(t *testing.T) {
	sql := sessionStateByUserSQL
	if !strings.Contains(sql, "FROM organization_memberships om") {
		t.Fatalf("expected session state lookup to load memberships, got %q", sql)
	}
	if !strings.Contains(sql, "JOIN organizations o ON o.id = om.organization_id") {
		t.Fatalf("expected session state lookup to join organizations, got %q", sql)
	}
	if !strings.Contains(sql, "WHERE om.user_id = $1") {
		t.Fatalf("expected session state lookup to filter by user id, got %q", sql)
	}
	if !strings.Contains(sql, "membership_status") {
		t.Fatalf("expected session state lookup to reject disabled memberships, got %q", sql)
	}
	if !strings.Contains(sql, "email_verified_at IS NOT NULL") {
		t.Fatalf("expected session state lookup to reject unverified users, got %q", sql)
	}
}

func TestCurrentSessionPrunesExpiredSessions(t *testing.T) {
	serviceSource := readSourceFile(t, "service.go")
	if !strings.Contains(serviceSource, "s.pruneExpiredSessions(ctx)") {
		t.Fatal("expected CurrentSession to prune expired sessions before session lookup")
	}
	if expiredSessionsCleanupSQL != "DELETE FROM sessions WHERE expires_at <= NOW()" {
		t.Fatal("expected expired session cleanup query")
	}
}

func readSourceFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}
