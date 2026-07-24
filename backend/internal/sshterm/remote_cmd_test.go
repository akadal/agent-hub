package sshterm

import (
	"strings"
	"testing"
)

func TestRemoteShellCmd_plainShell(t *testing.T) {
	cmd := remoteShellCmd("")
	if !strings.Contains(cmd, "/opt/homebrew/bin") {
		t.Fatalf("expected homebrew path: %s", cmd)
	}
	if !strings.Contains(cmd, "exec ${SHELL") {
		t.Fatalf("expected login shell: %s", cmd)
	}
	if strings.Contains(cmd, "tmux") {
		t.Fatalf("plain shell should not invoke tmux: %s", cmd)
	}
}

func TestRemoteShellCmd_tmuxSession(t *testing.T) {
	cmd := remoteShellCmd("ah_abc123")
	if !strings.Contains(cmd, "new-session -A -s ah_abc123") {
		t.Fatalf("missing tmux session: %s", cmd)
	}
	if !strings.Contains(cmd, "command -v tmux") {
		t.Fatalf("should resolve tmux quietly: %s", cmd)
	}
	// Reject injection / unsafe names
	if cmd2 := remoteShellCmd("bad;rm"); strings.Contains(cmd2, "new-session") {
		t.Fatalf("unsafe name must not use tmux: %s", cmd2)
	}
}
