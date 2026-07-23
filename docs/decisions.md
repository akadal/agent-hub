# Architecture Decision Records (ADR index)

Lightweight log of decisions that affect structure. Full product requirements stay in `docs/prd.md`.

**Status values:** `proposed` · `accepted` · `superseded` · `rejected`

---

## ADR-001: Product stack for v1

| Field | Value |
|-------|--------|
| Status | accepted |
| Date | 2026-07-23 |
| Context | Need a single stack for multi-machine web terminals that is easy to self-host. |
| Decision | Backend **Go**; frontend **React + xterm.js**; session persistence **tmux**; auth **JWT + local users** with **Tailscale preferred**; package with **Docker Compose**. Hosting platform is operator choice. |
| Consequences | Team optimizes for this stack; alternatives (e.g. pure SSH UI without tmux) are out of default path. No PaaS lock-in in docs or packaging. |

---

## ADR-002: Manual machine registration only

| Field | Value |
|-------|--------|
| Status | accepted |
| Date | 2026-07-23 |
| Context | Discovery vs explicit inventory. |
| Decision | v1 machines are registered manually (IP/address entry). No automatic discovery. |
| Consequences | Simpler security boundary; operators own the inventory. |

---

## ADR-003: tmux for terminal durability

| Field | Value |
|-------|--------|
| Status | accepted |
| Date | 2026-07-23 |
| Context | Web terminals often die on disconnect. |
| Decision | Use tmux for the most stable persistent web terminal experience. |
| Consequences | Target environment (or bridge) must support tmux; reconnect/attach is a core feature. |

---

## ADR-004: Local Compose first; deploy is optional but supported

| Field | Value |
|-------|--------|
| Status | accepted |
| Date | 2026-07-23 |
| Context | Public self-hosted project; operators run locally or remote. Mobile access needs a reachable instance. |
| Decision | Docker Compose is the primary packaging. Local run is first-class. Remote deploy uses the same stack; no required hosting vendor. |
| Consequences | Docs and samples stay vendor-neutral; responsive UI is required because remote/mobile clients are expected. |

---

## ADR-005: Remote execution channel — SSH by IP

| Field | Value |
|-------|--------|
| Status | accepted |
| Date | 2026-07-24 |
| Context | Dashboard must open shells on registered machines. Tailscale is a preferred *network* path for operators but must not be required for local e2e or self-host. |
| Decision | **v1 remote channel is SSH** to the manually registered **IP/hostname** (optional port, SSH user + password for bootstrap; keys later). **Tailscale is optional** (identity/ingress later) — not required to register a machine or open a terminal. Compose ships a **dummy SSH target** (`ssh-target`) for e2e. Interactive sessions use WebSocket + PTY; one-shot commands use `POST /api/machines/{id}/exec`. tmux persistence remains a follow-on enhancement on top of SSH. |
| Consequences | API and UI collect address + SSH credentials. Operators on a mesh simply register Tailscale IPs the same way. No Tailscale daemon dependency in the control plane for this path. |

---

## ADR-006: Frontend UI shell — satnaing/shadcn-admin

| Field | Value |
|-------|--------|
| Status | accepted |
| Date | 2026-07-23 |
| Context | Need a ready React admin shell that is minimal/modern, responsive, MIT-friendly, and easy for agentic development—without inventing a design system from scratch. |
| Decision | Base the Agent Hub web UI on **[satnaing/shadcn-admin](https://github.com/satnaing/shadcn-admin)** as the dashboard shell baseline: **Vite + React + TypeScript + Tailwind + shadcn/ui**. Keep shadcn primitives and admin layout patterns; strip demo-only surfaces (Clerk sample auth, chart-heavy showcase pages) as needed. Product pages (machines, terminals, users, permissions, audit) replace demos over milestones. Do **not** adopt MUI/Ant as the primary component system. |
| Consequences | FE lives under `web/` with Vite/React/shadcn stack; architecture docs and Docker packaging target this shell. xterm.js embeds into product routes later (M3). New UI work prefers shadcn/ui components and the established layout chrome. |

---

## How to add an ADR

1. Increment `ADR-NNN`.
2. Fill status, date, context, decision, consequences.
3. Link from `docs/architecture.md` if it changes system shape.
