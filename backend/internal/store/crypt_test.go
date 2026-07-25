package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testKeyPassphrase = "crypt-test-key"

func readStoreFile(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "store.json"))
	if err != nil {
		t.Fatalf("read store.json: %v", err)
	}
	return string(b)
}

func TestStoredCredentialsNeverHitDiskInPlaintext(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(CredentialKeyEnv, testKeyPassphrase)

	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.CreateMachine("owner-1", MachineSpec{
		Name:             "web",
		Address:          "10.0.0.5",
		Port:             22,
		SSHUser:          "ops",
		SSHPassword:      "hunter2-on-disk",
		SSHPrivateKey:    "-----BEGIN OPENSSH PRIVATE KEY-----\nsecret-key-body\n",
		SSHKeyPassphrase: "key-phrase-on-disk",
	})
	if err != nil {
		t.Fatal(err)
	}

	raw := readStoreFile(t, dir)
	for _, secret := range []string{"hunter2-on-disk", "secret-key-body", "key-phrase-on-disk"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("store.json contains %q in the clear", secret)
		}
	}
	if !strings.Contains(raw, encPrefix) {
		t.Fatalf("store.json has no sealed values at all:\n%s", raw)
	}

	// The SSH bridge needs the real values back.
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.GetMachine(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SSHPassword != "hunter2-on-disk" {
		t.Fatalf("password = %q after reload", got.SSHPassword)
	}
	if !strings.Contains(got.SSHPrivateKey, "secret-key-body") {
		t.Fatalf("private key did not survive the round trip: %q", got.SSHPrivateKey)
	}
	if got.SSHKeyPassphrase != "key-phrase-on-disk" {
		t.Fatalf("key passphrase = %q after reload", got.SSHKeyPassphrase)
	}
}

func TestAnExistingPlaintextStoreIsUpgradedOnOpen(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(CredentialKeyEnv, testKeyPassphrase)

	// Hand-write a store the way pre-encryption builds did.
	legacy := snapshot{
		Machines: []Machine{{
			ID:          "m-legacy",
			OwnerUserID: "owner-1",
			Name:        "old",
			Address:     "10.0.0.9",
			Port:        22,
			SSHUser:     "ops",
			SSHPassword: "legacy-plaintext",
		}},
	}
	b, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "store.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}

	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.GetMachine("m-legacy")
	if err != nil {
		t.Fatal(err)
	}
	if m.SSHPassword != "legacy-plaintext" {
		t.Fatalf("password = %q — the upgrade must not lose existing credentials", m.SSHPassword)
	}
	if raw := readStoreFile(t, dir); strings.Contains(raw, "legacy-plaintext") {
		t.Fatalf("store.json still holds the legacy password in the clear:\n%s", raw)
	}
}

func TestOpeningWithTheWrongKeyFailsLoudly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(CredentialKeyEnv, testKeyPassphrase)

	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateMachine("owner-1", MachineSpec{
		Name: "web", Address: "10.0.0.5", Port: 22, SSHUser: "ops", SSHPassword: "hunter2",
	}); err != nil {
		t.Fatal(err)
	}

	// Rotating CREDENTIAL_KEY without re-entering credentials must be an error,
	// not a machine that silently connects with an empty password.
	t.Setenv(CredentialKeyEnv, "a-different-key")
	if _, err := Open(dir); err == nil {
		t.Fatal("Open succeeded with the wrong credential key")
	}
}

func TestAKeyFileIsCreatedWhenNoEnvKeyIsSet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(CredentialKeyEnv, "")

	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, credentialKeyFile))
	if err != nil {
		t.Fatalf("no key file was generated: %v", err)
	}
	if info.Size() != 32 {
		t.Fatalf("key file is %d bytes, want 32", info.Size())
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("key file mode is %o, want 600", perm)
	}
}

func TestEmptyCredentialsStayEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(CredentialKeyEnv, testKeyPassphrase)

	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// A Tailscale SSH target carries no credential at all.
	m, err := st.CreateMachine("owner-1", MachineSpec{
		Name: "tailnet-host", Address: "100.64.0.1", Port: 22, SSHUser: "ops",
	})
	if err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.GetMachine(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SSHPassword != "" || got.SSHPrivateKey != "" || got.SSHKeyPassphrase != "" {
		t.Fatalf("empty credentials came back as %q/%q/%q", got.SSHPassword, got.SSHPrivateKey, got.SSHKeyPassphrase)
	}
}
