package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/akadal/agent-hub/backend/internal/api"
	"github.com/akadal/agent-hub/backend/internal/auth"
	"github.com/akadal/agent-hub/backend/internal/store"
)

func testServer(t *testing.T) (*api.Server, http.Handler) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureBootstrapAdmin("admin", "123456"); err != nil {
		t.Fatal(err)
	}
	tokens := auth.NewTokenService("test-secret", time.Hour)
	s := &api.Server{Store: st, Tokens: tokens}
	return s, s.NewMux()
}

func loginToken(t *testing.T, mux http.Handler, user, pass string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": user, "password": pass})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Token == "" {
		t.Fatal("empty token")
	}
	return resp.Token
}

func TestGrants_enforceMachineAccess(t *testing.T) {
	s, mux := testServer(t)
	adminTok := loginToken(t, mux, "admin", "123456")

	// create alice
	body, _ := json.Marshal(map[string]string{
		"username": "alice", "password": "alicepass", "role": "user",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create user status=%d %s", rr.Code, rr.Body.String())
	}
	var alice map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &alice)
	aliceID, _ := alice["id"].(string)

	// admin creates machine (owned by admin)
	mbody, _ := json.Marshal(map[string]any{
		"name": "box", "address": "10.0.0.9", "port": 22,
		"ssh_user": "root", "ssh_password": "x",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/machines", bytes.NewReader(mbody))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create machine %d %s", rr.Code, rr.Body.String())
	}
	var m map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &m)
	mid, _ := m["id"].(string)

	aliceTok := loginToken(t, mux, "alice", "alicepass")

	// alice cannot see machine yet
	req = httptest.NewRequest(http.MethodGet, "/api/machines/"+mid, nil)
	req.Header.Set("Authorization", "Bearer "+aliceTok)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404 before grant, got %d", rr.Code)
	}

	// grant
	gbody, _ := json.Marshal(map[string]string{"user_id": aliceID, "machine_id": mid})
	req = httptest.NewRequest(http.MethodPost, "/api/grants", bytes.NewReader(gbody))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("grant %d %s", rr.Code, rr.Body.String())
	}

	// alice can see
	req = httptest.NewRequest(http.MethodGet, "/api/machines/"+mid, nil)
	req.Header.Set("Authorization", "Bearer "+aliceTok)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 after grant, got %d %s", rr.Code, rr.Body.String())
	}

	// settings default
	req = httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	req.Header.Set("Authorization", "Bearer "+aliceTok)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("settings %d", rr.Code)
	}

	// audit is admin-only
	req = httptest.NewRequest(http.MethodGet, "/api/audit", nil)
	req.Header.Set("Authorization", "Bearer "+aliceTok)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("audit want 403 got %d", rr.Code)
	}
	_ = s // keep server ref for future assertions
}

func TestHealth_shippedHandler(t *testing.T) {
	_, mux := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" || body["service"] != "agent-hub" {
		t.Fatalf("unexpected body: %v", body)
	}
}

func TestLogin_bootstrapAdmin(t *testing.T) {
	_, mux := testServer(t)
	tok := loginToken(t, mux, "admin", "123456")
	if len(tok) < 20 {
		t.Fatalf("token too short: %q", tok)
	}
}

// Coolify path domains strip the /api prefix: browser still calls /api/auth/login
// but the API process may receive /auth/login. Both must work.
func TestLogin_coolifyStrippedPath(t *testing.T) {
	_, mux := testServer(t)
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "123456"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("stripped path status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Token == "" {
		t.Fatal("empty token on stripped path")
	}
}

func TestLogin_badPassword(t *testing.T) {
	_, mux := testServer(t)
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", rr.Code)
	}
}

func TestProtectedRoutes_requireAuth(t *testing.T) {
	_, mux := testServer(t)

	// without token
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("me unauth status=%d", rr.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/machines", nil)
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Fatalf("machines unauth status=%d", rr2.Code)
	}

	// with token
	tok := loginToken(t, mux, "admin", "123456")
	req3 := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req3.Header.Set("Authorization", "Bearer "+tok)
	rr3 := httptest.NewRecorder()
	mux.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Fatalf("me auth status=%d body=%s", rr3.Code, rr3.Body.String())
	}
	var me map[string]string
	_ = json.Unmarshal(rr3.Body.Bytes(), &me)
	if me["username"] != "admin" || me["role"] != "admin" {
		t.Fatalf("me body=%v", me)
	}
}

func TestMachineRegisterAndList(t *testing.T) {
	_, mux := testServer(t)
	tok := loginToken(t, mux, "admin", "123456")

	body, _ := json.Marshal(map[string]any{
		"name":         "dummy",
		"address":      "ssh-target",
		"port":         22,
		"ssh_user":     "root",
		"ssh_password": "targetpass",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/machines", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created["address"] != "ssh-target" {
		t.Fatalf("address=%v", created["address"])
	}
	if _, ok := created["ssh_password"]; ok {
		t.Fatal("ssh_password must not be exposed in response")
	}

	reqL := httptest.NewRequest(http.MethodGet, "/api/machines", nil)
	reqL.Header.Set("Authorization", "Bearer "+tok)
	rrL := httptest.NewRecorder()
	mux.ServeHTTP(rrL, reqL)
	if rrL.Code != http.StatusOK {
		t.Fatalf("list status=%d", rrL.Code)
	}
	var list struct {
		Machines []map[string]any `json:"machines"`
	}
	if err := json.Unmarshal(rrL.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Machines) != 1 {
		t.Fatalf("want 1 machine, got %d", len(list.Machines))
	}
	if list.Machines[0]["name"] != "dummy" {
		t.Fatalf("name=%v", list.Machines[0]["name"])
	}
}

func TestStorePathUsesTempDir(t *testing.T) {
	// sanity: store file is created under data dir used by Open
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureBootstrapAdmin("admin", "123456"); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "store.json")
	if _, err := filepath.Glob(p); err != nil {
		t.Fatal(err)
	}
}

func createMachine(t *testing.T, mux http.Handler, tok string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"name":         "dummy",
		"address":      "ssh-target",
		"port":         22,
		"ssh_user":     "root",
		"ssh_password": "targetpass",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/machines", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create machine status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("empty machine id")
	}
	return id
}

func TestTerminalSessions_createListClose(t *testing.T) {
	_, mux := testServer(t)
	tok := loginToken(t, mux, "admin", "123456")
	mid := createMachine(t, mux, tok)

	// create two sessions under the same machine
	var ids []string
	for _, name := range []string{"build", "debug"} {
		body, _ := json.Marshal(map[string]string{"name": name})
		req := httptest.NewRequest(http.MethodPost, "/api/machines/"+mid+"/terminals", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create terminal %s status=%d body=%s", name, rr.Code, rr.Body.String())
		}
		var term map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &term); err != nil {
			t.Fatal(err)
		}
		if term["name"] != name {
			t.Fatalf("name=%v want %s", term["name"], name)
		}
		if term["machine_id"] != mid {
			t.Fatalf("machine_id=%v want %s", term["machine_id"], mid)
		}
		id, _ := term["id"].(string)
		if id == "" {
			t.Fatal("empty terminal id")
		}
		ids = append(ids, id)
	}
	if ids[0] == ids[1] {
		t.Fatal("session ids must be distinct")
	}

	// list under machine — both present
	reqL := httptest.NewRequest(http.MethodGet, "/api/machines/"+mid+"/terminals", nil)
	reqL.Header.Set("Authorization", "Bearer "+tok)
	rrL := httptest.NewRecorder()
	mux.ServeHTTP(rrL, reqL)
	if rrL.Code != http.StatusOK {
		t.Fatalf("list status=%d", rrL.Code)
	}
	var list struct {
		Terminals []map[string]any `json:"terminals"`
	}
	if err := json.Unmarshal(rrL.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Terminals) != 2 {
		t.Fatalf("want 2 terminals, got %d", len(list.Terminals))
	}
	seen := map[string]bool{}
	for _, term := range list.Terminals {
		seen[term["id"].(string)] = true
	}
	for _, id := range ids {
		if !seen[id] {
			t.Fatalf("missing terminal id %s in list", id)
		}
	}

	// close one
	reqD := httptest.NewRequest(http.MethodDelete, "/api/terminals/"+ids[0], nil)
	reqD.Header.Set("Authorization", "Bearer "+tok)
	rrD := httptest.NewRecorder()
	mux.ServeHTTP(rrD, reqD)
	if rrD.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", rrD.Code, rrD.Body.String())
	}

	reqL2 := httptest.NewRequest(http.MethodGet, "/api/machines/"+mid+"/terminals", nil)
	reqL2.Header.Set("Authorization", "Bearer "+tok)
	rrL2 := httptest.NewRecorder()
	mux.ServeHTTP(rrL2, reqL2)
	var list2 struct {
		Terminals []map[string]any `json:"terminals"`
	}
	_ = json.Unmarshal(rrL2.Body.Bytes(), &list2)
	if len(list2.Terminals) != 1 {
		t.Fatalf("want 1 terminal after close, got %d", len(list2.Terminals))
	}
	if list2.Terminals[0]["id"] != ids[1] {
		t.Fatalf("remaining id=%v want %s", list2.Terminals[0]["id"], ids[1])
	}

	// get remaining
	reqG := httptest.NewRequest(http.MethodGet, "/api/terminals/"+ids[1], nil)
	reqG.Header.Set("Authorization", "Bearer "+tok)
	rrG := httptest.NewRecorder()
	mux.ServeHTTP(rrG, reqG)
	if rrG.Code != http.StatusOK {
		t.Fatalf("get status=%d", rrG.Code)
	}
}

func TestTailscaleStatus_notConfigured(t *testing.T) {
	_, mux := testServer(t)
	tok := loginToken(t, mux, "admin", "123456")
	req := httptest.NewRequest(http.MethodGet, "/api/machines/tailscale", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["configured"] != false {
		t.Fatalf("want configured=false, got %v", body)
	}
}

func TestTailscaleImport_notConfigured(t *testing.T) {
	_, mux := testServer(t)
	tok := loginToken(t, mux, "admin", "123456")
	body, _ := json.Marshal(map[string]any{"ssh_user": "root", "port": 22})
	req := httptest.NewRequest(http.MethodPost, "/api/machines/tailscale/import", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503 body=%s", rr.Code, rr.Body.String())
	}
}

func assertNoPasswordHash(t *testing.T, m map[string]any) {
	t.Helper()
	if _, ok := m["password_hash"]; ok {
		t.Fatal("password_hash must not be exposed in API response")
	}
	if _, ok := m["password"]; ok {
		t.Fatal("password must not be exposed in API response")
	}
}

// TestUsers_adminCRUD_andNewUserLogin exercises the shipped multi-user path:
// admin create → list → new user login/me → update role/password → delete.
func TestUsers_adminCRUD_andNewUserLogin(t *testing.T) {
	_, mux := testServer(t)
	adminTok := loginToken(t, mux, "admin", "123456")

	// create user
	body, _ := json.Marshal(map[string]string{
		"username": "operator",
		"password": "op-secret",
		"role":     "user",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	assertNoPasswordHash(t, created)
	if created["username"] != "operator" || created["role"] != "user" {
		t.Fatalf("created=%v", created)
	}
	userID, _ := created["id"].(string)
	if userID == "" {
		t.Fatal("empty user id")
	}

	// list users (admin)
	reqL := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	reqL.Header.Set("Authorization", "Bearer "+adminTok)
	rrL := httptest.NewRecorder()
	mux.ServeHTTP(rrL, reqL)
	if rrL.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rrL.Code, rrL.Body.String())
	}
	var list struct {
		Users []map[string]any `json:"users"`
	}
	if err := json.Unmarshal(rrL.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Users) < 2 {
		t.Fatalf("want >=2 users, got %d", len(list.Users))
	}
	for _, u := range list.Users {
		assertNoPasswordHash(t, u)
	}

	// new user can log in and /me matches stored role
	opTok := loginToken(t, mux, "operator", "op-secret")
	reqMe := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	reqMe.Header.Set("Authorization", "Bearer "+opTok)
	rrMe := httptest.NewRecorder()
	mux.ServeHTTP(rrMe, reqMe)
	if rrMe.Code != http.StatusOK {
		t.Fatalf("me status=%d body=%s", rrMe.Code, rrMe.Body.String())
	}
	var me map[string]string
	if err := json.Unmarshal(rrMe.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if me["username"] != "operator" || me["role"] != "user" || me["id"] != userID {
		t.Fatalf("me=%v", me)
	}

	// update password + promote to admin
	updBody, _ := json.Marshal(map[string]string{
		"password": "op-rotated",
		"role":     "admin",
	})
	reqU := httptest.NewRequest(http.MethodPatch, "/api/users/"+userID, bytes.NewReader(updBody))
	reqU.Header.Set("Authorization", "Bearer "+adminTok)
	reqU.Header.Set("Content-Type", "application/json")
	rrU := httptest.NewRecorder()
	mux.ServeHTTP(rrU, reqU)
	if rrU.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", rrU.Code, rrU.Body.String())
	}
	var updated map[string]any
	if err := json.Unmarshal(rrU.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	assertNoPasswordHash(t, updated)
	if updated["role"] != "admin" {
		t.Fatalf("role after update=%v", updated["role"])
	}

	// login with new password; /me role is admin
	opTok2 := loginToken(t, mux, "operator", "op-rotated")
	reqMe2 := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	reqMe2.Header.Set("Authorization", "Bearer "+opTok2)
	rrMe2 := httptest.NewRecorder()
	mux.ServeHTTP(rrMe2, reqMe2)
	var me2 map[string]string
	_ = json.Unmarshal(rrMe2.Body.Bytes(), &me2)
	if me2["role"] != "admin" {
		t.Fatalf("me after promote=%v", me2)
	}

	// delete the secondary admin
	reqD := httptest.NewRequest(http.MethodDelete, "/api/users/"+userID, nil)
	reqD.Header.Set("Authorization", "Bearer "+adminTok)
	rrD := httptest.NewRecorder()
	mux.ServeHTTP(rrD, reqD)
	if rrD.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", rrD.Code, rrD.Body.String())
	}

	// login as deleted user fails
	bodyLogin, _ := json.Marshal(map[string]string{"username": "operator", "password": "op-rotated"})
	reqBad := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(bodyLogin))
	reqBad.Header.Set("Content-Type", "application/json")
	rrBad := httptest.NewRecorder()
	mux.ServeHTTP(rrBad, reqBad)
	if rrBad.Code != http.StatusUnauthorized {
		t.Fatalf("login after delete status=%d", rrBad.Code)
	}
}

func TestUsers_nonAdminForbidden(t *testing.T) {
	s, mux := testServer(t)
	// seed a regular user via store (admin path creates via API in other test)
	if _, err := s.Store.CreateUser("regular", "regpass", store.RoleUser); err != nil {
		t.Fatal(err)
	}
	regTok := loginToken(t, mux, "regular", "regpass")

	// list forbidden
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+regTok)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("list status=%d want 403", rr.Code)
	}

	// create forbidden
	body, _ := json.Marshal(map[string]string{"username": "x", "password": "y", "role": "user"})
	reqC := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(body))
	reqC.Header.Set("Authorization", "Bearer "+regTok)
	reqC.Header.Set("Content-Type", "application/json")
	rrC := httptest.NewRecorder()
	mux.ServeHTTP(rrC, reqC)
	if rrC.Code != http.StatusForbidden {
		t.Fatalf("create status=%d want 403", rrC.Code)
	}

	// unauthenticated
	reqU := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rrU := httptest.NewRecorder()
	mux.ServeHTTP(rrU, reqU)
	if rrU.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d want 401", rrU.Code)
	}
}

func TestUsers_lastAdminDeleteRejected(t *testing.T) {
	_, mux := testServer(t)
	adminTok := loginToken(t, mux, "admin", "123456")

	// find bootstrap admin id via list
	reqL := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	reqL.Header.Set("Authorization", "Bearer "+adminTok)
	rrL := httptest.NewRecorder()
	mux.ServeHTTP(rrL, reqL)
	var list struct {
		Users []map[string]any `json:"users"`
	}
	if err := json.Unmarshal(rrL.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Users) != 1 {
		t.Fatalf("want only bootstrap admin, got %d", len(list.Users))
	}
	id, _ := list.Users[0]["id"].(string)

	reqD := httptest.NewRequest(http.MethodDelete, "/api/users/"+id, nil)
	reqD.Header.Set("Authorization", "Bearer "+adminTok)
	rrD := httptest.NewRecorder()
	mux.ServeHTTP(rrD, reqD)
	if rrD.Code != http.StatusConflict {
		t.Fatalf("delete last admin status=%d want 409 body=%s", rrD.Code, rrD.Body.String())
	}

	// demote last admin also refused
	updBody, _ := json.Marshal(map[string]string{"role": "user"})
	reqU := httptest.NewRequest(http.MethodPatch, "/api/users/"+id, bytes.NewReader(updBody))
	reqU.Header.Set("Authorization", "Bearer "+adminTok)
	reqU.Header.Set("Content-Type", "application/json")
	rrU := httptest.NewRecorder()
	mux.ServeHTTP(rrU, reqU)
	if rrU.Code != http.StatusConflict {
		t.Fatalf("demote last admin status=%d want 409 body=%s", rrU.Code, rrU.Body.String())
	}
}

// Dual-mount: Coolify may strip /api so /users must work without the prefix.
func TestUsers_strippedPath(t *testing.T) {
	_, mux := testServer(t)
	adminTok := loginToken(t, mux, "admin", "123456")
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("stripped list status=%d body=%s", rr.Code, rr.Body.String())
	}
}
