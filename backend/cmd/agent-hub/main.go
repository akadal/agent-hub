package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/akadal/agent-hub/backend/internal/api"
	"github.com/akadal/agent-hub/backend/internal/auth"
	"github.com/akadal/agent-hub/backend/internal/config"
	"github.com/akadal/agent-hub/backend/internal/store"
	"github.com/akadal/agent-hub/backend/internal/version"
)

func main() {
	cfg := config.Load()
	log.Printf("agent-hub %s", version.String())

	st, err := store.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	// A malformed TRUSTED_PROXIES must stop the process: starting with the
	// entry silently dropped would leave an access rule narrower or wider than
	// the operator wrote, with nothing to show for it.
	trustedProxies := api.DefaultTrustedProxies()
	if len(cfg.TrustedProxies) > 0 {
		p, err := api.ParsePrefixes(cfg.TrustedProxies)
		if err != nil {
			log.Fatalf("TRUSTED_PROXIES: %v", err)
		}
		trustedProxies = p
	}
	if cfg.AccessEnforcementDisabled {
		log.Printf("ACCESS_ENFORCEMENT=off — the tailnet-only setting is not enforced")
	}

	if cfg.AuditMaxEvents > 0 {
		st.SetAuditLimit(cfg.AuditMaxEvents)
	}
	if err := st.EnsureBootstrapAdmin(cfg.BootstrapAdminUsername, cfg.BootstrapAdminPassword); err != nil {
		log.Fatalf("bootstrap admin: %v", err)
	}
	log.Printf("bootstrap admin ready: user=%s", cfg.BootstrapAdminUsername)

	warnDemoCredentials(cfg)

	tokens := auth.NewTokenService(cfg.JWTSecret, cfg.JWTAccessTTL)
	srv := &api.Server{
		Store:                     st,
		Tokens:                    tokens,
		Log:                       log.Default(),
		TrustedProxies:            trustedProxies,
		AccessEnforcementDisabled: cfg.AccessEnforcementDisabled,
		TailscaleAPIKey:           cfg.TailscaleAPIKey,
		TailscaleTailnet:          cfg.TailscaleTailnet,
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

	// `docker stop` sends SIGTERM and then kills. Without this the process dies
	// mid-request — including mid-write of store.json, which is the file holding
	// every machine and credential.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	select {
	case err := <-errc:
		log.Fatal(err)
	case <-ctx.Done():
		stop() // a second signal now kills immediately instead of waiting
		log.Printf("shutting down…")
		// Hijacked WebSockets are not tracked by Shutdown, so this waits only on
		// plain HTTP requests; the grace period keeps it from hanging either way.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown failed, closing: %v", err)
			_ = httpSrv.Close()
		}
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
