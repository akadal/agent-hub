package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/akadal/agent-hub/backend/internal/auth"
	"github.com/akadal/agent-hub/backend/internal/store"
)

func guardServer(t *testing.T) (*Server, *store.Store, http.Handler, string) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureBootstrapAdmin("admin", "guard-test-pass"); err != nil {
		t.Fatal(err)
	}
	tokens := auth.NewTokenService("guard-test-secret", time.Hour)
	srv := &Server{Store: st, Tokens: tokens}
	admin, err := st.Authenticate("admin", "guard-test-pass")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := tokens.Issue(admin.ID, admin.Username, admin.Role)
	if err != nil {
		t.Fatal(err)
	}
	return srv, st, srv.NewMux(), token
}

// call issues a GET as if it arrived from `remote`, optionally forwarded.
func call(mux http.Handler, path, token, remote string, xff string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	r.RemoteAddr = remote
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	return rec
}

func enableTailnetOnly(t *testing.T, st *store.Store) {
	t.Helper()
	on := true
	if _, err := st.UpdateSettings("", &on); err != nil {
		t.Fatal(err)
	}
}

func TestGuardIsOffUntilTheOperatorTurnsItOn(t *testing.T) {
	_, _, mux, token := guardServer(t)

	// Default settings: a request from the public internet is served.
	if code := call(mux, "/api/machines", token, "203.0.113.9:443", "").Code; code != http.StatusOK {
		t.Fatalf("got %d, want 200 — the setting must default to off", code)
	}
}

func TestGuardBlocksNonTailnetCallersWhenOn(t *testing.T) {
	_, st, mux, token := guardServer(t)
	enableTailnetOnly(t, st)

	rec := call(mux, "/api/machines", token, "203.0.113.9:443", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403 for a public caller", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Tailscale") {
		t.Fatalf("refusal does not say why: %s", rec.Body.String())
	}

	// The tailnet still gets in — over IPv4 and over the IPv6 range an iPhone
	// commonly uses.
	if code := call(mux, "/api/machines", token, "100.64.0.7:51234", "").Code; code != http.StatusOK {
		t.Fatalf("got %d, want 200 for a tailnet caller", code)
	}
	if code := call(mux, "/api/machines", token, "[fd7a:115c:a1e0::9]:51234", "").Code; code != http.StatusOK {
		t.Fatalf("got %d, want 200 for an IPv6 tailnet caller", code)
	}
}

func TestGuardCannotBeBypassedWithAForwardedHeader(t *testing.T) {
	_, st, mux, token := guardServer(t)
	enableTailnetOnly(t, st)

	// Straight from the internet, claiming to be on the tailnet.
	if code := call(mux, "/api/machines", token, "203.0.113.9:443", "100.64.0.5").Code; code != http.StatusForbidden {
		t.Fatalf("got %d — a direct caller talked its way in with X-Forwarded-For", code)
	}

	// Through the real proxy chain, with the claim injected at the front. Every
	// real hop appended what it saw, so the lie sits leftmost and is ignored.
	rec := call(mux, "/api/machines", token, "172.18.0.4:38000", "100.64.0.5, 203.0.113.9, 172.18.0.9")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d — a spoofed forwarded entry got past the guard", rec.Code)
	}
}

func TestGuardDeniesWhenItCannotIdentifyTheCaller(t *testing.T) {
	_, st, mux, token := guardServer(t)
	enableTailnetOnly(t, st)

	// A proxy that forwards nothing. There is no honest answer, so the request
	// must be refused rather than assumed friendly.
	rec := call(mux, "/api/machines", token, "172.18.0.4:38000", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403 when the caller cannot be identified", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "TRUSTED_PROXIES") {
		t.Fatalf("refusal does not name the fix: %s", rec.Body.String())
	}
}

func TestHealthStaysReachableSoTheContainerIsNotKilled(t *testing.T) {
	_, st, mux, _ := guardServer(t)
	enableTailnetOnly(t, st)

	// Docker's health check runs inside the container and carries no token.
	if code := call(mux, "/health", "", "127.0.0.1:52000", "").Code; code != http.StatusOK {
		t.Fatalf("got %d — the health check must survive the lock", code)
	}
	// Even from outside: it reveals nothing a caller who reached the port
	// does not already know.
	if code := call(mux, "/health", "", "203.0.113.9:443", "").Code; code != http.StatusOK {
		t.Fatalf("got %d, want /health exempt", code)
	}
}

func TestLoopbackKeepsWorkingSoTheHostIsNotLockedOut(t *testing.T) {
	_, st, mux, token := guardServer(t)
	enableTailnetOnly(t, st)

	if code := call(mux, "/api/machines", token, "127.0.0.1:52000", "").Code; code != http.StatusOK {
		t.Fatalf("got %d — an operator on the box itself must keep access", code)
	}
}

func TestEnvEscapeHatchRestoresAccess(t *testing.T) {
	srv, st, _, token := guardServer(t)
	enableTailnetOnly(t, st)
	srv.AccessEnforcementDisabled = true
	mux := srv.NewMux()

	if code := call(mux, "/api/machines", token, "203.0.113.9:443", "").Code; code != http.StatusOK {
		t.Fatalf("got %d — ACCESS_ENFORCEMENT=off must let a locked-out operator back in", code)
	}
}

func patchSettings(mux http.Handler, token, remote, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("PATCH", "/api/settings", strings.NewReader(body))
	r.RemoteAddr = remote
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	return rec
}

func TestEnablingFromOutsideTheTailnetIsRefused(t *testing.T) {
	_, _, mux, token := guardServer(t)

	rec := patchSettings(mux, token, "203.0.113.9:443", `{"tailnet_only":true}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("got %d, want 409 — enabling from here would lock the operator out", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "lock yourself out") {
		t.Fatalf("refusal does not explain the consequence: %s", rec.Body.String())
	}

	// And nothing was persisted, so the next request still works.
	if code := call(mux, "/api/machines", token, "203.0.113.9:443", "").Code; code != http.StatusOK {
		t.Fatalf("got %d — a refused enable must not have taken effect anyway", code)
	}
}

func TestEnablingFromTheTailnetWorks(t *testing.T) {
	_, _, mux, token := guardServer(t)

	rec := patchSettings(mux, token, "100.64.0.7:51234", `{"tailnet_only":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	if code := call(mux, "/api/machines", token, "203.0.113.9:443", "").Code; code != http.StatusForbidden {
		t.Fatalf("got %d — the setting did not take effect", code)
	}
	// Turning it off again from the tailnet must still be possible.
	if rec := patchSettings(mux, token, "100.64.0.7:51234", `{"tailnet_only":false}`); rec.Code != http.StatusOK {
		t.Fatalf("got %d disabling: %s", rec.Code, rec.Body.String())
	}
	if code := call(mux, "/api/machines", token, "203.0.113.9:443", "").Code; code != http.StatusOK {
		t.Fatalf("got %d — the lock was not released", code)
	}
}

func TestEnablingIsRefusedWhenTheCallerCannotBeIdentified(t *testing.T) {
	_, _, mux, token := guardServer(t)

	rec := patchSettings(mux, token, "172.18.0.4:38000", `{"tailnet_only":true}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("got %d, want 409 — the server cannot tell where the operator is", rec.Code)
	}
}

func TestSettingsReportWhetherTheLockCanWork(t *testing.T) {
	_, _, mux, token := guardServer(t)

	// Behind a proxy that forwards nothing: the operator must be told the
	// setting would not protect them, not shown a tick.
	body := call(mux, "/api/settings", token, "172.18.0.4:38000", "").Body.String()
	if !strings.Contains(body, `"client_ip_known":false`) {
		t.Fatalf("settings did not report the unidentifiable caller: %s", body)
	}

	body = call(mux, "/api/settings", token, "100.64.0.7:51234", "").Body.String()
	if !strings.Contains(body, `"client_on_tailnet":true`) {
		t.Fatalf("settings did not report a tailnet caller: %s", body)
	}
	if !strings.Contains(body, `"client_ip":"100.64.0.7"`) {
		t.Fatalf("settings did not echo the address the API sees: %s", body)
	}
}

func TestSettingsSayWhetherThisCallerWouldGetIn(t *testing.T) {
	_, _, mux, token := guardServer(t)

	// Loopback is exempt, so the page must not warn about a lockout that will
	// not happen — the toggle would succeed from here.
	body := call(mux, "/api/settings", token, "127.0.0.1:52000", "").Body.String()
	if !strings.Contains(body, `"client_allowed":true`) {
		t.Fatalf("loopback should be reported as allowed: %s", body)
	}
	if !strings.Contains(body, `"client_on_tailnet":false`) {
		t.Fatalf("loopback is not a tailnet address: %s", body)
	}

	body = call(mux, "/api/settings", token, "203.0.113.9:443", "").Body.String()
	if !strings.Contains(body, `"client_allowed":false`) {
		t.Fatalf("a public caller must be told they would be locked out: %s", body)
	}
}

func TestEnablingFromLoopbackIsAllowed(t *testing.T) {
	_, _, mux, token := guardServer(t)

	// An operator on the box itself — the recovery position — must be able to
	// turn the lock on without being told they would lock themselves out.
	if rec := patchSettings(mux, token, "127.0.0.1:52000", `{"tailnet_only":true}`); rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	if code := call(mux, "/api/machines", token, "127.0.0.1:52000", "").Code; code != http.StatusOK {
		t.Fatalf("got %d — loopback lost access to its own instance", code)
	}
}
