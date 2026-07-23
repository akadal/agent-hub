package auth_test

import (
	"testing"
	"time"

	"github.com/akadal/agent-hub/backend/internal/auth"
)

func TestTokenService_IssueAndParse(t *testing.T) {
	ts := auth.NewTokenService("secret-key", time.Hour)
	tok, exp, err := ts.Issue("uid-1", "akadal", "admin")
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
	if claims.UserID != "uid-1" || claims.Username != "akadal" || claims.Role != "admin" {
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
