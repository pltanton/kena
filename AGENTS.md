# AGENTS.md — kena

Greek gap-fill trainer: htmx web UI + MCP authoring server + minimal OAuth
server, one Go binary, Postgres. User overview in README.md. Design:
`docs/superpowers/specs/2026-10-16-kena-design.md`, accounts:
`docs/superpowers/specs/2026-10-16-accounts-design.md`, mistakes report:
`docs/superpowers/specs/2026-10-17-mistakes-report-design.md`; client/OAuth research:
`docs/research.md`.

## Commands

```
go test ./...        # needs initdb + postgres on PATH (throwaway cluster) or KENA_TEST_DATABASE_URL
go vet ./...
nix build            # after go.mod changes: update vendorHash in nix/package.nix
nix flake check
```

## Layout

- `cmd/kena`: serve, migrate, user create/passwd, token create, version.
- `internal/app`: routing; `/mcp` behind `auth.RequireBearerToken`.
- `internal/db`: pool, embedded SQL migrations; `dbtest` starts a throwaway cluster per test binary.
- `internal/store`: all SQL. Does not lint.
- `internal/greek`: NFC, answer normalisation, strict check, error kinds (`Classify`), task lint.
- `internal/guide`: authoring guide markdown served by `get_authoring_guide`, `skill://kena/<topic>` and server instructions.
- `internal/mcpserver`: tools; every write goes through `prepare` (NFC + lint) before touching the store.
- `internal/oauth`: discovery metadata, DCR, CIMD (SSRF-guarded fetch), authorize page (login + Allow via the web session), token endpoint, bearer verifier.
- `internal/passphrase`: EFF short wordlist passphrases, argon2id hashing.
- `internal/session`: `kena_session` cookie, login, cross-origin protection; shared by `web` and `oauth` (oauth must not import web).
- `internal/web`: templates, htmx, `kena_device` cookie, cards/list modes, login, account and mistakes pages. Logged in, attempts go under `users.progress_id` instead of the device id.

## Rules

- Answers never reach the browser before an attempt; checking is server-side. `/mistakes` shows only tasks already attempted.
- `get_mistakes_report` reads only the caller's own `progress_id`; no tool reads another user's attempts.
- State-changing web and authorize POSTs go through `session.CrossOrigin` (net/http CrossOriginProtection, KENA_PUBLIC_URL trusted).
- MCP tool errors the model should see are returned as Go errors from handlers (the SDK then omits structuredContent).
- Keep MCP tool names stable; agents and the guide reference them.
- The guide must pass the task lint itself (tests enforce no polytonic, no final σ).
