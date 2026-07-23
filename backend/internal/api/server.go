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
	"github.com/gorilla/websocket"
)

// Server wires HTTP handlers to store, tokens, and SSH.
type Server struct {
	Store  *store.Store
	Tokens *auth.TokenService
	Log    *log.Logger
}

type ctxKey int

const claimsKey ctxKey = 1

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// NewMux returns the full Agent Hub HTTP surface.
func (s *Server) NewMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /api/hello", s.handleHello)

	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("GET /api/me", s.requireAuth(s.handleMe))

	mux.HandleFunc("GET /api/machines", s.requireAuth(s.handleListMachines))
	mux.HandleFunc("POST /api/machines", s.requireAuth(s.handleCreateMachine))
	mux.HandleFunc("GET /api/machines/{id}", s.requireAuth(s.handleGetMachine))
	mux.HandleFunc("DELETE /api/machines/{id}", s.requireAuth(s.handleDeleteMachine))
	mux.HandleFunc("POST /api/machines/{id}/exec", s.requireAuth(s.handleMachineExec))

	// Terminal sessions under a machine (1:N)
	mux.HandleFunc("GET /api/machines/{id}/terminals", s.requireAuth(s.handleListTerminals))
	mux.HandleFunc("POST /api/machines/{id}/terminals", s.requireAuth(s.handleCreateTerminal))

	mux.HandleFunc("GET /api/terminals", s.requireAuth(s.handleListAllTerminals))
	mux.HandleFunc("GET /api/terminals/{id}", s.requireAuth(s.handleGetTerminal))
	mux.HandleFunc("PATCH /api/terminals/{id}", s.requireAuth(s.handlePatchTerminal))
	mux.HandleFunc("DELETE /api/terminals/{id}", s.requireAuth(s.handleCloseTerminal))
	mux.HandleFunc("POST /api/terminals/{id}/exec", s.requireAuth(s.handleTerminalExec))
	mux.HandleFunc("GET /api/terminals/{id}/ws", s.handleTerminalWS) // auth via query/header

	// Legacy machine-level WS still works (opens ephemeral shell without session record)
	mux.HandleFunc("GET /api/machines/{id}/terminal", s.handleMachineTerminalWS)

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
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      userView  `json:"user"`
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
	writeJSON(w, http.StatusOK, loginResponse{
		Token:     tok,
		ExpiresAt: exp,
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
	m, err := s.Store.CreateMachine(req.Name, req.Address, req.Port, req.SSHUser, req.SSHPassword)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, m.Public())
}

func (s *Server) handleListMachines(w http.ResponseWriter, r *http.Request) {
	list := s.Store.ListMachines()
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
	writeJSON(w, http.StatusOK, m.Public())
}

func (s *Server) handleDeleteMachine(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
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
	t, err := s.Store.CreateTerminal(machineID, req.Name)
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
	s.bridgeSSH(w, r, m, "session "+t.Name)
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
	s.bridgeSSH(w, r, m, "ephemeral")
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

func (s *Server) bridgeSSH(w http.ResponseWriter, r *http.Request, m store.Machine, label string) {
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
	}, cols, rows)
	if err != nil {
		_ = conn.WriteJSON(wsServerMsg{Type: "error", Message: "ssh open failed: " + err.Error()})
		return
	}
	defer sess.Close()

	_ = conn.WriteJSON(wsServerMsg{Type: "ready", Message: "ssh session open (" + label + ")"})

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := sess.Stdout().Read(buf)
			if n > 0 {
				_ = conn.WriteJSON(wsServerMsg{Type: "stdout", Data: string(buf[:n])})
			}
			if err != nil {
				if err != io.EOF {
					_ = conn.WriteJSON(wsServerMsg{Type: "error", Message: err.Error()})
				}
				return
			}
		}
	}()

	for {
		var msg wsClientMsg
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}
		switch msg.Type {
		case "stdin", "input":
			if _, err := sess.Stdin().Write([]byte(msg.Data)); err != nil {
				_ = conn.WriteJSON(wsServerMsg{Type: "error", Message: "stdin write: " + err.Error()})
			}
		case "resize":
			if msg.Cols > 0 && msg.Rows > 0 {
				_ = sess.Resize(msg.Cols, msg.Rows)
			}
		case "ping":
			_ = conn.WriteJSON(wsServerMsg{Type: "pong"})
		}
	}
	<-done
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
