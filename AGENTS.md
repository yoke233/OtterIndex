# Repository Guidelines

## Project Structure & Module Organization

- `cmd/otidx/`: CLI entrypoint (index build + query against a local SQLite DB)
- `cmd/otidxd/`: daemon skeleton (TCP JSON-RPC: `ping` / `version`)
- `internal/core/`: walking, searching, unitizing output, Tree-sitter integration
- `internal/index/sqlite/`: SQLite schema, storage, and optional FTS plumbing
- `internal/otidxcli/`: Cobra commands, options parsing, and renderers (`--compact`, `-L`, `--jsonl`)
- `docs/`: design notes (`docs/**`)
- `bench/`: benchmark scripts + outputs (`bench/out/`, `bench/docs/`)

Local artifacts live under `.otidx/` (and `*.db`) and are intentionally ignored by Git.

## Build, Test, and Development Commands

```powershell
# Run without producing binaries
go run ./cmd/otidx --help

# Build binaries (Windows example)
go build -o otidx.exe ./cmd/otidx
go build -o otidxd.exe ./cmd/otidxd

# Build an index (creates .otidx/index.db in the current directory)
go run ./cmd/otidx index build .

# Query the index
go run ./cmd/otidx q "keyword"
```

Performance/benchmark helpers live under `bench/` (see `README.md`).

## Coding Style & Naming Conventions

- Go code must be `gofmt`-formatted: `gofmt -w .`
- Keep packages internal-first: prefer adding code under `internal/**` and keep package names short/lowercase.
- For CLI behavior changes, keep output formats stable where possible and update docs (`README.md`, `docs/**`).

## Testing Guidelines

- Tests use the standard Go `testing` package (`*_test.go`).
- Run all tests: `go test ./...`
- Run a focused test while iterating: `go test ./internal/core/query -run TestSession -count=1`

## Commit & Pull Request Guidelines

- Follow Conventional Commits (as used in history): `feat(scope): ...`, `docs: ...`, `test(scope): ...`, `chore: ...`
  - Common scopes: `otidx`, `cli`, `query`, `index`, `indexer`, `otidxd`
- PRs should include: what/why, how to verify, and test output. For performance-sensitive changes, include a short benchmark note (e.g., `bench/bench-vs-rg.ps1` results).

## Security & Configuration Tips

- Do not commit generated artifacts: `.otidx/`, `*.db`, or locally built binaries.
- Prefer relative paths in examples and output to keep results portable across machines.

## Agent-Specific Notes (optional)

- This repo is typically developed on Windows; prefer PowerShell (`pwsh`) commands in docs and scripts.
- When searching across files, use ripgrep: `rg -n "pattern" .`
