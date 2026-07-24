package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
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
	list := s.Store.ListMachines(c.UserID)
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
	if !s.canAccessMachine(r, m) {
		writeErr(w, http.StatusNotFound, "machine not found")
		return
	}
	if err := s.Store.DeleteMachine(id); err != nil {
		writeErr(w, http.StatusNotFound, "machine not found")
		return
	}
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
	s.runExec(w, r, m)
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
	_ = s.Store.TouchTerminal(id)
	s.runExec(w, r, m)
}

func (s *Server) runExec(w http.ResponseWriter, r *http.Request, m store.Machine) {
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
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleListTerminals(w http.ResponseWriter, r *http.Request) {
	machineID := r.PathValue("id")
	list, err := s.Store.ListTerminalsByMachine(machineID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "machine not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"terminals": list})
}

func (s *Server) handleListAllTerminals(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"terminals": s.Store.ListTerminals()})
}

func (s *Server) handleGetTerminal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := s.Store.GetTerminal(id)
	if err != nil {
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
	if m, err := s.Store.GetMachine(t.MachineID); err == nil {
		_ = sshterm.KillRemoteSession(sshterm.Target{
			Address:  m.Address,
			Port:     m.Port,
			User:     m.SSHUser,
			Password: m.SSHPassword,
		}, t.RemoteSession)
	}
	if err := s.Store.CloseTerminal(id); err != nil {
		writeErr(w, http.StatusNotFound, "terminal session not found")
		return
	}
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
}

func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeWS(w, r) {
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
	_ = s.Store.TouchTerminal(id)
	// Durable tmux name so mobile/web reconnect attaches to the same shell.
	s.bridgeSSH(w, r, m, t.RemoteSession, "session "+t.Name)
}

func (s *Server) handleMachineTerminalWS(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeWS(w, r) {
		return
	}
	id := r.PathValue("id")
	m, err := s.Store.GetMachine(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "machine not found")
		return
	}
	s.bridgeSSH(w, r, m, "", "ephemeral")
}

func (s *Server) canAccessMachine(r *http.Request, m store.Machine) bool {
	c := claimsFrom(r.Context())
	if c == nil {
		return false
	}
	// Legacy rows without owner are visible to any authenticated user.
	if m.OwnerUserID == "" {
		return true
	}
	return m.OwnerUserID == c.UserID || c.Role == "admin"
}

func (s *Server) authorizeWS(w http.ResponseWriter, r *http.Request) bool {
	token := bearerToken(r)
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		writeErr(w, http.StatusUnauthorized, "missing token")
		return false
	}
	if _, err := s.Tokens.Parse(token); err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid token")
		return false
	}
	return true
}

func (s *Server) bridgeSSH(w http.ResponseWriter, r *http.Request, m store.Machine, remoteSession, label string) {
	cols := 80
	rows := 24
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logf("ws upgrade: %v", err)
		return
	}
	defer conn.Close()

	sess, err := sshterm.OpenSession(sshterm.Target{
		Address:  m.Address,
		Port:     m.Port,
		User:     m.SSHUser,
		Password: m.SSHPassword,
	}, remoteSession, cols, rows)
	if err != nil {
		_ = conn.WriteJSON(wsServerMsg{Type: "error", Message: "ssh open failed: " + err.Error()})
		return
	}
	defer sess.Close()

	msg := "ssh session open (" + label + ")"
	if remoteSession != "" {
		msg += " [tmux:" + remoteSession + "]"
	}
	_ = conn.WriteJSON(wsServerMsg{Type: "ready", Message: msg})

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
					_ = conn.WriteJSON(wsServerMsg{Type: "stdout", Data: s})
				}
			}
			if err != nil {
				if tail := utf8buf.Flush(); tail != "" {
					_ = conn.WriteJSON(wsServerMsg{Type: "stdout", Data: tail})
				}
				if err != io.EOF {
					_ = conn.WriteJSON(wsServerMsg{Type: "error", Message: err.Error()})
				}
				return
			}
		}
	}()

readLoop:
	for {
		var msg wsClientMsg
		if err := conn.ReadJSON(&msg); err != nil {
			// Client closed WS (session switch/tab close). Tear down SSH
			// immediately so the stdout reader unblocks — otherwise we leak
			// ESTABLISHED SSH connections forever waiting on <-done.
			break readLoop
		}
		switch msg.Type {
		case "stdin", "input":
			if _, err := sess.Stdin().Write([]byte(msg.Data)); err != nil {
				_ = conn.WriteJSON(wsServerMsg{Type: "error", Message: "stdin write: " + err.Error()})
				break readLoop
			}
		case "resize":
			if msg.Cols > 0 && msg.Rows > 0 {
				_ = sess.Resize(msg.Cols, msg.Rows)
			}
		case "ping":
			_ = conn.WriteJSON(wsServerMsg{Type: "pong"})
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
