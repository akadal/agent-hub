package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/akadal/agent-hub/backend/internal/api"
	"github.com/akadal/agent-hub/backend/internal/auth"
	"github.com/akadal/agent-hub/backend/internal/store"
)

// A regular user gets their first password from an admin. Until POST
// /me/password existed there was no way for them to replace it — only admins
// could write to /users/{id}.
func TestRegularUserCanRotateTheirOwnPassword(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureBootstrapAdmin("admin", "admin-pass"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser("dev", "handed-out", "user"); err != nil {
		t.Fatal(err)
	}
	s := &api.Server{Store: st, Tokens: auth.NewTokenService("test-secret", time.Hour)}
	mux := s.NewMux()

	call := func(method, path, token string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}
	login := func(user, pass string) (string, int) {
		rr := call("POST", "/api/auth/login", "", map[string]string{"username": user, "password": pass})
		var out struct {
			Token string `json:"token"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &out)
		return out.Token, rr.Code
	}

	token, code := login("dev", "handed-out")
	if code != http.StatusOK || token == "" {
		t.Fatalf("login as dev: status %d", code)
	}

	// Wrong current password must not rotate anything.
	if rr := call("POST", "/api/me/password", token, map[string]string{
		"current_password": "not-it", "new_password": "chosen",
	}); rr.Code != http.StatusForbidden {
		t.Fatalf("wrong current password: status %d, want 403 (body %s)", rr.Code, rr.Body)
	}
	if _, code := login("dev", "handed-out"); code != http.StatusOK {
		t.Fatal("a rejected change altered the password anyway")
	}

	if rr := call("POST", "/api/me/password", token, map[string]string{
		"current_password": "handed-out", "new_password": "chosen-by-dev",
	}); rr.Code != http.StatusNoContent {
		t.Fatalf("change password: status %d (body %s)", rr.Code, rr.Body)
	}
	if _, code := login("dev", "chosen-by-dev"); code != http.StatusOK {
		t.Fatal("the new password does not log in")
	}
	if _, code := login("dev", "handed-out"); code == http.StatusOK {
		t.Fatal("the old password still logs in")
	}

	// Anonymous callers must not reach it at all.
	if rr := call("POST", "/api/me/password", "", map[string]string{
		"current_password": "chosen-by-dev", "new_password": "x",
	}); rr.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated change: status %d, want 401", rr.Code)
	}

	// The event is recorded, and the secrets are not.
	adminToken, _ := login("admin", "admin-pass")
	rr := call("GET", "/api/audit", adminToken, nil)
	body := rr.Body.String()
	if !bytes.Contains([]byte(body), []byte("user.password")) {
		t.Fatalf("audit log has no user.password event: %s", body)
	}
	for _, secret := range []string{"handed-out", "chosen-by-dev"} {
		if bytes.Contains([]byte(body), []byte(secret)) {
			t.Fatalf("audit log leaked the password %q", secret)
		}
	}
}
