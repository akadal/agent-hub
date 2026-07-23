package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func postJSONID(t *testing.T, mux http.Handler, tok, method, path string, body any, wantStatus int, idField string) string {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != wantStatus {
		t.Fatalf("%s %s status=%d body=%s", method, path, rr.Code, rr.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	id, _ := out[idField].(string)
	if id == "" {
		t.Fatalf("empty %s in %v", idField, out)
	}
	return id
}

// TestBridgeSSH_closesOnClientWSDisconnect proves that tearing down the browser
// WebSocket causes the shipped handler to release the remote SSH path promptly
// (no hang waiting on the stdout pump). Requires live ssh-target when
// SSH_E2E_ADDR is set; otherwise skipped.
func TestBridgeSSH_closesOnClientWSDisconnect(t *testing.T) {
	addr := os.Getenv("SSH_E2E_ADDR")
	if addr == "" {
		t.Skip("SSH_E2E_ADDR not set; skip live WS+SSH disconnect test")
	}
	port := 22
	if v := os.Getenv("SSH_E2E_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			t.Fatal(err)
		}
		port = p
	}
	user := os.Getenv("SSH_E2E_USER")
	if user == "" {
		user = "root"
	}
	pass := os.Getenv("SSH_E2E_PASSWORD")
	if pass == "" {
		pass = "targetpass"
	}

	srv, mux := testServer(t)
	_ = srv
	tok := loginToken(t, mux, "akadal", "123456")

	// register machine pointing at live target
	body := map[string]any{
		"name":         "ws-leak-probe",
		"address":      addr,
		"port":         port,
		"ssh_user":     user,
		"ssh_password": pass,
	}
	// use helper from handlers_test via HTTP
	mid := createMachineAt(t, mux, tok, body)
	// create terminal session
	tid := createTerminalSession(t, mux, tok, mid, "leak-test")

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/terminals/" + tid + "/ws?token=" + tok
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}

	// Wait for ready
	deadline := time.Now().Add(8 * time.Second)
	_ = conn.SetReadDeadline(deadline)
	gotReady := false
	for time.Now().Before(deadline) {
		var msg map[string]any
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("read ready: %v", err)
		}
		if msg["type"] == "ready" {
			gotReady = true
			break
		}
		if msg["type"] == "error" {
			t.Fatalf("ssh error: %v", msg["message"])
		}
	}
	if !gotReady {
		t.Fatal("no ready message")
	}

	// Closing the client must not hang the server handler indefinitely.
	// Previously bridgeSSH blocked on <-done without closing SSH first.
	done := make(chan error, 1)
	go func() {
		done <- conn.Close()
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Logf("conn.Close: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("client Close hung")
	}

	// Give server a moment to tear down; the test server shuts down on cleanup.
	// If the handler were stuck on <-done, httptest.Server.Close would hang —
	// we assert cleanup is prompt via a short sleep + explicit server close timeout.
	closed := make(chan struct{})
	go func() {
		ts.CloseClientConnections()
		ts.Close()
		close(closed)
	}()
	select {
	case <-closed:
		// ok — handler released
	case <-time.After(5 * time.Second):
		t.Fatal("httptest server did not close within 5s; bridgeSSH likely leaked waiting on SSH stdout")
	}
}

func createMachineAt(t *testing.T, mux http.Handler, tok string, body map[string]any) string {
	t.Helper()
	// reuse createMachine pattern with custom body
	return postJSONID(t, mux, tok, "POST", "/api/machines", body, http.StatusCreated, "id")
}

func createTerminalSession(t *testing.T, mux http.Handler, tok, machineID, name string) string {
	t.Helper()
	return postJSONID(t, mux, tok, "POST", "/api/machines/"+machineID+"/terminals",
		map[string]any{"name": name}, http.StatusCreated, "id")
}
