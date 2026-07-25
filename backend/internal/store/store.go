package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrInvalidCreds = errors.New("invalid credentials")
	ErrUserExists   = errors.New("user already exists")
	ErrLastAdmin    = errors.New("cannot delete or demote the last admin")
	ErrInvalidRole  = errors.New("invalid role")
)

// Valid local-account roles.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

func normalizeRole(role string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case RoleAdmin:
		return RoleAdmin, nil
	case RoleUser, "regular", "":
		// empty defaults to regular user when creating
		if role == "" {
			return RoleUser, nil
		}
		return RoleUser, nil
	default:
		return "", ErrInvalidRole
	}
}

// Cap audit growth in the JSON file store (MVP retention).
const maxAuditEvents = 1000

// Store is a file-backed persistence layer for users, machines, and terminals.
type Store struct {
	mu        sync.RWMutex
	path      string
	users     map[string]User     // by id
	byName    map[string]string   // username -> id
	machines  map[string]Machine  // by id
	terminals map[string]Terminal // by id
	// grants key: userID + "\x00" + machineID
	grants   map[string]MachineGrant
	audit    []AuditEvent
	settings AccessSettings
}

type snapshot struct {
	Users     []User          `json:"users"`
	Machines  []Machine       `json:"machines"`
	Terminals []Terminal      `json:"terminals"`
	Grants    []MachineGrant  `json:"grants,omitempty"`
	Audit     []AuditEvent    `json:"audit,omitempty"`
	Settings  *AccessSettings `json:"settings,omitempty"`
}

func grantKey(userID, machineID string) string {
	return userID + "\x00" + machineID
}

// Open creates or loads a store at dataDir/store.json.
//
// The directory is owner-only: store.json holds SSH passwords and private keys
// in plaintext (D-009), so a world-readable data dir would hand every local
// account the fleet's credentials.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "store.json")
	s := &Store{
		path:      path,
		users:     map[string]User{},
		byName:    map[string]string{},
		machines:  map[string]Machine{},
		terminals: map[string]Terminal{},
		grants:    map[string]MachineGrant{},
		audit:     nil,
		settings:  DefaultAccessSettings(),
	}
	if _, err := os.Stat(path); err == nil {
		if err := s.load(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// EnsureBootstrapAdmin creates the bootstrap admin if missing, or re-syncs the
// password from env when the user already exists. Coolify/env credential
// changes then take effect on every restart (self-hosted recovery path).
func (s *Store) EnsureBootstrapAdmin(username, password string) error {
	if username == "" || password == "" {
		return fmt.Errorf("bootstrap admin username and password required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.byName[username]; ok {
		u := s.users[id]
		u.PasswordHash = string(hash)
		u.Role = "admin"
		s.users[id] = u
		return s.saveLocked()
	}
	u := User{
		ID:           uuid.NewString(),
		Username:     username,
		PasswordHash: string(hash),
		Role:         "admin",
		CreatedAt:    time.Now().UTC(),
	}
	s.users[u.ID] = u
	s.byName[u.Username] = u.ID
	return s.saveLocked()
}

// dummyHash is a real bcrypt hash of a value nobody can supply. Authenticate
// compares against it when the username is unknown so both branches pay the
// same ~60ms; otherwise the response time alone tells an attacker which
// usernames exist.
var dummyHash = []byte("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy")

// Authenticate checks username/password and returns the user on success.
func (s *Store) Authenticate(username, password string) (User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.byName[username]
	if !ok {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return User{}, ErrInvalidCreds
	}
	u := s.users[id]
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return User{}, ErrInvalidCreds
	}
	return u, nil
}

// GetUser returns a user by id.
func (s *Store) GetUser(id string) (User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

// ListUsers returns all local users sorted by username.
func (s *Store) ListUsers() []User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Username == out[j].Username {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].Username < out[j].Username
	})
	return out
}

// CreateUser registers a new local user. Password is hashed; role is admin|user.
func (s *Store) CreateUser(username, password, role string) (User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return User{}, fmt.Errorf("username is required")
	}
	if password == "" {
		return User{}, fmt.Errorf("password is required")
	}
	role, err := normalizeRole(role)
	if err != nil {
		return User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.byName[username]; exists {
		return User{}, ErrUserExists
	}
	u := User{
		ID:           uuid.NewString(),
		Username:     username,
		PasswordHash: string(hash),
		Role:         role,
		CreatedAt:    time.Now().UTC(),
	}
	s.users[u.ID] = u
	s.byName[u.Username] = u.ID
	if err := s.saveLocked(); err != nil {
		return User{}, err
	}
	return u, nil
}

// UpdateUser patches password and/or role. Empty password means keep current.
// Empty role means keep current. Demoting the last admin is refused.
func (s *Store) UpdateUser(id, password, role string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return User{}, ErrNotFound
	}
	if password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return User{}, err
		}
		u.PasswordHash = string(hash)
	}
	if role != "" {
		r, err := normalizeRole(role)
		if err != nil {
			return User{}, err
		}
		// Refuse demoting the last admin.
		if u.Role == RoleAdmin && r != RoleAdmin && s.adminCountLocked() <= 1 {
			return User{}, ErrLastAdmin
		}
		u.Role = r
	}
	s.users[id] = u
	if err := s.saveLocked(); err != nil {
		return User{}, err
	}
	return u, nil
}

// DeleteUser removes a user by id. Deleting the last admin is refused.
// Also drops any machine grants for that user.
func (s *Store) DeleteUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return ErrNotFound
	}
	if u.Role == RoleAdmin && s.adminCountLocked() <= 1 {
		return ErrLastAdmin
	}
	delete(s.users, id)
	delete(s.byName, u.Username)
	for k, g := range s.grants {
		if g.UserID == id {
			delete(s.grants, k)
		}
	}
	return s.saveLocked()
}

func (s *Store) adminCountLocked() int {
	n := 0
	for _, u := range s.users {
		if u.Role == RoleAdmin {
			n++
		}
	}
	return n
}

// CreateMachine registers a new machine owned by ownerUserID.
func (s *Store) CreateMachine(ownerUserID string, spec MachineSpec) (Machine, error) {
	if spec.Name == "" || spec.Address == "" {
		return Machine{}, fmt.Errorf("name and address are required")
	}
	if spec.Port <= 0 {
		spec.Port = 22
	}
	if spec.SSHUser == "" {
		spec.SSHUser = "root"
	}
	m := Machine{
		ID:               uuid.NewString(),
		OwnerUserID:      ownerUserID,
		Name:             spec.Name,
		Address:          spec.Address,
		Port:             spec.Port,
		SSHUser:          spec.SSHUser,
		SSHPassword:      spec.SSHPassword,
		SSHPrivateKey:    spec.SSHPrivateKey,
		SSHKeyPassphrase: spec.SSHKeyPassphrase,
		CreatedAt:        time.Now().UTC(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.machines[m.ID] = m
	if err := s.saveLocked(); err != nil {
		return Machine{}, err
	}
	return m, nil
}

// PinMachineHostKey records the host key fingerprint observed on a machine's
// first successful connect. It never overwrites an existing pin: replacing a
// recorded key is exactly what an attacker would want, so a genuine host
// rebuild goes through delete-and-re-register instead.
//
// Reports whether it wrote anything.
func (s *Store) PinMachineHostKey(id, fingerprint string) (bool, error) {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.machines[id]
	if !ok {
		return false, ErrNotFound
	}
	if m.HostKeyFingerprint != "" {
		return false, nil
	}
	m.HostKeyFingerprint = fingerprint
	s.machines[id] = m
	return true, s.saveLocked()
}

// ListMachines returns machines for ownerUserID (empty ownerID = all).
// Prefer ListMachinesForUser for API access filtering (owner + grants + admin).
func (s *Store) ListMachines(ownerUserID string) []Machine {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Machine, 0, len(s.machines))
	for _, m := range s.machines {
		if ownerUserID != "" && m.OwnerUserID != "" && m.OwnerUserID != ownerUserID {
			continue
		}
		out = append(out, m)
	}
	sortMachines(out)
	return out
}

// ListMachinesForUser returns machines the principal may use.
// Admin sees all; others see owned, granted, or legacy unowned rows.
func (s *Store) ListMachinesForUser(userID, role string) []Machine {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Machine, 0, len(s.machines))
	for _, m := range s.machines {
		if s.userCanAccessMachineLocked(userID, role, m) {
			out = append(out, m)
		}
	}
	sortMachines(out)
	return out
}

func sortMachines(out []Machine) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].Name < out[j].Name
	})
}

// UserCanAccessMachine reports whether the principal may open/use the machine.
func (s *Store) UserCanAccessMachine(userID, role string, m Machine) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.userCanAccessMachineLocked(userID, role, m)
}

func (s *Store) userCanAccessMachineLocked(userID, role string, m Machine) bool {
	if role == RoleAdmin {
		return true
	}
	// Legacy rows without owner remain visible to any authenticated user.
	if m.OwnerUserID == "" {
		return true
	}
	if m.OwnerUserID == userID {
		return true
	}
	_, ok := s.grants[grantKey(userID, m.ID)]
	return ok
}

// UserCanManageMachine is owner or admin (delete / reassign-level ops).
func (s *Store) UserCanManageMachine(userID, role string, m Machine) bool {
	if role == RoleAdmin {
		return true
	}
	if m.OwnerUserID == "" {
		return true
	}
	return m.OwnerUserID == userID
}

// GetMachine returns a machine by id.
func (s *Store) GetMachine(id string) (Machine, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.machines[id]
	if !ok {
		return Machine{}, ErrNotFound
	}
	return m, nil
}

// DeleteMachine removes a machine, its terminal sessions, and related grants.
func (s *Store) DeleteMachine(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.machines[id]; !ok {
		return ErrNotFound
	}
	delete(s.machines, id)
	for tid, t := range s.terminals {
		if t.MachineID == id {
			delete(s.terminals, tid)
		}
	}
	for k, g := range s.grants {
		if g.MachineID == id {
			delete(s.grants, k)
		}
	}
	return s.saveLocked()
}

// --- Grants (user ↔ machine) ---

// GrantMachineAccess allows userID to use machineID. Idempotent.
func (s *Store) GrantMachineAccess(userID, machineID string) (MachineGrant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return MachineGrant{}, ErrNotFound
	}
	if _, ok := s.machines[machineID]; !ok {
		return MachineGrant{}, ErrNotFound
	}
	key := grantKey(userID, machineID)
	if g, ok := s.grants[key]; ok {
		return g, nil
	}
	g := MachineGrant{
		UserID:    userID,
		MachineID: machineID,
		CreatedAt: time.Now().UTC(),
	}
	s.grants[key] = g
	if err := s.saveLocked(); err != nil {
		return MachineGrant{}, err
	}
	return g, nil
}

// RevokeMachineAccess removes a grant. Missing grant is not an error.
func (s *Store) RevokeMachineAccess(userID, machineID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.grants, grantKey(userID, machineID))
	return s.saveLocked()
}

// ListGrants returns all machine grants (admin UI).
func (s *Store) ListGrants() []MachineGrant {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MachineGrant, 0, len(s.grants))
	for _, g := range s.grants {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MachineID == out[j].MachineID {
			return out[i].UserID < out[j].UserID
		}
		return out[i].MachineID < out[j].MachineID
	})
	return out
}

// ListGrantsForMachine returns grants for one machine.
func (s *Store) ListGrantsForMachine(machineID string) []MachineGrant {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MachineGrant, 0)
	for _, g := range s.grants {
		if g.MachineID == machineID {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UserID < out[j].UserID })
	return out
}

// --- Audit ---

// AppendAudit records a security-relevant event (best-effort persist).
func (s *Store) AppendAudit(e AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	s.audit = append(s.audit, e)
	if len(s.audit) > maxAuditEvents {
		s.audit = append([]AuditEvent(nil), s.audit[len(s.audit)-maxAuditEvents:]...)
	}
	return s.saveLocked()
}

// ListAudit returns newest-first events, optionally capped by limit (0 = default 200).
func (s *Store) ListAudit(limit int) []AuditEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 200
	}
	n := len(s.audit)
	if n == 0 {
		return nil
	}
	out := make([]AuditEvent, n)
	copy(out, s.audit)
	// newest first
	for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if limit < n {
		out = out[:limit]
	}
	return out
}

// --- Settings ---

// GetSettings returns the access policy singleton.
func (s *Store) GetSettings() AccessSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

// UpdateSettings patches network mode (empty fields keep current).
func (s *Store) UpdateSettings(networkMode string) (AccessSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if networkMode != "" {
		switch networkMode {
		case NetworkPrivateMesh, NetworkOpen:
			s.settings.NetworkMode = networkMode
		default:
			return AccessSettings{}, fmt.Errorf("network_mode must be %s or %s", NetworkPrivateMesh, NetworkOpen)
		}
	}
	if err := s.saveLocked(); err != nil {
		return AccessSettings{}, err
	}
	return s.settings, nil
}

// CreateTerminal adds a named terminal session under a machine.
// remoteSession is the durable tmux name on the remote host (stable across reconnects).
func (s *Store) CreateTerminal(machineID, ownerUserID, name string) (Terminal, error) {
	if name == "" {
		name = "Session"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.machines[machineID]; !ok {
		return Terminal{}, ErrNotFound
	}
	now := time.Now().UTC()
	id := uuid.NewString()
	// tmux-safe name: letters, digits, underscore, hyphen
	remote := "ah-" + strings.ReplaceAll(id, "-", "")[:12]
	t := Terminal{
		ID:            id,
		MachineID:     machineID,
		OwnerUserID:   ownerUserID,
		Name:          name,
		Status:        "open",
		RemoteSession: remote,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	s.terminals[t.ID] = t
	if err := s.saveLocked(); err != nil {
		return Terminal{}, err
	}
	return t, nil
}

// ListTerminalsByMachine returns sessions for a machine (newest first).
func (s *Store) ListTerminalsByMachine(machineID string) ([]Terminal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.machines[machineID]; !ok {
		return nil, ErrNotFound
	}
	out := make([]Terminal, 0)
	for _, t := range s.terminals {
		if t.MachineID == machineID {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// ListTerminals returns all terminal sessions.
func (s *Store) ListTerminals() []Terminal {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Terminal, 0, len(s.terminals))
	for _, t := range s.terminals {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// GetTerminal returns a terminal by id.
func (s *Store) GetTerminal(id string) (Terminal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.terminals[id]
	if !ok {
		return Terminal{}, ErrNotFound
	}
	return t, nil
}

// TouchTerminal updates the session's updated_at timestamp.
func (s *Store) TouchTerminal(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.terminals[id]
	if !ok {
		return ErrNotFound
	}
	t.UpdatedAt = time.Now().UTC()
	if t.Status == "closed" {
		t.Status = "open"
	}
	s.terminals[id] = t
	return s.saveLocked()
}

// CloseTerminal marks a session closed (or removes it). We delete for clean list UX.
func (s *Store) CloseTerminal(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.terminals[id]; !ok {
		return ErrNotFound
	}
	delete(s.terminals, id)
	return s.saveLocked()
}

// RenameTerminal updates a session display name.
func (s *Store) RenameTerminal(id, name string) (Terminal, error) {
	if name == "" {
		return Terminal{}, fmt.Errorf("name is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.terminals[id]
	if !ok {
		return Terminal{}, ErrNotFound
	}
	t.Name = name
	t.UpdatedAt = time.Now().UTC()
	s.terminals[id] = t
	if err := s.saveLocked(); err != nil {
		return Terminal{}, err
	}
	return t, nil
}

func (s *Store) load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var snap snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return err
	}
	for _, u := range snap.Users {
		s.users[u.ID] = u
		s.byName[u.Username] = u.ID
	}
	for _, m := range snap.Machines {
		s.machines[m.ID] = m
	}
	for _, t := range snap.Terminals {
		s.terminals[t.ID] = t
	}
	for _, g := range snap.Grants {
		s.grants[grantKey(g.UserID, g.MachineID)] = g
	}
	if snap.Audit != nil {
		s.audit = append([]AuditEvent(nil), snap.Audit...)
	}
	if snap.Settings != nil && snap.Settings.NetworkMode != "" {
		s.settings = *snap.Settings
	} else {
		s.settings = DefaultAccessSettings()
	}
	return nil
}

func (s *Store) saveLocked() error {
	settings := s.settings
	snap := snapshot{
		Users:     make([]User, 0, len(s.users)),
		Machines:  make([]Machine, 0, len(s.machines)),
		Terminals: make([]Terminal, 0, len(s.terminals)),
		Grants:    make([]MachineGrant, 0, len(s.grants)),
		Audit:     append([]AuditEvent(nil), s.audit...),
		Settings:  &settings,
	}
	for _, u := range s.users {
		snap.Users = append(snap.Users, u)
	}
	for _, m := range s.machines {
		snap.Machines = append(snap.Machines, m)
	}
	for _, t := range s.terminals {
		snap.Terminals = append(snap.Terminals, t)
	}
	for _, g := range s.grants {
		snap.Grants = append(snap.Grants, g)
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
