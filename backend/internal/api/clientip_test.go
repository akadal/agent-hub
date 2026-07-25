package api

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func req(remote string, xff ...string) *http.Request {
	r := httptest.NewRequest("GET", "/api/machines", nil)
	r.RemoteAddr = remote
	for _, v := range xff {
		r.Header.Add("X-Forwarded-For", v)
	}
	return r
}

func mustClientIP(t *testing.T, r *http.Request, trusted []netip.Prefix) string {
	t.Helper()
	ip, ok := ClientIP(r, trusted)
	if !ok {
		t.Fatalf("client IP could not be determined for %s / %v", r.RemoteAddr, r.Header.Values("X-Forwarded-For"))
	}
	return ip.String()
}

func TestClientIPWithNoProxyInFront(t *testing.T) {
	trusted := DefaultTrustedProxies()

	if got := mustClientIP(t, req("100.64.0.7:51234"), trusted); got != "100.64.0.7" {
		t.Fatalf("got %s, want the peer itself", got)
	}

	// A direct caller inventing a header must not be believed.
	got := mustClientIP(t, req("203.0.113.9:443", "100.64.0.5"), trusted)
	if got != "203.0.113.9" {
		t.Fatalf("got %s — a direct peer's own X-Forwarded-For was trusted", got)
	}
}

func TestClientIPThroughOneProxy(t *testing.T) {
	trusted := DefaultTrustedProxies()
	// nginx on the docker bridge appended the client it saw.
	got := mustClientIP(t, req("172.18.0.4:38000", "100.64.0.7"), trusted)
	if got != "100.64.0.7" {
		t.Fatalf("got %s, want the forwarded client", got)
	}
}

func TestClientIPThroughTwoProxies(t *testing.T) {
	trusted := DefaultTrustedProxies()
	// Coolify proxy → nginx → api. Each appended its own peer.
	got := mustClientIP(t, req("172.18.0.4:38000", "100.64.0.7, 172.18.0.9"), trusted)
	if got != "100.64.0.7" {
		t.Fatalf("got %s, want the real client behind both proxies", got)
	}
}

func TestSpoofedForwardedHeaderIsIgnored(t *testing.T) {
	trusted := DefaultTrustedProxies()

	// The caller sent "X-Forwarded-For: 100.64.0.5" from the public internet.
	// Every real proxy appended what it actually saw, so the invented entry is
	// leftmost — the walk from the right must never reach it.
	got := mustClientIP(t, req("172.18.0.4:38000", "100.64.0.5, 203.0.113.9, 172.18.0.9"), trusted)
	if got == "100.64.0.5" {
		t.Fatal("spoofed X-Forwarded-For entry was accepted as the client — the access rule is bypassable")
	}
	if got != "203.0.113.9" {
		t.Fatalf("got %s, want the public address the outermost proxy actually saw", got)
	}
}

func TestATailnetAddressIsNeverTreatedAsAProxy(t *testing.T) {
	// Even if an operator lists Tailscale's range as a trusted proxy — say by
	// writing fc00::/7, which contains it — a tailnet peer must not get to
	// name its own address, or the rule authenticates the attacker's claim.
	trusted, err := ParsePrefixes([]string{"127.0.0.0/8", "100.64.0.0/10", "fc00::/7"})
	if err != nil {
		t.Fatal(err)
	}

	got := mustClientIP(t, req("100.64.0.9:40000", "100.64.0.5"), trusted)
	if got != "100.64.0.9" {
		t.Fatalf("got %s, want the tailnet peer itself, not what it claimed", got)
	}

	got = mustClientIP(t, req("[fd7a:115c:a1e0::1]:40000", "100.64.0.5"), trusted)
	if got != "fd7a:115c:a1e0::1" {
		t.Fatalf("got %s, want the IPv6 tailnet peer itself", got)
	}
}

func TestClientIPIsUnknownWhenTheProxyForwardsNothing(t *testing.T) {
	trusted := DefaultTrustedProxies()
	// A proxy that was never configured to forward the client address. There
	// is no honest answer here, and guessing would mean allowing the world.
	if ip, ok := ClientIP(req("172.18.0.4:38000"), trusted); ok {
		t.Fatalf("got %s — an unidentifiable caller must be reported as unknown", ip)
	}
	// Same when the chain is nothing but our own proxies.
	if ip, ok := ClientIP(req("172.18.0.4:38000", "10.0.0.3, 172.18.0.9"), trusted); ok {
		t.Fatalf("got %s — a chain of only trusted proxies identifies nobody", ip)
	}
}

func TestClientIPSurvivesJunkInTheHeader(t *testing.T) {
	trusted := DefaultTrustedProxies()
	got := mustClientIP(t, req("172.18.0.4:38000", "not-an-ip, 100.64.0.7, unknown"), trusted)
	if got != "100.64.0.7" {
		t.Fatalf("got %s, want the one parsable client entry", got)
	}
}

func TestClientIPReadsRepeatedHeaders(t *testing.T) {
	trusted := DefaultTrustedProxies()
	// Some proxies add a second header rather than extending the first.
	got := mustClientIP(t, req("172.18.0.4:38000", "100.64.0.7", "172.18.0.9"), trusted)
	if got != "100.64.0.7" {
		t.Fatalf("got %s, want the client across both header lines", got)
	}
}

func TestTailscaleAddressRecognition(t *testing.T) {
	for _, s := range []string{"100.64.0.1", "100.100.100.100", "100.127.255.254", "fd7a:115c:a1e0::1"} {
		if !IsTailnetAddr(netip.MustParseAddr(s)) {
			t.Fatalf("%s should count as a tailnet address", s)
		}
	}
	for _, s := range []string{"100.63.255.255", "100.128.0.1", "10.0.0.5", "203.0.113.9", "127.0.0.1", "fd00::1"} {
		if IsTailnetAddr(netip.MustParseAddr(s)) {
			t.Fatalf("%s must not count as a tailnet address", s)
		}
	}
}

func TestParsePrefixesRejectsBadInput(t *testing.T) {
	if _, err := ParsePrefixes([]string{"10.0.0.0/8", "nonsense"}); err == nil {
		t.Fatal("a malformed CIDR must be an error, not a silently dropped rule")
	}
	got, err := ParsePrefixes([]string{" 10.0.0.0/8 ", "", "  "})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d prefixes, want 1", len(got))
	}
}
