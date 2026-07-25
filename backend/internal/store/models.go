package store

import (
	"strings"
	"time"
)

// User is a local account that can authenticate with username/password.
// PasswordHash is persisted in the store file; API layers must not expose it.
type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	Role         string    `json:"role"` // "admin" | "user"
	CreatedAt    time.Time `json:"created_at"`
}

// UserPublic is the API-safe view of a user (no password hash).
type UserPublic struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// Public returns an API-safe user without secrets.
func (u User) Public() UserPublic {
	return UserPublic{
		ID:        u.ID,
		Username:  u.Username,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
	}
}

// Machine is a manually registered remote host reachable over SSH by address.
// SSHPassword is persisted for the SSH bridge; Public() omits it.
// OwnerUserID scopes the machine to the user who registered it.
type Machine struct {
	ID          string `json:"id"`
	OwnerUserID string `json:"owner_user_id"`
	Name        string `json:"name"`
	Address     string `json:"address"`
	Port        int    `json:"port"`
	SSHUser     string `json:"ssh_user"`
	SSHPassword string `json:"ssh_password"`
	// SSHPrivateKey is a PEM key used in preference to the password. Hardened
	// targets set PasswordAuthentication=no, where a key is the only way in.
	SSHPrivateKey string `json:"ssh_private_key,omitempty"`
	// SSHKeyPassphrase decrypts SSHPrivateKey when it is encrypted.
	SSHKeyPassphrase string    `json:"ssh_key_passphrase,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// MachineSpec is the caller-supplied part of a machine registration. It is a
// struct rather than a parameter list because the credential set keeps growing.
type MachineSpec struct {
	Name             string
	Address          string
	Port             int
	SSHUser          string
	SSHPassword      string
	SSHPrivateKey    string
	SSHKeyPassphrase string
}

// MachinePublic is the API-safe view of a machine (no password).
type MachinePublic struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	SSHUser string `json:"ssh_user"`
	// HasPrivateKey tells the UI which credential is in play without ever
	// sending the key itself.
	HasPrivateKey bool      `json:"has_private_key"`
	CreatedAt     time.Time `json:"created_at"`
}

// Public returns an API-safe machine without secrets.
func (m Machine) Public() MachinePublic {
	return MachinePublic{
		ID:            m.ID,
		Name:          m.Name,
		Address:       m.Address,
		Port:          m.Port,
		SSHUser:       m.SSHUser,
		HasPrivateKey: strings.TrimSpace(m.SSHPrivateKey) != "",
		CreatedAt:     m.CreatedAt,
	}
}

// Terminal is an independent named shell session under a machine (1:N).
// RemoteSession is the durable tmux session name on the remote host so
// reconnect (web/mobile) attaches to the same shell.
type Terminal struct {
	ID            string    `json:"id"`
	MachineID     string    `json:"machine_id"`
	OwnerUserID   string    `json:"owner_user_id"`
	Name          string    `json:"name"`
	Status        string    `json:"status"` // open | closed
	RemoteSession string    `json:"remote_session"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// MachineGrant is a many-to-many allow: user may use a machine (and its terminals).
// Owner and admin always have access without a grant row.
type MachineGrant struct {
	UserID    string    `json:"user_id"`
	MachineID string    `json:"machine_id"`
	CreatedAt time.Time `json:"created_at"`
}

// AuditEvent is an append-only security-relevant action record (MVP).
type AuditEvent struct {
	ID         string    `json:"id"`
	At         time.Time `json:"at"`
	UserID     string    `json:"user_id,omitempty"`
	Username   string    `json:"username,omitempty"`
	Action     string    `json:"action"`
	MachineID  string    `json:"machine_id,omitempty"`
	TerminalID string    `json:"terminal_id,omitempty"`
	Detail     string    `json:"detail,omitempty"`
}

// Network mode values for AccessSettings.
const (
	NetworkPrivateMesh = "private_mesh"
	NetworkOpen        = "open"
)

// AccessSettings is the global access-policy singleton (MVP).
// Edge enforcement is operator reverse-proxy / mesh; this records intent.
type AccessSettings struct {
	NetworkMode string `json:"network_mode"` // private_mesh | open
}

// DefaultAccessSettings is the product default (private mesh).
func DefaultAccessSettings() AccessSettings {
	return AccessSettings{NetworkMode: NetworkPrivateMesh}
}
