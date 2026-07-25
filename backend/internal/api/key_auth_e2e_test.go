package api_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestBridgeSSH_keyAuthOpensRealShell drives the whole shipped path — register a
// machine with a PEM key, create a terminal, open the WebSocket, and read actual
// shell output — against a live host whose sshd refuses passwords.
//
// It is the end-to-end counterpart to the unit tests: those prove the key is
// parsed and offered, this proves a terminal really opens through the bridge.
//
// Set to run it (skipped otherwise):
//
//	E2E_KEY_ADDR=100.64.0.10 E2E_KEY_PORT=2222 E2E_KEY_USER=opsuser \
//	E2E_KEY_FILE=/path/to/id_ed25519 go test ./internal/api/ -run KeyAuth -v
func TestBridgeSSH_keyAuthOpensRealShell(t *testing.T) {
	addr := os.Getenv("E2E_KEY_ADDR")
	keyFile := os.Getenv("E2E_KEY_FILE")
	if addr == "" || keyFile == "" {
		t.Skip("E2E_KEY_ADDR / E2E_KEY_FILE not set; skip live key-auth terminal test")
	}
	key, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatalf("read key: %v", err)
	}
	port := 22
	if v := os.Getenv("E2E_KEY_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			t.Fatal(err)
		}
		port = p
	}
	user := os.Getenv("E2E_KEY_USER")
	if user == "" {
		user = "root"
	}

	_, mux := testServer(t)
	tok := loginToken(t, mux, "admin", "123456")

	// Deliberately no password: the target refuses password auth, so a shell
	// here can only mean the key was used.
	mid := createMachineAt(t, mux, tok, map[string]any{
		"name":            "key-auth-probe",
		"address":         addr,
		"port":            port,
		"ssh_user":        user,
		"ssh_password":    "",
		"ssh_private_key": string(key),
	})

	// The key must never come back out of the API.
	assertMachineHidesKey(t, mux, tok, mid)

	tid := createTerminalSession(t, mux, tok, mid, "keyprobe")

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/terminals/" + tid + "/ws?token=" + tok
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(40 * time.Second)
	_ = conn.SetReadDeadline(deadline)

	for ready := false; !ready; {
		var msg map[string]any
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("read before ready: %v", err)
		}
		switch msg["type"] {
		case "ready":
			ready = true
		case "error":
			// The classified frame is the whole point — show it on failure.
			t.Fatalf("ssh open failed: kind=%v message=%v hint=%v",
				msg["kind"], msg["message"], msg["hint"])
		}
	}

	// A PTY that never paints is the black-terminal bug; require real bytes.
	marker := "agent-hub-key-e2e-ok"
	if err := conn.WriteJSON(map[string]any{
		"type": "stdin",
		"data": "echo " + marker + "\n",
	}); err != nil {
		t.Fatalf("write stdin: %v", err)
	}

	var out strings.Builder
	for time.Now().Before(deadline) {
		var msg map[string]any
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("read stdout: %v (so far: %q)", err, out.String())
		}
		if msg["type"] == "stdout" {
			s, _ := msg["data"].(string)
			out.WriteString(s)
			// Skip the echoed command line; require the command's own output.
			if strings.Count(out.String(), marker) >= 2 {
				return
			}
		}
		if msg["type"] == "error" {
			t.Fatalf("error mid-session: %v", msg["message"])
		}
	}
	t.Fatalf("never saw %q in shell output; got %q", marker, out.String())
}

func assertMachineHidesKey(t *testing.T, mux http.Handler, tok, machineID string) {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/machines/"+machineID, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get machine status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, "BEGIN") || strings.Contains(body, "PRIVATE KEY") {
		t.Fatalf("private key leaked through the API: %s", body)
	}
	if !strings.Contains(body, `"has_private_key":true`) {
		t.Fatalf("machine should report has_private_key, got %s", body)
	}
}
