## What this changes

<!-- One or two sentences. Link the issue if there is one. -->

## Why

<!-- The problem being solved. For a bug: what went wrong, and the root cause. -->

## How it was verified

<!-- Be specific: which tests, and what you exercised by hand. -->

- [ ] `cd backend && gofmt -l . && go vet ./... && go test ./...`
- [ ] `cd web && npm run lint && npm test && npm run build`
- [ ] Exercised by hand (say what — e.g. "opened a session on the demo host and
      reconnected after killing the network")

## Notes

- [ ] Behaviour change is covered by a test that fails without it
- [ ] Structural decision recorded in `docs/decisions.md` (or: not structural)
- [ ] Known shortcut recorded in `docs/debt.md` (or: none)
- [ ] No private hostnames, personal emails, or real credentials in the diff
