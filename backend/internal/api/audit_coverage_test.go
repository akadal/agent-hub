package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/akadal/agent-hub/backend/internal/api"
	"github.com/akadal/agent-hub/backend/internal/auth"
	"github.com/akadal/agent-hub/backend/internal/store"
)

// Account changes are the most security-relevant thing an admin can do — the
// whole point of the audit log is that "who made this admin account, and when"
// has an answer. These paths used to mutate users and leave no trace at all.
func TestAuditCoversAdminMutations(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureBootstrapAdmin("admin", "admin-e2e"); err != nil {
		t.Fatal(err)
	}
	mux := (&api.Server{Store: st, Tokens: auth.NewTokenService("e2e-secret", time.Hour)}).NewMux()

	do := func(method, path, tok string, body any) (int, map[string]any, string) {
		var rdr *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
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
		return rr.Code, m, rr.Body.String()
	}

	code, login, _ := do("POST", "/api/auth/login", "", map[string]string{
		"username": "admin", "password": "admin-e2e",
	})
	if code != http.StatusOK {
		t.Fatalf("login status=%d", code)
	}
	tok, _ := login["token"].(string)

	code, created, _ := do("POST", "/api/users", tok, map[string]string{
		"username": "bob", "password": "bob-secret", "role": "user",
	})
	if code != http.StatusCreated {
		t.Fatalf("create user status=%d", code)
	}
	userID, _ := created["id"].(string)

	if code, _, _ = do("PATCH", "/api/users/"+userID, tok, map[string]string{
		"password": "bob-rotated", "role": "admin",
	}); code != http.StatusOK {
		t.Fatalf("update user status=%d", code)
	}
	if code, _, _ = do("DELETE", "/api/users/"+userID, tok, nil); code != http.StatusNoContent {
		t.Fatalf("delete user status=%d", code)
	}

	code, auditBody, auditRaw := do("GET", "/api/audit", tok, nil)
	if code != http.StatusOK {
		t.Fatalf("audit status=%d", code)
	}
	events, _ := auditBody["events"].([]any)

	actions := map[string]string{} // action -> detail
	for _, e := range events {
		ev, _ := e.(map[string]any)
		action, _ := ev["action"].(string)
		detail, _ := ev["detail"].(string)
		actions[action] = detail
		// Every event must name the actor, or the log answers "what" but not "who".
		if strings.HasPrefix(action, "user.") && ev["username"] != "admin" {
			t.Errorf("%s recorded actor %v, want admin", action, ev["username"])
		}
	}

	for _, want := range []string{"user.create", "user.update", "user.delete"} {
		if _, ok := actions[want]; !ok {
			t.Errorf("no %q event in audit log; got %v", want, keysOf(actions))
		}
	}
	if d := actions["user.create"]; !strings.Contains(d, "bob") {
		t.Errorf("user.create detail = %q, want it to name the account", d)
	}
	if d := actions["user.delete"]; !strings.Contains(d, "bob") {
		t.Errorf("user.delete detail = %q, want the username, not a bare id", d)
	}

	// An audit log that records the new password would be worse than no log.
	for _, secret := range []string{"bob-secret", "bob-rotated", "admin-e2e"} {
		if strings.Contains(auditRaw, secret) {
			t.Fatalf("audit log leaked the password %q", secret)
		}
	}
	if d := actions["user.update"]; !strings.Contains(d, "password") {
		t.Errorf("user.update detail = %q, want it to say a password was rotated", d)
	}
}

// A check authenticates with the stored credential, so it is a credential use
// and belongs in the trail — with its verdict, which is what makes the log
// readable when reconstructing a failure after the fact.
func TestAuditRecordsMachineCheckVerdict(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureBootstrapAdmin("admin", "admin-e2e"); err != nil {
		t.Fatal(err)
	}
	// TEST-NET-3: guaranteed unreachable, so the check fails fast and classified.
	m, err := st.CreateMachine("", store.MachineSpec{
		Name: "dead", Address: "203.0.113.1", Port: 22, SSHUser: "ops", SSHPassword: "pw",
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := (&api.Server{Store: st, Tokens: auth.NewTokenService("e2e-secret", time.Hour)}).NewMux()

	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(
		`{"username":"admin","password":"admin-e2e"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	var login map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &login)
	tok, _ := login["token"].(string)

	req = httptest.NewRequest("POST", "/api/machines/"+m.ID+"/check", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("check status=%d", rr.Code)
	}

	req = httptest.NewRequest("GET", "/api/audit", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	var auditBody map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &auditBody)
	events, _ := auditBody["events"].([]any)

	found := ""
	for _, e := range events {
		ev, _ := e.(map[string]any)
		if ev["action"] == "machine.check" {
			found, _ = ev["detail"].(string)
			if ev["machine_id"] != m.ID {
				t.Errorf("machine.check machine_id=%v, want %s", ev["machine_id"], m.ID)
			}
		}
	}
	if found == "" {
		t.Fatal("no machine.check event in the audit log")
	}
	if !strings.Contains(found, "failed") {
		t.Errorf("machine.check detail = %q, want the verdict recorded", found)
	}
	if strings.Contains(rr.Body.String(), "pw") && strings.Contains(rr.Body.String(), `"pw"`) {
		t.Fatal("audit log leaked the SSH password")
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
