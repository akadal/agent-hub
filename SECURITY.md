# Security Policy

Agent Hub opens interactive shells on machines you own and stores the
credentials used to reach them. Please treat findings here as you would in any
remote-access tool.

## Reporting a vulnerability

**Do not open a public issue for a security problem.**

Use GitHub's private reporting: go to the repository's **Security** tab →
**Report a vulnerability**. That opens a private advisory visible only to you
and the maintainers.

Please include:

- what an attacker can do, and what access they need to start
- the affected version (the sidebar and `GET /health` report it)
- steps to reproduce, ideally against the Compose demo stack
- logs or a diff if you already have a fix in mind

You should get a first response within a week. If a report is confirmed, the
fix and an advisory land together, and the reporter is credited unless they ask
otherwise.

## Supported versions

| Version | Supported |
|---------|-----------|
| 1.0.x   | ✅ |
| < 1.0   | ❌ — pre-release snapshots, please upgrade |

Agent Hub is a single-branch project: fixes go to `main` and into the next
release. There are no backport branches.

## Already known, not a vulnerability report

These are documented trade-offs, tracked in [`docs/debt.md`](docs/debt.md).
Reports that restate them are welcome as *design* discussion in an issue, but
they are not undisclosed flaws:

- **SSH credentials are encrypted at rest, with a key the process can read.**
  Passwords, private keys and key passphrases are sealed with AES-256-GCM
  inside `store.json`, so the store file alone is useless. The bridge has to
  open them unattended, so the key is reachable by the API: by default it is
  `credential.key` in the same `0700` data directory. Set `CREDENTIAL_KEY` to
  keep it elsewhere. Anyone who can read *both* holds your fleet's credentials.
- **Host keys are pinned trust-on-first-use** (ADR-009). A machine's first
  connect is trusted, later key changes are refused. An attacker already in
  position at registration time is not caught by TOFU.
- **The network policy setting records intent, not enforcement** (D-004). TLS,
  IP allowlists and mesh membership are the operator's edge to secure.
- **The demo credentials in `.env.example` are public.** They exist so
  `docker compose up` works with no setup. The API warns loudly at startup
  while a deployment still uses them.
- **Failed logins are rate-limited per account, not per IP.** Behind a reverse
  proxy every request shares the proxy's address, so an address-keyed limiter
  would lock out the whole instance. Broad request-rate limiting belongs at
  your edge.

## Hardening a deployment

- Set `JWT_SECRET` and `BOOTSTRAP_ADMIN_PASSWORD` before exposing the instance;
  change the admin password from **Settings → Change password** afterwards.
- Set `JWT_ACCESS_TTL` to a real duration if you do not need never-expiring
  sessions.
- Keep the stack on a private network (Tailscale, WireGuard, LAN) and terminate
  TLS at a reverse proxy. Do not publish the API port to the internet.
- Back up `DATA_DIR` as you would a password store, and restrict who can read
  the host filesystem or the Docker volume.
- Prefer SSH **keys** over passwords for registered machines, and give the hub
  its own account on each target rather than reusing a personal one.
