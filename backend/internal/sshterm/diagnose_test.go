package sshterm

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
)

func timeoutErr() error {
	return &net.OpError{Op: "read", Net: "tcp", Err: &timeoutError{}}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestDiagnoseTailscaleCheckFromBanner(t *testing.T) {
	tr := &authTrace{}
	tr.add("# Tailscale SSH requires an additional check.\r\n" +
		"# To approve this login, visit: https://login.tailscale.com/a/1a2b3c4d5e6f\r\n")

	f := Diagnose(StageHandshake, timeoutErr(), tr, Target{Address: "100.64.0.10", User: "opsuser"})

	if f.Kind != FailTailscaleCheck {
		t.Fatalf("kind = %q, want %q", f.Kind, FailTailscaleCheck)
	}
	if f.ApprovalURL != "https://login.tailscale.com/a/1a2b3c4d5e6f" {
		t.Fatalf("approval url = %q", f.ApprovalURL)
	}
	if !strings.Contains(f.Hint, "accept") {
		t.Fatalf("hint should mention the ACL accept fix, got %q", f.Hint)
	}
}

func TestDiagnoseTailscaleCheckFromKeyboardInteractive(t *testing.T) {
	tr := &authTrace{}
	// Tailscale also delivers the URL as a keyboard-interactive instruction.
	tr.add("To approve this login, visit https://login.tailscale.com/a/deadbeef99")

	f := Diagnose(StageHandshake, timeoutErr(), tr, Target{Address: "100.64.0.10"})

	if f.Kind != FailTailscaleCheck {
		t.Fatalf("kind = %q, want %q", f.Kind, FailTailscaleCheck)
	}
	if f.ApprovalURL != "https://login.tailscale.com/a/deadbeef99" {
		t.Fatalf("approval url = %q", f.ApprovalURL)
	}
}

// A handshake that stalls against a 100.x address with no banner is still
// almost always Tailscale SSH holding the session for approval.
func TestDiagnoseHandshakeTimeoutOnTailnetAddress(t *testing.T) {
	f := Diagnose(StageHandshake, timeoutErr(), &authTrace{}, Target{Address: "100.64.0.10"})

	if f.Kind != FailTailscaleCheck {
		t.Fatalf("kind = %q, want %q", f.Kind, FailTailscaleCheck)
	}
	if f.ApprovalURL != "" {
		t.Fatalf("no URL was seen, want empty, got %q", f.ApprovalURL)
	}
	if !strings.Contains(f.Hint, "tailscale") && !strings.Contains(f.Hint, "Tailscale") {
		t.Fatalf("hint should name Tailscale, got %q", f.Hint)
	}
}

func TestDiagnoseTCPUnreachable(t *testing.T) {
	f := Diagnose(StageDial, errors.New("connect: connection refused"), &authTrace{}, Target{Address: "100.64.0.10"})

	if f.Kind != FailUnreachable {
		t.Fatalf("kind = %q, want %q", f.Kind, FailUnreachable)
	}
}

// withLocalTailnet drives the "is this process on the tailnet" branch.
func withLocalTailnet(t *testing.T, on bool) {
	t.Helper()
	prev := hasLocalTailnetAddress
	hasLocalTailnetAddress = func() bool { return on }
	t.Cleanup(func() { hasLocalTailnetAddress = prev })
}

// A bridge-network container has no 100.x address of its own, so dialing the
// tailnet times out. The hint must point at host networking, not at the ACL.
func TestDiagnoseDialTimeoutOffTailnetIsRouting(t *testing.T) {
	withLocalTailnet(t, false)

	f := Diagnose(StageDial, timeoutErr(), &authTrace{}, Target{Address: "100.64.0.10"})

	if f.Kind != FailTailnetRouting {
		t.Fatalf("kind = %q, want %q", f.Kind, FailTailnetRouting)
	}
	if !strings.Contains(f.Hint, "network_mode") {
		t.Fatalf("hint should mention host networking, got %q", f.Hint)
	}
	if f.Retryable {
		t.Fatal("a networking-topology fault is not fixed by retrying")
	}
}

// Same timeout, but this process *is* on the tailnet: the peer is simply down.
// Telling the operator to rewrite Docker networking here would be wrong.
func TestDiagnoseDialTimeoutOnTailnetIsPeerDown(t *testing.T) {
	withLocalTailnet(t, true)

	f := Diagnose(StageDial, timeoutErr(), &authTrace{}, Target{Address: "100.64.0.10"})

	if f.Kind != FailUnreachable {
		t.Fatalf("kind = %q, want %q", f.Kind, FailUnreachable)
	}
	if strings.Contains(f.Hint, "network_mode") {
		t.Fatalf("must not blame Docker networking when we are on the tailnet: %q", f.Hint)
	}
	if !f.Retryable {
		t.Fatal("a sleeping peer may come back; this should be retryable")
	}
}

func TestDiagnoseAuthFailure(t *testing.T) {
	err := fmt.Errorf("ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password], no supported methods remain")
	f := Diagnose(StageHandshake, err, &authTrace{}, Target{Address: "192.168.1.10", User: "root"})

	if f.Kind != FailAuth {
		t.Fatalf("kind = %q, want %q", f.Kind, FailAuth)
	}
	if !strings.Contains(f.Hint, "password") {
		t.Fatalf("hint should mention credentials, got %q", f.Hint)
	}
}

// Auth rejection against a tailnet address means Tailscale SSH denied the node
// outright (no ACL rule), which no password can fix.
func TestDiagnoseAuthFailureOnTailnetAddress(t *testing.T) {
	err := errors.New("ssh: handshake failed: ssh: unable to authenticate, attempted methods [none], no supported methods remain")
	f := Diagnose(StageHandshake, err, &authTrace{}, Target{Address: "100.64.0.10", User: "opsuser"})

	if f.Kind != FailTailscaleDenied {
		t.Fatalf("kind = %q, want %q", f.Kind, FailTailscaleDenied)
	}
	if !strings.Contains(f.Hint, "ACL") {
		t.Fatalf("hint should mention the ACL, got %q", f.Hint)
	}
}

func TestDiagnosePlainTimeoutOnNonTailnet(t *testing.T) {
	f := Diagnose(StageHandshake, timeoutErr(), &authTrace{}, Target{Address: "10.0.0.5"})

	if f.Kind != FailTimeout {
		t.Fatalf("kind = %q, want %q", f.Kind, FailTimeout)
	}
}

func TestFailureRetryableClassification(t *testing.T) {
	// Waiting on a human to click an approval link must NOT be retried in a loop.
	if Diagnose(StageHandshake, timeoutErr(), &authTrace{}, Target{Address: "100.64.0.10"}).Retryable {
		t.Fatal("tailscale check-pending must be non-retryable")
	}
	if Diagnose(StageHandshake, errors.New("ssh: unable to authenticate, attempted methods [none]"), &authTrace{},
		Target{Address: "10.0.0.5"}).Retryable {
		t.Fatal("auth failure must be non-retryable")
	}
	if !Diagnose(StageDial, errors.New("connect: connection refused"), &authTrace{}, Target{Address: "10.0.0.5"}).Retryable {
		t.Fatal("a refused connection is transient and should be retryable")
	}
}

func TestIsTailnetAddress(t *testing.T) {
	for _, ok := range []string{"100.64.0.1", "100.64.0.10", "100.127.255.254"} {
		if !isTailnetAddress(ok) {
			t.Errorf("%s should be tailnet", ok)
		}
	}
	for _, no := range []string{"100.63.0.1", "100.128.0.1", "10.0.0.1", "192.168.1.1", "example.com", ""} {
		if isTailnetAddress(no) {
			t.Errorf("%s should NOT be tailnet", no)
		}
	}
}

func TestAuthTraceCapturesAndJoins(t *testing.T) {
	tr := &authTrace{}
	tr.add("first")
	tr.add("")
	tr.add("second")
	got := tr.text()
	if !strings.Contains(got, "first") || !strings.Contains(got, "second") {
		t.Fatalf("trace text = %q", got)
	}
}

func TestOpenErrorCarriesFailureAndUnwraps(t *testing.T) {
	base := errors.New("boom")
	oe := &OpenError{Failure: Failure{Kind: FailAuth, Message: "nope"}, Err: base}

	if !errors.Is(oe, base) {
		t.Fatal("OpenError must unwrap to its cause")
	}
	if !strings.Contains(oe.Error(), "nope") {
		t.Fatalf("error text = %q", oe.Error())
	}

	var target *OpenError
	if !errors.As(error(oe), &target) || target.Kind != FailAuth {
		t.Fatal("errors.As should recover the structured failure")
	}
}

// A real OpenSSH/dropbear that refuses keyboard-interactive answers with
// SSH_MSG_USERAUTH_FAILURE (51) where the client expected an info request (60).
// Go reports that verbatim — it contains nothing resembling "authenticate", so
// the classifier used to call it `unknown`, which is Retryable. The bridge then
// reconnects forever against a host that will never accept the credential: the
// exact storm ADR-007 exists to prevent.
//
// String observed by running `cmd/sshdiag` against the Compose ssh-target with
// a wrong password.
func TestWrongPasswordOnOrdinarySSHDIsAnAuthFailureNotUnknown(t *testing.T) {
	err := errors.New("ssh: handshake failed: ssh: unexpected message type 51 (expected 60)")
	f := Diagnose(StageHandshake, err, nil, Target{Address: "10.0.0.5", Port: 22, User: "root"})

	if f.Kind != FailAuth {
		t.Fatalf("kind = %q, want %q", f.Kind, FailAuth)
	}
	if f.Retryable {
		t.Fatal("reconnecting cannot fix a rejected credential")
	}
	if f.Hint == "" {
		t.Fatal("an auth failure must carry its fix")
	}
}

// The same rejection on port 22 of a tailnet address is Tailscale's ACL talking,
// not sshd — the port-aware split from ADR-008 must survive this new match.
func TestUserauthFailureOnTailscalePort22IsBlamedOnTheACL(t *testing.T) {
	err := errors.New("ssh: handshake failed: ssh: unexpected message type 51 (expected 60)")
	f := Diagnose(StageHandshake, err, nil, Target{Address: "100.64.0.10", Port: 22, User: "ops"})

	if f.Kind != FailTailscaleDenied {
		t.Fatalf("kind = %q, want %q", f.Kind, FailTailscaleDenied)
	}
}
