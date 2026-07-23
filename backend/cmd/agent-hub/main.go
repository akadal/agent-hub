package main

import (
	"log"
	"net/http"

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

	tokens := auth.NewTokenService(cfg.JWTSecret, cfg.JWTAccessTTL)
	srv := &api.Server{
		Store:  st,
		Tokens: tokens,
		Log:    log.Default(),
	}

	addr := cfg.HTTPAddr
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("agent-hub api listening on %s (data=%s)", addr, cfg.DataDir)
	if err := http.ListenAndServe(addr, srv.NewMux()); err != nil {
		log.Fatal(err)
	}
}
