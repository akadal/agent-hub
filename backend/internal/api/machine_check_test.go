package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
)

func postCheck(t *testing.T, mux http.Handler, tok, machineID string) map[string]any {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/machines/"+machineID+"/check", bytes.NewReader(nil))
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	// A machine that cannot be reached is still a successful check.
	if rr.Code != http.StatusOK {
		t.Fatalf("check status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rr.Body.String())
	}
	return out
}

// An unreachable machine must come back as a classified verdict, not an HTTP
// error — the operator needs the cause, not a 502.
func TestMachineCheck_unreachableIsAVerdictNotAnError(t *testing.T) {
	_, mux := testServer(t)
	tok := loginToken(t, mux, "admin", "123456")

	mid := createMachineAt(t, mux, tok, map[string]any{
		"name":         "dead-host",
		"address":      "203.0.113.1", // TEST-NET-3, never answers
		"port":         22,
		"ssh_user":     "root",
		"ssh_password": "x",
	})

	out := postCheck(t, mux, tok, mid)
	if out["ok"] != false {
		t.Fatalf("want ok=false, got %v", out["ok"])
	}
	f, _ := out["failure"].(map[string]any)
	if f == nil || f["kind"] == "" {
		t.Fatalf("want a classified failure, got %v", out["failure"])
	}
	if _, hasElapsed := out["elapsed_ms"]; !hasElapsed {
		t.Fatal("elapsed_ms should be reported so a slow host reads as slow")
	}
}

func TestMachineCheck_badKeyIsClassified(t *testing.T) {
	_, mux := testServer(t)
	tok := loginToken(t, mux, "admin", "123456")

	mid := createMachineAt(t, mux, tok, map[string]any{
		"name":            "bad-key-host",
		"address":         "203.0.113.1",
		"port":            22,
		"ssh_user":        "root",
		"ssh_private_key": "definitely not a PEM key",
	})

	out := postCheck(t, mux, tok, mid)
	f, _ := out["failure"].(map[string]any)
	if f == nil || f["kind"] != "bad_private_key" {
		t.Fatalf("want bad_private_key, got %v", out["failure"])
	}
	if f["retryable"] != false {
		t.Fatal("a malformed key is not fixed by retrying")
	}
}

// Live counterpart: a reachable host must report ok, and fast.
func TestMachineCheck_liveHostReportsOK(t *testing.T) {
	addr := os.Getenv("E2E_KEY_ADDR")
	keyFile := os.Getenv("E2E_KEY_FILE")
	if addr == "" || keyFile == "" {
		t.Skip("E2E_KEY_ADDR / E2E_KEY_FILE not set; skip live check test")
	}
	key, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	port := 22
	if v := os.Getenv("E2E_KEY_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}
	user := os.Getenv("E2E_KEY_USER")
	if user == "" {
		user = "root"
	}

	_, mux := testServer(t)
	tok := loginToken(t, mux, "admin", "123456")
	mid := createMachineAt(t, mux, tok, map[string]any{
		"name":            "live-check",
		"address":         addr,
		"port":            port,
		"ssh_user":        user,
		"ssh_private_key": string(key),
	})

	out := postCheck(t, mux, tok, mid)
	if out["ok"] != true {
		t.Fatalf("live host should check ok, got %v", out)
	}
}
