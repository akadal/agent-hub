package api

import (
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/akadal/agent-hub/backend/internal/auth"
	"github.com/akadal/agent-hub/backend/internal/store"
)

func exportServer(t *testing.T) (*store.Store, http.Handler, string) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureBootstrapAdmin("admin", "export-test-pass"); err != nil {
		t.Fatal(err)
	}
	tokens := auth.NewTokenService("export-test-secret", time.Hour)
	srv := &Server{Store: st, Tokens: tokens}
	admin, err := st.Authenticate("admin", "export-test-pass")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := tokens.Issue(admin.ID, admin.Username, admin.Role)
	if err != nil {
		t.Fatal(err)
	}
	return st, srv.NewMux(), token
}

func getExport(t *testing.T, mux http.Handler, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/audit/export", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestAuditExportReturnsEveryRetainedEvent(t *testing.T) {
	st, mux, token := exportServer(t)

	// More than the list endpoint's page of 200, so a truncating export shows up.
	for i := 0; i < 320; i++ {
		if err := st.AppendAudit(store.AuditEvent{Action: "login.ok", Username: "someone"}); err != nil {
			t.Fatal(err)
		}
	}

	rec := getExport(t, mux, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("Content-Type = %q, want text/csv", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("Content-Disposition = %q, want an attachment", cd)
	}

	rows, err := csv.NewReader(strings.NewReader(rec.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("export is not valid CSV: %v", err)
	}
	// header + the 320 events (the export itself is audited afterwards)
	if len(rows) != 321 {
		t.Fatalf("got %d rows, want 321 — export must not stop at the list page size", len(rows))
	}
	if rows[0][0] != "at" || rows[0][3] != "action" {
		t.Fatalf("unexpected header row: %v", rows[0])
	}
}

func TestAuditExportDefusesSpreadsheetFormulas(t *testing.T) {
	st, mux, token := exportServer(t)

	if err := st.AppendAudit(store.AuditEvent{
		Action:   "login.failed",
		Username: `=cmd|'/c calc'!A1`,
	}); err != nil {
		t.Fatal(err)
	}

	rows, err := csv.NewReader(strings.NewReader(getExport(t, mux, token).Body.String())).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 {
		t.Fatalf("no data rows: %v", rows)
	}
	username := rows[1][2]
	if strings.HasPrefix(username, "=") {
		t.Fatalf("username %q would run as a formula when the CSV is opened", username)
	}
	if !strings.Contains(username, "cmd|") {
		t.Fatalf("username %q lost its content — the escape must not mangle the value", username)
	}
}

func TestAuditExportIsAdminOnly(t *testing.T) {
	st, mux, _ := exportServer(t)

	if _, err := st.CreateUser("regular", "regular-pass", store.RoleUser); err != nil {
		t.Fatal(err)
	}
	tokens := auth.NewTokenService("export-test-secret", time.Hour)
	u, err := st.Authenticate("regular", "regular-pass")
	if err != nil {
		t.Fatal(err)
	}
	userToken, _, err := tokens.Issue(u.ID, u.Username, u.Role)
	if err != nil {
		t.Fatal(err)
	}

	if code := getExport(t, mux, userToken).Code; code != http.StatusForbidden {
		t.Fatalf("regular user got %d, want 403 — the audit log names every operator", code)
	}
	if code := getExport(t, mux, "").Code; code != http.StatusUnauthorized {
		t.Fatalf("anonymous got %d, want 401", code)
	}
}
