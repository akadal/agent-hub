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
