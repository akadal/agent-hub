package sshterm

import (
	"testing"
	"time"
)

func TestCheckReportsUnreachableWithoutHangingOnPTY(t *testing.T) {
	// TEST-NET-3: nothing answers, and nothing should be left half-open.
	res := Check(Target{Address: "203.0.113.1", Port: 22, User: "ops", Password: "x"})

	if res.OK {
		t.Fatal("unreachable host must not report OK")
	}
	if res.Failure == nil {
		t.Fatal("a failed check must carry a classified failure")
	}
	if res.Failure.Kind == "" {
		t.Fatalf("failure kind must be set, got %+v", res.Failure)
	}
}

func TestCheckReportsBadKeyImmediately(t *testing.T) {
	res := Check(Target{Address: "203.0.113.1", Port: 22, User: "ops", PrivateKey: "bogus"})

	if res.OK {
		t.Fatal("a malformed key must not report OK")
	}
	if res.Failure.Kind != FailBadKey {
		t.Fatalf("kind = %q, want %q", res.Failure.Kind, FailBadKey)
	}
}

// The check must never expose the credential it used.
func TestCheckResultCarriesNoSecrets(t *testing.T) {
	res := Check(Target{Address: "203.0.113.1", Port: 22, User: "ops", Password: "hunter2", PrivateKey: "bogus"})

	blob := res.Failure.Message + res.Failure.Hint + res.Failure.ApprovalURL
	if blob == "" {
		t.Fatal("expected some failure text")
	}
	for _, secret := range []string{"hunter2", "bogus"} {
		if contains(blob, secret) {
			t.Fatalf("check result leaked %q: %s", secret, blob)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		}()
}

// The preflight must honour its own budget end to end. A black-holed address
// makes the TCP connect hang, and if the dial used its own longer timeout the
// deadline the caller picked would be a fiction.
func TestCheckHonoursItsBudgetOnADeadDial(t *testing.T) {
	start := time.Now()
	res := Check(Target{Address: "203.0.113.1", Port: 22, User: "ops", Password: "x"})
	elapsed := time.Since(start)

	if res.OK {
		t.Fatal("black-holed address must not report OK")
	}
	if elapsed > checkTimeout+2*time.Second {
		t.Fatalf("check ran %s, budget is %s — the dial is escaping the deadline", elapsed, checkTimeout)
	}
}
