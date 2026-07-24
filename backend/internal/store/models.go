package store

import "time"

// User is a local account that can authenticate with username/password.
// PasswordHash is persisted in the store file; API layers must not expose it.
type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

// Machine is a manually registered remote host reachable over SSH by address.
// SSHPassword is persisted for the SSH bridge; Public() omits it.
// OwnerUserID scopes the machine to the user who registered it.
type Machine struct {
	ID          string    `json:"id"`
	OwnerUserID string    `json:"owner_user_id"`
	Name        string    `json:"name"`
	Address     string    `json:"address"`
	Port        int       `json:"port"`
	SSHUser     string    `json:"ssh_user"`
	SSHPassword string    `json:"ssh_password"`
	CreatedAt   time.Time `json:"created_at"`
}

// MachinePublic is the API-safe view of a machine (no password).
type MachinePublic struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	Port      int       `json:"port"`
	SSHUser   string    `json:"ssh_user"`
	CreatedAt time.Time `json:"created_at"`
}

// Public returns an API-safe machine without secrets.
func (m Machine) Public() MachinePublic {
	return MachinePublic{
		ID:        m.ID,
		Name:      m.Name,
		Address:   m.Address,
		Port:      m.Port,
		SSHUser:   m.SSHUser,
		CreatedAt: m.CreatedAt,
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
