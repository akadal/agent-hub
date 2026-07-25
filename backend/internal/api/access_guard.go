package api

import (
	"net/http"
	"net/netip"
	"strings"
)

// Tailnet-only access.
//
// This is a *second* lock. The one that actually holds is not publishing the
// port at all — serve the app on the tailnet (`tailscale serve`) and there is
// nothing for the internet to reach. What this adds is a backstop for the day
// the edge is misconfigured: a domain accidentally re-attached in Coolify, a
// port left published. Both locks are documented in docs/ops.md §5d.
//
// It is off by default and it refuses to lie: when the caller's address cannot
// be established (see clientip.go), the request is denied rather than waved
// through, and the Settings page says the setting is not protecting anything
// rather than showing a reassuring tick.

// accessDecision is what the guard concluded about one request.
type accessDecision struct {
	// Allowed is false only when the guard actively refused the request.
	Allowed bool
	// ClientIP is the resolved caller, empty when it could not be established.
	ClientIP string
	// Addr is the same value parsed; only valid when Known.
	Addr netip.Addr
	// Known reports whether the caller's address could be established at all.
	Known bool
	// Reason explains a refusal, for the response body and the audit line.
	Reason string
}

// trustedProxies returns the configured proxy ranges, or the built-in default.
func (s *Server) trustedProxies() []netip.Prefix {
	if len(s.TrustedProxies) > 0 {
		return s.TrustedProxies
	}
	return DefaultTrustedProxies()
}

// evaluateAccess decides whether a request may proceed under the current
// policy. It never mutates anything and is safe to call from the settings
// handler to answer "would this lock me out?".
func (s *Server) evaluateAccess(r *http.Request) accessDecision {
	ip, known := ClientIP(r, s.trustedProxies())
	d := accessDecision{Allowed: true, Known: known}
	if known {
		d.ClientIP = ip.String()
		d.Addr = ip
	}

	if s.AccessEnforcementDisabled {
		return d
	}
	if !s.Store.GetSettings().TailnetOnly {
		return d
	}

	if !known {
		d.Allowed = false
		d.Reason = "tailnet-only is on, but this server cannot tell which address the request came from. " +
			"Set TRUSTED_PROXIES to the reverse proxies in front of it, or turn the setting off."
		return d
	}
	// Loopback is the box itself. Refusing it would lock an operator out of
	// their own host and break the container health check for no gain: anyone
	// who can call from loopback is already inside.
	if ip.IsLoopback() {
		return d
	}
	if !IsTailnetAddr(ip) {
		d.Allowed = false
		d.Reason = "this instance only accepts requests from its Tailscale network"
		return d
	}
	return d
}

// wouldAllowUnderTailnetOnly answers "if the lock were on, would this caller
// get in?" — the one question the Settings page actually needs. Computing it
// here keeps the loopback exemption in a single place instead of having the UI
// re-derive it from an IP string and get it subtly wrong.
func (s *Server) wouldAllowUnderTailnetOnly(d accessDecision) bool {
	if !d.Known {
		return false
	}
	return d.Addr.IsLoopback() || IsTailnetAddr(d.Addr)
}

// accessGuardExempt lists paths that stay reachable with the lock on.
//
// Only /health: it is how a container platform decides the service is alive,
// and how an operator checks which build is running. It exposes no data beyond
// the version, which a caller who can reach the port learns anyway.
func accessGuardExempt(path string) bool {
	return path == "/health" || strings.HasSuffix(path, "/health")
}

// withAccessGuard wraps the whole mux, so a new route cannot forget the check.
func (s *Server) withAccessGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if accessGuardExempt(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if d := s.evaluateAccess(r); !d.Allowed {
			writeErr(w, http.StatusForbidden, d.Reason)
			return
		}
		next.ServeHTTP(w, r)
	})
}
