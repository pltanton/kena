# AGENTS.md — kena

Greek gap-fill trainer: htmx web UI + MCP authoring server + minimal OAuth
server, one Go binary, Postgres. User overview in README.md. Design:
`docs/superpowers/specs/2026-10-16-kena-design.md`, accounts:
`docs/superpowers/specs/2026-10-16-accounts-design.md`, mistakes report:
`docs/superpowers/specs/2026-10-17-mistakes-report-design.md`, UI and sharing:
`docs/superpowers/specs/2026-10-17-ui-sharing-design.md`, stars:
`docs/superpowers/specs/2026-10-17-stars-design.md`, visibility and editor:
`docs/superpowers/specs/2026-10-17-visibility-editor-design.md`, practice flow, QR login and theme:
`docs/superpowers/specs/2026-10-17-practice-flow-design.md`, deck rules:
`docs/superpowers/specs/2026-10-17-deck-rules-design.md`, admin:
`docs/superpowers/specs/2026-10-17-admin-design.md`; client/OAuth research:
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
- `internal/greek`: NFC, answer normalisation, strict check, error kinds (`Classify`), task lint, `Prepare`/`PrepareAgainst` (NFC + lint + duplicate, repeated-explanation and batch style warnings).
- `internal/deckfmt`: the editor's plain-text deck format; `Format` and `Parse` round-trip, parse errors carry line numbers.
- `internal/guide`: authoring guide markdown served by `get_authoring_guide`, `skill://kena/<topic>` and server instructions.
- `internal/mcpserver`: tools; every write goes through `prepare` (`greek.PrepareAgainst`, with the set's existing tasks) before touching the store.
- `internal/oauth`: discovery metadata, DCR, CIMD (SSRF-guarded fetch), authorize page (login + Allow via the web session), token endpoint, bearer verifier.
- `internal/passphrase`: EFF short wordlist passphrases, argon2id hashing.
- `internal/session`: `kena_session` cookie, login, cross-origin protection; shared by `web` and `oauth` (oauth must not import web).
- `internal/web`: templates, htmx, `kena_device` cookie, deck overview (`/s/{slug}`: which-tasks chooser, Practise and Worksheet), focus-mode practice (`/s/{slug}/play`), worksheet (`/s/{slug}/list`), login, account and mistakes pages, deck editor (`/new`, `/s/{slug}/edit`, `/s/{slug}/delete`; saves through `greek.Prepare` and `store.SaveDeck`). Logged in, attempts go under `users.progress_id` instead of the device id.

## Rules

- Answers never reach the practice page before Give up or a correct check: a wrong check re-renders the form with the typed text and right blanks locked; checking is server-side. `/mistakes` shows expected answers, but only for tasks already attempted.
- QR login codes (`login_codes`) are hashed, one-time, 5 minutes, one unused code per user; the GET page never consumes a code, only the POST does. kena's logs redact `/login/qr/<secret>` (`session.RedactPath`); a reverse proxy's own access log still records full paths, so keep that log private or redact there too.
- `get_mistakes_report` reads only the caller's own `progress_id`; no tool reads another user's attempts. The report (MCP and `/mistakes`) leaves out other users' private sets.
- State-changing web and authorize POSTs go through `session.CrossOrigin` (net/http CrossOriginProtection, KENA_PUBLIC_URL trusted).
- MCP tool errors the model should see are returned as Go errors from handlers (the SDK then omits structuredContent).
- `sets.visibility`: `private` = owner only (web 404 for everyone else, admins included; MCP not found except for admins), `friends` = any logged-in user, `public` = everyone. `store.CanView` is the single rule; listings use the same SQL conditions.
- Stars only add non-private sets to a user's home list; a starred set that turns private disappears and the star stays dormant.
- `/admin` answers 404 to everyone but logged-in admins. The store enforces the user guards (no changing your own admin flag, no deleting yourself, never zero admins) for web and MCP alike.
- Editor, visibility and delete endpoints are owner-only (404 otherwise, admins included) and behind `session.CrossOrigin`. Editor saves are all-or-nothing: any parse or lint error writes nothing, and a save over a deck changed since the editor opened (`sets.updated_at`, bumped by every task change) is refused with 409.
- Lint rejects whatever the editor's text format cannot hold (`{`, `}`, `|`, `/` in answers, empty lines or lines starting with `>` in text, line breaks outside text and explanation, control characters), so every lint-clean task round-trips through `deckfmt`. Text may span lines (dialogues); `deckfmt` reads consecutive non-`>` lines as one task.
- Keep MCP tool names stable; agents and the guide reference them.
- The guide must pass the task lint itself (tests enforce no polytonic, no final σ).
