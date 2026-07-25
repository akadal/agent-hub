package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/akadal/agent-hub/backend/internal/auth"
	"github.com/akadal/agent-hub/backend/internal/store"
)

// fakeTailnet serves the Tailscale devices endpoint with a fixed fleet:
// two hosts seen just now, one that has been away for a day.
func fakeTailnet(t *testing.T) *httptest.Server {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	stale := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	body := fmt.Sprintf(`{"devices":[
	  {"id":"1","name":"alpha.example.ts.net","hostname":"alpha","os":"linux","addresses":["100.64.0.1"],"authorized":true,"lastSeen":%q},
	  {"id":"2","name":"beta.example.ts.net","hostname":"beta","os":"linux","addresses":["100.64.0.2"],"authorized":true,"lastSeen":%q},
	  {"id":"3","name":"gamma.example.ts.net","hostname":"gamma","os":"linux","addresses":["100.64.0.3"],"authorized":true,"lastSeen":%q}
	]}`, now, now, stale)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/devices") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

func importServer(t *testing.T, baseURL string) (http.Handler, string) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := st.EnsureBootstrapAdmin("admin", "import-test-pass"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	tokens := auth.NewTokenService("import-test-secret", time.Hour)
	srv := &Server{Store: st, Tokens: tokens, TailscaleAPIKey: "tskey-test", TailscaleBaseURL: baseURL}

	admin, err := st.Authenticate("admin", "import-test-pass")
	if err != nil {
		t.Fatalf("authenticate admin: %v", err)
	}
	token, _, err := tokens.Issue(admin.ID, admin.Username, admin.Role)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return srv.NewMux(), token
}

func postImport(t *testing.T, mux http.Handler, token, body string) map[string]any {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/machines/tailscale/import", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("import: got %d, body %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func addedAddresses(t *testing.T, res map[string]any) []string {
	t.Helper()
	raw, _ := res["added"].([]any)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("added entry is %T, want object", item)
		}
		addr, _ := m["address"].(string)
		out = append(out, addr)
	}
	return out
}

func TestImportAddsOnlyTheDevicesThatWerePicked(t *testing.T) {
	ts := fakeTailnet(t)
	defer ts.Close()
	mux, token := importServer(t, ts.URL)

	res := postImport(t, mux, token, `{"addresses":["100.64.0.2"],"ssh_user":"ops"}`)

	got := addedAddresses(t, res)
	if len(got) != 1 || got[0] != "100.64.0.2" {
		t.Fatalf("added %v, want only 100.64.0.2 — an unticked host must never be registered", got)
	}
}

func TestImportHonoursAPickedDeviceThatIsOffline(t *testing.T) {
	ts := fakeTailnet(t)
	defer ts.Close()
	mux, token := importServer(t, ts.URL)

	// gamma has not been seen for a day, so the online filter would drop it.
	// An explicit tick is the operator saying they want it anyway.
	res := postImport(t, mux, token, `{"addresses":["100.64.0.3"],"ssh_user":"ops"}`)

	got := addedAddresses(t, res)
	if len(got) != 1 || got[0] != "100.64.0.3" {
		t.Fatalf("added %v, want the picked offline host 100.64.0.3", got)
	}
}

func TestImportWithoutAPickKeepsTheOldAllOnlineBehaviour(t *testing.T) {
	ts := fakeTailnet(t)
	defer ts.Close()
	mux, token := importServer(t, ts.URL)

	res := postImport(t, mux, token, `{"ssh_user":"ops"}`)

	got := addedAddresses(t, res)
	if len(got) != 2 {
		t.Fatalf("added %v, want the two online hosts (scripts still post no address list)", got)
	}
}

func TestImportNeedsNoCredential(t *testing.T) {
	ts := fakeTailnet(t)
	defer ts.Close()
	mux, token := importServer(t, ts.URL)

	// Tailscale SSH authorises by tailnet ACL, so the UI lets operators skip
	// credentials entirely. That must register a usable machine, not fail.
	res := postImport(t, mux, token, `{"addresses":["100.64.0.1"]}`)

	if got := addedAddresses(t, res); len(got) != 1 {
		t.Fatalf("added %v, want one machine imported with no credential", got)
	}
}

func TestImportSkipsAddressesAlreadyRegistered(t *testing.T) {
	ts := fakeTailnet(t)
	defer ts.Close()
	mux, token := importServer(t, ts.URL)

	first := postImport(t, mux, token, `{"addresses":["100.64.0.1"],"ssh_user":"ops"}`)
	if got := addedAddresses(t, first); len(got) != 1 {
		t.Fatalf("first import added %v, want one", got)
	}

	second := postImport(t, mux, token, `{"addresses":["100.64.0.1"],"ssh_user":"ops"}`)
	if got := addedAddresses(t, second); len(got) != 0 {
		t.Fatalf("second import added %v, want none — re-importing must not duplicate a host", got)
	}
}
