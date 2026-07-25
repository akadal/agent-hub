# Changelog

All notable changes to Agent Hub are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The running build reports its own version — in the web sidebar, and in
`GET /health`.

## [1.1.0] — 2026-07-26

### Added

- **Light / dark / system theme.** A three-way appearance switch in the sidebar
  and Settings. `System` follows the OS and keeps following it. The terminal
  palette follows the app theme too — the light palette is a darkened ANSI set,
  because the default one is built for a black background.
- **Mobile terminal.** On phones the machines/sessions picker moved into a
  sheet, a session strip (with `+`) is always visible, and the app header
  collapses to one line on the workspace route. The shell now gets close to the
  whole screen instead of about ten lines.
- **Soft keys.** Esc, Tab, a sticky Ctrl, `^C`, `^D`, `^Z`, `^L`, `^R`, arrows
  and shell punctuation, above the safe area. Sticky Ctrl folds the next key
  typed on the OS keyboard into its control code, so `Ctrl+R` works on a
  keyboard that has no Ctrl. On by default for touch input.
- **Terminal text size** control, per device, in the workspace picker.
- **Stream view is back.** The readable feed behind the Classic/Stream toggle
  had been a "Coming soon" placeholder since it froze new sessions. Switching
  to it seeds from the existing scrollback so the feed is not blank.
- **Pick-your-machines Tailscale import.** The device list is now a checklist
  instead of "import everything online", an offline device you tick is honoured,
  and credentials are opt-in — a tailnet host on port 22 is authorised by
  Tailscale ACL and ignores anything stored here.
- **Audit export.** `GET /audit/export` and an Export CSV button return every
  retained event. Cells that start `=`, `+`, `-` or `@` are escaped so a crafted
  username cannot run as a formula in a spreadsheet.
- **`AUDIT_MAX_EVENTS`** to raise or lower how much audit history is kept.
- **`CREDENTIAL_KEY`** to supply the credential encryption key from outside the
  data directory.
- **Tailnet-only access** (Settings). Off by default. When on, the API refuses
  any request whose client address is not in Tailscale's ranges — in front of
  login, so an outside caller cannot even try a password. iPhones and other
  devices get in over the tailnet as usual. It refuses to be switched on from
  an address that would be blocked, loopback and `/health` keep working, and
  `ACCESS_ENFORCEMENT=off` is the recovery path. This is documented as the
  *second* lock; the one that holds is serving the app on the tailnet and not
  publishing the port at all (`docs/ops.md` §5d). Closes D-004.
- **`TRUSTED_PROXIES`** so the API can identify the real client behind a
  reverse proxy without trusting a caller-supplied header. Unset, it defaults
  to loopback plus the private ranges. Tailscale's own ranges are never trusted
  as proxies.

### Changed

- **SSH credentials are encrypted at rest.** Passwords, private keys and key
  passphrases are sealed with AES-256-GCM before they reach `store.json`. The
  key comes from `CREDENTIAL_KEY`, or from a `credential.key` file generated in
  the data directory on first start. Existing plaintext stores are read and
  re-sealed automatically. (D-009)
- **Machines registered before ownership existed** are adopted by the bootstrap
  admin on startup. They used to be readable by every account. (D-011)
- The web image and CI install with `npm ci`. The lockfile was regenerated on
  linux so it no longer omits linux-only optional packages. (D-002)

### Fixed

- The stream reader no longer grows one unbounded string when a program prints
  megabytes without a newline, and drops a truncated escape sequence instead of
  re-parsing it into every following chunk. This is what froze new sessions and
  got the view disabled in the first place.
- tmux's status bar no longer collects in the stream reader once a minute as
  its clock ticks.
- Soft keys fire on `pointerdown`, so they work on touch devices — preventing
  the default on `touchstart` also cancels the click that used to trigger them.

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
unencrypted in the data directory (fixed in 1.1.0), the network policy setting
records intent rather than enforcing it, and Tailscale identity login is not
implemented.

[1.1.0]: https://github.com/akadal/agent-hub/releases/tag/v1.1.0
[1.0.0]: https://github.com/akadal/agent-hub/releases/tag/v1.0.0
