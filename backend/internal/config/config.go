package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds runtime settings from the environment.
type Config struct {
	HTTPAddr                 string
	JWTSecret                string
	JWTAccessTTL             time.Duration
	BootstrapAdminUsername   string
	BootstrapAdminPassword   string
	DataDir                  string
	AccessDefaultTailscaleOnly bool
}

// Load reads configuration from environment variables with safe local defaults.
func Load() Config {
	ttl := 24 * time.Hour
	if v := os.Getenv("JWT_ACCESS_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			ttl = d
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
	user := os.Getenv("BOOTSTRAP_ADMIN_USERNAME")
	if user == "" {
		user = "akadal"
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
