// Command sshdiag opens one SSH session exactly the way the terminal bridge
// does, then prints either "open OK" or the classified failure as JSON.
//
// Run it on the host that runs the API — the whole point is to reproduce what
// the hub sees, from where the hub sees it, without going through the UI.
//
//	go run ./cmd/sshdiag 100.64.0.10 22 opsuser
//
// The SSH password is read from AGENT_HUB_SSH_PASSWORD so it does not show up
// in the process list. Tailscale SSH targets ignore it entirely.
//
// On success it prints the host key fingerprint, so you can read a host's
// identity before registering it and compare it with `ssh-keyscan` after.
// Set AGENT_HUB_SSH_HOST_KEY to check against an expected fingerprint.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/akadal/agent-hub/backend/internal/sshterm"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("usage: sshdiag <address> <port> <user>   (password via AGENT_HUB_SSH_PASSWORD)")
		os.Exit(2)
	}
	port, err := strconv.Atoi(os.Args[2])
	if err != nil {
		fmt.Println("invalid port:", os.Args[2])
		os.Exit(2)
	}
	t := sshterm.Target{
		Address:       os.Args[1],
		Port:          port,
		User:          os.Args[3],
		Password:      os.Getenv("AGENT_HUB_SSH_PASSWORD"),
		KeyPassphrase: os.Getenv("AGENT_HUB_SSH_KEY_PASSPHRASE"),
		HostKey:       os.Getenv("AGENT_HUB_SSH_HOST_KEY"),
	}
	if path := os.Getenv("AGENT_HUB_SSH_KEY_FILE"); path != "" {
		key, err := os.ReadFile(path)
		if err != nil {
			fmt.Println("cannot read key file:", err)
			os.Exit(2)
		}
		t.PrivateKey = string(key)
	}

	sess, err := sshterm.OpenSession(t, "", 80, 24)
	if err == nil {
		defer sess.Close()
		fmt.Println("RESULT: open OK")
		// Printing it makes this the tool for reading a host's fingerprint
		// before registering the machine, and for confirming one afterwards.
		fmt.Println("host key:", sess.HostKey())
		return
	}

	fmt.Println("RESULT: open FAILED")
	fmt.Println("raw error:", err)
	var oe *sshterm.OpenError
	if errors.As(err, &oe) {
		b, _ := json.MarshalIndent(oe.Failure, "", "  ")
		fmt.Println("classified:", string(b))
		return
	}
	fmt.Println("classified: <none>")
}
