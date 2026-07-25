package sshterm

import (
	"errors"
	"net"
	"regexp"
	"strings"
	"sync"
)

// Stage marks how far the open path got before failing. It is the difference
// between "the packet never arrived" and "the packet arrived and the remote
// refused to finish the handshake" — the two need opposite fixes.
type Stage string

const (
	// StageAuthSetup is local credential preparation, before any network I/O.
	StageAuthSetup Stage = "auth_setup"
	// StageDial is the TCP connect.
	StageDial Stage = "dial"
	// StageHandshake is the SSH transport + auth exchange.
	StageHandshake Stage = "handshake"
	// StageSession is PTY request / shell start, after auth succeeded.
	StageSession Stage = "session"
)

// FailureKind is a machine-readable cause the UI can branch on.
type FailureKind string

const (
	FailUnknown FailureKind = "unknown"
	// FailUnreachable — nothing accepted the TCP connection.
	FailUnreachable FailureKind = "unreachable"
	// FailTailnetRouting — 100.x is not routable from this process (classic
	// Docker bridge → tailnet).
	FailTailnetRouting FailureKind = "tailnet_routing"
	// FailTailscaleCheck — Tailscale SSH is holding the session waiting for a
	// human to approve it in a browser ("action": "check").
	FailTailscaleCheck FailureKind = "tailscale_check_pending"
	// FailTailscaleDenied — Tailscale SSH refused the node outright; no ACL rule
	// grants this src→dst→user.
	FailTailscaleDenied FailureKind = "tailscale_denied"
	// FailAuth — ordinary sshd rejected the credentials.
	FailAuth FailureKind = "auth_failed"
	// FailBadKey — the configured private key could not be parsed at all.
	FailBadKey FailureKind = "bad_private_key"
	// FailTimeout — the remote accepted TCP then stopped responding.
	FailTimeout FailureKind = "timeout"
)

// Failure is a structured, user-actionable explanation of a failed open.
type Failure struct {
	Kind FailureKind `json:"kind"`
	// Message is the short human summary.
	Message string `json:"message"`
	// ApprovalURL is set when the remote handed us a link to click.
	ApprovalURL string `json:"approval_url,omitempty"`
	// Hint is the concrete fix.
	Hint string `json:"hint,omitempty"`
	// Retryable is false when reconnecting cannot possibly help (waiting on a
	// human, wrong credentials, wrong network topology). The bridge uses this
	// to stop reconnect storms.
	Retryable bool `json:"retryable"`
}

// OpenError carries a Failure alongside the underlying cause.
type OpenError struct {
	Failure
	Err error
}

func (e *OpenError) Error() string {
	if e.Err == nil {
		return e.Message
	}
	return e.Message + ": " + e.Err.Error()
}

func (e *OpenError) Unwrap() error { return e.Err }

// authTrace collects anything the server told us during authentication —
// banners and keyboard-interactive instructions. Tailscale SSH delivers its
// approval URL here, and dropping it is why a check-mode stall used to surface
// as an unexplained blank terminal.
type authTrace struct {
	mu    sync.Mutex
	lines []string
}

func (a *authTrace) add(s string) {
	if a == nil {
		return
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	a.mu.Lock()
	a.lines = append(a.lines, s)
	a.mu.Unlock()
}

func (a *authTrace) text() string {
	if a == nil {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return strings.Join(a.lines, "\n")
}

// tailscaleURLRe matches the approval link Tailscale SSH sends in check mode.
var tailscaleURLRe = regexp.MustCompile(`https://login\.tailscale\.com/[A-Za-z0-9/_.\-]+`)

// isTailnetAddress reports whether addr is in Tailscale's CGNAT range 100.64.0.0/10.
func isTailnetAddress(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127
}

// hasLocalTailnetAddress reports whether this process itself holds a
// 100.64.0.0/10 address, i.e. it is on the tailnet rather than behind a Docker
// bridge. This is what separates "we cannot route to the tailnet at all" from
// "we are on the tailnet and that particular peer did not answer".
//
// Indirected through a var so tests can drive both branches.
var hasLocalTailnetAddress = func() bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip != nil && isTailnetAddress(ip.String()) {
			return true
		}
	}
	return false
}

// isTailscaleSSHTarget reports whether this address:port is served by Tailscale
// SSH rather than the host's own sshd.
//
// Tailscale only intercepts port 22 on the tailnet address. Any other port on
// the same 100.x host reaches ordinary OpenSSH — so a rejection there is about
// authorized_keys, not about the tailnet ACL. Blaming the ACL for a plain key
// problem sends the operator to the wrong console entirely.
func isTailscaleSSHTarget(t Target) bool {
	return isTailnetAddress(t.Address) && t.normalized().Port == 22
}

func isTimeout(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "i/o timeout") ||
		strings.Contains(s, "context deadline exceeded") ||
		strings.Contains(s, "timed out")
}

func isAuthRejection(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unable to authenticate") ||
		strings.Contains(s, "no supported methods remain") ||
		strings.Contains(s, "permission denied")
}

const (
	hintTailscaleCheck = "Tailscale SSH is holding this session for interactive approval " +
		`("action": "check" in the tailnet policy). A headless server cannot click that link, ` +
		`so the grant lapses every checkPeriod and the connection dies. Fix: give this hub node a tag ` +
		`and add an SSH rule with "action": "accept" for it, or open the approval URL above.`

	hintTailscaleDenied = "Tailscale SSH refused this node. No ssh rule in the tailnet ACL grants " +
		"this source node access to this destination as this user. Add an ACL ssh rule " +
		`with "action": "accept", src = the hub node/tag, dst = this machine.`

	hintTailnetRouting = "This process cannot route to the tailnet. The API is on a Docker bridge " +
		"network while Tailscale runs on the host. Fix: run the api service with network_mode: host " +
		"(docker-compose.coolify.yml) or on the host directly (scripts/run-api-host.sh)."

	hintAuth = "The remote rejected the credentials. Check the SSH user and password stored for " +
		"this machine, and that sshd allows password authentication for that user " +
		"(hardened hosts often set PasswordAuthentication no — then only a key works)."

	hintAuthKey = "The remote rejected the key. Check that the matching public key is in " +
		"~/.ssh/authorized_keys for this SSH user on the target, that the file is mode 600 " +
		"and its directory 700, and that sshd allows public key authentication."

	hintBadKey = "The stored private key could not be parsed. Paste the full PEM block including " +
		"the BEGIN and END lines, and supply the passphrase if the key is encrypted."

	hintUnreachable = "Nothing is accepting SSH on that address and port. Check the machine is up, " +
		"the port is right, and any firewall allows it."

	hintTailnetPeerDown = "This process is on the tailnet, so routing is fine — the peer itself did " +
		"not answer. Check the machine is powered on and its tailscaled is connected " +
		"(`tailscale status` should not show it as offline)."

	hintTimeout = "The remote accepted the connection then stopped responding before the shell " +
		"started. Usually a loaded or half-suspended host."
)

// Diagnose turns a raw open failure into an actionable Failure. It reads the
// authentication trace first: when the remote actually told us what it wants,
// that beats any inference from the error string.
func Diagnose(stage Stage, err error, tr *authTrace, t Target) Failure {
	trace := tr.text()
	tailnet := isTailnetAddress(t.Address)

	// Strongest signal: the remote handed us an approval link.
	if url := tailscaleURLRe.FindString(trace); url != "" {
		return Failure{
			Kind:        FailTailscaleCheck,
			Message:     "Tailscale SSH is waiting for this login to be approved in a browser",
			ApprovalURL: strings.TrimRight(url, ".,)"),
			Hint:        hintTailscaleCheck,
			Retryable:   false,
		}
	}

	switch stage {
	case StageAuthSetup:
		return Failure{
			Kind:      FailBadKey,
			Message:   "the stored SSH private key is not usable",
			Hint:      hintBadKey,
			Retryable: false,
		}

	case StageDial:
		if tailnet && isTimeout(err) {
			// Both "we are off the tailnet" and "that peer is asleep" time out
			// here. Our own addresses tell them apart — without this check the
			// operator gets sent to rewrite Docker networking for a host that
			// is merely powered off.
			if !hasLocalTailnetAddress() {
				return Failure{
					Kind:      FailTailnetRouting,
					Message:   "this process is not on the tailnet, so " + t.Address + " is unroutable",
					Hint:      hintTailnetRouting,
					Retryable: false,
				}
			}
			return Failure{
				Kind:      FailUnreachable,
				Message:   "tailnet peer " + t.Address + " did not answer",
				Hint:      hintTailnetPeerDown,
				Retryable: true,
			}
		}
		return Failure{
			Kind:      FailUnreachable,
			Message:   "cannot open a TCP connection to " + t.addr(),
			Hint:      hintUnreachable,
			Retryable: true,
		}

	case StageHandshake:
		if isTimeout(err) {
			if isTailscaleSSHTarget(t) {
				// TCP completed but the SSH handshake never finished against a
				// tailnet peer: Tailscale SSH is parking the session pending
				// approval. This is the check-mode stall with no banner.
				return Failure{
					Kind:      FailTailscaleCheck,
					Message:   "Tailscale SSH accepted the connection then stalled without completing authentication",
					Hint:      hintTailscaleCheck,
					Retryable: false,
				}
			}
			return Failure{
				Kind:      FailTimeout,
				Message:   "SSH handshake timed out",
				Hint:      hintTimeout,
				Retryable: true,
			}
		}
		if isAuthRejection(err) {
			if isTailscaleSSHTarget(t) {
				return Failure{
					Kind:      FailTailscaleDenied,
					Message:   "Tailscale SSH denied this node",
					Hint:      hintTailscaleDenied,
					Retryable: false,
				}
			}
			// Which hint is right depends on what we actually offered.
			hint := hintAuth
			if strings.TrimSpace(t.PrivateKey) != "" {
				hint = hintAuthKey
			}
			return Failure{
				Kind:      FailAuth,
				Message:   "authentication failed for " + t.normalized().User,
				Hint:      hint,
				Retryable: false,
			}
		}
	}

	return Failure{
		Kind:      FailUnknown,
		Message:   "ssh open failed",
		Retryable: true,
	}
}
