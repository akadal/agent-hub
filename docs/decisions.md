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

## ADR-005: Remote execution channel

| Field | Value |
|-------|--------|
| Status | proposed |
| Date | 2026-07-23 |
| Context | Dashboard must open shells on registered machines. |
| Decision | **TBD** before Milestone 3 (SSH+tmux vs agent vs other). |
| Consequences | Blocks final architecture of terminal gateway; track in `docs/architecture.md` and `docs/debt.md` watch list. |

---

## How to add an ADR

1. Increment `ADR-NNN`.
2. Fill status, date, context, decision, consequences.
3. Link from `docs/architecture.md` if it changes system shape.
