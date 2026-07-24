package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/akadal/agent-hub/backend/internal/api"
	"github.com/akadal/agent-hub/backend/internal/auth"
	"github.com/akadal/agent-hub/backend/internal/store"
)

// TestUsers_httpEvidence drives the shipped mux end-to-end and, when
// USER_MGMT_EVIDENCE is set, writes request/response lines for verification.
func TestUsers_httpEvidence(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureBootstrapAdmin("admin", "admin-e2e"); err != nil {
		t.Fatal(err)
	}
	tokens := auth.NewTokenService("e2e-secret", time.Hour)
	s := &api.Server{Store: st, Tokens: tokens}
	mux := s.NewMux()

	var logBuf bytes.Buffer
	logf := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		t.Log(line)
		logBuf.WriteString(line)
		logBuf.WriteByte('\n')
	}

	do := func(method, path, tok string, body any) (int, map[string]any, string) {
		var rdr *bytes.Reader
		rawBody := ""
		if body != nil {
			b, _ := json.Marshal(body)
			rawBody = string(b)
			rdr = bytes.NewReader(b)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, path, rdr)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var m map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &m)
		logf("%s %s req=%s -> status=%d body=%s", method, path, rawBody, rr.Code, rr.Body.String())
		return rr.Code, m, rr.Body.String()
	}

	// 1. admin login
	code, login, _ := do("POST", "/api/auth/login", "", map[string]string{
		"username": "admin", "password": "admin-e2e",
	})
	if code != http.StatusOK {
		t.Fatalf("login status=%d", code)
	}
	adminTok, _ := login["token"].(string)
	if adminTok == "" {
		t.Fatal("empty admin token")
	}

	// 2. create user
	code, created, raw := do("POST", "/api/users", adminTok, map[string]string{
		"username": "alice", "password": "alice-secret", "role": "user",
	})
	if code != http.StatusCreated {
		t.Fatalf("create status=%d", code)
	}
	if _, ok := created["password_hash"]; ok {
		t.Fatal("password_hash leaked on create")
	}
	if bytes.Contains([]byte(raw), []byte("alice-secret")) {
		t.Fatal("plaintext password leaked on create")
	}
	userID, _ := created["id"].(string)
	if userID == "" || created["username"] != "alice" || created["role"] != "user" {
		t.Fatalf("created=%v", created)
	}

	// 3. list
	code, listBody, listRaw := do("GET", "/api/users", adminTok, nil)
	if code != http.StatusOK {
		t.Fatalf("list status=%d", code)
	}
	if bytes.Contains([]byte(listRaw), []byte("password_hash")) {
		t.Fatal("password_hash in list")
	}
	users, _ := listBody["users"].([]any)
	if len(users) < 2 {
		t.Fatalf("want >=2 users, got %d", len(users))
	}

	// 4. new user login + /me
	code, aliceLogin, _ := do("POST", "/api/auth/login", "", map[string]string{
		"username": "alice", "password": "alice-secret",
	})
	if code != http.StatusOK {
		t.Fatalf("alice login status=%d", code)
	}
	aliceTok, _ := aliceLogin["token"].(string)
	code, me, _ := do("GET", "/api/me", aliceTok, nil)
	if code != http.StatusOK || me["username"] != "alice" || me["role"] != "user" {
		t.Fatalf("me=%v status=%d", me, code)
	}

	// 5. non-admin forbidden
	code, _, _ = do("GET", "/api/users", aliceTok, nil)
	if code != http.StatusForbidden {
		t.Fatalf("non-admin list status=%d want 403", code)
	}

	// 6. update password + role
	code, upd, _ := do("PATCH", "/api/users/"+userID, adminTok, map[string]string{
		"password": "alice-new", "role": "admin",
	})
	if code != http.StatusOK || upd["role"] != "admin" {
		t.Fatalf("update status=%d body=%v", code, upd)
	}
	if _, ok := upd["password_hash"]; ok {
		t.Fatal("password_hash on update")
	}

	// 7. login with new password / role
	code, aliceLogin2, _ := do("POST", "/api/auth/login", "", map[string]string{
		"username": "alice", "password": "alice-new",
	})
	if code != http.StatusOK {
		t.Fatalf("alice re-login status=%d", code)
	}
	u2, _ := aliceLogin2["user"].(map[string]any)
	if u2["role"] != "admin" {
		t.Fatalf("role after promote=%v", u2)
	}

	// 8. delete
	code, _, _ = do("DELETE", "/api/users/"+userID, adminTok, nil)
	if code != http.StatusNoContent {
		t.Fatalf("delete status=%d", code)
	}

	// 9. last-admin delete rejected
	code, listAfter, _ := do("GET", "/api/users", adminTok, nil)
	usersAfter, _ := listAfter["users"].([]any)
	if len(usersAfter) != 1 {
		t.Fatalf("want 1 user after delete, got %d", len(usersAfter))
	}
	only, _ := usersAfter[0].(map[string]any)
	onlyID, _ := only["id"].(string)
	code, _, _ = do("DELETE", "/api/users/"+onlyID, adminTok, nil)
	if code != http.StatusConflict {
		t.Fatalf("last admin delete status=%d want 409", code)
	}

	logf("ALL USER MGMT E2E CHECKS PASSED")

	if path := os.Getenv("USER_MGMT_EVIDENCE"); path != "" {
		if err := os.WriteFile(path, logBuf.Bytes(), 0o644); err != nil {
			t.Fatalf("write evidence: %v", err)
		}
		t.Logf("wrote evidence to %s", path)
	}
}
