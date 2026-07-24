package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime settings from the environment.
type Config struct {
	HTTPAddr                   string
	JWTSecret                  string
	JWTAccessTTL               time.Duration // <=0 means never expire
	BootstrapAdminUsername     string
	BootstrapAdminPassword     string
	DataDir                    string
	AccessDefaultTailscaleOnly bool
}

// Load reads configuration from environment variables with safe local defaults.
func Load() Config {
	// Default: non-expiring JWT (demo / long-lived terminal sessions).
	// Set JWT_ACCESS_TTL=24h (or any Go duration) to enable expiry.
	// JWT_ACCESS_TTL=0|forever|never → no expiry.
	ttl := time.Duration(0)
	if v := strings.TrimSpace(os.Getenv("JWT_ACCESS_TTL")); v != "" {
		switch strings.ToLower(v) {
		case "0", "forever", "never", "none", "-1":
			ttl = 0
		default:
			if d, err := time.ParseDuration(v); err == nil {
				ttl = d
			}
		}
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "dev-only-change-me-agent-hub"
	}
	// Demo defaults only — override in real deploys via env / .env
	user := os.Getenv("BOOTSTRAP_ADMIN_USERNAME")
	if user == "" {
		user = "admin"
	}
	pass := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	if pass == "" {
		pass = "123456"
	}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	tailscaleOnly := false
	if v := os.Getenv("ACCESS_DEFAULT_TAILSCALE_ONLY"); v != "" {
		tailscaleOnly, _ = strconv.ParseBool(v)
	}
	return Config{
		HTTPAddr:                   addr,
		JWTSecret:                  secret,
		JWTAccessTTL:               ttl,
		BootstrapAdminUsername:     user,
		BootstrapAdminPassword:     pass,
		DataDir:                    dataDir,
		AccessDefaultTailscaleOnly: tailscaleOnly,
	}
}
