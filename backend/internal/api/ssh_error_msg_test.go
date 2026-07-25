package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/akadal/agent-hub/backend/internal/sshterm"
)

func TestSSHOpenErrorMsgCarriesClassifiedFailure(t *testing.T) {
	oe := &sshterm.OpenError{
		Failure: sshterm.Failure{
			Kind:        sshterm.FailTailscaleCheck,
			Message:     "Tailscale SSH is waiting for approval",
			ApprovalURL: "https://login.tailscale.com/a/abc123",
			Hint:        "set the ACL action to accept",
			Retryable:   false,
		},
		Err: errors.New("i/o timeout"),
	}
	// The bridge wraps the error on its way up; classification must survive that.
	msg := sshOpenErrorMsg(fmt.Errorf("ssh dial: %w", oe))

	if msg.Type != "error" {
		t.Fatalf("type = %q", msg.Type)
	}
	if msg.Kind != string(sshterm.FailTailscaleCheck) {
		t.Fatalf("kind = %q", msg.Kind)
	}
	if msg.ApprovalURL != "https://login.tailscale.com/a/abc123" {
		t.Fatalf("approval url = %q", msg.ApprovalURL)
	}
	if msg.Hint == "" {
		t.Fatal("hint must be forwarded to the client")
	}
	if msg.Retryable == nil || *msg.Retryable {
		t.Fatal("check-pending must be reported as non-retryable")
	}
}

func TestSSHOpenErrorMsgFallsBackForUnclassifiedErrors(t *testing.T) {
	msg := sshOpenErrorMsg(errors.New("something odd"))

	if msg.Type != "error" || !strings.Contains(msg.Message, "something odd") {
		t.Fatalf("unexpected msg %+v", msg)
	}
	if msg.Kind != "" {
		t.Fatalf("kind should be empty for unclassified errors, got %q", msg.Kind)
	}
	// Absent classification must not be encoded as "do not retry".
	if msg.Retryable != nil {
		t.Fatal("retryable should be omitted when unknown")
	}
}

func TestSSHOpenErrorMsgJSONOmitsEmptyFields(t *testing.T) {
	b, err := json.Marshal(sshOpenErrorMsg(errors.New("boom")))
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"kind", "hint", "approval_url", "retryable"} {
		if strings.Contains(string(b), `"`+k+`"`) {
			t.Fatalf("%s should be omitted, json = %s", k, b)
		}
	}
}

func TestSSHOpenErrorMsgRetryableTrueIsEncoded(t *testing.T) {
	oe := &sshterm.OpenError{
		Failure: sshterm.Failure{Kind: sshterm.FailUnreachable, Message: "no route", Retryable: true},
		Err:     errors.New("connection refused"),
	}
	b, err := json.Marshal(sshOpenErrorMsg(oe))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"retryable":true`) {
		t.Fatalf("retryable:true must be explicit, json = %s", b)
	}
}
