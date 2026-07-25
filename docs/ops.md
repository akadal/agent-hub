# Ops & Runbook — Agent Hub

Operational notes for local run, self-hosted deploy, and day-2. Product scope: `docs/prd.md`. Architecture: `docs/architecture.md`.

**Public-repo rule:** Use placeholders (`your-host.example`, env vars). Do not commit private hostnames, personal domains, or real secrets.

---

## 1. Environments

| Mode | Intent |
|------|--------|
| **Local** | Primary dev and single-operator use via Docker Compose |
| **Self-hosted deploy** | Same Compose stack on a VPS, home server, or any container host—required if you want phone/off-LAN access |
| **CI deploy** | Optional; operators wire their own pipeline if they want commit → ship |

| Item | Default / note |
|------|----------------|
| Packaging | Docker Compose |
| Network default | Tailscale / private-mesh only (configurable in app Settings) |
| TLS / reverse proxy | Operator-provided when exposing beyond localhost |

No specific PaaS or control panel is required. Coolify, Traefik, Caddy, nginx, cloud load balancers, etc. are examples operators may choose.

---

## 2. Local run (working e2e)

1. Clone the repository.
2. Optional: `cp .env.example .env` — bootstrap admin defaults to **admin / 123456** (change in real deploys).
3. From repo root: `docker compose up --build`
4. Open **http://localhost:27342** → sign in with bootstrap admin.
5. Register a machine:
   - **Address:** `ssh-target` (Compose DNS; API reaches dummy SSH on the same network)
   - **Port:** `27343`
   - **SSH user / password:** `root` / `targetpass`
6. Use **Workspace** → new sessions (xterm.js over WebSocket).

| Service | Compose name | Host / container port | Notes |
|---------|--------------|------------------------|--------|
| API | `api` | host **27341** → container **27341** | JWT; machines; SSH |
| Web | `web` | host **27342** → container **80** | SPA; Coolify must target **80** |
| Dummy SSH | `ssh-target` | host **27343** → container **27343** | e2e (`root`/`targetpass`) |

**Coolify checklist (critical)**

| Service | Public domain | Port |
|---------|---------------|------|
| **web** | `https://your.host` **only this one** | **80** |
| **api** | **none** (internal) | 27341 |
| **ssh-target** | **none** (internal) | 27343 |

If **both** web and api have the same domain (`agents.example`), Coolify may send **all** traffic to **api**. Then `/` is a Go 404 (no SPA), while `/api/hello` still works. Fix: **remove domain from api**, keep it only on **web**. Web nginx proxies `/api` → `api:27341` on the Docker network.

- Do **not** set `VITE_API_BASE_URL` to localhost  
- Credentials: `BOOTSTRAP_ADMIN_*` on **api** service, re-applied every restart  
- After env change: restart/redeploy **api**  
- ssh-target: no public domain

**Remote channel:** SSH to registered IP/hostname. The **API process** (not your laptop browser) must be able to open TCP+SSH to that address.

### Tailscale `100.x` from Docker (important)

| Where API runs | SSH to `100.x` |
|----------------|----------------|
| Bridge container (`10.0.10.x` / `172.x`) | **Broken** — `handshake … i/o timeout` even if VPS has Tailscale |
| API **host network** (Coolify) | **Works** — outbound via host `tailscale0` (e.g. `100.72.x`) |
| API on host process | **Works** if host is on the tailnet |
| Docker Desktop macOS bridge | Often broken — use `docker-compose.host-api.yml` |

**Symptom (Coolify):**  
`ssh dial: handshake failed: read tcp 10.0.10.4:…->100.118.x.x:22: i/o timeout`  
Host has Tailscale and `tailscale status` shows both VPS + target **active**, but the **container** is not on the tailnet.

**Coolify fix (Linux VPS + Tailscale on host):**

1. Install/login Tailscale on the **VPS** (not only on your laptop). Confirm: `tailscale status` lists target machines.
2. Deploy with host-network overlay so API uses the host stack:

```bash
# In the Coolify compose / git deploy, use both files:
docker compose -f docker-compose.yml -f docker-compose.coolify.yml up -d --build
```

Or in Coolify UI for the **api** service only: **Network mode = host** (same idea). Keep domain only on **web**; set web `API_UPSTREAM=host.docker.internal:27341` (or the host gateway).

3. Redeploy. From VPS: `curl -sS http://127.0.0.1:27341/health` and open a terminal to a `100.x` machine.

**Local Mac + Tailscale machines:**

```bash
docker compose -f docker-compose.yml -f docker-compose.host-api.yml up -d --build web ssh-target
./scripts/run-api-host.sh   # terminal 2 — uses ./data, listens :27341
# UI: http://localhost:27342  (or WEB_PORT)
```

**Why it “worked yesterday”:** older deploy may have used host networking / different Coolify network settings; a rebuild onto a pure bridge network (`10.0.10.0/24`) drops Tailscale routing for the API container.

**Optional — Import from Tailscale (Machines UI)**

1. Create an **API access token** (Admin → Settings → Keys), not a node auth key.
2. On the **api** service set:
   - `TAILSCALE_API_KEY=tskey-api-…`
   - `TAILSCALE_TAILNET=-` (default tailnet for the key)
3. Redeploy/restart api. Open **Machines** → **Import from Tailscale** → enter shared SSH user/password/port → add devices.
4. Import uses each device’s Tailscale IPv4 (`100.x`) as address. API host must still be able to **route SSH** to those IPs (typically: api also on the tailnet).
5. Re-import is safe: already-registered addresses are skipped.

Automated smoke:

```bash
API_BASE=http://localhost:27341 FE_BASE=http://localhost:27342 E2E_SSH_PORT=27343 ./scripts/e2e-smoke.sh
```

Local dev without Compose:

```bash
cd backend && go run ./cmd/agent-hub
cd web && npm install && npm run dev   # proxies /api and /health to :27341
# Point a machine at a real SSH host or map ssh-target via host port 27343
```

---

## 3. Deploy (generic)

1. Build or pull images: `docker compose build` (or your registry workflow).
2. Run the same stack on a host reachable from clients that need access (including mobile).
3. Put TLS and hostname in front via your reverse proxy / platform.
4. Restrict ingress (prefer Tailscale or VPN; open WAN only with clear AccessPolicy intent).
5. Verify `GET /health` and the web UI from a second device (e.g. phone).

Optional: hook git push to rebuild/redeploy on your platform of choice. Override ports with `API_PORT` / `WEB_PORT` env if needed.

---

## 4. Bootstrap (first install)

- Create initial admin/local user (mechanism TBD with M0.5 / M1.*).
- Confirm AccessPolicy default remains private-mesh/Tailscale-only unless intentionally opened.
- Register first machine via UI: **New device** → IP → register.
- Grant permissions before non-admin users expect terminal access.

---

## 5. Auth ops

| Mode | When |
|------|------|
| Tailscale identity | Preferred for users on the mesh |
| Username + password | Fallback / non-Tailscale paths |
| Bootstrap admin | First install and recovery |

---

## 6. Mobile access notes

- Mobile browsers need a **deployed or otherwise reachable** instance (not only the laptop’s `localhost` unless the phone shares that network path, e.g. same Tailscale).
- UI must remain usable on small screens (see PRD §9).
- Prefer private mesh access over exposing the control plane to the public internet.

### 6.1 Terminal disconnects on mobile (known cause + mitigations)

**Symptom:** xterm shows disconnect / “session closed” after the phone sleeps, switches apps, or sits idle — even when the target is the same host that runs Agent Hub.

**Why (not JWT):**

| Layer | What happens on mobile |
|-------|------------------------|
| Browser | Safari/Chrome **suspend JS timers** and often **kill idle WebSockets** when the screen locks or the tab is backgrounded. Client-only `setInterval` pings are unreliable. |
| Coolify / Traefik / edge | Idle HTTP/WS connections are closed if no traffic for the edge timeout. |
| Agent Hub API | WebSocket bridge closes → SSH client closes. **tmux on the host keeps the shell.** |
| Target sshd | Some hardened configs drop quiet SSH if client keepalives are ignored/disabled. |

**What we force in-app (no host config required for the common case):**

1. JWT default **`JWT_ACCESS_TTL=forever`** — login does not expire under long sessions.
2. SSH TCP keepalive + `keepalive@openssh.com` every **30s** from the API → host.
3. **Server-side WebSocket Ping** every **25s** (protocol frames; no client JS) + app-level JSON ping as backup.
4. nginx (`web`) `proxy_read/send_timeout 7d`, buffering off for `/api/`.
5. **Client auto-reconnect** with backoff + immediate reconnect on `visibilitychange` / `online` / `pageshow`. Reattach uses the same `remote_session` (tmux).

**Operator checklist when it still drops:**

1. Coolify: public domain only on **web:80**; do not short-timeout the service.
2. Edge idle timeout ≥ a few minutes (WS pings keep it warm while the phone is awake).
3. On the **target machine** (if sshd is strict):

```text
# /etc/ssh/sshd_config (or drop-in under sshd_config.d/)
ClientAliveInterval 30
ClientAliveCountMax 240
# then: sudo systemctl reload sshd
```

4. Install **tmux** on targets so reconnect restores the same shell (otherwise a plain login shell is used and scrollback is not durable).

---

## 6b. Tailscale SSH targets — the hub is headless

If a target runs **Tailscale SSH** (`tailscale set --ssh`, i.e. `RunSSH: true`),
Tailscale intercepts port 22 **on the tailnet address** and answers it itself.
The machine's own OpenSSH is bypassed for that address, so:

- The SSH **password stored for the machine is never used**. Authentication is
  the tailnet node identity of the hub, resolved against the tailnet ACL.
- Whether the hub can connect is decided entirely by the `ssh` section of the
  tailnet policy file.

Check what a target is running:

```bash
ssh -v <user>@<tailnet-ip> exit 2>&1 | grep "remote software version"
# "remote software version Tailscale"  -> Tailscale SSH (ACL-governed)
# "remote software version OpenSSH_x"  -> ordinary sshd (password/key)
```

### Reproduce what the hub sees

`sshdiag` opens a session through the exact same code path as the terminal
bridge and prints the classified failure — run it **on the host running the
API**, so you are testing from where the hub actually dials:

```bash
cd backend && go run ./cmd/sshdiag <address> <port> <user>
```

It takes no password argument (set `AGENT_HUB_SSH_PASSWORD` if the target needs
one). If it prints `open OK` with no password set, the target is authenticating
you by tailnet identity and the stored password is doing nothing.

### `"action": "check"` cannot work for the hub

Tailscale's **default** SSH policy is:

```jsonc
"ssh": [
  { "action": "check", "src": ["autogroup:member"], "dst": ["autogroup:self"],
    "users": ["autogroup:nonroot", "root"] },
]
```

`check` parks each new session and requires a human to approve it in a browser.
The approval is cached only for `checkPeriod` (default **12h**). Agent Hub is a
headless server: it can never click that link. So a target under a `check` rule
works right after a human approves it and then **breaks by itself hours later** —
typically overnight. In the target's log this looks like:

```
tailscaled: ssh-conn-…: failed to fetch next SSH action:
  fetch failed from https://unused/machine/ssh/wait/… : context deadline exceeded
tailscaled: ssh-conn-…: failed to send auth banner: EOF
```

`/machine/ssh/wait/` is the check-mode hold. Confirm the source with:

```bash
# on the target
sudo journalctl -u tailscaled --since "24 hours ago" \
  | grep -E "handling conn|access granted|failed to fetch next SSH action"
```

### Fix: give the hub node an `accept` rule

Edit the tailnet policy file (Tailscale admin console → **Access controls**) and
add an `accept` rule for the hub **before** the default `check` rule. Rules are
evaluated in order, so the hub stops needing approval while humans keep `check`:

```jsonc
{
  "hosts": {
    "agent-hub": "100.64.0.20",   // tailnet IP of the node running the API
  },

  "ssh": [
    // Agent Hub is headless — it cannot complete a browser approval.
    {
      "action": "accept",
      "src":    ["agent-hub"],
      "dst":    ["autogroup:self"],
      "users":  ["youruser"],       // the remote login(s) the hub uses
    },

    // Interactive humans keep check mode.
    {
      "action": "check",
      "src":    ["autogroup:member"],
      "dst":    ["autogroup:self"],
      "users":  ["autogroup:nonroot", "root"],
    },
  ],
}
```

Verify from the hub host after saving — no password should be needed:

```bash
ssh -o BatchMode=yes <user>@<tailnet-ip> 'echo ok'
```

> Do not "fix" this by putting the password back in. On a Tailscale SSH target
> the password is inert; a login that starts working after a password change was
> re-approved in a browser, not authenticated by the password.

### Alternative: key auth on a second port (no ACL edit)

Tailscale intercepts **only port 22** on the tailnet address. Any other port
reaches the host's own OpenSSH, so exposing sshd on a second port gives the hub
a path that does not depend on the tailnet ACL at all. Use this when you cannot
edit the policy file, or want the hub independent of `checkPeriod` for good.

It also covers the common case where the target sets `PasswordAuthentication no`
— there, a key is the only credential that can ever work.

On the target (Ubuntu 24.04 socket-activates sshd, so the port must be set on
the **socket unit** — `Port` in `sshd_config` alone is ignored):

```bash
# 1. authorize the hub's public key
mkdir -p ~/.ssh && chmod 700 ~/.ssh
printf '%s\n' "ssh-ed25519 AAAA... agent-hub@hub" >> ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys

# 2. add the port (keep 22 so your own Tailscale SSH keeps working)
sudo tee /etc/systemd/system/ssh.socket.d/10-agent-hub.conf >/dev/null <<'EOF'
[Socket]
ListenStream=
ListenStream=22
ListenStream=2222
EOF
sudo systemctl daemon-reload && sudo systemctl restart ssh.socket

# 3. verify BOTH ports are listening before you log out
ss -lnt | grep -E ':(22|2222)\b'
```

Then register the machine with **port 2222** and paste the private key into the
**SSH private key** field. The key is preferred over the password whenever both
are present.

Confirm the second port really is OpenSSH and not Tailscale:

```bash
ssh -p 2222 -v <user>@<tailnet-ip> exit 2>&1 | grep "remote software version"
# expect OpenSSH_x, not Tailscale
```

If `ufw` is active, tailnet traffic is usually already allowed via a
`Anywhere on tailscale0 ALLOW` rule — check before assuming you need a new one.

---

## 7. Incident checklist (draft)

1. Can the site be reached from the expected network (Tailscale / VPN / LAN)?
2. Is JWT login / Tailscale identity failing?
3. Is the target machine still registered and reachable?
4. Is tmux/session bridge healthy on that machine?
5. Check audit log for recent access and errors (when M5 ships).

**Read the terminal's error frame first.** A failed open reports a classified
cause; each one has a different fix:

| `kind` | Meaning | Fix |
|--------|---------|-----|
| `tailscale_check_pending` | Tailscale SSH is holding the session for browser approval | §6b — add an `accept` ACL rule for the hub node |
| `tailscale_denied` | No ACL rule grants hub → target for that login | §6b — add/extend the `ssh` rule |
| `tailnet_routing` | The API process cannot route to 100.x | Run the API with `network_mode: host` (§2) |
| `auth_failed` | Ordinary sshd rejected the credentials | Fix the machine's SSH user/password, or add a key (§6b) |
| `bad_private_key` | The stored PEM key does not parse | Re-paste the full key incl. BEGIN/END; set the passphrase |
| `unreachable` | Nothing accepted TCP | Host down, wrong port, or firewall |
| `timeout` | Connected, then the remote stopped responding | Loaded or suspended host |

Causes marked non-retryable disable auto-reconnect on purpose — retrying a
pending browser approval just piles up hung dials on the target.

> **"It worked yesterday, it doesn't today"** on a Tailscale target is almost
> always the `check` approval lapsing overnight. Go straight to §6b.

---

## 8. Related docs

| Need | Doc |
|------|-----|
| What we build | `docs/prd.md` |
| What to implement next | `docs/backlog.md` |
| Domain terms | `docs/ddd.md` |
| Shortcuts / risks | `docs/debt.md` |
| System shape | `docs/architecture.md` |
