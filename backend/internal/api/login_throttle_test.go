package api

import (
	"testing"
	"time"
)

func TestThrottleBlocksAfterBudgetAndExpires(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	th := newLoginThrottle(3, time.Minute)
	th.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if _, blocked := th.blocked("admin"); blocked {
			t.Fatalf("blocked after %d failures, budget is 3", i)
		}
		th.fail("admin")
	}
	wait, blocked := th.blocked("admin")
	if !blocked {
		t.Fatal("still accepting attempts after the budget is spent")
	}
	if wait <= 0 || wait > time.Minute {
		t.Fatalf("retry-after = %v, want within the window", wait)
	}

	// Another account is unaffected.
	if _, blocked := th.blocked("someone-else"); blocked {
		t.Fatal("one account's failures blocked a different account")
	}

	// The window is a window, not a permanent lockout.
	now = now.Add(time.Minute + time.Second)
	if _, blocked := th.blocked("admin"); blocked {
		t.Fatal("account still blocked after the window elapsed")
	}
}

func TestSuccessfulLoginClearsThePenalty(t *testing.T) {
	th := newLoginThrottle(2, time.Minute)
	th.fail("admin")
	th.fail("admin")
	if _, blocked := th.blocked("admin"); !blocked {
		t.Fatal("expected block after spending the budget")
	}
	th.succeed("admin")
	if _, blocked := th.blocked("admin"); blocked {
		t.Fatal("a correct password must end the penalty")
	}
}

// Usernames are case- and space-insensitive, so " Admin " cannot be used to
// mint a fresh budget for the same account.
func TestThrottleKeyNormalizesUsername(t *testing.T) {
	if throttleKey("  Admin ") != throttleKey("admin") {
		t.Fatal("throttle key is not normalized")
	}
}
