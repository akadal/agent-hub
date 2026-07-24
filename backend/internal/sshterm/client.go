package sshterm

import (
	"bytes"
	"fmt"
	"io"
	"net"
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
const dialTimeout = 15 * time.Second

// keepaliveInterval sends SSH-level keepalives so idle shells are not dropped.
const keepaliveInterval = 30 * time.Second

// Dial opens an authenticated SSH client with TCP + SSH keepalives.
// The client does not impose an idle timeout; sessions stay up until closed.
func Dial(t Target) (*ssh.Client, error) {
	if t.Port <= 0 {
		t.Port = 22
	}
	if t.User == "" {
		t.User = "root"
	}
	cfg := &ssh.ClientConfig{
		User: t.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(t.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // local e2e / operator-managed hosts
		Timeout:         dialTimeout,
		// No ClientConfig field for session idle timeout — we keep the connection alive.
	}
	addr := net.JoinHostPort(t.Address, fmt.Sprintf("%d", t.Port))

	raw, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return nil, err
	}
	if tcp, ok := raw.(*net.TCPConn); ok {
		_ = tcp.SetKeepAlive(true)
		_ = tcp.SetKeepAlivePeriod(keepaliveInterval)
	}

	cc, chans, reqs, err := ssh.NewClientConn(raw, addr, cfg)
	if err != nil {
		_ = raw.Close()
		return nil, err
	}
	return ssh.NewClient(cc, chans, reqs), nil
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

	stopOnce sync.Once
	stopKA   chan struct{}
}

// OpenSession starts an interactive shell with a PTY and keepalives.
func OpenSession(t Target, cols, rows int) (*Session, error) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	client, err := Dial(t)
	if err != nil {
		return nil, fmt.Errorf("ssh dial: %w", err)
	}
	sess, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("ssh session: %w", err)
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		sess.Close()
		client.Close()
		return nil, fmt.Errorf("request pty: %w", err)
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		client.Close()
		return nil, err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		client.Close()
		return nil, err
	}
	if err := sess.Shell(); err != nil {
		sess.Close()
		client.Close()
		return nil, fmt.Errorf("start shell: %w", err)
	}

	s := &Session{
		client:  client,
		session: sess,
		stdin:   stdin,
		stdout:  stdout,
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
			// OpenSSH-compatible keepalive; prevents idle NAT/firewall drops.
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

// Close tears down the session and client.
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
