package sshterm_test

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/akadal/agent-hub/backend/internal/sshterm"
)

// Integration test against a real SSH target (Compose dummy).
// Enabled when SSH_E2E_ADDR is set (e.g. 127.0.0.1 with port 2222).
func TestRunCommand_liveSSH(t *testing.T) {
	addr := os.Getenv("SSH_E2E_ADDR")
	if addr == "" {
		t.Skip("SSH_E2E_ADDR not set; skip live SSH integration")
	}
	port := 22
	if v := os.Getenv("SSH_E2E_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			t.Fatal(err)
		}
		port = p
	}
	user := envOr("SSH_E2E_USER", "root")
	pass := envOr("SSH_E2E_PASSWORD", "targetpass")

	res, err := sshterm.RunCommand(sshterm.Target{
		Address:  addr,
		Port:     port,
		User:     user,
		Password: pass,
	}, "echo agent-hub-e2e && whoami")
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q stdout=%q", res.ExitCode, res.Stderr, res.Stdout)
	}
	if !strings.Contains(res.Stdout, "agent-hub-e2e") {
		t.Fatalf("stdout missing marker: %q", res.Stdout)
	}
	if !strings.Contains(res.Stdout, user) && !strings.Contains(res.Stdout, "root") {
		t.Fatalf("stdout missing whoami: %q", res.Stdout)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
