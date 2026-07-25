package api

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// Working out who is actually calling.
//
// Every access rule that talks about "where the request came from" needs one
// honest answer to this, and behind a reverse proxy it is not `RemoteAddr` —
// that is the proxy. The usual shortcut, "just read X-Forwarded-For", is worse
// than nothing: the header is caller-supplied, so an access rule built on it
// hands the attacker the key. `X-Forwarded-For: 100.64.0.5` and they are
// "inside the tailnet".
//
// The rule that does hold: a conforming proxy *appends* the peer it saw. So
// walking the chain from the right and stopping at the first address that is
// not one of our own proxies yields the real client — anything the caller
// invented sits further left and is never reached. That only works if we know
// which addresses are ours, which is what TRUSTED_PROXIES declares.
//
// When the answer cannot be established, this says so rather than guessing.
// Callers must fail closed: an access rule that silently allows whatever it
// could not identify is decoration.

// defaultTrustedProxyCIDRs covers loopback and the private ranges a container
// platform puts its proxies on (docker bridge, LAN reverse proxy).
//
// No IPv6 ULA range is listed. The obvious candidate, fc00::/7, *contains*
// Tailscale's own fd7a:115c:a1e0::/48 — trusting it would mean an IPv6 tailnet
// peer counts as one of our proxies and gets to dictate its own address. An
// operator whose proxy really is on a ULA can name it precisely instead.
var defaultTrustedProxyCIDRs = []string{
	"127.0.0.0/8",
	"::1/128",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
}

// tailscaleCIDRs are the addresses a tailnet peer can have: the CGNAT block
// Tailscale hands out for IPv4, and its ULA prefix for IPv6. An iPhone on the
// tailnet is often reached over the IPv6 one, so both have to be here.
var tailscaleCIDRs = []string{
	"100.64.0.0/10",
	"fd7a:115c:a1e0::/48",
}

var (
	defaultTrustedProxies = mustPrefixes(defaultTrustedProxyCIDRs)
	tailscalePrefixes     = mustPrefixes(tailscaleCIDRs)
)

func mustPrefixes(cidrs []string) []netip.Prefix {
	p, err := ParsePrefixes(cidrs)
	if err != nil {
		// These literals are constants; a failure here is a programming bug.
		panic("api: bad built-in CIDR: " + err.Error())
	}
	return p
}

// ParsePrefixes converts CIDR strings, ignoring blanks. A bad entry is
// returned as an error rather than skipped: silently dropping one would
// quietly widen or narrow an access rule.
func ParsePrefixes(cidrs []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		p, err := netip.ParsePrefix(c)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// DefaultTrustedProxies is used when TRUSTED_PROXIES is unset.
func DefaultTrustedProxies() []netip.Prefix {
	return append([]netip.Prefix(nil), defaultTrustedProxies...)
}

// IsTailnetAddr reports whether an address belongs to a Tailscale peer.
func IsTailnetAddr(ip netip.Addr) bool {
	return inPrefixes(ip, tailscalePrefixes)
}

func inPrefixes(ip netip.Addr, prefixes []netip.Prefix) bool {
	ip = ip.Unmap()
	for _, p := range prefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// isTrustedProxy reports whether an address may speak for someone else.
//
// A tailnet address never can, whatever TRUSTED_PROXIES says. Those are the
// addresses the access rule authenticates; letting one of them also be a
// trusted forwarder would close the loop an attacker needs.
func isTrustedProxy(ip netip.Addr, trusted []netip.Prefix) bool {
	if IsTailnetAddr(ip) {
		return false
	}
	return inPrefixes(ip, trusted)
}

// parseAddr accepts "host:port", a bare host, or a bracketed IPv6 form.
func parseAddr(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netip.Addr{}, false
	}
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().Unmap(), true
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		if a, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
			return a.Unmap(), true
		}
	}
	if a, err := netip.ParseAddr(strings.Trim(s, "[]")); err == nil {
		return a.Unmap(), true
	}
	return netip.Addr{}, false
}

// ClientIP returns the address the request really came from.
//
// ok is false when it cannot be established — the peer is one of our proxies
// and the forwarded chain holds nothing but more of our proxies, or is missing
// entirely, which is what a misconfigured proxy looks like.
func ClientIP(r *http.Request, trusted []netip.Prefix) (netip.Addr, bool) {
	peer, ok := parseAddr(r.RemoteAddr)
	if !ok {
		return netip.Addr{}, false
	}
	// Direct connection: the peer *is* the client, and any forwarding header
	// it sent is its own invention.
	if !isTrustedProxy(peer, trusted) {
		return peer, true
	}

	chain := forwardedChain(r)
	for i := len(chain) - 1; i >= 0; i-- {
		if !isTrustedProxy(chain[i], trusted) {
			return chain[i], true
		}
	}

	// Nothing was forwarded. For loopback that is not ambiguity — it means
	// nobody forwarded anything, so the caller really is on this machine, and
	// loopback cannot be reached from off-box. (Tailscale Serve also proxies to
	// loopback, but it sets X-Forwarded-For, so it is handled by the walk above.)
	if peer.IsLoopback() && len(chain) == 0 {
		return peer, true
	}
	// For any other proxy address this is a genuine unknown: it could be a
	// proxy that was never told to forward the client, or a caller inside the
	// private network. Saying "unknown" is the only honest answer.
	return netip.Addr{}, false
}

// forwardedChain parses X-Forwarded-For left to right, dropping unparsable
// entries. A caller can put junk there; it must not break the walk.
func forwardedChain(r *http.Request) []netip.Addr {
	var out []netip.Addr
	for _, header := range r.Header.Values("X-Forwarded-For") {
		for _, part := range strings.Split(header, ",") {
			if a, ok := parseAddr(part); ok {
				out = append(out, a)
			}
		}
	}
	return out
}
