# Contributing to Helm

## Commit messages — Conventional Commits (required)

Releases are automated with [release-please](https://github.com/googleapis/release-please), which
parses commit messages to decide the next version and to write `CHANGELOG.md`. Commits on `main`
**must** follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<optional scope>): <description>

<optional body>

<optional footer>
```

**Types**

| Type | Use for | Version effect (0.x) |
|------|---------|----------------------|
| `feat` | a new feature | **minor** bump (0.x → 0.(x+1).0) |
| `fix` | a bug fix | **patch** bump (0.x.y → 0.x.(y+1)) |
| `docs`, `test`, `refactor`, `chore`, `ci`, `perf`, `style`, `build` | non-release-worthy changes | none |

**Breaking changes** use a `!` after the type/scope (`feat!:`, `fix!:`) or a `BREAKING CHANGE:`
footer. While the project is pre-1.0, breaking changes bump the **minor** version
(`bump-minor-pre-major`), not the major.

Scope where it clarifies intent, e.g. `fix(tray): stop popover auto-hiding on Wayland`,
`feat(service): add process-kill control`, `ci(release): attach macOS artifacts`.

### Examples

```
feat(update): in-app update check with checksum-verified download
fix(service): detect ollama serve when the systemd unit is inactive
docs: document the services.json config format
chore(deps): bump golang.org/x/sys to v0.46.0
```

## Release flow (how it works)

1. You merge Conventional Commits into `main`.
2. release-please opens/updates a **Release PR** bumping `version.txt` and `CHANGELOG.md`.
3. Merging the Release PR tags the version and creates the GitHub Release.
4. Gated CI jobs build the macOS `.app` + Linux packages and attach them with `SHA256SUMS`.

Do **not** hand-edit `version.txt`, `CHANGELOG.md`, or create tags/releases manually — the
automation owns them. `version.txt` is the single source of truth for the embedded app version.

## Engineering conventions

- **Zero third-party Go dependencies** beyond Wails — hand-roll small helpers instead.
- Every external command uses a **bounded timeout** (`exec.CommandContext`).
- Background loops follow the poller pattern (ticker + `StopPolling` + `sync.Once` + panic
  recovery).
- Frontend is **strict TypeScript**; never set `innerHTML` from untrusted data; keep the CSP
  intact.
- Add/keep **unit tests** for new pure logic; `go vet`, `staticcheck`, `go test ./...`, `tsc`,
  and `wails3 build` must all pass.

## Local checks before pushing

```bash
go vet ./... && go test ./...
( cd frontend && npx tsc --noEmit )
wails3 doctor && wails3 build
```
