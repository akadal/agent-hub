# Agent Hub

**Multi-Machine Terminal Dashboard (v1)** — a Go-based web dashboard for managing terminals on machines on your private network (typically Tailscale).

Multi-user access, a permission matrix, mandatory audit logging, and tmux-backed persistent sessions in the browser (React + xterm.js). Run it locally with Docker Compose, or deploy it anywhere Compose works so you can reach it from a phone or other devices.

## Stack (planned)

| Layer | Choice |
|-------|--------|
| Backend | Go |
| Frontend | React + xterm.js (responsive UI) |
| Sessions | tmux |
| Auth | JWT + local users; Tailscale identity preferred when available |
| Run / ship | Docker Compose (local first; any host/Paas that runs Compose) |

## Documentation

| Audience | Start here |
|----------|------------|
| **Coding agents / AI** | [`AGENTS.md`](AGENTS.md) — maps every task to the right doc |
| **Product requirements** | [`docs/prd.md`](docs/prd.md) |
| **Backlog** | [`docs/backlog.md`](docs/backlog.md) |
| **Domain model** | [`docs/ddd.md`](docs/ddd.md) |
| **Tech debt** | [`docs/debt.md`](docs/debt.md) |
| **Architecture** | [`docs/architecture.md`](docs/architecture.md) |
| **Ops / run / deploy** | [`docs/ops.md`](docs/ops.md) |
| **Decisions (ADRs)** | [`docs/decisions.md`](docs/decisions.md) |

This README is a short overview. Full product contract and v1 non-goals live in the PRD, not here.

## Quick intent

- **Config:** copy [`.env.example`](.env.example) → `.env` (never commit secrets).
- **Local:** `docker compose up` (once packaging lands) for development and single-operator use.
- **Remote / mobile:** Deploy the same Compose stack behind your reverse proxy or platform of choice; UI must be usable on small screens.

## v1 non-goals (summary)

- No RDP/VNC  
- No file transfer  
- No automatic device discovery  

## Status

Repository bootstrap: documentation structure in place. Application code and packaging land with the backlog milestones.
