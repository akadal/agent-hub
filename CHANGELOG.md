# Changelog

All notable changes to Agent Hub are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The running build reports its own version — in the web sidebar, and in
`GET /health`.

## [1.0.0] — 2026-07-25

First release. Everything below works end to end against the Compose stack and
against real SSH hosts.

### Added

- **Machines.** Register hosts by IP or hostname with an SSH user and a
  password *or* a PEM private key (encrypted keys supported). Keys never leave
  the API. Optional one-shot import of authorized devices from a Tailscale
  tailnet.
- **Workspace.** Multiple independent, named terminal sessions per machine,
  backed by tmux so a session survives a disconnect. xterm.js in the browser
  over a WebSocket→SSH bridge, plus a one-shot `exec` API.
- **Reconnect.** WebSocket and SSH keepalives with automatic re-attach after a
  network flap or a phone waking up.
- **Users and permissions.** Local accounts with admin/user roles, per-machine
  grants, and a permission matrix UI.
- **Self-service password change** (`POST /me/password`, Settings page). A
  regular user could previously never replace the password an admin issued.
- **Connection check.** Per-machine preflight that names the cause of a failure
  — unreachable, bad key, auth refused, Tailscale approval pending — instead of
  a blank terminal, and refuses to auto-retry causes a retry cannot fix.
- **SSH host key pinning**, trust-on-first-use. The first successful connect
  records the host's fingerprint; a later mismatch aborts the handshake and is
  reported as `host_key_changed`.
- **Failed-login throttle.** Ten misses per account per five minutes, then
  `429` with `Retry-After`.
- **Audit log.** Append-only record of logins, machine/user/grant changes,
  password changes and terminal use, with an operator UI. Credentials are never
  written to it.
- **Version reporting** in `GET /health`, on the API root, in the startup log
  and in the UI sidebar.
- **Graceful shutdown** on `SIGTERM`/`SIGINT`, so `docker stop` cannot cut the
  process mid-write of the store.
- **Packaging.** Docker Compose (`api`, `web`, and a throwaway `ssh-target` for
  demos and e2e), a smoke-test script, a machine registration script, and an
  `sshdiag` CLI that reproduces the API's own diagnosis.
- **CI.** Go tests, web lint/test/build, and a full Compose end-to-end run.

### Changed

- `BOOTSTRAP_ADMIN_PASSWORD` is applied when it *changes*, not on every
  restart. Previously a restart silently reverted any password the admin had
  set in the UI; the env value remains the documented recovery path.
- The bridge classifies an SSH auth rejection as `auth_failed` (not retryable)
  where it previously reported an unexplained timeout and reconnected forever.

### Security

- Replaced `ssh.InsecureIgnoreHostKey()` with the pinning described above.
  Before this, anything that could answer on a machine's address received the
  stored credential and an interactive shell.
- Unknown usernames now pay the same bcrypt cost as known ones, so response
  timing no longer reveals which accounts exist.
- The data directory is created `0700` — it holds SSH credentials in plaintext.
- Admin mutations (user create/update/delete, machine create, machine check)
  are recorded in the audit log; the update entry names which fields changed
  and never the value.

### Known gaps

Tracked openly in [`docs/debt.md`](docs/debt.md): SSH credentials are stored
unencrypted in the data directory, the network policy setting records intent
rather than enforcing it, and Tailscale identity login is not implemented.

[1.0.0]: https://github.com/akadal/agent-hub/releases/tag/v1.0.0
