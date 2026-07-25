package sshterm

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"regexp"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Target describes how to reach a remote machine over SSH.
type Target struct {
	Address  string
	Port     int
	User     string
	Password string
}

// dialTimeout is only for the initial TCP connect, not session lifetime.
const dialTimeout = 12 * time.Second

// openTimeout caps the whole OpenSession path (dial + auth + pty + shell start).
// Applied as a deadline on the underlying TCP conn so hung Tailscale/sshd
// handshakes unblock instead of sitting until an outer timer fires.
const openTimeout = 25 * time.Second

// keepaliveInterval sends SSH-level keepalives so idle shells are not dropped.
const keepaliveInterval = 30 * time.Second

var safeSessionName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func (t Target) normalized() Target {
	if t.Port <= 0 {
		t.Port = 22
	}
	if t.User == "" {
		t.User = "root"
	}
	return t
}

func (t Target) addr() string {
	t = t.normalized()
	return net.JoinHostPort(t.Address, fmt.Sprintf("%d", t.Port))
}

func clientConfig(t Target) *ssh.ClientConfig {
	t = t.normalized()
	pass := t.Password
	return &ssh.ClientConfig{
		User: t.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
			// Tailscale SSH and some sshd configs prefer keyboard-interactive.
			ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
				_ = user
				_ = instruction
				_ = echos
				answers := make([]string, len(questions))
				for i := range questions {
					answers[i] = pass
				}
				return answers, nil
			}),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         dialTimeout,
	}
}

// Dial opens an authenticated SSH client with TCP + SSH keepalives.
func Dial(t Target) (*ssh.Client, error) {
	client, _, err := dialRaw(t, time.Time{})
	return client, err
}

// dialRaw dials and authenticates. If openDeadline is non-zero, it is set on the
// TCP connection for the handshake (and cleared before return on success).
// The raw conn is returned so callers can keep a deadline during PTY/start.
func dialRaw(t Target, openDeadline time.Time) (*ssh.Client, net.Conn, error) {
	t = t.normalized()
	addr := t.addr()

	raw, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return nil, nil, fmt.Errorf("%w%s", err, tailscaleHint(t.Address))
	}
	if tcp, ok := raw.(*net.TCPConn); ok {
		_ = tcp.SetKeepAlive(true)
		_ = tcp.SetKeepAlivePeriod(keepaliveInterval)
	}
	if !openDeadline.IsZero() {
		_ = raw.SetDeadline(openDeadline)
	}

	cc, chans, reqs, err := ssh.NewClientConn(raw, addr, clientConfig(t))
	if err != nil {
		_ = raw.Close()
		return nil, nil, fmt.Errorf("%w%s", err, tailscaleHint(t.Address))
	}
	return ssh.NewClient(cc, chans, reqs), raw, nil
}

// tailscaleHint appends actionable text when dialing CGNAT / Tailscale ranges
// from a network that cannot complete the handshake (common: Docker Desktop
// bridge → 100.x, or a Coolify host that is not on the tailnet).
func tailscaleHint(address string) string {
	ip := net.ParseIP(address)
	if ip == nil {
		return ""
	}
	// Tailscale / CGNAT: 100.64.0.0/10
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return " (Tailscale IP: the Agent Hub *API process* must route to the tailnet — " +
				"Docker bridge often cannot. Run API on the host with scripts/run-api-host.sh " +
				"and docker-compose.host-api.yml, or install Tailscale on the API host)"
		}
	}
	return ""
}

// ExecResult is the outcome of a one-shot remote command.
type ExecResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// RunCommand executes cmd on the remote host and returns combined streams + exit code.
func RunCommand(t Target, cmd string) (ExecResult, error) {
	client, err := Dial(t)
	if err != nil {
		return ExecResult{}, fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return ExecResult{}, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	runErr := session.Run(cmd)
	res := ExecResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if runErr != nil {
		if ee, ok := runErr.(*ssh.ExitError); ok {
			res.ExitCode = ee.ExitStatus()
			return res, nil
		}
		return res, runErr
	}
	res.ExitCode = 0
	return res, nil
}

// Session is an interactive SSH PTY session with background keepalives.
type Session struct {
	client  *ssh.Client
	session *ssh.Session
	stdin   io.WriteCloser
	stdout  io.Reader
	raw     net.Conn

	stopOnce sync.Once
	stopKA   chan struct{}
}

// OpenSession starts an interactive shell with a PTY and keepalives.
// If remoteSession is non-empty, attaches/creates a tmux session of that name
// so reconnects (other device/tab) resume the same shell.
//
// The whole open path is bounded by openTimeout via TCP deadline so a hung
// remote never leaves the WebSocket bridge stuck before the first "ready" frame.
func OpenSession(t Target, remoteSession string, cols, rows int) (*Session, error) {
	t = t.normalized()
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	deadline := time.Now().Add(openTimeout)

	client, raw, err := dialRaw(t, deadline)
	if err != nil {
		return nil, fmt.Errorf("ssh dial: %w", err)
	}
	// Always clear the open-phase deadline once we leave (success or fail).
	clearDeadline := func() { _ = raw.SetDeadline(time.Time{}) }

	sess, err := client.NewSession()
	if err != nil {
		clearDeadline()
		client.Close()
		return nil, fmt.Errorf("ssh session: %w", err)
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.ECHOCTL:       0,
		ssh.IUTF8:         1, // terminal input is UTF-8 (Turkish and other wide sets)
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	// RequestPty(term, height, width, modes) — note h,w order.
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		sess.Close()
		clearDeadline()
		client.Close()
		return nil, fmt.Errorf("request pty: %w", err)
	}
	// Best-effort: many sshd configs reject Setenv; shell wrapper below is the real fix.
	_ = sess.Setenv("LANG", "C.UTF-8")
	_ = sess.Setenv("LC_ALL", "C.UTF-8")
	_ = sess.Setenv("LC_CTYPE", "C.UTF-8")
	_ = sess.Setenv("TERM", "xterm-256color")

	stdin, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		clearDeadline()
		client.Close()
		return nil, err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		clearDeadline()
		client.Close()
		return nil, err
	}

	// Prefer durable tmux when available; plain login shell otherwise.
	// See remoteShellCmd: macOS SSH often lacks Homebrew on PATH.
	if err := sess.Start(remoteShellCmd(remoteSession)); err != nil {
		sess.Close()
		clearDeadline()
		client.Close()
		return nil, fmt.Errorf("start shell: %w", err)
	}

	// Open succeeded — remove deadline so the interactive session can live forever.
	clearDeadline()

	s := &Session{
		client:  client,
		session: sess,
		stdin:   stdin,
		stdout:  stdout,
		raw:     raw,
		stopKA:  make(chan struct{}),
	}
	go s.keepaliveLoop()
	return s, nil
}

func (s *Session) keepaliveLoop() {
	t := time.NewTicker(keepaliveInterval)
	defer t.Stop()
	for {
		select {
		case <-s.stopKA:
			return
		case <-t.C:
			_, _, err := s.client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				return
			}
		}
	}
}

// Stdin returns the remote shell stdin.
func (s *Session) Stdin() io.WriteCloser { return s.stdin }

// Stdout returns the remote shell stdout.
func (s *Session) Stdout() io.Reader { return s.stdout }

// Resize updates the remote PTY size.
func (s *Session) Resize(cols, rows int) error {
	return s.session.WindowChange(rows, cols)
}

// Close tears down the SSH client link (tmux session keeps running remotely).
func (s *Session) Close() error {
	s.stopOnce.Do(func() {
		if s.stopKA != nil {
			close(s.stopKA)
		}
	})
	var err error
	if s.session != nil {
		err = s.session.Close()
	}
	if s.client != nil {
		if e := s.client.Close(); e != nil && err == nil {
			err = e
		}
	}
	return err
}

// Wait blocks until the remote shell exits.
func (s *Session) Wait() error {
	return s.session.Wait()
}

// KillRemoteSession destroys the tmux session on the host (when user closes terminal).
func KillRemoteSession(t Target, remoteSession string) error {
	if remoteSession == "" || !safeSessionName.MatchString(remoteSession) {
		return nil
	}
	// Same PATH hint as interactive sessions (Homebrew on macOS).
	cmd := `export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:$PATH"; ` +
		fmt.Sprintf("tmux kill-session -t %s 2>/dev/null || true", remoteSession)
	_, err := RunCommand(t, cmd)
	return err
}

// remoteShellCmd is the remote PTY command: UTF-8 locale, Homebrew-aware PATH,
// optional durable tmux (-A attach-or-create, -u UTF-8), else login shell.
// Non-interactive SSH skips ~/.zprofile, so brew's /opt/homebrew/bin is often missing
// even when `brew install tmux` succeeded in a local Terminal window.
func remoteShellCmd(remoteSession string) string {
	// Force UTF-8 so Turkish and other multi-byte chars survive (C/POSIX locales mangle them).
	// -i forces interactive (prompt + rc files) even when SSH runs a command string
	// instead of a pure login shell — avoids a black PTY with only a cursor.
	const prefix = `export LANG="${LANG:-C.UTF-8}" LC_ALL="${LC_ALL:-C.UTF-8}" LC_CTYPE="${LC_CTYPE:-C.UTF-8}"; ` +
		`export TERM="${TERM:-xterm-256color}"; ` +
		`export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:$PATH"; `
	const shell = `exec ${SHELL:-/bin/bash} -il`
	if remoteSession == "" || !safeSessionName.MatchString(remoteSession) {
		return prefix + shell
	}
	// Resolve tmux without printing "command not found" into the PTY.
	return prefix + fmt.Sprintf(
		`TMUX_BIN="$(command -v tmux 2>/dev/null || true)"; `+
			`if [ -z "$TMUX_BIN" ] && [ -x /opt/homebrew/bin/tmux ]; then TMUX_BIN=/opt/homebrew/bin/tmux; fi; `+
			`if [ -z "$TMUX_BIN" ] && [ -x /usr/local/bin/tmux ]; then TMUX_BIN=/usr/local/bin/tmux; fi; `+
			`if [ -n "$TMUX_BIN" ]; then exec "$TMUX_BIN" -u new-session -A -s %s; fi; `+
			shell,
		remoteSession,
	)
}
