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

## ADR-002: Manual machine registration (+ optional Tailscale import)

| Field | Value |
|-------|--------|
| Status | accepted (amended) |
| Date | 2026-07-23; amended 2026-07-24 |
| Context | Discovery vs explicit inventory. |
| Decision | Machines are an **explicit inventory** owned by the operator. Primary path remains manual IP/hostname register. **Optional one-shot import** from the Tailscale API (`TAILSCALE_API_KEY`) lists authorized devices and creates machine rows on user click — not continuous auto-discovery, not a Tailscale daemon dependency. |
| Consequences | Security boundary stays operator-owned. Import needs an API access token on the api service; SSH user/password are still entered once per import batch. |

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

## ADR-007: Classify SSH open failures; never silently retry a human-gated one

| Field | Value |
|-------|--------|
| Status | accepted |
| Date | 2026-07-25 |
| Context | A target running **Tailscale SSH** answers port 22 on its tailnet address itself, so the machine's stored SSH password is inert and access is decided by the tailnet ACL. Under Tailscale's default `"action": "check"` rule a session needs browser approval that is cached only for `checkPeriod` (12h). Agent Hub is headless, so such a target works right after a human approves it and then fails on its own hours later. The client surfaced this as an unexplained `i/o timeout` on a black terminal, and retried — piling hung dials on the target (137 stalled connections in 24h against one host) while giving the operator nothing to act on. The approval URL was in fact being sent by the remote, in the SSH auth banner, and thrown away. |
| Decision | The SSH layer **records the authentication trace** (banner + keyboard-interactive instruction) and **classifies** every open failure into a `sshterm.Failure{Kind, Message, Hint, ApprovalURL, Retryable}` — `tailscale_check_pending`, `tailscale_denied`, `tailnet_routing`, `auth_failed`, `unreachable`, `timeout`. The bridge forwards this structurally on the WebSocket `error` frame; the terminal renders the cause, the fix and any approval link instead of a bare timeout. **`Retryable: false` disables auto-reconnect** — a pending browser approval, bad credentials or wrong network topology cannot be fixed by reconnecting. Diagnosis is inference over evidence, not a guess: what the remote said wins over the error string. |
| Consequences | Failure causes are machine-readable, so the UI and the runbook can branch on `kind` (see `docs/ops.md` §6b and §7). Tailnet targets must be granted an `accept` ACL rule for the hub node — that is infrastructure configuration, not something the app can work around, and the app now says so explicitly. Adding a new failure mode means adding a `FailureKind` plus its hint, keeping cause and remedy in one place. |

---

## ADR-008: SSH private key as a first-class machine credential

| Field | Value |
|-------|--------|
| Status | accepted |
| Date | 2026-07-25 |
| Context | ADR-005 bootstrapped the SSH channel with a user + password. That leaves the hub unable to reach two common targets: hosts hardened with `PasswordAuthentication no` (where no password can ever succeed), and Tailscale SSH hosts under an `"action": "check"` ACL (where authentication is a human-gated browser approval the hub cannot perform). Both were hit on the same target, so password-only auth had no path in at all. |
| Decision | A machine may carry an **optional PEM private key** (plus passphrase) alongside the password, offered **before** the password because a hardened sshd accepts nothing else. A key that fails to parse is a hard error raised **before any network I/O**, classified `bad_private_key` — never a silent fallback to a password the host will refuse. `Machine.Public()` exposes only `has_private_key`; the key itself never leaves the API. `CreateMachine` takes a `MachineSpec` struct rather than a growing parameter list. Because Tailscale intercepts only port 22, publishing sshd on a second port gives a key-authenticated path that is independent of the tailnet ACL (see `docs/ops.md` §6b). |
| Consequences | Operators can reach hardened and ACL-gated hosts without weakening either. Failure diagnosis is port-aware: a rejection on port 22 of a tailnet address means the ACL, on any other port it means `authorized_keys` — conflating them sends the operator to the wrong console. Keys are stored in the plaintext JSON store like the existing passwords; encryption at rest is tracked as D-009. |

---

## ADR-009: Trust-on-first-use host key pinning

| Field | Value |
|-------|--------|
| Status | accepted |
| Date | 2026-07-25 |
| Context | The SSH bridge shipped with `ssh.InsecureIgnoreHostKey()`, so it accepted whatever key answered on a machine's address. For a product whose function is opening interactive root shells, that removes the one check that makes SSH safe on a network you do not fully control — anything that can answer on that address gets the session, the credential and the shell. Standard `known_hosts` is not directly available: machines are registered through a web form by operators who typically do not have the target's fingerprint at hand, so requiring one up front would block ordinary onboarding. |
| Decision | **Trust on first use.** A machine starts unpinned; the first successful connect records the presented key as `Machine.HostKeyFingerprint` (`SHA256:…`). Every later connect must present the same key or the handshake is aborted and classified `host_key_changed` with `Retryable: false`. `Store.PinMachineHostKey` **never overwrites** an existing pin — accepting a replacement is exactly the outcome an impostor wants — so a legitimately rebuilt host is re-adopted by deleting and re-registering the machine. The fingerprint is public data and is returned in `MachinePublic` so an operator can compare it against `ssh-keyscan`. All four SSH call sites now build their target through one `sshTarget(m)` helper, so a new credential field cannot reach three paths and miss the fourth. |
| Consequences | A man-in-the-middle on the path to a registered machine is refused rather than silently served. The cost is a real failure mode operators must understand: re-imaging a host breaks its pin on purpose, and the fix is re-registration, which `docs/ops.md` §7 spells out. TOFU still trusts the very first connect, so it does not protect an attacker who is already in position at registration time — pinning by hand at registration would, and remains open as a later refinement. |

---

## How to add an ADR

1. Increment `ADR-NNN`.
2. Fill status, date, context, decision, consequences.
3. Link from `docs/architecture.md` if it changes system shape.
