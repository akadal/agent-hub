# Contributing to Agent Hub

Thanks for taking a look. Issues and pull requests are welcome — including
"this documentation is wrong", which is a real bug in a self-hosted tool.

Security problems are the exception: report those privately, see
[`SECURITY.md`](SECURITY.md).

## Getting the stack up

```bash
git clone https://github.com/akadal/agent-hub.git
cd agent-hub
cp .env.example .env
docker compose up --build
```

The web UI is on <http://localhost:27342>, the API on
<http://localhost:27341>, and a throwaway SSH host (`root` / `targetpass`) on
port 27343 so you can exercise terminals without touching a real machine.

Running the pieces directly instead:

```bash
cd backend && go run ./cmd/agent-hub     # API on :27341
cd web && npm install && npm run dev     # SPA on :27342, proxies /api
```

Requirements for the non-Docker path: Go 1.22+ and Node 20+.

## Before you open a pull request

Run what CI runs — [`.github/workflows/ci.yml`](.github/workflows/ci.yml) is
the source of truth, and it is three jobs:

```bash
cd backend && gofmt -l . && go vet ./... && go test ./...
cd web && npm run lint && npm test && npm run build
./scripts/e2e-smoke.sh                   # needs the Compose stack running
```

`e2e-smoke.sh` **deletes every machine** in the instance it points at. Run it
against a scratch stack, never against one you use.

Expectations for a change:

- **Tests come with behaviour.** Bug fixes get a test that fails before the fix.
  The backend suites live next to the code they cover; frontend logic that can
  be tested without a browser lives under `web/src/lib`.
- **Keep it focused.** One concern per PR; unrelated cleanups are their own PR.
- **Say what you verified.** "Ran the terminal against the demo host and
  reconnected after a network drop" is worth more than a green checkmark.
- **Comments explain why**, not what. Match the density of the surrounding code.

## Where things are decided

This repo keeps its reasoning in files, not in issue threads:

| You want to… | Read |
|---|---|
| know what the product is and is not | [`docs/prd.md`](docs/prd.md) |
| pick up planned work | [`docs/backlog.md`](docs/backlog.md) |
| understand a structural choice | [`docs/decisions.md`](docs/decisions.md) (ADRs) |
| see the known shortcuts | [`docs/debt.md`](docs/debt.md) |
| run or deploy it | [`docs/ops.md`](docs/ops.md) |
| navigate as a coding agent | [`AGENTS.md`](AGENTS.md) |

If your change alters the system's shape (a new remote channel, a new auth
path, a new storage model), add an ADR in `docs/decisions.md` in the same PR.
If you knowingly ship a shortcut, add a row to `docs/debt.md` — a documented
gap is fine, a silent one is not.

For anything large, open an issue first so we can agree on scope before you
spend the time.

## Repository hygiene

This is a public repository for software that holds infrastructure
credentials. Please do not commit:

- private hostnames, tailnet addresses, or personal domains — use placeholders
  such as `your-host.example`, `10.0.0.5`, `100.64.0.10`
- personal email addresses
- real credentials, keys, or tokens, including in tests and sample configs

`data/` and `.env` are git-ignored for this reason; keep it that way.

## Code of conduct

By participating you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).
