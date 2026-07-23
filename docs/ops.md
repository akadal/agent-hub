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

## 2. Local run (intended)

1. Clone the repository.
2. Configure env from a sample file (e.g. `.env.example` → `.env`) — secrets never committed.
3. `docker compose up` (exact command TBD when packaging lands).
4. Open the printed local URL; complete bootstrap admin if first start.
5. Verify health endpoint and login.

*Service list and ports will be filled with M0.3.*

---

## 3. Deploy (intended, generic)

1. Build or pull images defined by the Compose file.
2. Run the stack on a host reachable from clients that need access (including mobile).
3. Put TLS and hostname in front via your reverse proxy / platform.
4. Restrict ingress (prefer Tailscale or VPN; open WAN only with clear AccessPolicy intent).
5. Verify health endpoint and login from a second device (e.g. phone).

Optional: hook git push to rebuild/redeploy on your platform of choice.

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

---

## 7. Incident checklist (draft)

1. Can the site be reached from the expected network (Tailscale / VPN / LAN)?
2. Is JWT login / Tailscale identity failing?
3. Is the target machine still registered and reachable?
4. Is tmux/session bridge healthy on that machine?
5. Check audit log for recent access and errors (when M5 ships).

---

## 8. Related docs

| Need | Doc |
|------|-----|
| What we build | `docs/prd.md` |
| What to implement next | `docs/backlog.md` |
| Domain terms | `docs/ddd.md` |
| Shortcuts / risks | `docs/debt.md` |
| System shape | `docs/architecture.md` |
