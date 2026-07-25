package sshterm

import (
	"errors"
	"time"
)

// checkTimeout bounds a preflight. Shorter than openTimeout: a check is a
// question, not a session, and the operator is waiting on the answer.
const checkTimeout = 15 * time.Second

// CheckResult is the outcome of a preflight connection test.
type CheckResult struct {
	OK bool `json:"ok"`
	// Failure is nil when OK. It never contains the credentials that were used.
	Failure *Failure `json:"failure,omitempty"`
	// Elapsed is how long the attempt took, so a slow-but-working host is
	// visible as slow rather than mistaken for broken.
	ElapsedMS int64 `json:"elapsed_ms"`
}

// Check dials and authenticates, then hangs up. It answers "can the hub reach
// this machine, and if not why" without opening a PTY or touching tmux — so an
// operator can diagnose from the Machines page instead of opening a terminal
// and reading a black screen.
func Check(t Target) CheckResult {
	start := time.Now()
	client, raw, err := dialRaw(t, time.Now().Add(checkTimeout))
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		var oe *OpenError
		if errors.As(err, &oe) {
			f := oe.Failure
			return CheckResult{Failure: &f, ElapsedMS: elapsed}
		}
		return CheckResult{
			Failure:   &Failure{Kind: FailUnknown, Message: err.Error(), Retryable: true},
			ElapsedMS: elapsed,
		}
	}

	_ = raw.SetDeadline(time.Time{})
	_ = client.Close()
	return CheckResult{OK: true, ElapsedMS: elapsed}
}
