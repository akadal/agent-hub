package sshterm

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// startSSHServer runs a throwaway sshd that accepts any password and returns
// its address plus the fingerprint of the host key it serves. Two servers with
// different keys are what makes a "the host changed" test possible at all.
func startSSHServer(t *testing.T, signer ssh.Signer) (addr, fingerprint string) {
	t.Helper()
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
			return &ssh.Permissions{}, nil
		},
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				sc, chans, reqs, err := ssh.NewServerConn(conn, cfg)
				if err != nil {
					return
				}
				defer sc.Close()
				go ssh.DiscardRequests(reqs)
				for ch := range chans {
					_ = ch.Reject(ssh.Prohibited, "no channels in this test server")
				}
			}()
		}
	}()
	return ln.Addr().String(), ssh.FingerprintSHA256(signer.PublicKey())
}

func testSigner(t *testing.T) ssh.Signer {
	t.Helper()
	// 1024-bit: this is a throwaway key generated per test run, and RSA keygen
	// dominates the runtime otherwise.
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	var port int
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}
	return host, port
}

// First connect has nothing to compare against, so it must succeed and report
// the key it saw — that is the whole trust-on-first-use contract.
func TestFirstConnectLearnsTheHostKey(t *testing.T) {
	signer := testSigner(t)
	addr, want := startSSHServer(t, signer)
	host, port := splitHostPort(t, addr)

	client, raw, got, err := dialRaw(Target{Address: host, Port: port, User: "ops", Password: "x"}, time.Time{})
	if err != nil {
		t.Fatalf("first connect must succeed with no pin: %v", err)
	}
	defer client.Close()
	_ = raw

	if got != want {
		t.Fatalf("reported fingerprint %q, want %q", got, want)
	}
	if !strings.HasPrefix(got, "SHA256:") {
		t.Fatalf("fingerprint %q is not in the SHA256 form operators can compare with ssh-keyscan", got)
	}
}

// The pin must actually be checked, not merely stored.
func TestPinnedHostKeyStillConnects(t *testing.T) {
	signer := testSigner(t)
	addr, fp := startSSHServer(t, signer)
	host, port := splitHostPort(t, addr)

	client, _, _, err := dialRaw(Target{
		Address: host, Port: port, User: "ops", Password: "x", HostKey: fp,
	}, time.Time{})
	if err != nil {
		t.Fatalf("matching pin must connect: %v", err)
	}
	client.Close()
}

// The point of the whole exercise: a host presenting a different key is refused
// and the operator is told why, instead of being silently connected to it.
func TestChangedHostKeyIsRefusedAndClassified(t *testing.T) {
	original := testSigner(t)
	_, originalFP := startSSHServer(t, original)

	// A second server, different key, same role — a rebuilt host, or an impostor.
	impostor := testSigner(t)
	addr, impostorFP := startSSHServer(t, impostor)
	host, port := splitHostPort(t, addr)

	_, _, _, err := dialRaw(Target{
		Address: host, Port: port, User: "ops", Password: "x", HostKey: originalFP,
	}, time.Time{})
	if err == nil {
		t.Fatal("a changed host key must not connect")
	}

	var oe *OpenError
	if !errors.As(err, &oe) {
		t.Fatalf("want *OpenError, got %T: %v", err, err)
	}
	if oe.Kind != FailHostKeyChanged {
		t.Fatalf("kind = %q, want %q — a mismatch must not be reported as an auth or timeout problem",
			oe.Kind, FailHostKeyChanged)
	}
	if oe.Retryable {
		t.Fatal("retrying cannot resolve a changed host key")
	}
	if !strings.Contains(oe.Message, originalFP) || !strings.Contains(oe.Message, impostorFP) {
		t.Fatalf("message should name both fingerprints so the operator can compare, got %q", oe.Message)
	}
}

// A blank pin must not be treated as "the empty fingerprint", which would
// refuse every first connect.
func TestEmptyPinDoesNotBlockConnect(t *testing.T) {
	signer := testSigner(t)
	addr, _ := startSSHServer(t, signer)
	host, port := splitHostPort(t, addr)

	client, _, _, err := dialRaw(Target{
		Address: host, Port: port, User: "ops", Password: "x", HostKey: "   ",
	}, time.Time{})
	if err != nil {
		t.Fatalf("whitespace-only pin must be treated as unpinned: %v", err)
	}
	client.Close()
}
