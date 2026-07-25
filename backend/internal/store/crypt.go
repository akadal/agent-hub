package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Credentials at rest.
//
// store.json holds SSH passwords, private keys and key passphrases. Anything
// that reads that one file — a stray backup, a copied volume, a debug paste —
// used to get the whole fleet. Those three fields are now sealed with
// AES-256-GCM before they are written, and opened again on load, so the JSON
// on disk is useless without the key.
//
// This is encryption at rest, not a secret manager: the process must be able
// to open the credentials unattended, so the key is reachable by the process.
// It raises the bar from "read the file" to "read the file *and* get the key".

// encPrefix marks a sealed value. Anything without it is legacy plaintext and
// is read as-is, then sealed on the next save.
const encPrefix = "enc:v1:"

// credentialKeyFile is created with 32 random bytes on first use when no
// CREDENTIAL_KEY is configured, so an existing deployment gains encryption
// without the operator having to set anything.
const credentialKeyFile = "credential.key"

// CredentialKeyEnv lets operators supply the key themselves — the way to keep
// it off the same disk as the store (Docker secret, injected env, KMS-fed).
const CredentialKeyEnv = "CREDENTIAL_KEY"

var errNotSealed = errors.New("value is not sealed")

// loadCredentialKey resolves the AES key for dataDir.
//
// CREDENTIAL_KEY wins when set; its text is hashed to 32 bytes so any
// passphrase length works. Otherwise a random key file next to the store is
// created once and reused.
func loadCredentialKey(dataDir string) ([]byte, error) {
	if v := strings.TrimSpace(os.Getenv(CredentialKeyEnv)); v != "" {
		sum := sha256.Sum256([]byte(v))
		return sum[:], nil
	}

	path := filepath.Join(dataDir, credentialKeyFile)
	b, err := os.ReadFile(path)
	if err == nil && len(b) == 32 {
		return b, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read credential key: %w", err)
	}
	if err == nil {
		// A file exists but is the wrong size. Refuse rather than overwrite it:
		// generating a fresh key here would silently orphan every stored
		// credential, and the operator would find out one failed SSH at a time.
		return nil, fmt.Errorf("credential key %s is %d bytes, want 32 — restore it or set %s", path, len(b), CredentialKeyEnv)
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate credential key: %w", err)
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, fmt.Errorf("write credential key: %w", err)
	}
	return key, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// seal encrypts a credential for storage. Empty stays empty: a machine with no
// password should keep an empty field, not a ciphertext that decrypts to "".
func sealValue(key []byte, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if len(key) == 0 {
		return plaintext, nil
	}
	if strings.HasPrefix(plaintext, encPrefix) {
		return plaintext, nil // already sealed; never double-wrap
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(out), nil
}

// open decrypts a stored credential. A value without the marker is legacy
// plaintext and is returned unchanged — that is what makes an existing store
// keep working across the upgrade.
func openValue(key []byte, stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, encPrefix) {
		return stored, errNotSealed
	}
	if len(key) == 0 {
		return "", errors.New("credential is sealed but no key is available")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, encPrefix))
	if err != nil {
		return "", fmt.Errorf("decode sealed credential: %w", err)
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("sealed credential is truncated")
	}
	nonce, body := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		// Wrong key, or a tampered file. Say which knob fixes it.
		return "", fmt.Errorf("cannot open sealed credential — wrong %s or a replaced %s: %w", CredentialKeyEnv, credentialKeyFile, err)
	}
	return string(plain), nil
}

// sealMachine returns a copy with its credentials sealed, ready to marshal.
func sealMachine(key []byte, m Machine) (Machine, error) {
	var err error
	if m.SSHPassword, err = sealValue(key, m.SSHPassword); err != nil {
		return m, err
	}
	if m.SSHPrivateKey, err = sealValue(key, m.SSHPrivateKey); err != nil {
		return m, err
	}
	if m.SSHKeyPassphrase, err = sealValue(key, m.SSHKeyPassphrase); err != nil {
		return m, err
	}
	return m, nil
}

// openMachine decrypts a loaded machine in place and reports whether any field
// was still legacy plaintext, so the caller can re-save and finish the upgrade.
func openMachine(key []byte, m Machine) (Machine, bool, error) {
	plaintextFound := false
	for _, f := range []struct {
		get func() string
		set func(string)
	}{
		{func() string { return m.SSHPassword }, func(v string) { m.SSHPassword = v }},
		{func() string { return m.SSHPrivateKey }, func(v string) { m.SSHPrivateKey = v }},
		{func() string { return m.SSHKeyPassphrase }, func(v string) { m.SSHKeyPassphrase = v }},
	} {
		v, err := openValue(key, f.get())
		if errors.Is(err, errNotSealed) {
			plaintextFound = true
			continue
		}
		if err != nil {
			return m, false, err
		}
		f.set(v)
	}
	return m, plaintextFound, nil
}
