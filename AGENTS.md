# AGENTS.md — kena

Greek gap-fill trainer: htmx web UI + MCP authoring server + minimal OAuth
server, one Go binary, Postgres. User overview in README.md. Design:
`docs/superpowers/specs/2026-10-16-kena-design.md`; client/OAuth research:
`docs/research.md`.

## Commands

```
go test ./...        # needs initdb + postgres on PATH (throwaway cluster) or KENA_TEST_DATABASE_URL
go vet ./...
nix build            # after go.mod changes: update vendorHash in nix/package.nix
nix flake check
```

## Layout

- `cmd/kena`: serve, migrate, token create, version.
- `internal/app`: routing; `/mcp` behind `auth.RequireBearerToken`.
- `internal/db`: pool, embedded SQL migrations; `dbtest` starts a throwaway cluster per test binary.
- `internal/store`: all SQL. Does not lint.
- `internal/greek`: NFC, answer normalisation, strict check, task lint.
- `internal/guide`: authoring guide markdown served by `get_authoring_guide`, `skill://kena/<topic>` and server instructions.
- `internal/mcpserver`: tools; every write goes through `prepare` (NFC + lint) before touching the store.
- `internal/oauth`: discovery metadata, DCR, CIMD (SSRF-guarded fetch), authorize page (paste token), token endpoint, bearer verifier.
- `internal/web`: templates, htmx, `kena_device` cookie, cards/list modes.

## Rules

- Answers never reach the browser before an attempt; checking is server-side.
- MCP tool errors the model should see are returned as Go errors from handlers (the SDK then omits structuredContent).
- Keep MCP tool names stable; agents and the guide reference them.
- The guide must pass the task lint itself (tests enforce no polytonic, no final σ).
