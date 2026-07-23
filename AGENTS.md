# AGENTS.md — agent entry map

This file is the **routing table** for humans and coding agents working in this repo.  
It is **not** a second PRD. Open the linked doc for full content.

---

## What this repo is

**Agent Hub** — Multi-Machine Terminal Dashboard (v1).

Open-source Go web dashboard for managing terminals on private-network machines (Tailscale-centric by default): multi-user, permission matrix, mandatory audit log, tmux-persistent sessions, React + xterm.js UI (responsive), Docker Compose for local run and self-hosted deploy.

**Stack (v1):** Go · React + xterm.js · tmux · JWT + local users (Tailscale preferred) · Docker Compose  
**FE shell (ADR-006):** Vite + React + TypeScript + shadcn/ui (baseline [satnaing/shadcn-admin](https://github.com/satnaing/shadcn-admin)) under `web/`; API under `backend/`.

**Status:** Working e2e — JWT admin, machines, **multi-session workspace** (1:N terminals per machine), SSH exec + xterm, Compose dummy `ssh-target`. Tailscale not required (ADR-005).

Do **not** hardcode operator-specific hostnames, personal domains, or a single PaaS into docs or code. Treat deploy target as operator-configured.

---

## When working on X → open Y

| Task / question | Open this file |
|-----------------|----------------|
| Product scope, goals, non-goals, auth/machine/terminal/audit requirements | [`docs/prd.md`](docs/prd.md) |
| What to build next; milestones and epics | [`docs/backlog.md`](docs/backlog.md) |
| Domain language, entities, relations (User, Machine, Terminal, Permission, Session, AuditEvent) | [`docs/ddd.md`](docs/ddd.md) |
| Known shortcuts, accepted risks, watch list | [`docs/debt.md`](docs/debt.md) |
| System shape, components, terminal bridge, deploy topology | [`docs/architecture.md`](docs/architecture.md) |
| Local run, deploy patterns, bootstrap, runbook | [`docs/ops.md`](docs/ops.md) |
| Architecture decision records (ADRs) | [`docs/decisions.md`](docs/decisions.md) |
| Human project overview (short) | [`README.md`](README.md) |

---

## Doc tree

```
.
├── AGENTS.md          ← you are here (agent router)
├── README.md          ← human-facing overview
├── docker-compose.yml
├── backend/           ← Go API (cmd/agent-hub, internal/api)
├── web/               ← Vite + React + shadcn/ui shell
└── docs/
    ├── prd.md         ← product source of truth
    ├── backlog.md     ← ordered v1 work
    ├── ddd.md         ← domain model
    ├── debt.md        ← tech debt + watch list
    ├── architecture.md
    ├── ops.md
    └── decisions.md
```

All product/engineering markdown **except** this file and `README.md` lives under `docs/`.

---

## Working rules for agents

1. **Product truth** = `docs/prd.md`. Do not invent v1 features that contradict it (especially out-of-scope: RDP/VNC, file transfer, auto discovery).
2. **Domain names** = `docs/ddd.md`. Prefer ubiquitous language in code and APIs.
3. **Priorities** = `docs/backlog.md`. Prefer milestone order unless the user directs otherwise.
4. **Shortcuts** = record in `docs/debt.md` when you knowingly ship incomplete design.
5. **Structural decisions** = add or update an ADR in `docs/decisions.md` and keep `docs/architecture.md` consistent.
6. **Run / deploy** = `docs/ops.md`. Prefer generic Compose + self-host docs; never bake personal domains or one vendor as required.
7. **Public repo hygiene** = no private hostnames, personal emails, or operator-only secrets in docs or sample configs. Use placeholders (`your-host.example`, env vars).

---

## Implementation status

v0.1 skeleton is in-repo (`backend/`, `web/`, `docker-compose.yml`). Remaining product features follow `docs/backlog.md` milestones (auth, machines, terminals, …).

When implementation starts:

- Keep `AGENTS.md` as the router; put long-form content in `docs/`.
- Update `docs/backlog.md` statuses as milestones complete.
- Update `docs/architecture.md` when the remote terminal channel (SSH vs agent, etc.) is decided.
