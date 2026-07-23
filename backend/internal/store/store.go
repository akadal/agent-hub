package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
)

// Store is a file-backed persistence layer for users, machines, and terminals.
type Store struct {
	mu        sync.RWMutex
	path      string
	users     map[string]User     // by id
	byName    map[string]string   // username -> id
	machines  map[string]Machine  // by id
	terminals map[string]Terminal // by id
}

type snapshot struct {
	Users     []User     `json:"users"`
	Machines  []Machine  `json:"machines"`
	Terminals []Terminal `json:"terminals"`
}

// Open creates or loads a store at dataDir/store.json.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "store.json")
	s := &Store{
		path:      path,
		users:     map[string]User{},
		byName:    map[string]string{},
		machines:  map[string]Machine{},
		terminals: map[string]Terminal{},
	}
	if _, err := os.Stat(path); err == nil {
		if err := s.load(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// EnsureBootstrapAdmin creates the bootstrap admin if no user with that username exists.
func (s *Store) EnsureBootstrapAdmin(username, password string) error {
	if username == "" || password == "" {
		return fmt.Errorf("bootstrap admin username and password required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byName[username]; ok {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
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

// Authenticate checks username/password and returns the user on success.
func (s *Store) Authenticate(username, password string) (User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.byName[username]
	if !ok {
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

// CreateMachine registers a new machine.
func (s *Store) CreateMachine(name, address string, port int, sshUser, sshPassword string) (Machine, error) {
	if name == "" || address == "" {
		return Machine{}, fmt.Errorf("name and address are required")
	}
	if port <= 0 {
		port = 22
	}
	if sshUser == "" {
		sshUser = "root"
	}
	m := Machine{
		ID:          uuid.NewString(),
		Name:        name,
		Address:     address,
		Port:        port,
		SSHUser:     sshUser,
		SSHPassword: sshPassword,
		CreatedAt:   time.Now().UTC(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.machines[m.ID] = m
	if err := s.saveLocked(); err != nil {
		return Machine{}, err
	}
	return m, nil
}

// ListMachines returns all machines sorted by name.
func (s *Store) ListMachines() []Machine {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Machine, 0, len(s.machines))
	for _, m := range s.machines {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].Name < out[j].Name
	})
	return out
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

// DeleteMachine removes a machine and all of its terminal sessions.
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
	return s.saveLocked()
}

// CreateTerminal adds a named terminal session under a machine.
func (s *Store) CreateTerminal(machineID, name string) (Terminal, error) {
	if name == "" {
		name = "Session"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.machines[machineID]; !ok {
		return Terminal{}, ErrNotFound
	}
	now := time.Now().UTC()
	t := Terminal{
		ID:        uuid.NewString(),
		MachineID: machineID,
		Name:      name,
		Status:    "open",
		CreatedAt: now,
		UpdatedAt: now,
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
	return nil
}

func (s *Store) saveLocked() error {
	snap := snapshot{
		Users:     make([]User, 0, len(s.users)),
		Machines:  make([]Machine, 0, len(s.machines)),
		Terminals: make([]Terminal, 0, len(s.terminals)),
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
