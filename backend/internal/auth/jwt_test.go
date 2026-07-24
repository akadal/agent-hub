package auth_test

import (
	"testing"
	"time"

	"github.com/akadal/agent-hub/backend/internal/auth"
)

func TestTokenService_IssueAndParse(t *testing.T) {
	ts := auth.NewTokenService("secret-key", time.Hour)
	tok, exp, err := ts.Issue("uid-1", "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if tok == "" {
		t.Fatal("empty token")
	}
	if exp.Before(time.Now()) {
		t.Fatal("expiry in the past")
	}
	claims, err := ts.Parse(tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != "uid-1" || claims.Username != "admin" || claims.Role != "admin" {
		t.Fatalf("claims=%+v", claims)
	}
}

func TestTokenService_RejectsTampered(t *testing.T) {
	ts := auth.NewTokenService("secret-key", time.Hour)
	tok, _, err := ts.Issue("u", "x", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ts.Parse(tok + "x"); err == nil {
		t.Fatal("expected error for tampered token")
	}
}

func TestTokenService_ForeverNoExpiry(t *testing.T) {
	ts := auth.NewTokenService("secret-key", 0) // forever
	tok, exp, err := ts.Issue("uid-1", "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if tok == "" {
		t.Fatal("empty token")
	}
	if !exp.IsZero() {
		t.Fatalf("expected zero expiresAt for forever token, got %v", exp)
	}
	claims, err := ts.Parse(tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.ExpiresAt != nil {
		t.Fatalf("expected no ExpiresAt claim, got %v", claims.ExpiresAt)
	}
	if claims.Username != "admin" {
		t.Fatalf("username=%q", claims.Username)
	}
}
