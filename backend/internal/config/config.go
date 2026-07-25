package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Demo fallbacks. They keep `docker compose up` working with no .env, but a
// deployment still running on them is one stolen JWT away from a root shell —
// so main warns loudly when they are in play.
const (
	DefaultJWTSecret     = "dev-only-change-me-agent-hub"
	DefaultAdminPassword = "123456"
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
	// Optional Tailscale API for one-shot machine import (no daemon required).
	TailscaleAPIKey  string
	TailscaleTailnet string // empty or "-" = key owner's default tailnet
	// AuditMaxEvents is how many audit rows the file store keeps. 0 = default.
	AuditMaxEvents int
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
		secret = DefaultJWTSecret
	}
	// Demo defaults only — override in real deploys via env / .env
	user := os.Getenv("BOOTSTRAP_ADMIN_USERNAME")
	if user == "" {
		user = "admin"
	}
	pass := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	if pass == "" {
		pass = DefaultAdminPassword
	}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":27341"
	}
	tailscaleOnly := false
	if v := os.Getenv("ACCESS_DEFAULT_TAILSCALE_ONLY"); v != "" {
		tailscaleOnly, _ = strconv.ParseBool(v)
	}
	auditMax := 0
	if v := strings.TrimSpace(os.Getenv("AUDIT_MAX_EVENTS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			auditMax = n
		}
	}
	tsKey := strings.TrimSpace(os.Getenv("TAILSCALE_API_KEY"))
	tsNet := strings.TrimSpace(os.Getenv("TAILSCALE_TAILNET"))
	if tsNet == "" {
		tsNet = "-"
	}
	return Config{
		HTTPAddr:                   addr,
		JWTSecret:                  secret,
		JWTAccessTTL:               ttl,
		BootstrapAdminUsername:     user,
		BootstrapAdminPassword:     pass,
		DataDir:                    dataDir,
		AccessDefaultTailscaleOnly: tailscaleOnly,
		TailscaleAPIKey:            tsKey,
		TailscaleTailnet:           tsNet,
		AuditMaxEvents:             auditMax,
	}
}
