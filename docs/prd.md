# PRD: Agent Hub

**Product:** Multi-Machine Terminal Dashboard (v1)  
**Repo:** agent-hub (public / self-hostable)  
**Status:** Draft / v1 planning

---

## 1. Summary

Go-based web dashboard that manages terminals on machines connected to a private network (typically Tailscale). Supports multi-user access, a permission matrix, mandatory audit logging, and persistent terminal sessions. Designed as an open-source, self-hosted tool: run locally or deploy on your own infrastructure.

---

## 2. Goals

| Goal | Detail |
|------|--------|
| Local run | First-class: Docker Compose on a developer or home machine |
| Deploy | Same Compose stack on any host or platform that runs containers (operator choice; no single PaaS required) |
| Mobile | Usable from phones/tablets when the instance is network-reachable (responsive UI) |
| Experience | Browser-based multi-machine terminal control with durable sessions |
| CI (optional) | Operators may wire commit → deploy; not a product dependency |

---

## 3. Authentication

| Preference | Mechanism |
|------------|-----------|
| **Primary** | Tailscale identity (when available) |
| **Fallback** | Username + password (local users) |
| **Ops note** | Bootstrap admin / local ops accounts for first install and recovery |

**Session tech:** JWT for API/session tokens; local user store as Tailscale fallback.

---

## 4. Machine Management

- **Registration:** Manual only — “New device” → enter IP (or address) → register.
- **Discovery:** No automatic device discovery in v1.
- Machines are expected to be reachable on the private/Tailscale network (or as configured by access policy).

---

## 5. Terminals

- Under each machine: **1:N** terminals.
- UI: general-purpose terminal via **xterm.js**.
- Session persistence: **tmux** (chosen for the most stable web terminal experience).
- Sessions survive browser disconnect; reconnect attaches to the same tmux session where possible.

---

## 6. Users & Permissions

- Multi-user system.
- **Many-to-many** authorization: user ↔ machine and/or user ↔ terminal.
- Permission matrix controls who can open, use, or administer which resources.

---

## 7. Audit Log

**Mandatory** for v1. Each relevant action records at least:

- Who (user identity)
- When (timestamp)
- Which machine / terminal
- What (command / action)

Audit data is a first-class product requirement, not optional logging.

---

## 8. Access Control (Network)

- Configured under **Settings**.
- **Default:** access only from within Tailscale (or equivalent private mesh).
- Operators can change access policy via settings (not hard-coded only to Tailscale after first config).

---

## 9. UX / clients

- **Desktop and mobile browsers** are in scope for v1.
- Layout and terminal chrome must be **responsive** (usable on small viewports; terminal readable and operable on touch devices as far as a shell UI reasonably allows).
- Mobile use implies the instance is deployed (or otherwise reachable) from the device’s network path—not only `localhost`.

---

## 10. Technical Stack

| Layer | Choice |
|-------|--------|
| Backend | Go |
| Frontend | React + xterm.js |
| Session persistence | tmux |
| Auth | JWT + local users (Tailscale preferred when possible) |
| Packaging | Docker Compose |
| Hosting | Operator-managed (local Compose, VPS, k8s later, PaaS optional) |

---

## 11. Out of Scope (v1)

Explicit non-goals for the first release:

- RDP / VNC
- File transfer
- Automatic device discovery
- Vendor lock-in to a specific hosting panel or PaaS

---

## 12. Success Criteria (v1)

- Authenticated users can register machines by IP and open 1:N web terminals backed by tmux.
- Permissions restrict machine/terminal access on a many-to-many basis.
- Audit log captures identity, time, resource, and command/action.
- Default network access is Tailscale/private-mesh-only; settings can adjust policy.
- App runs locally via Docker Compose.
- Same stack can be deployed for remote (including mobile) access.
- UI is usable on narrow viewports (responsive).

---

## 13. Open / TBD

- Exact Tailscale identity integration mechanism (headers, WhoIs API, sidecar, etc.).
- Agent vs agentless remote execution model on registered machines (how the dashboard reaches the shell/tmux).
- Retention policy for audit logs.
- Admin bootstrap / first-user flow for self-hosted installs.
- Whether permission is only machine-level, only terminal-level, or both with inheritance.
- Recommended reverse-proxy / TLS patterns for public or mobile access (examples only; not required vendors).

These TBDs must not expand v1 scope beyond sections 1–11 without an explicit PRD update.
