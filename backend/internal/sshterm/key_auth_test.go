package sshterm

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// A throwaway ed25519 key generated for these tests. Not used anywhere real.
const testEd25519Key = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACDQp77IsajxHtAMYMznUg4xY61tX1z5k3mUBBYFbs7LNwAAAKjwSfA+8Enw
PgAAAAtzc2gtZWQyNTUxOQAAACDQp77IsajxHtAMYMznUg4xY61tX1z5k3mUBBYFbs7LNw
AAAEBSSKuGNKTuU2DnGoKuXsm8+ZaH+UvToo2w3udOTNOH29CnvsixqPEe0AxgzOdSDjFj
rW1fXPmTeZQEFgVuzss3AAAAH2FnZW50LWh1Yi10ZXN0LWtleS1ub3QtYS1zZWNyZXQBAg
MEBQY=
-----END OPENSSH PRIVATE KEY-----`

func TestAuthPlanUsesPublicKeyWhenKeyProvided(t *testing.T) {
	plan, err := buildAuthPlan(Target{User: "ops", PrivateKey: testEd25519Key}, &authTrace{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !plan.usesKey {
		t.Fatal("public key auth should be offered when a key is configured")
	}
	// The key must be offered first: a hardened sshd with
	// PasswordAuthentication=no only ever accepts the key.
	if len(plan.methods) == 0 {
		t.Fatal("no auth methods built")
	}
}

// Surrounding whitespace is what you get from pasting a key into a textarea.
func TestAuthPlanAcceptsPaddedKey(t *testing.T) {
	plan, err := buildAuthPlan(Target{User: "ops", PrivateKey: "\n\n  " + testEd25519Key + "  \n\n"}, &authTrace{})
	if err != nil {
		t.Fatalf("a pasted key with surrounding whitespace must parse: %v", err)
	}
	if !plan.usesKey {
		t.Fatal("key should be in use")
	}
}

func TestAuthPlanWithoutKeyFallsBackToPassword(t *testing.T) {
	plan, err := buildAuthPlan(Target{User: "ops", Password: "pw"}, &authTrace{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.usesKey {
		t.Fatal("no key configured, so no public key method should be offered")
	}
	if len(plan.methods) < 2 {
		t.Fatalf("want password + keyboard-interactive, got %d methods", len(plan.methods))
	}
}

// A malformed key must fail loudly, not silently degrade to a password that a
// hardened remote will refuse anyway — that is how you lose an hour.
func TestAuthPlanRejectsMalformedKey(t *testing.T) {
	_, err := buildAuthPlan(Target{User: "ops", PrivateKey: "not a key"}, &authTrace{})
	if err == nil {
		t.Fatal("expected an error for a malformed private key")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "private key") {
		t.Fatalf("error should name the private key, got %q", err)
	}
}

func TestDialSurfacesBadKeyAsClassifiedFailure(t *testing.T) {
	_, _, _, err := dialRaw(Target{Address: "100.64.0.10", User: "ops", PrivateKey: "bogus"}, time.Time{})
	if err == nil {
		t.Fatal("expected failure")
	}

	var oe *OpenError
	if !errors.As(err, &oe) {
		t.Fatalf("want *OpenError, got %T: %v", err, err)
	}
	if oe.Kind != FailBadKey {
		t.Fatalf("kind = %q, want %q", oe.Kind, FailBadKey)
	}
	if oe.Retryable {
		t.Fatal("a malformed key is not fixed by retrying")
	}
}

// A bad key must be reported before any network I/O — otherwise the operator
// waits out a dial timeout to learn their key did not parse.
func TestBadKeyFailsBeforeDialing(t *testing.T) {
	start := time.Now()
	// 203.0.113.0/24 is TEST-NET-3: guaranteed not to answer.
	_, _, _, err := dialRaw(Target{Address: "203.0.113.1", User: "ops", PrivateKey: "bogus"}, time.Time{})
	if err == nil {
		t.Fatal("expected failure")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("key validation should short-circuit the dial, took %s", elapsed)
	}
}

func TestDiagnoseAuthFailureMentionsKeyWhenOneIsConfigured(t *testing.T) {
	err := errors.New("ssh: handshake failed: ssh: unable to authenticate, attempted methods [none publickey], no supported methods remain")
	f := Diagnose(StageHandshake, err, &authTrace{}, Target{
		Address: "192.168.1.10", User: "ops", PrivateKey: testEd25519Key,
	})

	if f.Kind != FailAuth {
		t.Fatalf("kind = %q", f.Kind)
	}
	if !strings.Contains(strings.ToLower(f.Hint), "authorized_keys") {
		t.Fatalf("hint should point at authorized_keys when a key is in use, got %q", f.Hint)
	}
}

func TestDiagnoseAuthFailureMentionsPasswordWhenNoKey(t *testing.T) {
	err := errors.New("ssh: unable to authenticate, attempted methods [none password]")
	f := Diagnose(StageHandshake, err, &authTrace{}, Target{Address: "192.168.1.10", User: "ops"})

	if !strings.Contains(strings.ToLower(f.Hint), "password") {
		t.Fatalf("hint should mention the password when that is all we have, got %q", f.Hint)
	}
}

// Tailscale SSH only intercepts port 22. A rejection on any other port of the
// same tailnet host is ordinary OpenSSH refusing the key — pointing the
// operator at the tailnet ACL there wastes their time in the wrong console.
func TestDiagnoseAuthFailureOnTailnetNonStandardPortIsPlainAuth(t *testing.T) {
	err := errors.New("ssh: handshake failed: ssh: unable to authenticate, attempted methods [none], no supported methods remain")
	f := Diagnose(StageHandshake, err, &authTrace{}, Target{
		Address: "100.64.0.10", Port: 2222, User: "opsuser", PrivateKey: testEd25519Key,
	})

	if f.Kind != FailAuth {
		t.Fatalf("kind = %q, want %q", f.Kind, FailAuth)
	}
	if strings.Contains(f.Hint, "ACL") {
		t.Fatalf("must not blame the tailnet ACL on a non-22 port: %q", f.Hint)
	}
}

// The same rejection on port 22 IS the tailnet ACL.
func TestDiagnoseAuthFailureOnTailnetPort22IsACL(t *testing.T) {
	err := errors.New("ssh: unable to authenticate, attempted methods [none], no supported methods remain")
	f := Diagnose(StageHandshake, err, &authTrace{}, Target{Address: "100.64.0.10", Port: 22, User: "opsuser"})

	if f.Kind != FailTailscaleDenied {
		t.Fatalf("kind = %q, want %q", f.Kind, FailTailscaleDenied)
	}
}

// A stalled handshake on a non-22 port is a plain timeout, not check mode.
func TestDiagnoseHandshakeTimeoutOnTailnetNonStandardPort(t *testing.T) {
	f := Diagnose(StageHandshake, timeoutErr(), &authTrace{}, Target{Address: "100.64.0.10", Port: 2222})

	if f.Kind != FailTimeout {
		t.Fatalf("kind = %q, want %q", f.Kind, FailTimeout)
	}
}
