package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/akadal/agent-hub/backend/internal/auth"
	"github.com/akadal/agent-hub/backend/internal/sshterm"
	"github.com/akadal/agent-hub/backend/internal/store"
	"github.com/akadal/agent-hub/backend/internal/tailscale"
	"github.com/gorilla/websocket"
)

// Server wires HTTP handlers to store, tokens, and SSH.
type Server struct {
	Store  *store.Store
	Tokens *auth.TokenService
	Log    *log.Logger
	// Optional Tailscale API import (empty key = feature off).
	TailscaleAPIKey  string
	TailscaleTailnet string
}

type ctxKey int

const claimsKey ctxKey = 1

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// NewMux returns the full Agent Hub HTTP surface.
//
// Routes are registered twice:
//   - with /api prefix — Docker Compose / nginx reverse-proxy (full path)
//   - without /api     — Coolify path-based domains that strip the /api prefix
//     (e.g. Domain for api: https://host/api → backend sees /auth/login)
func (s *Server) NewMux() http.Handler {
	mux := http.NewServeMux()
	mount := func(method, path string, h http.HandlerFunc) {
		// path must start with /
		mux.HandleFunc(method+" "+path, h)
		if path == "/health" {
			return // only one health
		}
		// dual: /api + path (path already may be /auth/login etc.)
		if !strings.HasPrefix(path, "/api") {
			mux.HandleFunc(method+" /api"+path, h)
		}
	}

	mux.HandleFunc("GET /health", s.handleHealth)
	// Coolify often maps /api/health → strip → /health (already above)

	// If the public domain is accidentally pointed at the API service (not web),
	// bare / used to show a blank Go 404. Explain how to fix Coolify routing.
	mux.HandleFunc("GET /{$}", s.handleAPIRoot)
	mux.HandleFunc("GET /", s.handleAPIRoot)

	mount("GET", "/hello", s.handleHello)
	mount("POST", "/auth/login", s.handleLogin)
	mount("GET", "/me", s.requireAuth(s.handleMe))

	// Admin-only local user management (M4.1)
	mount("GET", "/users", s.requireAuth(s.requireAdmin(s.handleListUsers)))
	mount("POST", "/users", s.requireAuth(s.requireAdmin(s.handleCreateUser)))
	mount("PATCH", "/users/{id}", s.requireAuth(s.requireAdmin(s.handleUpdateUser)))
	mount("DELETE", "/users/{id}", s.requireAuth(s.requireAdmin(s.handleDeleteUser)))

	// Permissions: user ↔ machine grants (terminals inherit machine access)
	mount("GET", "/grants", s.requireAuth(s.requireAdmin(s.handleListGrants)))
	mount("POST", "/grants", s.requireAuth(s.requireAdmin(s.handleCreateGrant)))
	mount("DELETE", "/grants", s.requireAuth(s.requireAdmin(s.handleDeleteGrant)))

	// Audit (admin list; writes happen on security-relevant actions)
	mount("GET", "/audit", s.requireAuth(s.requireAdmin(s.handleListAudit)))

	// Access policy settings
	mount("GET", "/settings", s.requireAuth(s.handleGetSettings))
	mount("PATCH", "/settings", s.requireAuth(s.requireAdmin(s.handlePatchSettings)))

	mount("GET", "/machines", s.requireAuth(s.handleListMachines))
	mount("POST", "/machines", s.requireAuth(s.handleCreateMachine))
	// Tailscale import — register before /machines/{id} is fine; patterns are distinct.
	mount("GET", "/machines/tailscale", s.requireAuth(s.handleTailscaleStatus))
	mount("POST", "/machines/tailscale/import", s.requireAuth(s.handleTailscaleImport))
	mount("GET", "/machines/{id}", s.requireAuth(s.handleGetMachine))
	mount("DELETE", "/machines/{id}", s.requireAuth(s.handleDeleteMachine))
	mount("POST", "/machines/{id}/exec", s.requireAuth(s.handleMachineExec))

	mount("GET", "/machines/{id}/terminals", s.requireAuth(s.handleListTerminals))
	mount("POST", "/machines/{id}/terminals", s.requireAuth(s.handleCreateTerminal))

	mount("GET", "/terminals", s.requireAuth(s.handleListAllTerminals))
	mount("GET", "/terminals/{id}", s.requireAuth(s.handleGetTerminal))
	mount("PATCH", "/terminals/{id}", s.requireAuth(s.handlePatchTerminal))
	mount("DELETE", "/terminals/{id}", s.requireAuth(s.handleCloseTerminal))
	mount("POST", "/terminals/{id}/exec", s.requireAuth(s.handleTerminalExec))
	mount("GET", "/terminals/{id}/ws", s.handleTerminalWS)

	mount("GET", "/machines/{id}/terminal", s.handleMachineTerminalWS)

	return withCORS(mux)
}

// NewMux is kept for simple health-only tests of the package entry.
func NewMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "agent-hub"})
	})
	mux.HandleFunc("GET /api/hello", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"message": "hello from agent-hub",
			"service": "agent-hub",
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})
	return withCORS(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "agent-hub"})
}

func (s *Server) handleAPIRoot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "agent-hub-api",
		"ok":      true,
		"hint":    "This is the API process, not the web UI. In Coolify attach your public domain ONLY to the web service (port 80). Leave the api service without a public domain — web proxies /api internally.",
		"health":  "/health",
		"login":   "POST /api/auth/login or POST /auth/login",
	})
}

func (s *Server) handleHello(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"message": "hello from agent-hub",
		"service": "agent-hub",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
	// ExpiresAt is null when JWT has no expiry (forever).
	ExpiresAt *time.Time `json:"expires_at"`
	User      userView   `json:"user"`
}

type userView struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	u, err := s.Store.Authenticate(req.Username, req.Password)
	if err != nil {
		if errors.Is(err, store.ErrInvalidCreds) {
			_ = s.Store.AppendAudit(store.AuditEvent{
				Username: req.Username,
				Action:   "login.failed",
				Detail:   "invalid credentials",
			})
			writeErr(w, http.StatusUnauthorized, "invalid username or password")
			return
		}
		writeErr(w, http.StatusInternalServerError, "auth failed")
		return
	}
	tok, exp, err := s.Tokens.Issue(u.ID, u.Username, u.Role)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token issue failed")
		return
	}
	_ = s.Store.AppendAudit(store.AuditEvent{
		UserID:   u.ID,
		Username: u.Username,
		Action:   "login.ok",
	})
	var expPtr *time.Time
	if !exp.IsZero() {
		expPtr = &exp
	}
	writeJSON(w, http.StatusOK, loginResponse{
		Token:     tok,
		ExpiresAt: expPtr,
		User:      userView{ID: u.ID, Username: u.Username, Role: u.Role},
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	c := claimsFrom(r.Context())
	writeJSON(w, http.StatusOK, userView{ID: c.UserID, Username: c.Username, Role: c.Role})
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"` // admin | user (default user)
}

type updateUserRequest struct {
	Password string `json:"password"` // optional; empty = unchanged
	Role     string `json:"role"`     // optional; empty = unchanged
}

type userListResponse struct {
	Users []store.UserPublic `json:"users"`
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users := s.Store.ListUsers()
	out := make([]store.UserPublic, 0, len(users))
	for _, u := range users {
		out = append(out, u.Public())
	}
	writeJSON(w, http.StatusOK, userListResponse{Users: out})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	u, err := s.Store.CreateUser(req.Username, req.Password, req.Role)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrUserExists):
			writeErr(w, http.StatusConflict, "username already exists")
		case errors.Is(err, store.ErrInvalidRole):
			writeErr(w, http.StatusBadRequest, "role must be admin or user")
		default:
			if strings.Contains(err.Error(), "required") {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			writeErr(w, http.StatusInternalServerError, "create user failed")
		}
		return
	}
	writeJSON(w, http.StatusCreated, u.Public())
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.Password == "" && req.Role == "" {
		writeErr(w, http.StatusBadRequest, "password or role required")
		return
	}
	u, err := s.Store.UpdateUser(id, req.Password, req.Role)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeErr(w, http.StatusNotFound, "user not found")
		case errors.Is(err, store.ErrLastAdmin):
			writeErr(w, http.StatusConflict, "cannot demote the last admin")
		case errors.Is(err, store.ErrInvalidRole):
			writeErr(w, http.StatusBadRequest, "role must be admin or user")
		default:
			writeErr(w, http.StatusInternalServerError, "update user failed")
		}
		return
	}
	writeJSON(w, http.StatusOK, u.Public())
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Store.DeleteUser(id); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeErr(w, http.StatusNotFound, "user not found")
		case errors.Is(err, store.ErrLastAdmin):
			writeErr(w, http.StatusConflict, "cannot delete the last admin")
		default:
			writeErr(w, http.StatusInternalServerError, "delete user failed")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type createMachineRequest struct {
	Name        string `json:"name"`
	Address     string `json:"address"`
	Port        int    `json:"port"`
	SSHUser     string `json:"ssh_user"`
	SSHPassword string `json:"ssh_password"`
}

func (s *Server) handleCreateMachine(w http.ResponseWriter, r *http.Request) {
	var req createMachineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	c := claimsFrom(r.Context())
	m, err := s.Store.CreateMachine(c.UserID, req.Name, req.Address, req.Port, req.SSHUser, req.SSHPassword)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "machine.create", m.ID, "", m.Name+" @ "+m.Address)
	writeJSON(w, http.StatusCreated, m.Public())
}

func (s *Server) tailscaleClient() *tailscale.Client {
	return &tailscale.Client{
		APIKey:  s.TailscaleAPIKey,
		Tailnet: s.TailscaleTailnet,
	}
}

func (s *Server) handleTailscaleStatus(w http.ResponseWriter, r *http.Request) {
	client := s.tailscaleClient()
	if !client.Configured() {
		writeJSON(w, http.StatusOK, map[string]any{
			"configured": false,
			"hint":       "Set TAILSCALE_API_KEY on the api service (Keys page in Tailscale admin). Optional: TAILSCALE_TAILNET=-",
		})
		return
	}
	devices, err := client.ListDevices(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	c := claimsFrom(r.Context())
	existing := s.Store.ListMachines(c.UserID)
	known := make(map[string]bool, len(existing))
	for _, m := range existing {
		known[m.Address] = true
	}
	type devView struct {
		Name             string `json:"name"`
		Hostname         string `json:"hostname"`
		OS               string `json:"os"`
		PreferredAddress string `json:"preferred_address"`
		Online           bool   `json:"online"`
		AlreadyAdded     bool   `json:"already_added"`
	}
	out := make([]devView, 0, len(devices))
	for _, d := range devices {
		out = append(out, devView{
			Name:             d.Name,
			Hostname:         d.Hostname,
			OS:               d.OS,
			PreferredAddress: d.PreferredAddress,
			Online:           d.Online,
			AlreadyAdded:     known[d.PreferredAddress],
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": true,
		"devices":    out,
	})
}

type tailscaleImportRequest struct {
	SSHUser     string `json:"ssh_user"`
	SSHPassword string `json:"ssh_password"`
	Port        int    `json:"port"`
	// OnlineOnly skips devices not seen recently (default true).
	OnlineOnly *bool `json:"online_only"`
}

func (s *Server) handleTailscaleImport(w http.ResponseWriter, r *http.Request) {
	client := s.tailscaleClient()
	if !client.Configured() {
		writeErr(w, http.StatusServiceUnavailable, "Tailscale import not configured — set TAILSCALE_API_KEY on api")
		return
	}
	var req tailscaleImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.Port <= 0 {
		req.Port = 22
	}
	if strings.TrimSpace(req.SSHUser) == "" {
		req.SSHUser = "root"
	}
	onlineOnly := true
	if req.OnlineOnly != nil {
		onlineOnly = *req.OnlineOnly
	}

	devices, err := client.ListDevices(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	c := claimsFrom(r.Context())
	existing := s.Store.ListMachines(c.UserID)
	known := make(map[string]bool, len(existing))
	for _, m := range existing {
		known[m.Address] = true
	}

	added := make([]store.MachinePublic, 0)
	skipped := 0
	for _, d := range devices {
		if onlineOnly && !d.Online {
			skipped++
			continue
		}
		if known[d.PreferredAddress] {
			skipped++
			continue
		}
		m, err := s.Store.CreateMachine(
			c.UserID,
			d.Name,
			d.PreferredAddress,
			req.Port,
			req.SSHUser,
			req.SSHPassword,
		)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		known[d.PreferredAddress] = true
		added = append(added, m.Public())
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"added":   added,
		"skipped": skipped,
		"message": fmt.Sprintf("Added %d machine(s), skipped %d", len(added), skipped),
	})
}

func (s *Server) handleListMachines(w http.ResponseWriter, r *http.Request) {
	c := claimsFrom(r.Context())
	list := s.Store.ListMachinesForUser(c.UserID, c.Role)
	out := make([]store.MachinePublic, 0, len(list))
	for _, m := range list {
		out = append(out, m.Public())
	}
	writeJSON(w, http.StatusOK, map[string]any{"machines": out})
}

func (s *Server) handleGetMachine(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m, err := s.Store.GetMachine(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "machine not found")
		return
	}
	if !s.canAccessMachine(r, m) {
		writeErr(w, http.StatusNotFound, "machine not found")
		return
	}
	writeJSON(w, http.StatusOK, m.Public())
}

func (s *Server) handleDeleteMachine(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m, err := s.Store.GetMachine(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "machine not found")
		return
	}
	if !s.canManageMachine(r, m) {
		writeErr(w, http.StatusForbidden, "not allowed to delete this machine")
		return
	}
	if err := s.Store.DeleteMachine(id); err != nil {
		writeErr(w, http.StatusNotFound, "machine not found")
		return
	}
	s.audit(r, "machine.delete", id, "", m.Name)
	w.WriteHeader(http.StatusNoContent)
}

type execRequest struct {
	Command string `json:"command"`
}

func (s *Server) handleMachineExec(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m, err := s.Store.GetMachine(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "machine not found")
		return
	}
	if !s.canAccessMachine(r, m) {
		writeErr(w, http.StatusNotFound, "machine not found")
		return
	}
	s.runExec(w, r, m, id, "")
}

func (s *Server) handleTerminalExec(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := s.Store.GetTerminal(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "terminal session not found")
		return
	}
	m, err := s.Store.GetMachine(t.MachineID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "machine not found")
		return
	}
	if !s.canAccessMachine(r, m) {
		writeErr(w, http.StatusNotFound, "terminal session not found")
		return
	}
	_ = s.Store.TouchTerminal(id)
	s.runExec(w, r, m, t.MachineID, id)
}

func (s *Server) runExec(w http.ResponseWriter, r *http.Request, m store.Machine, machineID, terminalID string) {
	var req execRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Command) == "" {
		writeErr(w, http.StatusBadRequest, "command is required")
		return
	}
	res, err := sshterm.RunCommand(sshterm.Target{
		Address:  m.Address,
		Port:     m.Port,
		User:     m.SSHUser,
		Password: m.SSHPassword,
	}, req.Command)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "ssh exec failed: "+err.Error())
		return
	}
	// Log action only (not full command payload — keep audit lean/MVP).
	cmd := strings.TrimSpace(req.Command)
	if len(cmd) > 120 {
		cmd = cmd[:120] + "…"
	}
	s.audit(r, "exec", machineID, terminalID, cmd)
	writeJSON(w, http.StatusOK, res)
}

type createTerminalRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleCreateTerminal(w http.ResponseWriter, r *http.Request) {
	machineID := r.PathValue("id")
	m, err := s.Store.GetMachine(machineID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "machine not found")
		return
	}
	if !s.canAccessMachine(r, m) {
		writeErr(w, http.StatusNotFound, "machine not found")
		return
	}
	var req createTerminalRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	// default name: Session N
	if strings.TrimSpace(req.Name) == "" {
		existing, err := s.Store.ListTerminalsByMachine(machineID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "machine not found")
			return
		}
		req.Name = fmt.Sprintf("Session %d", len(existing)+1)
	}
	c := claimsFrom(r.Context())
	t, err := s.Store.CreateTerminal(machineID, c.UserID, req.Name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "machine not found")
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "terminal.create", machineID, t.ID, t.Name)
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleListTerminals(w http.ResponseWriter, r *http.Request) {
	machineID := r.PathValue("id")
	m, err := s.Store.GetMachine(machineID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "machine not found")
		return
	}
	if !s.canAccessMachine(r, m) {
		writeErr(w, http.StatusNotFound, "machine not found")
		return
	}
	list, err := s.Store.ListTerminalsByMachine(machineID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "machine not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"terminals": list})
}

func (s *Server) handleListAllTerminals(w http.ResponseWriter, r *http.Request) {
	c := claimsFrom(r.Context())
	all := s.Store.ListTerminals()
	out := make([]store.Terminal, 0, len(all))
	for _, t := range all {
		m, err := s.Store.GetMachine(t.MachineID)
		if err != nil {
			continue
		}
		if s.Store.UserCanAccessMachine(c.UserID, c.Role, m) {
			out = append(out, t)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"terminals": out})
}

func (s *Server) handleGetTerminal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := s.Store.GetTerminal(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "terminal session not found")
		return
	}
	m, err := s.Store.GetMachine(t.MachineID)
	if err != nil || !s.canAccessMachine(r, m) {
		writeErr(w, http.StatusNotFound, "terminal session not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

type patchTerminalRequest struct {
	Name string `json:"name"`
}

func (s *Server) handlePatchTerminal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := s.Store.GetTerminal(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "terminal session not found")
		return
	}
	if m, err := s.Store.GetMachine(existing.MachineID); err != nil || !s.canAccessMachine(r, m) {
		writeErr(w, http.StatusNotFound, "terminal session not found")
		return
	}
	var req patchTerminalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	t, err := s.Store.RenameTerminal(id, req.Name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "terminal session not found")
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleCloseTerminal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := s.Store.GetTerminal(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "terminal session not found")
		return
	}
	m, err := s.Store.GetMachine(t.MachineID)
	if err != nil || !s.canAccessMachine(r, m) {
		writeErr(w, http.StatusNotFound, "terminal session not found")
		return
	}
	_ = sshterm.KillRemoteSession(sshterm.Target{
		Address:  m.Address,
		Port:     m.Port,
		User:     m.SSHUser,
		Password: m.SSHPassword,
	}, t.RemoteSession)
	if err := s.Store.CloseTerminal(id); err != nil {
		writeErr(w, http.StatusNotFound, "terminal session not found")
		return
	}
	s.audit(r, "terminal.close", t.MachineID, id, t.Name)
	w.WriteHeader(http.StatusNoContent)
}

type wsClientMsg struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

type wsServerMsg struct {
	Type    string `json:"type"`
	Data    string `json:"data,omitempty"`
	Message string `json:"message,omitempty"`

	// Set on "error" frames when the SSH open path produced a classified cause.
	// The client uses Kind/Retryable to decide whether reconnecting can help,
	// and renders Hint/ApprovalURL instead of a blank terminal.
	Kind        string `json:"kind,omitempty"`
	Hint        string `json:"hint,omitempty"`
	ApprovalURL string `json:"approval_url,omitempty"`
	Retryable   *bool  `json:"retryable,omitempty"`
}

// sshOpenErrorMsg turns a failed SSH open into an "error" frame. When sshterm
// classified the cause, the frame carries the machine-readable kind, the concrete
// fix, and any approval URL the remote handed us — so the UI explains itself
// instead of showing a black terminal, and stops retrying causes that only a
// human can clear (browser approval, wrong credentials, wrong network topology).
func sshOpenErrorMsg(err error) wsServerMsg {
	msg := wsServerMsg{Type: "error", Message: "ssh open failed: " + err.Error()}
	var oe *sshterm.OpenError
	if errors.As(err, &oe) {
		retryable := oe.Retryable
		msg.Message = oe.Message
		msg.Kind = string(oe.Kind)
		msg.Hint = oe.Hint
		msg.ApprovalURL = oe.ApprovalURL
		msg.Retryable = &retryable
	}
	return msg
}

func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	r, ok := s.authorizeWS(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	t, err := s.Store.GetTerminal(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "terminal session not found")
		return
	}
	m, err := s.Store.GetMachine(t.MachineID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "machine not found")
		return
	}
	if !s.canAccessMachine(r, m) {
		writeErr(w, http.StatusForbidden, "access denied")
		return
	}
	_ = s.Store.TouchTerminal(id)
	s.audit(r, "terminal.attach", t.MachineID, id, t.Name)
	// Durable tmux name so mobile/web reconnect attaches to the same shell.
	s.bridgeSSH(w, r, m, t.RemoteSession, "session "+t.Name)
}

func (s *Server) handleMachineTerminalWS(w http.ResponseWriter, r *http.Request) {
	r, ok := s.authorizeWS(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	m, err := s.Store.GetMachine(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "machine not found")
		return
	}
	if !s.canAccessMachine(r, m) {
		writeErr(w, http.StatusForbidden, "access denied")
		return
	}
	s.audit(r, "terminal.attach", m.ID, "", "ephemeral")
	s.bridgeSSH(w, r, m, "", "ephemeral")
}

func (s *Server) canAccessMachine(r *http.Request, m store.Machine) bool {
	c := claimsFrom(r.Context())
	if c == nil {
		return false
	}
	return s.Store.UserCanAccessMachine(c.UserID, c.Role, m)
}

func (s *Server) canManageMachine(r *http.Request, m store.Machine) bool {
	c := claimsFrom(r.Context())
	if c == nil {
		return false
	}
	return s.Store.UserCanManageMachine(c.UserID, c.Role, m)
}

// authorizeWS validates JWT (header or ?token=) and attaches claims to the request.
func (s *Server) authorizeWS(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	token := bearerToken(r)
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		writeErr(w, http.StatusUnauthorized, "missing token")
		return r, false
	}
	claims, err := s.Tokens.Parse(token)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid token")
		return r, false
	}
	ctx := context.WithValue(r.Context(), claimsKey, claims)
	return r.WithContext(ctx), true
}

func (s *Server) audit(r *http.Request, action, machineID, terminalID, detail string) {
	c := claimsFrom(r.Context())
	e := store.AuditEvent{
		Action:     action,
		MachineID:  machineID,
		TerminalID: terminalID,
		Detail:     detail,
	}
	if c != nil {
		e.UserID = c.UserID
		e.Username = c.Username
	}
	_ = s.Store.AppendAudit(e)
}

type grantRequest struct {
	UserID    string `json:"user_id"`
	MachineID string `json:"machine_id"`
}

func (s *Server) handleListGrants(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"grants": s.Store.ListGrants()})
}

func (s *Server) handleCreateGrant(w http.ResponseWriter, r *http.Request) {
	var req grantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if strings.TrimSpace(req.UserID) == "" || strings.TrimSpace(req.MachineID) == "" {
		writeErr(w, http.StatusBadRequest, "user_id and machine_id required")
		return
	}
	g, err := s.Store.GrantMachineAccess(req.UserID, req.MachineID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "user or machine not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "grant failed")
		return
	}
	s.audit(r, "grant.add", req.MachineID, "", "user="+req.UserID)
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) handleDeleteGrant(w http.ResponseWriter, r *http.Request) {
	var req grantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Also accept query params for simple clients.
		req.UserID = r.URL.Query().Get("user_id")
		req.MachineID = r.URL.Query().Get("machine_id")
	}
	if strings.TrimSpace(req.UserID) == "" || strings.TrimSpace(req.MachineID) == "" {
		writeErr(w, http.StatusBadRequest, "user_id and machine_id required")
		return
	}
	if err := s.Store.RevokeMachineAccess(req.UserID, req.MachineID); err != nil {
		writeErr(w, http.StatusInternalServerError, "revoke failed")
		return
	}
	s.audit(r, "grant.revoke", req.MachineID, "", "user="+req.UserID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"events": s.Store.ListAudit(200)})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Store.GetSettings())
}

type patchSettingsRequest struct {
	NetworkMode string `json:"network_mode"`
}

func (s *Server) handlePatchSettings(w http.ResponseWriter, r *http.Request) {
	var req patchSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	st, err := s.Store.UpdateSettings(req.NetworkMode)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "settings.update", "", "", "network_mode="+st.NetworkMode)
	writeJSON(w, http.StatusOK, st)
}

// WebSocket keepalive for long-lived terminals (especially mobile browsers).
// Mobile Safari/Chrome suspend JS timers when the screen locks or the tab is
// backgrounded; protocol-level Ping frames do not need client JS and also
// reset reverse-proxy idle timers (Coolify/Traefik/nginx).
const (
	wsWriteWait  = 10 * time.Second
	wsPongWait   = 90 * time.Second
	wsPingPeriod = 25 * time.Second // must be < wsPongWait
)

func (s *Server) bridgeSSH(w http.ResponseWriter, r *http.Request, m store.Machine, remoteSession, label string) {
	cols := 80
	rows := 24
	if c := r.URL.Query().Get("cols"); c != "" {
		if n, err := strconv.Atoi(c); err == nil && n > 0 {
			cols = n
		}
	}
	if rw := r.URL.Query().Get("rows"); rw != "" {
		if n, err := strconv.Atoi(rw); err == nil && n > 0 {
			rows = n
		}
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logf("ws upgrade: %v", err)
		return
	}
	defer conn.Close()

	// Serialize all writes: stdout pump, app pong, and protocol pings share conn.
	var writeMu sync.Mutex
	writeMsg := func(v any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
		err := conn.WriteJSON(v)
		_ = conn.SetWriteDeadline(time.Time{})
		return err
	}
	writePing := func() error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(wsWriteWait))
	}

	// Browsers auto-reply to Ping with Pong. Extend read deadline on pong so
	// a suspended phone eventually times out cleanly (client reconnects on wake).
	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
		return nil
	})

	// Tell the client we are past WS upgrade and about to dial SSH — otherwise
	// the UI sits on "connected" with a black xterm and no prompt for the full
	// dial timeout (looks like a dead session).
	target := fmt.Sprintf("%s@%s:%d", m.SSHUser, m.Address, m.Port)
	_ = writeMsg(wsServerMsg{
		Type:    "status",
		Message: "opening ssh to " + target + "…",
	})

	openStart := time.Now()
	sess, err := sshterm.OpenSession(sshterm.Target{
		Address:  m.Address,
		Port:     m.Port,
		User:     m.SSHUser,
		Password: m.SSHPassword,
	}, remoteSession, cols, rows)
	if err != nil {
		s.logf("bridgeSSH open failed %s after %s: %v", target, time.Since(openStart).Round(time.Millisecond), err)
		_ = writeMsg(sshOpenErrorMsg(err))
		return
	}
	defer sess.Close()
	s.logf("bridgeSSH open ok %s in %s (%s)", target, time.Since(openStart).Round(time.Millisecond), label)

	msg := "ssh session open (" + label + ")"
	if remoteSession != "" {
		msg += " [tmux:" + remoteSession + "]"
	}
	_ = writeMsg(wsServerMsg{Type: "ready", Message: msg})

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		// Hold incomplete multi-byte UTF-8 across reads so Turkish (and other)
		// characters are not corrupted when a rune straddles a packet boundary.
		var utf8buf sshterm.UTF8Buffer
		for {
			n, err := sess.Stdout().Read(buf)
			if n > 0 {
				if s := utf8buf.Take(buf[:n]); s != "" {
					if werr := writeMsg(wsServerMsg{Type: "stdout", Data: s}); werr != nil {
						return
					}
				}
			}
			if err != nil {
				if tail := utf8buf.Flush(); tail != "" {
					_ = writeMsg(wsServerMsg{Type: "stdout", Data: tail})
				}
				if err != io.EOF {
					_ = writeMsg(wsServerMsg{Type: "error", Message: err.Error()})
				}
				return
			}
		}
	}()

	// Server-driven keepalive independent of mobile JS timers.
	pingStop := make(chan struct{})
	go func() {
		t := time.NewTicker(wsPingPeriod)
		defer t.Stop()
		for {
			select {
			case <-pingStop:
				return
			case <-t.C:
				if err := writePing(); err != nil {
					// Unblock ReadJSON so we tear down promptly.
					_ = conn.Close()
					return
				}
			}
		}
	}()
	defer close(pingStop)

readLoop:
	for {
		var msg wsClientMsg
		if err := conn.ReadJSON(&msg); err != nil {
			// Client closed WS, read deadline (stale phone sleep), or ping failure.
			// Tear down SSH immediately so the stdout reader unblocks — otherwise
			// we leak ESTABLISHED SSH connections forever waiting on <-done.
			break readLoop
		}
		// Any client frame (stdin/resize/app-ping) proves the link is alive.
		_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
		switch msg.Type {
		case "stdin", "input":
			if _, err := sess.Stdin().Write([]byte(msg.Data)); err != nil {
				_ = writeMsg(wsServerMsg{Type: "error", Message: "stdin write: " + err.Error()})
				break readLoop
			}
		case "resize":
			if msg.Cols > 0 && msg.Rows > 0 {
				_ = sess.Resize(msg.Cols, msg.Rows)
			}
		case "ping":
			_ = writeMsg(wsServerMsg{Type: "pong"})
		}
	}
	// Always close SSH before waiting: unblocks stdout Read and drops TCP.
	_ = sess.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		s.logf("bridgeSSH: stdout pump did not exit within 5s after WS close")
	}
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := bearerToken(r)
		if tok == "" {
			writeErr(w, http.StatusUnauthorized, "missing authorization")
			return
		}
		claims, err := s.Tokens.Parse(tok)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "invalid token")
			return
		}
		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next(w, r.WithContext(ctx))
	}
}

// requireAdmin must be nested inside requireAuth so claims are present.
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := claimsFrom(r.Context())
		if c == nil || c.Role != store.RoleAdmin {
			writeErr(w, http.StatusForbidden, "admin required")
			return
		}
		next(w, r)
	}
}

func claimsFrom(ctx context.Context) *auth.Claims {
	c, _ := ctx.Value(claimsKey).(*auth.Claims)
	return c
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	const p = "Bearer "
	if strings.HasPrefix(h, p) {
		return strings.TrimSpace(h[len(p):])
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) logf(format string, args ...any) {
	if s.Log != nil {
		s.Log.Printf(format, args...)
		return
	}
	log.Printf(format, args...)
}
