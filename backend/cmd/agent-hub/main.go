package main

import (
	"log"
	"net/http"
	"time"

	"github.com/akadal/agent-hub/backend/internal/api"
	"github.com/akadal/agent-hub/backend/internal/auth"
	"github.com/akadal/agent-hub/backend/internal/config"
	"github.com/akadal/agent-hub/backend/internal/store"
)

func main() {
	cfg := config.Load()

	st, err := store.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	if err := st.EnsureBootstrapAdmin(cfg.BootstrapAdminUsername, cfg.BootstrapAdminPassword); err != nil {
		log.Fatalf("bootstrap admin: %v", err)
	}
	log.Printf("bootstrap admin ready: user=%s", cfg.BootstrapAdminUsername)

	warnDemoCredentials(cfg)

	tokens := auth.NewTokenService(cfg.JWTSecret, cfg.JWTAccessTTL)
	srv := &api.Server{
		Store:            st,
		Tokens:           tokens,
		Log:              log.Default(),
		TailscaleAPIKey:  cfg.TailscaleAPIKey,
		TailscaleTailnet: cfg.TailscaleTailnet,
	}

	addr := cfg.HTTPAddr
	if addr == "" {
		addr = ":27341"
	}
	if cfg.TailscaleAPIKey != "" {
		log.Printf("tailscale import enabled (tailnet=%s)", cfg.TailscaleTailnet)
	}
	log.Printf("agent-hub api listening on %s (data=%s)", addr, cfg.DataDir)
	// Explicit server: the zero-value http.Server has no header timeout, so a
	// handful of slow clients can hold every connection open indefinitely.
	// Read/WriteTimeout stay unset on purpose — terminal WebSockets are
	// long-lived by design and any deadline here would cut live shells.
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.NewMux(),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if err := httpSrv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// warnDemoCredentials shouts when a deployment is still running on the
// compose-demo secrets. Both are published in .env.example, so leaving them in
// place on a reachable host means anyone can mint an admin JWT.
func warnDemoCredentials(cfg config.Config) {
	if cfg.JWTSecret == config.DefaultJWTSecret {
		log.Printf("WARNING: JWT_SECRET is the public demo default — set JWT_SECRET (e.g. `openssl rand -base64 48`) before exposing this host")
	}
	if cfg.BootstrapAdminPassword == config.DefaultAdminPassword {
		log.Printf("WARNING: BOOTSTRAP_ADMIN_PASSWORD is the public demo default — set BOOTSTRAP_ADMIN_PASSWORD before exposing this host")
	}
}
